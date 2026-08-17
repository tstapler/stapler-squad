package sshremote

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// RemoteConnectionState identifies where a configured remote's SSH
// connection currently stands, as tracked by RemoteHealthProber. No prior
// definition of this concept exists anywhere in the codebase (grepped
// session/, server/, pkg/ for RemoteConnectionState/ConnectionState before
// adding this) -- defined here, alongside the prober that owns the state
// machine, rather than in pkg/events: pkg/events imports session, and
// session (session/instance.go) already imports session/sshremote, so
// session/sshremote importing pkg/events back would be an import cycle
// (confirmed via `go list -deps`). This mirrors why PermissionRequestHandler
// is defined in THIS package rather than imported from server/services (see
// approval_relay.go's doc comment) -- same cycle shape, one layer over.
// pkg/events references this type instead (session/sshremote has no
// dependency back on pkg/events or session, so that direction is cycle-free)
// -- the same shape session/detection.DetectedStatus already has via
// pkg/events.Event.DetectedStatusTyped.
type RemoteConnectionState string

const (
	// RemoteConnectionStateDisconnected is a RemoteHealthProber's initial
	// state, and the state entered when either (a) the pooled *ssh.Client's
	// Wait() returns (a hard, push-driven disconnect signal), or (b) a
	// reconnect attempt has not yet succeeded.
	RemoteConnectionStateDisconnected RemoteConnectionState = "disconnected"
	// RemoteConnectionStateConnected is entered once the pooled *ssh.Client
	// is live and the most recent liveness check succeeded.
	RemoteConnectionStateConnected RemoteConnectionState = "connected"
	// RemoteConnectionStateReconnecting is a soft, between-hard-disconnects
	// signal: the pooled *ssh.Client's Wait() has not returned (no hard
	// teardown observed), but the most recent periodic liveness check (a
	// trivial no-op remote command) failed -- e.g. a stalled or degraded
	// network path, or the remote host rejecting new channels while the
	// underlying transport connection is still technically up.
	RemoteConnectionStateReconnecting RemoteConnectionState = "reconnecting"
)

// HealthEventPublisher is anything that can be notified of a remote's
// connection-state transition. Defined here (the consumer package) rather
// than importing pkg/events.EventBus/NewRemoteHealthChangedEvent directly,
// which would create the import cycle described on RemoteConnectionState's
// doc comment -- mirrors PermissionRequestHandler's identical rationale in
// approval_relay.go.
//
// Unlike PermissionRequestHandler (which *server/services.ApprovalHandler
// already satisfies structurally with zero changes), no existing production
// type satisfies this interface yet: pkg/events.EventBus.Publish takes a
// *pkg/events.Event, not this signature. The production wiring that starts
// a RemoteHealthProber per configured remote (Task 6.4.1c, server/server.go
// -- out of scope for this change) is expected to supply a small adapter
// that calls events.NewRemoteHealthChangedEvent(remoteName, state,
// previousState) and hands the result to an *events.EventBus.Publish.
type HealthEventPublisher interface {
	// PublishRemoteHealthChanged is called synchronously on every actual
	// state transition (never on a no-op "transitioned to the state it was
	// already in" check) -- implementations that do I/O should not block
	// the prober's watcher/liveness goroutines for long.
	PublishRemoteHealthChanged(remoteName string, state, previousState RemoteConnectionState)
}

// defaultLivenessCheckInterval bounds how often RemoteHealthProber issues a
// trivial no-op remote command over the shared pooled connection to detect
// RemoteConnectionStateReconnecting, per Task 6.4.1b. Every check also
// doubles as a reconnect attempt when nothing is currently pooled --
// SSHRunner.Run's own client() step dials through the shared
// SSHClientPool, governed by SSHRunner's own backoff/circuit-breaker, so
// this prober does not reimplement backoff/reconnect-retry logic itself.
const defaultLivenessCheckInterval = 15 * time.Second

// defaultLivenessCheckTimeout bounds a single liveness check so a stalled
// remote can't wedge the prober's liveness-check goroutine indefinitely.
// Mirrors approval_relay.go's defaultDialTimeout convention.
const defaultLivenessCheckTimeout = 10 * time.Second

// RemoteHealthProber tracks one configured remote's SSH connection health
// and publishes push-driven connected/reconnecting/disconnected state
// transitions via HealthEventPublisher, per Epic 6.4 / Story 6.4.1.
//
// It never dials a dedicated connection of its own: runner must be
// constructed against the SAME SSHClientPool passed as pool (and the same
// SSHTarget.Name as remoteName), so both this prober's liveness checks and
// any session's own SSHRunner/RemoteApprovalRelay for the same remote share
// exactly one dialed *ssh.Client (see tmux.SSHRunner.Dial's doc comment,
// which names RemoteHealthProber as its reuse case). Disconnect detection
// is driven primarily by that shared client's Wait() -- a push/blocking
// signal, not a poll loop -- with a periodic lightweight liveness check
// (SSHRunner.Run(ctx, "", "true")) filling the gap Wait() alone can't see:
// a connection that hasn't hard-failed yet but also isn't currently
// answering commands.
type RemoteHealthProber struct {
	pool       *tmux.SSHClientPool
	runner     *tmux.SSHRunner
	remoteName string
	publisher  HealthEventPublisher

	livenessInterval time.Duration
	livenessTimeout  time.Duration

	stateMu sync.Mutex
	state   RemoteConnectionState

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once
}

// RemoteHealthProberOption configures a RemoteHealthProber at construction
// time.
type RemoteHealthProberOption func(*RemoteHealthProber)

// withLivenessCheckInterval overrides the default liveness-check interval.
// Unexported: only same-package tests need a faster-than-production
// interval to keep test runtime bounded, mirroring approval_relay.go's
// withPollInterval.
func withLivenessCheckInterval(d time.Duration) RemoteHealthProberOption {
	return func(p *RemoteHealthProber) { p.livenessInterval = d }
}

// withLivenessCheckTimeout overrides the default per-check timeout.
// Unexported for the same reason as withLivenessCheckInterval.
func withLivenessCheckTimeout(d time.Duration) RemoteHealthProberOption {
	return func(p *RemoteHealthProber) { p.livenessTimeout = d }
}

// NewRemoteHealthProber constructs a RemoteHealthProber for a single
// configured remote: pool is the shared SSH connection pool remoteName is
// dialed/pooled under; runner is used for this prober's periodic liveness
// checks and MUST be constructed against the same pool and the same
// SSHTarget.Name as remoteName (see this type's doc comment); publisher
// receives every actual state transition.
func NewRemoteHealthProber(
	pool *tmux.SSHClientPool,
	runner *tmux.SSHRunner,
	remoteName string,
	publisher HealthEventPublisher,
	opts ...RemoteHealthProberOption,
) (*RemoteHealthProber, error) {
	if pool == nil {
		return nil, errors.New("sshremote: NewRemoteHealthProber: pool must not be nil")
	}
	if runner == nil {
		return nil, errors.New("sshremote: NewRemoteHealthProber: runner must not be nil")
	}
	if remoteName == "" {
		return nil, errors.New("sshremote: NewRemoteHealthProber: remoteName must not be empty")
	}
	if publisher == nil {
		return nil, errors.New("sshremote: NewRemoteHealthProber: publisher must not be nil")
	}

	p := &RemoteHealthProber{
		pool:             pool,
		runner:           runner,
		remoteName:       remoteName,
		publisher:        publisher,
		livenessInterval: defaultLivenessCheckInterval,
		livenessTimeout:  defaultLivenessCheckTimeout,
		state:            RemoteConnectionStateDisconnected,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// State returns the prober's current view of the remote's connection state.
// Exposed for tests and observability, mirroring SSHClientPool.RefCount's
// convention.
func (p *RemoteHealthProber) State() RemoteConnectionState {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.state
}

// Start begins the prober's two background loops: watchReconnects (the
// push-driven path, subscribed to pool's reconnect signal for remoteName)
// and runLivenessLoop (the periodic soft-degradation/reconnect-attempt
// path). Safe to call at most once per RemoteHealthProber; later calls are
// a no-op.
func (p *RemoteHealthProber) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		p.ctx, p.cancel = context.WithCancel(ctx)

		ch, unsubscribe := p.pool.Subscribe(p.remoteName)
		p.wg.Add(2)
		go p.watchReconnects(ch, unsubscribe)
		go p.runLivenessLoop()

		log.Info("remote health prober started", "remote", p.remoteName)
	})
}

// Stop halts the prober's watchReconnects and runLivenessLoop goroutines
// and waits for both to exit. Safe to call multiple times, or without a
// prior Start.
//
// Stop does NOT wait for any in-flight watchClientDeath goroutine (see its
// doc comment for why: it blocks on the pooled *ssh.Client's own Wait(),
// which has no relationship to this prober's ctx and may not return for as
// long as the underlying connection stays alive -- exactly the same
// unbounded-lifetime shape session/tmux/ssh_pool.go's own internal
// Client.Wait() eviction watcher already has, and for the same reason: the
// shared client can legitimately outlive any single consumer). Those
// goroutines check p.ctx.Done() before publishing, so a stale post-Stop
// disconnect signal is dropped rather than delivered.
func (p *RemoteHealthProber) Stop() {
	p.stopOnce.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		p.wg.Wait()
		log.Info("remote health prober stopped", "remote", p.remoteName)
	})
}

// watchReconnects consumes pool's reconnect-notification channel for
// remoteName for as long as the prober is running -- both the initial
// priming value (if a client is already pooled when Start is called) and
// every subsequent redial. Each delivered client is treated as "now
// connected" and gets its own watchClientDeath watcher.
func (p *RemoteHealthProber) watchReconnects(ch <-chan *ssh.Client, unsubscribe func()) {
	defer p.wg.Done()
	defer unsubscribe()
	for {
		select {
		case <-p.ctx.Done():
			return
		case client, ok := <-ch:
			if !ok {
				return
			}
			p.transition(RemoteConnectionStateConnected)
			go p.watchClientDeath(client)
		}
	}
}

// watchClientDeath blocks on client.Wait() -- the push/blocking hard-
// disconnect signal Story 6.4.1's acceptance criteria calls for. Safe to
// run alongside session/tmux/ssh_pool.go's OWN internal Client.Wait()-based
// eviction watcher (SSHClientPool.register spawns one per dial too):
// golang.org/x/crypto/ssh's mux.Wait() blocks on a sync.Cond and the
// connection's read loop calls Broadcast (not Signal) once on close
// (verified against the vendored golang.org/x/crypto v0.53.0 source), so
// every concurrent Wait() caller wakes independently rather than racing to
// "win" a single wakeup.
//
// Once Wait() returns, this only transitions to disconnected if client is
// STILL the current pooled client for p.remoteName -- mirrors
// ssh_pool.go's evictIfCurrent "current-check" idiom (`e.client != client`)
// -- so that a stale death signal for an already-replaced connection can't
// clobber a fresher reconnect's own connected transition back to
// disconnected. Deliberately not tracked by p.wg (see Stop's doc comment).
func (p *RemoteHealthProber) watchClientDeath(client *ssh.Client) {
	_ = client.Wait()

	select {
	case <-p.ctx.Done():
		return
	default:
	}

	if current, ok := p.pool.Peek(p.remoteName); ok && current != client {
		return
	}
	p.transition(RemoteConnectionStateDisconnected)
}

// runLivenessLoop periodically calls checkLiveness until the prober is
// stopped.
func (p *RemoteHealthProber) runLivenessLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.livenessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkLiveness()
		}
	}
}

// checkLiveness exercises the shared pooled connection with a trivial
// no-op remote command (SSHRunner.Run(ctx, "", "true")), bounded by
// p.livenessTimeout. A failure while currently connected is exactly the
// RemoteConnectionStateReconnecting soft-degradation signal a Wait()-only
// disconnect check can't see on its own (Task 6.4.1b); a failure while
// already disconnected or reconnecting is a no-op transition (no re-
// publish -- see transition's doc comment) rather than a repeated signal,
// so a remote that's down for an extended period doesn't flood the event
// bus with one event per tick. A success always converges the state to
// connected, whether recovering from Reconnecting (the expected path) or
// from Disconnected (e.g. this check's own reconnect attempt won the race
// against watchReconnects's Subscribe notification).
func (p *RemoteHealthProber) checkLiveness() {
	ctx, cancel := context.WithTimeout(p.ctx, p.livenessTimeout)
	defer cancel()

	_, err := p.runner.Run(ctx, "", "true")

	// Stop() cancelling p.ctx while this check is in flight surfaces here as a
	// context.Canceled-wrapping error from runner.Run, indistinguishable in shape
	// from a genuine liveness failure -- without this guard, a prober with a
	// check in flight at shutdown would publish a spurious connected->reconnecting
	// transition on every stop (e.g. every service restart), exactly the "publish
	// on every tick" noise this type exists to avoid. Mirrors watchClientDeath's
	// identical guard.
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	if err != nil {
		if p.State() == RemoteConnectionStateConnected {
			p.transition(RemoteConnectionStateReconnecting)
		}
		return
	}
	p.transition(RemoteConnectionStateConnected)
}

// transition atomically compares-and-sets p.state, publishing via
// p.publisher ONLY when newState actually differs from the prior state --
// the "publish on state transitions... not on every tick" requirement from
// this epic's goal. The compare-and-set itself happens under stateMu; the
// publisher call happens after releasing it, so a slow/blocking publisher
// implementation can't stall the next transition's lock acquisition.
func (p *RemoteHealthProber) transition(newState RemoteConnectionState) {
	p.stateMu.Lock()
	old := p.state
	if old == newState {
		p.stateMu.Unlock()
		return
	}
	p.state = newState
	p.stateMu.Unlock()

	log.Info("remote health state transition", "remote", p.remoteName, "from", string(old), "to", string(newState))
	p.publisher.PublishRemoteHealthChanged(p.remoteName, newState, old)
}
