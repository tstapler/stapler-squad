package sshremote

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// remoteApprovalSocketName is the fixed filename RemoteApprovalRelay expects
// a remote session's injected agent hooks (Epic 5.2 -- not this package) to
// write approval-request payloads to, rooted under the session's remote
// base path. See RemoteApprovalSocketPath and Task 5.1.1a in
// project_plans/ssh-remote-workspaces/implementation/plan.md.
const remoteApprovalSocketName = ".stapler-squad-approval.sock"

// RemoteApprovalSocketPath returns the fixed remote-host Unix domain socket
// path convention for a session rooted at basePath on the remote host.
// Uses path.Join (POSIX-style forward slashes), deliberately NOT
// filepath.Join: basePath always names a location on the REMOTE host (a
// Linux/macOS SSH target, per RemoteConfig's base_path), so joining it must
// not switch to backslash separators when stapler-squad itself happens to
// be built on Windows.
func RemoteApprovalSocketPath(basePath string) string {
	return path.Join(basePath, remoteApprovalSocketName)
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

// relayedApprovalPayload is the JSON envelope the remote hook script (Epic
// 5.2, not this package) writes to the fixed Unix socket for a single
// approval request. Token must equal the relay's current bearer credential
// and must not be expired (see verifyToken); a payload failing either check
// is dropped without being forwarded into ExternalApprovalMonitor.
type relayedApprovalPayload struct {
	Token   string                    `json:"token"`
	Request detection.ApprovalRequest `json:"request"`
}

// RemoteApprovalRelay reads approval-request payloads a remote agent
// process writes to a fixed Unix domain socket on the remote host
// (RemoteApprovalSocketPath) via a direct-streamlocal@openssh.com channel
// dialed over the SAME pooled *ssh.Client the session's terminal stream
// already uses, and forwards them into the local ExternalApprovalMonitor.
// See ADR-003 (project_plans/ssh-remote-workspaces/decisions/) for why this
// reuses the existing multiplexed SSH connection instead of a reverse
// tunnel or a third-party tunneling library.
//
// This type implements Story 5.1.1's REQUEST direction only: a relayed
// approval request flows from the remote host to the local dashboard, but
// the local approve/deny decision is not yet written back over the channel
// to unblock the remote agent's blocking hook-script read -- that round
// trip is Epic 5.3 (Story 5.3.1), deliberately scoped out of Epic 5.1 (see
// plan.md's Epic split). Each accepted direct-streamlocal connection is
// therefore closed immediately after its single JSON payload is decoded
// and forwarded; Epic 5.3 will need to keep the connection open (or pair it
// with a second one) to write the response instead of closing it here.
//
// RemoteApprovalRelay never dials the SSH connection pool itself -- it only
// subscribes (via pool.Subscribe) to whichever *ssh.Client is CURRENTLY
// pooled for remoteName. Some other component (terminal streaming, per
// Phase 4/session/tmux.SSHRunner) is responsible for establishing and
// redialing that connection; decoupling the relay from dial config/
// credentials entirely means it has nothing more to configure than "which
// pooled connection" and "which remote-side socket."
type RemoteApprovalRelay struct {
	pool       *tmux.SSHClientPool
	monitor    *session.ExternalApprovalMonitor
	remoteName string
	socketPath string
	sessionKey string
	title      string

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
// string parameters: RemoteName/BasePath/SessionKey/Title are independently
// meaningful, same-typed strings a caller could silently transpose at a
// call site with no compiler error (e.g. passing SessionKey where Title is
// expected still compiles) -- exactly what
// .claude/rules/primitive-obsession-checklist.md exists to catch. Field
// names carry the disambiguation a positional four-string parameter list
// can't.
type RemoteApprovalRelayTarget struct {
	// RemoteName is the SSHClientPool key this relay's channel is dialed
	// under (session/tmux.SSHClientPool).
	RemoteName string
	// BasePath is the session's remote-host working-directory root;
	// RemoteApprovalSocketPath is derived from it.
	BasePath string
	// SessionKey is the key ExternalApprovalMonitor stores pending
	// approvals under. Must be unique per session, e.g.
	// "remote:<RemoteName>:<sessionID>" -- callers own this convention,
	// this package does not invent one.
	SessionKey string
	// Title is the display title shown in the local approval UI.
	Title string
}

// NewRemoteApprovalRelay constructs a RemoteApprovalRelay for a single
// remote session: pool is the shared SSH connection pool target.RemoteName
// is dialed under; monitor is the local ExternalApprovalMonitor relayed
// requests are forwarded into. See RemoteApprovalRelayTarget's field docs
// for the rest.
//
// A fresh, random bearer credential is minted immediately (see BearerToken)
// -- callers wiring hook injection (Epic 5.2) read it back via BearerToken
// to embed in the generated hook command.
func NewRemoteApprovalRelay(
	pool *tmux.SSHClientPool,
	monitor *session.ExternalApprovalMonitor,
	target RemoteApprovalRelayTarget,
	opts ...RemoteApprovalRelayOption,
) (*RemoteApprovalRelay, error) {
	if pool == nil {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: pool must not be nil")
	}
	if monitor == nil {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: monitor must not be nil")
	}
	if target.RemoteName == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.RemoteName must not be empty")
	}
	if target.BasePath == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.BasePath must not be empty")
	}
	if target.SessionKey == "" {
		return nil, errors.New("sshremote: NewRemoteApprovalRelay: target.SessionKey must not be empty")
	}

	cred, err := newBearerCredential(defaultBearerTokenTTL)
	if err != nil {
		return nil, err
	}

	r := &RemoteApprovalRelay{
		pool:         pool,
		monitor:      monitor,
		remoteName:   target.RemoteName,
		socketPath:   RemoteApprovalSocketPath(target.BasePath),
		sessionKey:   target.SessionKey,
		title:        target.Title,
		pollInterval: defaultPollInterval,
		dialTimeout:  defaultDialTimeout,
		cred:         cred,
		wake:         make(chan struct{}, 1),
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

		log.Info("remote approval relay started", "remote", r.remoteName, "socketPath", r.socketPath, "sessionKey", r.sessionKey)
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
// verifies its bearer token, and -- on success -- forwards the request into
// ExternalApprovalMonitor via IngestRelayedApproval. The connection is
// always closed before returning: Story 5.1.1 is request-direction only
// (see RemoteApprovalRelay's doc comment), so there is no response to write
// back yet.
func (r *RemoteApprovalRelay) handleConnection(conn net.Conn) {
	defer conn.Close() //nolint:errcheck // best-effort; the payload has already been read (or reading failed) by the time this runs

	payload, err := r.decodePayload(conn)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Warn("remote approval relay: decode payload failed", "remote", r.remoteName, "err", err)
		}
		return
	}

	if !r.verifyToken(payload.Token) {
		log.Warn("remote approval relay: rejected payload with invalid or expired bearer token", "remote", r.remoteName, "sessionKey", r.sessionKey)
		return
	}

	request := payload.Request
	r.monitor.IngestRelayedApproval(r.sessionKey, r.title, &request)
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
