package sshremote

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// remoteApprovalSocketNamePrefix/Suffix bracket the per-session disambiguator
// in the filename RemoteApprovalRelay expects a remote session's injected
// agent hooks (Epic 5.2 -- not this package) to write approval-request
// payloads to, rooted under the session's remote base path. See
// RemoteApprovalSocketPath and Task 5.1.1a in
// project_plans/ssh-remote-workspaces/implementation/plan.md.
//
// Kept deliberately terse (not e.g. ".stapler-squad-approval-<hash>.sock"):
// AF_UNIX socket paths are capped at 108 bytes total (Linux sockaddr_un.
// sun_path, null terminator included; macOS/BSD's sockaddr_un caps at 104) --
// a kernel constant, not something basePath's caller controls -- and this
// filename is appended onto a caller-supplied basePath that can already be
// deep. A verbose name here silently turns "listen unix: bind: invalid
// argument" into that basePath's problem.
const (
	remoteApprovalSocketNamePrefix = ".ssq-appr-"
	remoteApprovalSocketNameSuffix = ".sock"
)

// remoteApprovalSocketIDHashBytes is truncated to 4 bytes (8 hex chars): this
// disambiguates sessions that happen to share a basePath (see below), not a
// security boundary, so collision resistance only needs to cover the
// small number of sessions realistically sharing one remote directory --
// full-width SHA-256 would just burn AF_UNIX's tight path budget for no
// benefit.
const remoteApprovalSocketIDHashBytes = 4

// RemoteApprovalSocketPath returns the remote-host Unix domain socket path
// convention for a session rooted at basePath on the remote host, keyed by
// stableSessionID (the same value server/services.ApprovalHandler.
// resolveSessionID correlates a relayed request back to -- see
// RemoteApprovalRelayTarget.StableSessionID's doc comment). Hashed rather
// than used verbatim in the filename: two sessions can share the exact same
// basePath (e.g. two SessionTypeDirectory sessions both pointed at the same
// remote directory), and a fixed, basePath-only filename would let the
// second session's `unlink-early` socket bind silently steal the first
// session's listener out from under it -- found in pre-ship review. A short
// SHA-256 prefix of the ID sidesteps filename-safety concerns a raw
// title-derived StableSessionID would raise (arbitrary characters, unbounded
// length) without needing a bespoke sanitizer.
//
// Uses path.Join (POSIX-style forward slashes), deliberately NOT
// filepath.Join: basePath always names a location on the REMOTE host (a
// Linux/macOS SSH target, per RemoteConfig's base_path), so joining it must
// not switch to backslash separators when stapler-squad itself happens to
// be built on Windows.
func RemoteApprovalSocketPath(basePath, stableSessionID string) string {
	sum := sha256.Sum256([]byte(stableSessionID))
	name := fmt.Sprintf("%s%x%s", remoteApprovalSocketNamePrefix, sum[:remoteApprovalSocketIDHashBytes], remoteApprovalSocketNameSuffix)
	return path.Join(basePath, name)
}

// defaultBearerTokenTTL bounds how long a RemoteApprovalRelay's bearer
// credential remains valid before every relayed payload is rejected
// outright, regardless of whether it presents the (now-expired) token.
// Per research/pitfalls.md §4, the callback channel a remote agent uses to
// reach back into the local dashboard must never accept traffic from an
// unauthenticated/static-secret source -- a generous but bounded TTL keeps
// the credential from becoming a de facto permanent shared secret baked
// into a long-lived remote session, while not being so short that ordinary
// approval traffic starts failing mid-session. RotateToken lets a caller
// (e.g. Epic 5.2's hook injection, on its own re-injection cadence) refresh
// the credential without tearing down and recreating the whole relay.
const defaultBearerTokenTTL = 24 * time.Hour

// bearerTokenBytes is the amount of raw entropy in each minted bearer
// token, before base64 encoding -- 256 bits, matching this repo's other
// keychain-adjacent secret material (see session/sshremote/keygen.go).
const bearerTokenBytes = 32

// defaultPollInterval bounds how often the relay retries dialing the
// remote-side Unix socket when no connection is currently available
// (nothing listening yet, or the last dial attempt failed) -- this is the
// "bounded poll interval" Story 5.1.1's acceptance criteria references.
const defaultPollInterval = 500 * time.Millisecond

// defaultDialTimeout bounds a single direct-streamlocal dial attempt, and
// also the read of one relayed payload once a connection is accepted, so a
// remote-side peer that connects but never writes (or writes only
// partially) can't wedge the relay's single poll goroutine indefinitely.
// Mirrors keystore.go's defaultIdentityTimeout budgeting convention.
const defaultDialTimeout = 5 * time.Second

// pendingRedeliveryTTL bounds how long a buffered-but-undelivered decision
// stays redeliverable. Review found (independently, twice) that an unbounded
// buffer matched on raw request bytes (no nonce in classifier.
// PermissionRequestPayload) can replay a stale decision onto a genuinely
// later, byte-identical request -- e.g. the same command re-run minutes
// later auto-gets the earlier human's answer with no prompt. Sized to the
// network-reconnect window this buffering exists to bridge (SSHClientPool's
// own dial timeout is on the order of seconds), not to the much longer
// human-decision wait a single hook attempt can now take
// (server/services.remoteApprovalHookAttemptTimeoutSeconds) -- those are
// deliberately different budgets.
const pendingRedeliveryTTL = 60 * time.Second

// bearerCredential is the per-session, short-TTL token RemoteApprovalRelay
// requires in every relayed payload (research/pitfalls.md §4): without
// this, the direct-streamlocal channel the relay reads from would accept a
// payload from literally any process on the remote host able to connect to
// the fixed socket path -- not just the intended agent hook script.
type bearerCredential struct {
	token     string
	expiresAt time.Time
}

func newBearerCredential(ttl time.Duration) (bearerCredential, error) {
	buf := make([]byte, bearerTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return bearerCredential{}, fmt.Errorf("sshremote: generate bearer token: %w", err)
	}
	return bearerCredential{
		token:     base64.RawURLEncoding.EncodeToString(buf),
		expiresAt: time.Now().Add(ttl),
	}, nil
}

// relayedApprovalPayload is the JSON envelope the remote hook script
// (server/services.remoteApprovalHookCommand, Epic 5.2, not this package)
// writes to the fixed Unix socket for a single approval request. Token must
// equal the relay's current bearer credential and must not be expired (see
// verifyToken); a payload failing either check is dropped without being
// forwarded to PermissionRequestHandler.
//
// Request is the RAW, untouched bytes Claude Code itself wrote to the
// PermissionRequest hook's stdin -- a classifier.PermissionRequestPayload-
// shaped JSON object (pkg/classifier/classifier.go) -- deliberately kept as
// json.RawMessage rather than decoded into a concrete type here: this
// package never needs to interpret the payload, only pass it through
// unmodified as an HTTP request body to PermissionRequestHandler, which
// decodes it exactly the way the real HTTP hook endpoint does. Earlier
// (Epic 5.1) this field was typed detection.ApprovalRequest and fed into
// session.ExternalApprovalMonitor -- the wrong target entirely; see
// ADR-003's addendum for why.
type relayedApprovalPayload struct {
	Token   string          `json:"token"`
	Request json.RawMessage `json:"request"`
}

// PermissionRequestHandler is anything that can process a raw
// PermissionRequest hook payload the same way
// server/services.ApprovalHandler.HandlePermissionRequest does -- defined
// here (the consumer package) rather than imported from server/services,
// which would create an import cycle (server/services already imports
// session/sshremote for KeyStore/KnownHostsStore). *server/services.
// ApprovalHandler already has exactly this method signature and satisfies
// this interface structurally, with zero changes there.
type PermissionRequestHandler interface {
	HandlePermissionRequest(w http.ResponseWriter, r *http.Request)
}

// RemoteApprovalRelay reads approval-request payloads a remote agent
// process writes to a fixed Unix domain socket on the remote host
// (RemoteApprovalSocketPath) via a direct-streamlocal@openssh.com channel
// dialed over the SAME pooled *ssh.Client the session's terminal stream
// already uses, and drives them through PermissionRequestHandler exactly
// the way Claude Code's real HTTP PermissionRequest hook does -- see
// ADR-003 (project_plans/ssh-remote-workspaces/decisions/) for why this
// reuses the existing multiplexed SSH connection instead of a reverse
// tunnel or a third-party tunneling library, and its addendum for why the
// forwarding target changed from Epic 5.1's original
// session.ExternalApprovalMonitor to this handler-based design.
//
// Unlike Epic 5.1's original design (request-direction-only, closing the
// connection immediately after decoding), this merges the request and
// response into a single blocking round trip: handleConnection decodes the
// payload, builds a synthetic *http.Request from its raw bytes, calls
// PermissionRequestHandler.HandlePermissionRequest -- which BLOCKS until a
// human decision is made or its own configured timeout fires, exactly as
// it does for a real local HTTP hook -- and writes the resulting response
// body back onto the SAME connection before closing it, so the remote-side
// hook script (server/services.remoteApprovalHookCommand's socat pipeline)
// gets the identical bytes curl's stdout would have gotten locally. This
// subsumes what was originally planned as a separate Epic 5.3 response-
// delivery step: merging request+response into one blocking round trip is
// simpler than two independent half-duplex pieces and was never separately
// buildable once PermissionRequestHandler (not ExternalApprovalMonitor)
// was identified as the correct target.
//
// Known v1 constraint (not addressed here): the remote-host socket path is
// fixed per session, so only one approval can be in flight per remote
// session at a time -- handleConnection blocks the poll loop for the
// duration of the human decision, and a second concurrent request has
// nowhere to connect until the first completes. Acceptable per the
// original plan design; not solved by this change.
//
// RemoteApprovalRelay never dials the SSH connection pool itself -- it only
// subscribes (via pool.Subscribe) to whichever *ssh.Client is CURRENTLY
// pooled for remoteName. Some other component (terminal streaming, per
// Phase 4/session/tmux.SSHRunner) is responsible for establishing and
// redialing that connection; decoupling the relay from dial config/
// credentials entirely means it has nothing more to configure than "which
// pooled connection" and "which remote-side socket."
type RemoteApprovalRelay struct {
	pool            *tmux.SSHClientPool
	handler         PermissionRequestHandler
	remoteName      string
	socketPath      string
	stableSessionID string
	title           string

	pollInterval time.Duration
	dialTimeout  time.Duration

	credMu sync.RWMutex
	cred   bearerCredential

	clientMu sync.RWMutex
	client   *ssh.Client

	// wake is a capacity-1, non-blocking signal: "a fresh client just
	// became available, don't wait out the rest of pollInterval before
	// trying it." Story 5.1.2's reconnect responsiveness relies on this --
	// without it, a redial could sit unused for up to pollInterval before
	// the relay's poll loop notices.
	wake chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once

	// pendingRequest/pendingResponse/pendingAt buffer an already-computed
	// decision that handleConnection failed to deliver (AC4's network-blip
	// case): a retry resending the same request bytes within
	// pendingRedeliveryTTL gets the buffered decision instead of a second
	// r.handler.HandlePermissionRequest call, which would re-prompt a human
	// who already decided (ApprovalHandler has no request-level de-dup).
	// Only ever touched from run()'s single goroutine, so unguarded by a
	// mutex -- see handleConnection's doc comment for the TTL's reasoning.
	pendingRequest  []byte
	pendingResponse []byte
	pendingAt       time.Time
}

// RemoteApprovalRelayOption configures a RemoteApprovalRelay at construction
// time.
type RemoteApprovalRelayOption func(*RemoteApprovalRelay)

// withPollInterval overrides the default poll interval. Unexported: only
// same-package tests need a faster-than-production interval to keep
// poll-driven tests fast.
func withPollInterval(d time.Duration) RemoteApprovalRelayOption {
	return func(r *RemoteApprovalRelay) { r.pollInterval = d }
}

// withDialTimeout overrides the default per-attempt dial/read timeout.
// Unexported for the same reason as withPollInterval.
func withDialTimeout(d time.Duration) RemoteApprovalRelayOption {
	return func(r *RemoteApprovalRelay) { r.dialTimeout = d }
}

// withBearerTokenTTL overrides the default bearer-credential TTL by
// re-minting the credential (same construction path as NewRemoteApprovalRelay,
// just with a caller-chosen TTL instead of defaultBearerTokenTTL) with ttl
// applied from "now" at option-application time. Unexported: only
// same-package tests need a short TTL to exercise expiry without sleeping
// for defaultBearerTokenTTL.
func withBearerTokenTTL(ttl time.Duration) RemoteApprovalRelayOption {
	return func(r *RemoteApprovalRelay) {
		cred, err := newBearerCredential(ttl)
		if err != nil {
			// newBearerCredential only fails if crypto/rand.Read fails,
			// which indicates a broken system entropy source -- not
			// something a test option can meaningfully recover from.
			panic(fmt.Sprintf("sshremote: withBearerTokenTTL: %v", err))
		}
		r.credMu.Lock()
		r.cred = cred
		r.credMu.Unlock()
	}
}

// RemoteApprovalRelayTarget bundles the four caller-supplied identifiers
// NewRemoteApprovalRelay needs. Deliberately a struct, not four adjacent
// string parameters: RemoteName/BasePath/StableSessionID/Title are
// independently meaningful, same-typed strings a caller could silently
// transpose at a call site with no compiler error (e.g. passing
// StableSessionID where Title is expected still compiles) -- exactly what
// .claude/rules/primitive-obsession-checklist.md exists to catch. Field
// names carry the disambiguation a positional four-string parameter list
// can't.
type RemoteApprovalRelayTarget struct {
	// RemoteName is the SSHClientPool key this relay's channel is dialed
	// under (session/tmux.SSHClientPool). Must match the same name the
	// session's own tmux.SSHRunner/tmux.SSHTarget was constructed with, so
	// this relay shares the SAME pooled *ssh.Client the terminal stream
	// uses (ADR-003) rather than dialing an unrelated connection.
	RemoteName string
	// BasePath is the session's remote-host working-directory root -- i.e.
	// its worktree path on the remote host (session.Instance.
	// GetEffectiveRootDir()), NOT the remote's shared config.RemoteConfig.
	// BasePath. RemoteApprovalSocketPath is derived from it, so the fixed
	// socket this relay reads from is scoped to ONE session, not shared
	// across every session on the same remote (which would make the "one
	// approval in flight at a time" constraint apply across unrelated
	// sessions instead of within a single one).
	BasePath string
	// StableSessionID is written into the synthetic HTTP request's
	// X-CS-Session-ID header before it's handed to PermissionRequestHandler
	// -- must be the SAME stable ID server/services.ApprovalHandler.
	// resolveSessionID resolves this session to locally
	// (session.Instance.GetStableID(): UUID, falling back to Title), or the
	// handler silently fails to correlate the relayed request with the
	// right session's approval UI/notifications. Renamed from Epic 5.1's
	// "SessionKey" (a session.ExternalApprovalMonitor lookup key that no
	// longer applies once this relay stopped forwarding into that
	// subsystem -- see ADR-003's addendum) to name what this value is
	// actually used for now.
	StableSessionID string
	// Title is used only for this relay's own log messages; the local
	// approval UI's session title comes from PermissionRequestHandler's own
	// session lookup (ApprovalHandler.resolveSessionName), not from here.
	Title string
}

// NewRemoteApprovalRelay constructs a RemoteApprovalRelay for a single
// remote session: pool is the shared SSH connection pool target.RemoteName
// is dialed under; handler is what every relayed request is driven through
// (production callers pass a *server/services.ApprovalHandler, which
// satisfies PermissionRequestHandler structurally). See
// RemoteApprovalRelayTarget's field docs for the rest.
//
// A fresh, random bearer credential is minted immediately (see BearerToken)
// -- callers wiring hook injection (Epic 5.2) read it back via BearerToken
// to embed in the generated hook command.
func NewRemoteApprovalRelay(
	pool *tmux.SSHClientPool,
	handler PermissionRequestHandler,
	target RemoteApprovalRelayTarget,
	opts ...RemoteApprovalRelayOption,
) (*RemoteApprovalRelay, error) {
	if pool == nil {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: pool must not be nil")
	}
	if handler == nil {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: handler must not be nil")
	}
	if target.RemoteName == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.RemoteName must not be empty")
	}
	if target.BasePath == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.BasePath must not be empty")
	}
	if target.StableSessionID == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.StableSessionID must not be empty")
	}

	cred, err := newBearerCredential(defaultBearerTokenTTL)
	if err != nil {
		return nil, err
	}

	r := &RemoteApprovalRelay{
		pool:            pool,
		handler:         handler,
		remoteName:      target.RemoteName,
		socketPath:      RemoteApprovalSocketPath(target.BasePath, target.StableSessionID),
		stableSessionID: target.StableSessionID,
		title:           target.Title,
		pollInterval:    defaultPollInterval,
		dialTimeout:     defaultDialTimeout,
		cred:            cred,
		wake:            make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// BearerToken returns the relay's current bearer credential and its expiry.
// Epic 5.2's hook injection uses this to embed the token in the remote
// agent's generated hook command so its payload passes verifyToken.
func (r *RemoteApprovalRelay) BearerToken() (token string, expiresAt time.Time) {
	r.credMu.RLock()
	defer r.credMu.RUnlock()
	return r.cred.token, r.cred.expiresAt
}

// RotateToken replaces the relay's current bearer credential with a freshly
// generated one, valid for another defaultBearerTokenTTL. Payloads bearing
// the OLD token are rejected immediately after this call returns -- a
// caller that rotates must also re-inject the new token (via BearerToken)
// into any hook command not yet run on the remote side.
func (r *RemoteApprovalRelay) RotateToken() error {
	cred, err := newBearerCredential(defaultBearerTokenTTL)
	if err != nil {
		return err
	}
	r.credMu.Lock()
	r.cred = cred
	r.credMu.Unlock()
	return nil
}

// Start begins the relay's poll loop: it subscribes to pool's reconnect
// signal for remoteName (Task 5.1.2a) and repeatedly dials the remote-side
// Unix socket over whichever *ssh.Client is currently pooled, reading and
// forwarding relayed approval payloads until ctx is done or Stop is called.
// Safe to call at most once per RemoteApprovalRelay; later calls are a
// no-op.
func (r *RemoteApprovalRelay) Start(ctx context.Context) {
	r.startOnce.Do(func() {
		r.ctx, r.cancel = context.WithCancel(ctx)

		ch, unsubscribe := r.pool.Subscribe(r.remoteName)
		r.wg.Add(2)
		go r.watchReconnects(ch, unsubscribe)
		go r.run()

		log.Info("remote approval relay started", "remote", r.remoteName, "socketPath", r.socketPath, "sessionID", r.stableSessionID)
	})
}

// Stop halts the relay's poll loop and unsubscribes from the pool's
// reconnect signal, waiting for both background goroutines to exit. Safe to
// call multiple times, or without a prior Start.
func (r *RemoteApprovalRelay) Stop() {
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		r.wg.Wait()
		log.Info("remote approval relay stopped", "remote", r.remoteName)
	})
}

// watchReconnects consumes the pool's reconnect-notification channel for as
// long as the relay is running, updating the relay's notion of "the current
// client" (Task 5.1.2b) on every notification -- both the initial priming
// value and every subsequent redial.
func (r *RemoteApprovalRelay) watchReconnects(ch <-chan *ssh.Client, unsubscribe func()) {
	defer r.wg.Done()
	defer unsubscribe()
	for {
		select {
		case <-r.ctx.Done():
			return
		case client, ok := <-ch:
			if !ok {
				return
			}
			r.setClient(client)
		}
	}
}

func (r *RemoteApprovalRelay) setClient(client *ssh.Client) {
	r.clientMu.Lock()
	r.client = client
	r.clientMu.Unlock()

	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *RemoteApprovalRelay) currentClient() *ssh.Client {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()
	return r.client
}

// run is the relay's main poll loop: while no client is pooled yet, or
// while the last dial attempt failed (nothing listening on the remote
// socket, or the connection is currently down), it waits out pollInterval
// (or wakes early on reconnect) before retrying. Once a direct-streamlocal
// connection is accepted, handleConnection processes exactly one relayed
// payload from it before the loop dials again.
func (r *RemoteApprovalRelay) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		client := r.currentClient()
		if client == nil {
			if !r.sleep() {
				return
			}
			continue
		}

		conn, err := r.dial(client)
		if err != nil {
			if !r.sleep() {
				return
			}
			continue
		}

		r.handleConnection(conn)
	}
}

// sleep waits out r.pollInterval, waking early if r.ctx is cancelled or a
// fresh client becomes available via r.wake (Story 5.1.2's reconnect
// responsiveness). Returns false in the cancellation case so run's caller
// knows to return immediately rather than loop once more.
func (r *RemoteApprovalRelay) sleep() bool {
	timer := time.NewTimer(r.pollInterval)
	defer timer.Stop()
	select {
	case <-r.ctx.Done():
		return false
	case <-timer.C:
		return true
	case <-r.wake:
		return true
	}
}

// dial opens a direct-streamlocal@openssh.com channel to r.socketPath over
// client -- golang.org/x/crypto/ssh's Client.DialContext("unix", ...) is
// exactly this primitive (see tcpip.go's Dial, which routes network "unix"
// to dialStreamLocal internally), the same mechanism ssh -L/-R use for Unix
// domain socket forwarding, per ADR-003 and research/stack.md §5. Bounded
// by r.dialTimeout so a stalled handshake can't wedge the poll loop.
func (r *RemoteApprovalRelay) dial(client *ssh.Client) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.dialTimeout)
	defer cancel()
	return client.DialContext(ctx, "unix", r.socketPath)
}

// handleConnection reads exactly one JSON relayedApprovalPayload from conn,
// verifies its bearer token, and -- on success -- drives the raw request
// bytes through r.handler exactly as server/services.ApprovalHandler.
// HandlePermissionRequest would process a real local HTTP POST, then writes
// the resulting response body back onto conn before closing it. This call
// BLOCKS for as long as r.handler takes to resolve the request (a human
// decision, or the handler's own configured timeout) -- see this type's doc
// comment for why that's intentional and what it constrains.
//
// A decode/token failure closes conn with nothing written, mirroring the
// original request-only behavior for those failure paths: there is no
// well-formed request to hand the handler, so there is nothing meaningful
// to respond with either.
//
// Redelivery (requirements.md AC4's network-blip-mid-request case): if
// payload.Request matches r.pendingRequest -- a decision this method already
// computed on a PRIOR connection but failed to deliver, because that
// connection died before the write completed -- this is the hook script's
// own retry (server/services.remoteApprovalHookCommand) resending the exact
// same captured request, not a new one. r.handler is NOT called again;
// r.pendingResponse is written back directly. Re-invoking the handler here
// would be wrong even though it would "work" mechanically: ApprovalHandler
// has no request-level de-dup, so it would create a second pending-approval
// record and could re-prompt a human who already made a decision for the
// first one.
func (r *RemoteApprovalRelay) handleConnection(conn net.Conn) {
	defer conn.Close() //nolint:errcheck // best-effort; the response (if any) has already been written by the time this runs

	payload, err := r.decodePayload(conn)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Warn("remote approval relay: decode payload failed", "remote", r.remoteName, "err", err)
		}
		return
	}

	if !r.verifyToken(payload.Token) {
		log.Warn("remote approval relay: rejected payload with invalid or expired bearer token", "remote", r.remoteName, "sessionID", r.stableSessionID)
		return
	}

	if r.pendingResponse != nil && time.Since(r.pendingAt) > pendingRedeliveryTTL {
		r.pendingRequest = nil
		r.pendingResponse = nil
	}
	if r.pendingResponse != nil && bytes.Equal(r.pendingRequest, payload.Request) {
		if _, err := conn.Write(r.pendingResponse); err != nil {
			log.Warn("remote approval relay: redelivery of a buffered decision failed, keeping it buffered for the next retry", "remote", r.remoteName, "sessionID", r.stableSessionID, "err", err)
			return
		}
		log.Info("remote approval relay: redelivered a previously-undelivered decision after reconnect", "remote", r.remoteName, "sessionID", r.stableSessionID)
		r.pendingRequest = nil
		r.pendingResponse = nil
		return
	}
	// A genuinely different request supersedes any stale buffered one --
	// per the "only one approval in flight at a time" constraint, an
	// unrelated new request means the previous one's retry window (if any)
	// has passed.
	r.pendingRequest = nil
	r.pendingResponse = nil

	// httptest.NewRequest, not http.NewRequest: this synthetic request never
	// travels over a real net/http transport (it's fed directly into
	// r.handler's http.HandlerFunc signature), so httptest's simpler
	// constructor (no URL-scheme/host validation, always succeeds) is the
	// right tool -- the same one this package's own tests already use to
	// build the fake sshd side.
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload.Request))
	req.Header.Set("X-CS-Session-ID", r.stableSessionID)
	// Bind the synthetic request's context to the relay's own lifecycle
	// (r.ctx) rather than leaving it as httptest.NewRequest's default
	// context.Background(): ApprovalHandler.HandlePermissionRequest selects
	// on r.Context().Done() as one of its three ways to stop waiting (see
	// its "Claude Code disconnected" case) -- wiring r.ctx here means
	// RemoteApprovalRelay.Stop() (e.g. the owning session being torn down)
	// unblocks an in-flight wait promptly instead of leaking it until
	// ApprovalHandler's own multi-minute timeout fires.
	req = req.WithContext(r.ctx)

	rec := httptest.NewRecorder()
	r.handler.HandlePermissionRequest(rec, req)

	if _, err := conn.Write(rec.Body.Bytes()); err != nil {
		log.Warn("remote approval relay: failed to write decision back to remote, buffering for redelivery on retry", "remote", r.remoteName, "sessionID", r.stableSessionID, "err", err)
		r.pendingRequest = append([]byte(nil), payload.Request...)
		r.pendingResponse = append([]byte(nil), rec.Body.Bytes()...)
		r.pendingAt = time.Now()
	}
}

// decodePayload reads and JSON-decodes exactly one relayedApprovalPayload
// from conn, bounded by r.dialTimeout. golang.org/x/crypto/ssh's Channel
// (what client.Dial("unix", ...) returns wrapped in a net.Conn) does NOT
// support SetReadDeadline -- calling it returns an explicit "deadline not
// supported" error rather than silently no-op'ing, confirmed empirically
// against this package's own test server -- so, following the
// force-close-on-timeout half of this repo's established goroutine-race
// pattern for blocking calls with no native cancellation support
// (session/tmux/ssh_pool.go's dialSSHContext takes the same approach; note
// session/sshremote/keystore.go's raceKeyringOp deliberately does NOT
// force-abort, since a keychain call has no connection to close -- the two
// precedents differ in that respect, only dialSSHContext's shape applies
// here), the decode runs in a goroutine raced against a ctx-bounded
// timeout, force-closing conn on expiry to unblock it. A slow
// or stuck remote-side writer therefore still can't wedge the relay's
// single poll goroutine indefinitely, even though the bound is enforced by
// closing the connection rather than by a deadline on the read itself.
func (r *RemoteApprovalRelay) decodePayload(conn net.Conn) (relayedApprovalPayload, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.dialTimeout)
	defer cancel()

	type decodeResult struct {
		payload relayedApprovalPayload
		err     error
	}
	resCh := make(chan decodeResult, 1)
	go func() {
		var payload relayedApprovalPayload
		err := json.NewDecoder(conn).Decode(&payload)
		resCh <- decodeResult{payload: payload, err: err}
	}()

	select {
	case res := <-resCh:
		return res.payload, res.err
	case <-ctx.Done():
		_ = conn.Close() // force-close to unblock the decode goroutine
		return relayedApprovalPayload{}, fmt.Errorf("sshremote: decode relayed approval payload: %w", ctx.Err())
	}
}

// verifyToken reports whether token matches the relay's current bearer
// credential and has not expired. Per research/pitfalls.md §4, this is what
// stops the relay from being an open channel any process on the remote
// host could forge approval traffic into: a payload failing this check is
// dropped, never forwarded into ExternalApprovalMonitor. Uses
// crypto/subtle.ConstantTimeCompare to avoid a timing side-channel on token
// comparison, consistent with how this repo treats other bearer-style
// credentials.
func (r *RemoteApprovalRelay) verifyToken(token string) bool {
	r.credMu.RLock()
	cred := r.cred
	r.credMu.RUnlock()

	if token == "" || cred.token == "" {
		return false
	}
	if time.Now().After(cred.expiresAt) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(cred.token)) == 1
}
