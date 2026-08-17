package services

import (
	"context"
	"errors"
	"fmt"
	"net"

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
	remoteName := req.Msg.RemoteName
	if remoteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote_name is required"))
	}

	remote, ok := s.loadConfig().RemoteByName(remoteName)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no remote configured named %q", remoteName))
	}

	target := tmux.SSHTarget{Name: remote.Name, Addr: remoteAddr(remote.Host)}
	clientConfig := ssh.ClientConfig{
		User:            remote.User,
		Auth:            s.resolveAuthMethods(ctx, remote),
		HostKeyCallback: s.knownHosts.HostKeyCallback(),
	}
	runner := tmux.NewSSHRunner(target, clientConfig)

	if err := runner.Dial(ctx); err != nil {
		var unknownErr *tmux.ErrUnknownHostKey
		if errors.As(err, &unknownErr) {
			return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{
				HostKeyUnknown: true,
				Fingerprint:    unknownErr.Fingerprint,
			}), nil
		}

		return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{
			ErrorMessage: hostKeyAwareErrorMessage(remote.Host, err),
		}), nil
	}

	return connect.NewResponse(&sessionv1.TestRemoteConnectionResponse{Success: true}), nil
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
	remoteName := req.Msg.RemoteName
	if remoteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote_name is required"))
	}
	if req.Msg.Fingerprint == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fingerprint is required"))
	}

	remote, ok := s.loadConfig().RemoteByName(remoteName)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no remote configured named %q", remoteName))
	}

	addr := remoteAddr(remote.Host)
	key, probeErr := probeHostKey(ctx, addr, remote.User)
	if key == nil {
		return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{
			ErrorMessage: fmt.Sprintf("could not reach %s to verify its host key: %v", addr, probeErr),
		}), nil
	}

	actual := sshremote.HostKeyFingerprint(key)
	if actual != req.Msg.Fingerprint {
		return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{
			ErrorMessage: fmt.Sprintf(
				"fingerprint mismatch: %s is currently presenting %s, not the %s you were shown -- refusing to trust; the key may have changed",
				remote.Host, actual, req.Msg.Fingerprint,
			),
		}), nil
	}

	if err := s.knownHosts.Trust(remote.Host, key); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("trust host key for %q: %w", remoteName, err))
	}
	return connect.NewResponse(&sessionv1.TrustRemoteHostKeyResponse{Success: true}), nil
}

// resolveAuthMethods builds SSH auth methods for remote from its stored
// identity (sshremote.KeyStore, Epic 3.2), if one is registered. See
// resolveIdentityAuthMethods for the shared implementation (also used by
// SessionService.CreateSession's remote-target mode-specific block, Epic
// 4.2 -- extracted to a package-level function rather than duplicated so
// the two call sites can't drift).
func (s *RemoteService) resolveAuthMethods(ctx context.Context, remote *config.RemoteConfig) []ssh.AuthMethod {
	return resolveIdentityAuthMethods(ctx, s.keyStore, remote.IdentityRef)
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
