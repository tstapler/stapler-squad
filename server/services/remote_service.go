package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"connectrpc.com/connect"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Compile-time check: RemoteService must implement the generated handler.
var _ sessionv1connect.RemoteServiceHandler = (*RemoteService)(nil)

// defaultRemotePort is the SSH port dialed when remote.Host doesn't already
// specify one. config.RemoteConfig.Host is documented to normally carry no
// port suffix (see its doc comment) -- production remotes always resolve to
// this default -- but remoteAddr still honors an explicit "host:port" value
// if given, so this isn't a hard restriction, just the common case.
const defaultRemotePort = "22"

// remoteAddr returns host as a dialable "host:port" address, defaulting to
// defaultRemotePort when host doesn't already specify one. Mirrors
// session/sshremote's normalizeHostPort helper (kept as a separate,
// intentionally-duplicated few lines rather than an import: that helper is
// unexported, and this package's callers -- ordinary config.RemoteConfig
// values -- have a different default-behavior story worth documenting
// separately, see defaultRemotePort above).
func remoteAddr(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, defaultRemotePort)
}

// RemoteService implements the ConnectRPC RemoteServiceHandler: it drives
// Epic 3.3's TOFU (trust-on-first-use) host-key confirmation flow for
// configured SSH remotes (config.RemoteConfig), backed by a
// sshremote.KnownHostsStore for the trust decision and a
// sshremote.KeyStore for the remote's stored identity, if any.
type RemoteService struct {
	knownHosts *sshremote.KnownHostsStore
	keyStore   *sshremote.KeyStore
	loadConfig func() *config.Config
}

// NewRemoteService constructs a RemoteService. loadConfig is called fresh on
// every RPC (mirroring the BacklogService feature-flag interceptor's
// re-read-every-request pattern in server/server.go) so a remote added or
// edited in Settings is visible immediately, without restarting the server.
func NewRemoteService(knownHosts *sshremote.KnownHostsStore, keyStore *sshremote.KeyStore, loadConfig func() *config.Config) *RemoteService {
	return &RemoteService{knownHosts: knownHosts, keyStore: keyStore, loadConfig: loadConfig}
}

// +api: remote:test-connection
// TestRemoteConnection attempts to dial the named remote via a real
// tmux.SSHRunner, using s.knownHosts.HostKeyCallback() as the connection's
// trust check. An unknown host key is mapped to the structured
// host_key_unknown/fingerprint response shape rather than surfaced as a raw
// ConnectRPC error, per Epic 3.3's TOFU confirmation flow -- the frontend
// renders that as a "Trust and connect" / "Cancel" dialog rather than an
// opaque failure toast. A host-key MISMATCH (the key changed since it was
// trusted -- knownhosts' own MITM-relevant case) is deliberately reported
// through error_message instead, never as host_key_unknown=true: conflating
// the two would let a stale-key MITM scenario reuse the same "just trust it"
// UI path as a legitimate first connection.
func (s *RemoteService) TestRemoteConnection(
	ctx context.Context,
	req *connect.Request[sessionv1.TestRemoteConnectionRequest],
) (*connect.Response[sessionv1.TestRemoteConnectionResponse], error) {
	target, err := s.resolveRemoteTarget(req.Msg.RemoteName, req.Msg.Draft)
	if err != nil {
		return nil, err
	}

	sshTarget := tmux.SSHTarget{Name: target.name, Addr: target.addr}
	clientConfig := ssh.ClientConfig{
		User:            target.user,
		Auth:            resolveIdentityAuthMethods(ctx, s.keyStore, target.identityRef),
		HostKeyCallback: s.knownHosts.HostKeyCallback(),
	}
	runner := tmux.NewSSHRunner(sshTarget, clientConfig)

	if err := runner.Dial(ctx); err != nil {
		var unknownErr *tmux.ErrUnknownHostKey
		if errors.As(err, &unknownErr) {
			return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{
				HostKeyUnknown: true,
				Fingerprint:    unknownErr.Fingerprint,
			}), nil
		}

		return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{
			ErrorMessage: hostKeyAwareErrorMessage(target.hostForTrust, err),
		}), nil
	}

	return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{Success: true}), nil
}

// resolvedRemoteTarget is the common shape TestRemoteConnection and
// TrustRemoteHostKey dial against, regardless of whether the caller supplied
// an already-saved remote_name or an inline draft (see both requests' doc
// comments in remote.proto).
type resolvedRemoteTarget struct {
	name string
	// hostForTrust is the raw host string as stored (or as it WOULD be
	// stored, for a draft) in config.RemoteConfig.Host -- e.g. "prod.example.com"
	// or "prod.example.com:2222" for a non-default port, but never with the
	// default port (22) appended. This is what KnownHostsStore.Trust is
	// keyed on, matching what HostKeyCallback observes during the real dial
	// (see hostWithPort's doc comment).
	hostForTrust string
	// addr is the dialable "host:port" form (default port folded in),
	// matching remoteAddr's contract.
	addr string
	user string
	// identityRef is the sshremote.KeyStore key for this target's SSH
	// identity -- remote.IdentityRef for a saved remote, or the draft's name
	// (identities generated via GenerateRemoteIdentity are keyed by name;
	// see DraftRemoteTarget's doc comment).
	identityRef string
}

// resolveRemoteTarget resolves either an already-saved remote (remoteName)
// or inline draft connection coordinates (draft) into a resolvedRemoteTarget.
// Exactly one of remoteName/draft must be set. The returned error is already
// a connect.Error with the appropriate code (InvalidArgument/NotFound), so
// callers can return it directly.
func (s *RemoteService) resolveRemoteTarget(remoteName string, draft *sessionv1.DraftRemoteTarget) (*resolvedRemoteTarget, error) {
	switch {
	case remoteName != "" && draft != nil:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote_name and draft are mutually exclusive"))
	case remoteName != "":
		remote, ok := s.loadConfig().RemoteByName(remoteName)
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no remote configured named %q", remoteName))
		}
		return &resolvedRemoteTarget{
			name:         remote.Name,
			hostForTrust: remote.Host,
			addr:         remoteAddr(remote.Host),
			user:         remote.User,
			identityRef:  remote.IdentityRef,
		}, nil
	case draft != nil:
		if draft.Name == "" || draft.Host == "" || draft.User == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("draft.name, draft.host, and draft.user are required"))
		}
		hostForTrust := hostWithPort(draft.Host, draft.Port)
		return &resolvedRemoteTarget{
			name:         draft.Name,
			hostForTrust: hostForTrust,
			addr:         remoteAddr(hostForTrust),
			user:         draft.User,
			identityRef:  draft.Name,
		}, nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("one of remote_name or draft is required"))
	}
}

// +api: remote:trust-host-key
// TrustRemoteHostKey commits a TOFU decision: it independently re-dials the
// remote with a callback that unconditionally accepts and captures whatever
// host key is currently presented (deliberately bypassing s.knownHosts'
// trust check -- that's the whole point of this probe), then requires the
// captured key's fingerprint to exactly match req.Fingerprint before calling
// s.knownHosts.Trust. This re-verification (rather than trusting
// req.Fingerprint at face value) is the defense against a caller blindly
// trusting a different key than the one actually shown to the user: without
// it, a stale or forged fingerprint string would be enough to make Trust
// pin an attacker's key.
//
// The probe deliberately does NOT go through tmux.SSHRunner/the shared
// connection pool: an always-accept HostKeyCallback must never risk getting
// pooled under the remote's name, where a later, properly trust-checked
// TestRemoteConnection call could reuse it via the pool's "already have a
// live client" fast path and skip host-key verification entirely.
func (s *RemoteService) TrustRemoteHostKey(
	ctx context.Context,
	req *connect.Request[sessionv1.TrustRemoteHostKeyRequest],
) (*connect.Response[sessionv1.TrustRemoteHostKeyResponse], error) {
	if req.Msg.Fingerprint == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fingerprint is required"))
	}
	target, err := s.resolveRemoteTarget(req.Msg.RemoteName, req.Msg.Draft)
	if err != nil {
		return nil, err
	}

	key, probeErr := probeHostKey(ctx, target.addr, target.user)
	if key == nil {
		return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{
			ErrorMessage: fmt.Sprintf("could not reach %s to verify its host key: %v", target.addr, probeErr),
		}), nil
	}

	actual := sshremote.HostKeyFingerprint(key)
	if actual != req.Msg.Fingerprint {
		return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{
			ErrorMessage: fmt.Sprintf(
				"fingerprint mismatch: %s is currently presenting %s, not the %s you were shown -- refusing to trust; the key may have changed",
				target.hostForTrust, actual, req.Msg.Fingerprint,
			),
		}), nil
	}

	if err := s.knownHosts.Trust(target.hostForTrust, key); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("trust host key for %q: %w", target.name, err))
	}
	return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{Success: true}), nil
}

// +api: remote:generate-identity
// GenerateRemoteIdentity is the idempotent identity-generation step the Add
// Remote form's "Test connection" flow calls before the remote exists in
// config.json (ssh-remote-workspaces Phase 6, Epic 6.1) -- see
// sshremote.KeyStore.GenerateOrDescribeIdentity's doc comment for the
// idempotency guarantee (repeated clicks never rotate the key).
func (s *RemoteService) GenerateRemoteIdentity(
	ctx context.Context,
	req *connect.Request[sessionv1.GenerateRemoteIdentityRequest],
) (*connect.Response[sessionv1.GenerateRemoteIdentityResponse], error) {
	name := req.Msg.Name
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	identity, err := s.keyStore.GenerateOrDescribeIdentity(ctx, name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate identity for %q: %w", name, err))
	}

	return connect.NewResponse(&sessionv1.GenerateRemoteIdentityResponse{
		PublicKeyText:      identity.PublicKeyText,
		AuthorizedKeysLine: identity.AuthorizedKeysLine,
	}), nil
}

// +api: remote:list
// ListRemotes returns every configured remote, for the Settings -> Remotes
// list view (ssh-remote-workspaces Phase 6, Epic 6.1, Task 6.1.1a).
func (s *RemoteService) ListRemotes(
	ctx context.Context,
	req *connect.Request[sessionv1.ListRemotesRequest],
) (*connect.Response[sessionv1.ListRemotesResponse], error) {
	cfg := s.loadConfig()
	remotes := make([]*sessionv1.RemoteConfigProto, 0, len(cfg.Remotes))
	for i := range cfg.Remotes {
		remotes = append(remotes, remoteConfigToProto(&cfg.Remotes[i]))
	}
	return connect.NewResponse(&sessionv1.ListRemotesResponse{Remotes: remotes}), nil
}

// +api: remote:create
// CreateRemote persists a new RemoteConfig to config.json -- see its RPC
// doc comment in remote.proto for when the frontend calls this (after a
// draft-mode TestRemoteConnection/TrustRemoteHostKey has confirmed the
// remote is reachable).
//
// Enforces the TOFU precondition itself (review-found gap, previously only
// enforced by frontend flow discipline -- AC1's "verifies... before the
// remote is saved" is a contract of this RPC, not just of the shipped UI):
// s.knownHosts.IsHostTrusted must report a trusted key already on file for
// this host before the config write proceeds. This does NOT re-dial the
// remote (that's still TestRemoteConnection/TrustRemoteHostKey's job,
// deliberately not re-entangled here) -- it only asks whether SOME prior
// Trust() call already ran for this exact host string, independent of which
// key it pinned. A caller that bypasses the draft-mode Test/Trust sequence
// now fails fast here with CodeFailedPrecondition, instead of only failing
// later at actual dial time.
func (s *RemoteService) CreateRemote(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateRemoteRequest],
) (*connect.Response[sessionv1.CreateRemoteResponse], error) {
	name := req.Msg.Name
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if req.Msg.Host == "" || req.Msg.User == "" || req.Msg.BasePath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("host, user, and base_path are required"))
	}

	cfg := s.loadConfig()
	if _, ok := cfg.RemoteByName(name); ok {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a remote named %q is already configured", name))
	}

	hostForTrust := hostWithPort(req.Msg.Host, req.Msg.Port)
	trusted, err := s.knownHosts.IsHostTrusted(hostForTrust)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check host key trust for %q: %w", hostForTrust, err))
	}
	if !trusted {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("host key for %q has not been verified yet -- call TestRemoteConnection and TrustRemoteHostKey first", hostForTrust))
	}

	// The intended onboarding flow already generated an identity for name
	// via GenerateRemoteIdentity (called from "Test connection"); generate
	// one on demand here too so CreateRemote can never persist a
	// RemoteConfig with a dangling IdentityRef.
	if _, err := s.keyStore.GenerateOrDescribeIdentity(ctx, name); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ensure identity for %q: %w", name, err))
	}

	remote := config.RemoteConfig{
		Name:        name,
		Host:        hostWithPort(req.Msg.Host, req.Msg.Port),
		User:        req.Msg.User,
		BasePath:    req.Msg.BasePath,
		IdentityRef: name,
	}
	cfg.Remotes = append(cfg.Remotes, remote)
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	return connect.NewResponse(&sessionv1.CreateRemoteResponse{Remote: remoteConfigToProto(&remote)}), nil
}

// +api: remote:delete
// DeleteRemote removes name's config.json entry (if present) and its stored
// SSH identity (if present) -- see its RPC doc comment in remote.proto for
// why either alone is treated as success (it serves both the Remotes list's
// "Delete" row action and the Add Remote form's Cancel-cleanup path, which
// only ever has an orphaned identity to remove, never a config entry).
func (s *RemoteService) DeleteRemote(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteRemoteRequest],
) (*connect.Response[sessionv1.DeleteRemoteResponse], error) {
	name := req.Msg.Name
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	cfg := s.loadConfig()
	kept := make([]config.RemoteConfig, 0, len(cfg.Remotes))
	removedFromConfig := false
	for _, r := range cfg.Remotes {
		if r.Name == name {
			removedFromConfig = true
			continue
		}
		kept = append(kept, r)
	}

	deletedIdentity := false
	if err := s.keyStore.DeleteIdentity(ctx, name); err == nil {
		deletedIdentity = true
	} else if !errors.Is(err, sshremote.ErrIdentityNotFound) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete identity for %q: %w", name, err))
	}

	if !removedFromConfig && !deletedIdentity {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no remote or stored identity found for %q", name))
	}

	if removedFromConfig {
		cfg.Remotes = kept
		if err := config.SaveConfig(cfg); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
		}
	}

	return connect.NewResponse(&sessionv1.DeleteRemoteResponse{}), nil
}

// remoteConfigToProto converts a config.RemoteConfig to its RPC transport
// shape, splitting a non-default port back out of Host so the frontend never
// has to parse a "host:port" string itself (see RemoteConfigProto's doc
// comment).
func remoteConfigToProto(remote *config.RemoteConfig) *sessionv1.RemoteConfigProto {
	host, port := splitHostPort(remote.Host)
	return &sessionv1.RemoteConfigProto{
		Name:        remote.Name,
		Host:        host,
		User:        remote.User,
		Port:        port,
		BasePath:    remote.BasePath,
		HasIdentity: remote.IdentityRef != "",
	}
}

// hostWithPort folds a non-default port into a "host:port" string suitable
// for storage as config.RemoteConfig.Host, matching that field's documented
// shape (a bare host normally; an explicit "host:port" only for a
// non-default port). port == 0 or port == defaultRemotePort's numeric value
// (22) yields host unchanged.
func hostWithPort(host string, port int32) string {
	if port == 0 || strconv.Itoa(int(port)) == defaultRemotePort {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

// splitHostPort splits a config.RemoteConfig.Host value into a bare host and
// an int32 port (0 when Host carries no explicit port -- the common,
// default-22 case). Best-effort: a Host value net.SplitHostPort can't parse
// falls back to returning it unchanged with port 0 rather than an error --
// this runs on read paths (ListRemotes/CreateRemote's response) that must
// always succeed.
func splitHostPort(host string) (string, int32) {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return host, 0
	}
	portNum, err := strconv.Atoi(p)
	if err != nil {
		return host, 0
	}
	return h, int32(portNum)
}

// resolveIdentityAuthMethods builds SSH auth methods from identityRef's
// stored identity (sshremote.KeyStore, Epic 3.2), if one is registered.
// Returns nil when identityRef is empty, no identity is registered for it,
// or it can't be parsed -- Dial then fails at the authentication step, which
// happens strictly AFTER host-key verification in the SSH handshake
// (golang.org/x/crypto/ssh's clientHandshake runs the key exchange -- where
// HostKeyCallback fires -- before clientAuthenticate), so an auth failure
// here can never masquerade as a host-key problem. Shared by RemoteService
// (TestRemoteConnection) and SessionService (CreateSession's remote-target
// mode-specific block, Epic 4.2) rather than duplicated per call site.
func resolveIdentityAuthMethods(ctx context.Context, keyStore *sshremote.KeyStore, identityRef string) []ssh.AuthMethod {
	if identityRef == "" {
		return nil
	}
	kind, value, err := keyStore.GetIdentity(ctx, identityRef)
	if err != nil || kind != sshremote.IdentityKindPrivateKey {
		return nil
	}
	signer, err := ssh.ParsePrivateKey(value)
	if err != nil {
		return nil
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}
}

// hostKeyAwareErrorMessage renders err for TestRemoteConnectionResponse's
// error_message field, giving the host-key-mismatch case (a possible MITM
// signal) a specific, actionable message instead of whatever generic dial
// error text golang.org/x/crypto/ssh happens to wrap it in.
//
// This dials via s.knownHosts.HostKeyCallback(), which -- by design (see its
// doc comment) -- returns the RAW, untranslated *knownhosts.KeyError on a
// mismatch, never a *sshremote.ErrHostKeyMismatch (that type is only ever
// constructed by KnownHostsStore.Verify, which this dial path doesn't call).
// So the mismatch check here must use sshremote.IsHostKeyMismatch -- the
// same classification Verify itself uses internally -- rather than
// errors.As against ErrHostKeyMismatch, which would never match anything
// produced by this call path.
func hostKeyAwareErrorMessage(host string, err error) string {
	if sshremote.IsHostKeyMismatch(err) {
		return fmt.Sprintf(
			"host key for %s does not match the previously trusted key -- refusing to connect (possible MITM attack); if the remote's key was legitimately rotated, remove and re-trust it",
			host,
		)
	}
	return err.Error()
}

// probeHostKey dials addr just far enough to observe the remote's currently
// presented host key, accepting unconditionally -- it makes no trust
// decision of its own (that's KnownHostsStore.Verify's job) and is used only
// to re-verify a fingerprint the caller claims to have already been shown.
// It never uses tmux.SSHRunner or the shared connection pool (see
// TrustRemoteHostKey's doc comment for why). Auth is intentionally omitted:
// the host-key callback always fires before authentication is attempted, so
// no credentials are needed to capture the key, and the probe connection is
// always closed immediately regardless of whether authentication would have
// succeeded.
func probeHostKey(ctx context.Context, addr, user string) (ssh.PublicKey, error) {
	var captured ssh.PublicKey
	clientConfig := &ssh.ClientConfig{
		User: user,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return nil
		},
		Timeout: tmux.DialTimeout,
	}

	type dialResult struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan dialResult, 1)
	go func() {
		client, err := ssh.Dial("tcp", addr, clientConfig)
		ch <- dialResult{client: client, err: err}
	}()

	select {
	case res := <-ch:
		if res.client != nil {
			_ = res.client.Close()
		}
		return captured, res.err
	case <-ctx.Done():
		// captured may still be nil here -- the goroutine above keeps running
		// in the background until ssh.Dial's own Timeout gives up, mirroring
		// session/tmux/ssh_runner.go's documented newSession gap (ctx expiry
		// can't force-abort an in-flight dial without a second layer of
		// plumbing this one-off probe doesn't need).
		return nil, ctx.Err()
	}
}

// BuildRemoteHealthProber constructs a sshremote.RemoteHealthProber for
// remote, wiring its liveness-check tmux.SSHRunner the exact same way
// TestRemoteConnection (above) and SessionService.CreateSession's
// remote-target block (session_service.go) already build one: remoteAddr
// for the dialable address and resolveIdentityAuthMethods for the stored
// identity, kept here rather than duplicated since both helpers are
// unexported to this package.
//
// pool MUST be the same SSHClientPool a session's own SSHRunner/
// RemoteApprovalRelay for this remote shares (tmux.DefaultSSHClientPool()
// in production) -- passing an isolated pool here would defeat
// RemoteHealthProber's whole point of reusing the connection instead of
// opening a dedicated one (see its doc comment). Exported for server.go's
// startup wiring (Epic 6.4, Task 6.4.1c) -- the only caller outside this
// package.
func BuildRemoteHealthProber(
	ctx context.Context,
	remote *config.RemoteConfig,
	pool *tmux.SSHClientPool,
	knownHosts *sshremote.KnownHostsStore,
	keyStore *sshremote.KeyStore,
	publisher sshremote.HealthEventPublisher,
) (*sshremote.RemoteHealthProber, error) {
	target := tmux.SSHTarget{Name: remote.Name, Addr: remoteAddr(remote.Host)}
	clientConfig := ssh.ClientConfig{
		User:            remote.User,
		Auth:            resolveIdentityAuthMethods(ctx, keyStore, remote.IdentityRef),
		HostKeyCallback: knownHosts.HostKeyCallback(),
	}
	runner := tmux.NewSSHRunner(target, clientConfig, tmux.WithSSHClientPool(pool))
	return sshremote.NewRemoteHealthProber(pool, runner, remote.Name, publisher)
}
