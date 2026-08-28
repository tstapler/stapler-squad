package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// ErrRegistryUnavailable is returned when the registry is not healthy and cannot
// serve a request that requires the registry to be up.
var ErrRegistryUnavailable = errors.New("tmux registry unavailable")

// Compile-time interface checks.
var _ SessionExistenceChecker = (*TmuxServerRegistry)(nil)
var _ SessionLister = (*TmuxServerRegistry)(nil)
var _ PaneExitSubscriber = (*TmuxServerRegistry)(nil)
var _ TmuxStatePort = (*TmuxServerRegistry)(nil)

// paneExitSub is a single pane-exit subscriber. sync.Once ensures the channel
// is closed exactly once regardless of which code path (ctx cancel, firePaneExit,
// or Stop) reaches the close first.
type paneExitSub struct {
	ch   chan struct{}
	once sync.Once
}

func (s *paneExitSub) close() { s.once.Do(func() { close(s.ch) }) }

// TmuxServerRegistry maintains a single tmux control-mode connection to a tmux
// server and pushes session-lifecycle events into an in-memory map. Callers
// query the map directly instead of forking tmux subprocesses.
type TmuxServerRegistry struct {
	serverSocket string

	mu       sync.RWMutex
	sessions map[string]bool

	// closedAt pins a session as absent for closedPinWindow after an explicit
	// close (NotifySessionClosed or a %session-closed event), so a
	// syncSessionsLocked fetch that was already in flight when the close
	// happened can't resurrect it by overwriting r.sessions with a stale
	// "still exists" snapshot. See BUG-074.
	closedAt map[string]time.Time

	// subsMu guards subscribers. CRITICAL: never close(ch) while holding subsMu.
	// Copy subscribers out under the lock, release the lock, then close outside.
	subsMu      sync.Mutex
	subscribers map[string][]*paneExitSub

	healthMu sync.RWMutex
	healthy  bool

	// syncMu serializes syncSessionsLocked()'s fetch -> diff -> swap sequence so
	// two calls can never interleave and let a slower, stale call overwrite a
	// newer snapshot already applied to r.sessions. The 3 pre-existing callers
	// (debounce timer, reconnectLoop post-connect, Start) acquire it via the
	// blocking syncSessions(); the fast-recheck path acquires it via the
	// non-blocking syncSessionsFastRecheck() (TryLock, skip-if-busy) so a
	// fast-recheck attempt never waits on this mutex -- see
	// waitBackoffWithFastRecheck's ponytail: comment for why that matters.
	syncMu sync.Mutex

	// hookMu guards fastRecheckWaitStartHook, a test-only observability hook.
	// See SetFastRecheckWaitStartHook.
	hookMu                   sync.Mutex
	fastRecheckWaitStartHook func(backoff time.Duration)

	ctx    context.Context
	cancel context.CancelFunc
}

// NewTmuxServerRegistry creates a new registry for the given server socket.
// Call Start(ctx) to begin listening for events.
func NewTmuxServerRegistry(serverSocket string) *TmuxServerRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &TmuxServerRegistry{
		serverSocket: serverSocket,
		sessions:     make(map[string]bool),
		closedAt:     make(map[string]time.Time),
		subscribers:  make(map[string][]*paneExitSub),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start launches the control-mode process and begins processing events.
// It bootstraps the session map from list-sessions before marking the registry
// healthy. The returned error is non-nil only when the initial setup fails in a
// way that makes a retry impossible.
func (r *TmuxServerRegistry) Start(ctx context.Context) error {
	// Derive a child context so Stop() can cancel without affecting the caller.
	childCtx, childCancel := context.WithCancel(ctx)
	r.healthMu.Lock()
	r.healthy = false
	r.healthMu.Unlock()

	// Replace internal ctx/cancel with the derived pair so Stop() works.
	r.cancel()
	r.ctx = childCtx
	r.cancel = childCancel

	// Bootstrap session map from list-sessions before connecting control mode.
	if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {
		log.Warn("[registry] initial list-sessions failed, continuing", "err", err)
	}

	// Launch the auto-reconnect loop; it starts the first control-mode process.
	go r.reconnectLoop()

	return nil
}

// Stop shuts down the registry and closes all pending subscriber channels.
func (r *TmuxServerRegistry) Stop() {
	r.cancel()

	// Copy all subscribers under the lock, then close outside.
	r.subsMu.Lock()
	var allSubs []*paneExitSub
	for _, subs := range r.subscribers {
		allSubs = append(allSubs, subs...)
	}
	r.subscribers = make(map[string][]*paneExitSub)
	r.subsMu.Unlock()

	for _, sub := range allSubs {
		sub.close()
	}

	r.healthMu.Lock()
	r.healthy = false
	r.healthMu.Unlock()
}

// SessionExists implements SessionExistenceChecker.
func (r *TmuxServerRegistry) SessionExists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[name]
}

// ListSessions implements SessionLister. Returns a copy of the live sessions map.
func (r *TmuxServerRegistry) ListSessions() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool, len(r.sessions))
	for k, v := range r.sessions {
		out[k] = v
	}
	return out
}

// SetFastRecheckWaitStartHook installs a callback invoked synchronously, on
// reconnectLoop's own goroutine, at the moment a wait's backoff reaches
// fastRecheckMinBackoff -- i.e. a wait that is about to run its fast-recheck
// attempts. It exists so tests can deterministically synchronize with the
// start of a fast-recheck window instead of estimating cycle timing; see
// TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff in
// server_registry_integration_test.go for why the estimate approach was flaky.
//
// The hook must not block: it runs inline on reconnectLoop's goroutine and
// delays the fast-recheck attempts themselves for as long as it runs. Pass
// nil to remove the hook (the default; safe in production since nothing
// else populates this field).
func (r *TmuxServerRegistry) SetFastRecheckWaitStartHook(hook func(backoff time.Duration)) {
	r.hookMu.Lock()
	r.fastRecheckWaitStartHook = hook
	r.hookMu.Unlock()
}

func (r *TmuxServerRegistry) fireFastRecheckWaitStartHook(backoff time.Duration) {
	r.hookMu.Lock()
	hook := r.fastRecheckWaitStartHook
	r.hookMu.Unlock()
	if hook != nil {
		hook(backoff)
	}
}

// IsHealthy implements SessionExistenceChecker and SessionLister.
func (r *TmuxServerRegistry) IsHealthy() bool {
	r.healthMu.RLock()
	defer r.healthMu.RUnlock()
	return r.healthy
}

// NotifySessionCreated proactively marks a session as existing in the registry.
// Called by TmuxSession.start() after a new session is confirmed via list-sessions,
// so that DoesSessionExist() returns true before the async %session-created
// control-mode event is processed.
func (r *TmuxServerRegistry) NotifySessionCreated(name string) {
	r.mu.Lock()
	r.sessions[name] = true
	delete(r.closedAt, name)
	r.mu.Unlock()
}

// NotifySessionClosed proactively marks a session as gone in the registry.
// Called by TmuxSession.Close() right after a synchronous "kill-session"
// subprocess confirms the session is dead, so DoesSessionExist()'s registry
// fast path returns false immediately instead of trusting a stale "exists"
// entry until the async %session-closed control-mode event is processed --
// the symmetric counterpart of NotifySessionCreated's create-side fast path.
func (r *TmuxServerRegistry) NotifySessionClosed(name string) {
	r.mu.Lock()
	delete(r.sessions, name)
	r.closedAt[name] = time.Now()
	r.mu.Unlock()
}

// SubscribePaneExit implements PaneExitSubscriber. The returned channel is
// closed when the named session/pane exits or when ctx is cancelled.
func (r *TmuxServerRegistry) SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{} {
	sub := &paneExitSub{ch: make(chan struct{}, 1)}

	r.subsMu.Lock()
	r.subscribers[sessionName] = append(r.subscribers[sessionName], sub)
	r.subsMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			// Remove our subscriber from the list; close via Once (safe if
			// firePaneExit already closed it concurrently).
			r.subsMu.Lock()
			existing := r.subscribers[sessionName]
			filtered := existing[:0]
			for _, s := range existing {
				if s != sub {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) == 0 {
				delete(r.subscribers, sessionName)
			} else {
				r.subscribers[sessionName] = filtered
			}
			r.subsMu.Unlock()
			sub.close()
		case <-sub.ch:
			// Closed by firePaneExit or Stop; nothing to do.
		case <-r.ctx.Done():
			// Registry is stopping; channel will be closed by Stop().
		}
	}()

	return sub.ch
}

// firePaneExit closes all subscriber channels for sessionName. It copies the
// subscribers out under the lock and then closes them outside to prevent deadlock.
func (r *TmuxServerRegistry) firePaneExit(sessionName string) {
	r.subsMu.Lock()
	subs := r.subscribers[sessionName]
	delete(r.subscribers, sessionName)
	r.subsMu.Unlock()
	// Lock NOT held here — close outside the critical section.
	for _, sub := range subs {
		sub.close()
	}
}

// defaultSyncTimeout is the list-sessions subprocess budget used by every
// syncSessions() caller except the fast-recheck path (see
// fastRecheckSyncTimeout in waitBackoffWithFastRecheck).
const defaultSyncTimeout = 10 * time.Second

// closedPinWindow bounds how long a name recently marked closed (via
// NotifySessionClosed or a %session-closed event) is protected from being
// resurrected by a syncSessionsLocked swap. Needed because a list-sessions
// fetch already in flight when the close happens can return a stale
// "still exists" snapshot that would otherwise clobber the close. 2s comfortably
// covers list-sessions' subprocess latency plus the 50ms debounce delay.
const closedPinWindow = 2 * time.Second

// syncSessions acquires syncMu and runs the fetch-diff-swap sequence. Used
// by the 3 pre-existing callers with no tight latency budget (Start,
// reconnectLoop's post-connect sync, the debounce callback). The
// fast-recheck path uses syncSessionsFastRecheck instead (see
// waitBackoffWithFastRecheck) so it never blocks on syncMu. ctx bounds
// cancellation (Stop() aborts a call in flight); timeout bounds the
// subprocess itself.
func (r *TmuxServerRegistry) syncSessions(ctx context.Context, timeout time.Duration) error {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	return r.syncSessionsLocked(ctx, timeout)
}

// syncSessionsLocked runs list-sessions, diffs the result against
// r.sessions, swaps the map, and fires pane-exit for every session that
// disappeared. Callers MUST already hold syncMu for the whole call -- this
// method never acquires or releases it itself, so the two lock-acquisition
// strategies (blocking Lock in syncSessions, non-blocking TryLock in
// syncSessionsFastRecheck) share one fetch-diff-swap implementation instead
// of duplicating it.
func (r *TmuxServerRegistry) syncSessionsLocked(ctx context.Context, timeout time.Duration) error {
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := prependSocket(r.serverSocket, []string{"list-sessions", "-F", "#{session_name}"})
	cmd := safeexec.CommandContext(fetchCtx, Binary(), args...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list-sessions: %w", err)
	}

	sessions := make(map[string]bool)
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" {
			sessions[name] = true
		}
	}

	// Identify sessions that existed before but are now gone. We fire
	// pane-exit for them after releasing the lock so subscribers are
	// notified even when control-mode events were missed (e.g. in a
	// headless environment where the control-mode connection is short-lived).
	r.mu.Lock()
	now := time.Now()
	for name, closedAt := range r.closedAt {
		if now.Sub(closedAt) < closedPinWindow {
			// A close happened after (or concurrently with) this fetch was
			// issued -- honor the more recent close rather than the stale
			// "still exists" snapshot list-sessions may have captured.
			delete(sessions, name)
		} else {
			delete(r.closedAt, name)
		}
	}
	var disappeared []string
	for name := range r.sessions {
		if !sessions[name] {
			disappeared = append(disappeared, name)
		}
	}
	r.sessions = sessions
	r.mu.Unlock()

	for _, name := range disappeared {
		r.firePaneExit(name)
	}

	return nil
}

// startControlMode ensures the keepalive sentinel session exists, then launches
// "tmux [-L socket] -C attach-session -t keepalive" and returns the command
// along with a scanner for its stdout. It does NOT block.
//
// IMPORTANT: tmux control-mode exits with %exit when it reads EOF on stdin.
// We must create a stdin pipe and hold it open for the lifetime of the process.
// The returned io.WriteCloser is the stdin pipe; Close() it to signal shutdown.
func (r *TmuxServerRegistry) startControlMode() (*exec.Cmd, *bufio.Scanner, io.WriteCloser, error) {
	keepaliveName := TmuxPrefix + "keepalive"

	// Only create the keepalive sentinel on the default server (empty socket).
	// Isolated servers (e.g., test harnesses using -L <socket>) manage their own
	// session lifecycle and must not have a keepalive injected into them.
	if r.serverSocket == "" {
		// Ensure the sentinel session exists so attach-session doesn't exit immediately.
		// "new-session -d -s <name>" is idempotent: if the session already exists tmux
		// exits with a non-zero code which we intentionally ignore.
		//
		// Routed through prependSocket like the attach-session call below: r.serverSocket == ""
		// here means "this registry didn't request an explicitly isolated socket," but the
		// actual command must still resolve through ResolveSocket so a `go test` binary never
		// issues an unscoped call straight at the real shared default socket.
		createArgs := prependSocket(r.serverSocket, []string{"new-session", "-d", "-s", keepaliveName})
		keepaliveCtx, keepaliveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer keepaliveCancel()
		keepaliveCmd := safeexec.CommandContext(keepaliveCtx, Binary(), createArgs...)
		_ = keepaliveCmd.Run()
	}

	// No -r flag: read-only is irrelevant for event monitoring, and it caused
	// immediate %exit on some tmux versions.
	baseArgs := []string{"-C", "attach-session", "-t", keepaliveName}
	args := prependSocket(r.serverSocket, baseArgs)
	// lifecycle managed by caller via r.ctx (see reconnectLoop) — but ctx
	// cancellation never runs if this process is SIGKILLed (e.g. a
	// `--mcp` invocation killed by its parent), so EnsurePdeathsig backs
	// that up at the kernel level.
	cmd := exec.CommandContext(r.ctx, Binary(), args...) //nolint:norawexec long-running cmd.Start() process
	safeexec.EnsurePdeathsig(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("StdoutPipe: %w", err)
	}
	// Hold stdin open — tmux sends %exit and terminates when it reads EOF on stdin.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		return nil, nil, nil, fmt.Errorf("StdinPipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stdin.Close()
		return nil, nil, nil, fmt.Errorf("cmd.Start: %w", err)
	}
	TrackChildPID(cmd.Process.Pid, "tmux registry control-mode socket="+r.serverSocket)
	return cmd, bufio.NewScanner(stdout), stdin, nil
}

// reconnectLoop starts the control-mode process and reconnects with exponential
// backoff whenever it exits. It exits when the registry context is cancelled.
func (r *TmuxServerRegistry) reconnectLoop() {
	const (
		backoffBase = 100 * time.Millisecond
		backoffCap  = 30 * time.Second
	)
	backoff := backoffBase

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		cmd, scanner, stdin, err := r.startControlMode()
		if err != nil {
			log.Warn("[registry] control-mode start failed, retrying", "err", err, "backoff", backoff)
			r.waitBackoffWithFastRecheck(backoff)
			select {
			case <-r.ctx.Done():
				return
			default:
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
			continue
		}

		// Resync the session map before marking healthy so that sessions
		// created while the control-mode connection was down are not missed.
		if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {
			log.Warn("[registry] syncSessions on reconnect failed", "err", err)
		}

		// Mark healthy now that the control-mode process is running.
		r.healthMu.Lock()
		r.healthy = true
		r.healthMu.Unlock()

		// Yield so that other goroutines can observe the healthy state before
		// readLines processes the first event (which may immediately clear it).
		runtime.Gosched()

		log.Info("[registry] control-mode connected", "socket", r.serverSocket)

		connectTime := time.Now()

		// readLines blocks until the process exits or the context is cancelled.
		r.readLines(scanner)

		// Only reset backoff if the connection was stable for a meaningful
		// duration. Resetting on a connection that dies immediately (e.g. tmux
		// server unhealthy, keepalive session missing) would prevent exponential
		// backoff from protecting against fork-rate explosion.
		const minStableConnection = 5 * time.Second
		if time.Since(connectTime) >= minStableConnection {
			backoff = backoffBase
		}

		// Closing stdin signals tmux to exit cleanly (it sends %exit on EOF).
		stdin.Close()
		// Clean up the process.
		UntrackChildPID(cmd.Process.Pid)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		r.healthMu.Lock()
		r.healthy = false
		r.healthMu.Unlock()

		select {
		case <-r.ctx.Done():
			return
		default:
			log.Info("[registry] control-mode exited; reconnecting", "backoff", backoff)
			r.waitBackoffWithFastRecheck(backoff)
			select {
			case <-r.ctx.Done():
				return
			default:
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
		}
	}
}

// syncSessionsFastRecheck attempts syncSessionsLocked's fetch-diff-swap
// sequence only if syncMu is immediately available. If another caller
// (Start, reconnectLoop's post-connect sync, or the debounce callback) is
// already mid-sync, this returns nil without doing any work instead of
// blocking -- that in-flight call will itself observe and fire any
// disappearance once it completes, so no detection is permanently lost,
// only this specific fast-recheck attempt is skipped. This is what keeps
// waitBackoffWithFastRecheck's own worst-case latency bounded to
// fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval)
// regardless of what any other caller is doing -- see the ponytail: comment
// below.
func (r *TmuxServerRegistry) syncSessionsFastRecheck(ctx context.Context, timeout time.Duration) error {
	if !r.syncMu.TryLock() {
		return nil
	}
	defer r.syncMu.Unlock()
	return r.syncSessionsLocked(ctx, timeout)
}

// fastRecheckAttempts, fastRecheckSyncTimeout, and fastRecheckInterval are
// package-level (rather than function-local to waitBackoffWithFastRecheck)
// so other files in package tmux -- e.g. server_registry_test.go's
// white-box tests -- can reference the real values directly instead of
// keeping their own copies that could silently drift out of sync.
//
// ponytail: fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval)
// = 700ms is a hard ceiling on fast-recheck's OWN polling delay during a
// backoff sleep -- not a ceiling on total detection latency regardless of
// what other callers are doing. Each attempt goes through
// syncSessionsFastRecheck, which TryLocks syncMu and skips (does no work,
// returns nil) rather than blocking if another caller (Start,
// reconnectLoop's post-connect sync, or the debounce callback -- all of
// which use the blocking syncSessions and can hold syncMu for up to
// defaultSyncTimeout=10s) is already mid-sync. So a fast-recheck attempt
// itself never waits on syncMu, and this loop's own elapsed time is
// always bounded by this ceiling. In the specific case where an attempt
// is skipped due to contention, the in-flight caller that holds the lock
// will itself observe and fire the disappearance once it finishes --
// just not necessarily within this 700ms window. backoff itself climbs
// 100ms..30s (backoffBase..backoffCap) and is tuned to protect a
// possibly-unhealthy tmux server from a reconnect fork-rate explosion
// (see minStableConnection above) -- detection latency is a separate,
// caller-facing guarantee (SubscribePaneExit) that must not inherit
// backoff's slowness. Keep these decoupled: widen backoff freely, but
// any change to these three constants must keep this ceiling accurate.
const (
	fastRecheckAttempts    = 2
	fastRecheckSyncTimeout = 150 * time.Millisecond
	fastRecheckInterval    = 200 * time.Millisecond
)

// waitBackoffWithFastRecheck blocks for up to backoff (or until r.ctx is
// cancelled) — the same contract as a plain time.After(backoff) wait — but
// also makes a small, bounded number of independent syncSessionsFastRecheck
// calls while it waits, so a pane exit during a long backoff sleep is
// detected quickly instead of only on the next successful reconnect.
func (r *TmuxServerRegistry) waitBackoffWithFastRecheck(backoff time.Duration) {
	// fastRecheckMinBackoff gates fast-recheck to backoff waits that could
	// plausibly outlast a caller's detection budget (e.g. the 3s deadline in
	// TestTmuxServerRegistry_PaneExitChannel). Below this threshold the plain
	// wait alone already leaves ample margin, so skip fast-recheck entirely --
	// otherwise every early, short cycle (100/200/400/800ms) pays an extra
	// list-sessions subprocess fork for zero detection benefit, adding fork
	// pressure right during the reconnect-storm window. Added after this fix's
	// own verification measured a real failure-rate regression without it; see
	// requirements.md's "Post-implementation finding" for the evidence. Below
	// this threshold, detection latency is NOT decoupled from backoff -- see
	// that same section and plan.md Story 1.1.2 for the narrowed guarantee.
	const fastRecheckMinBackoff = 1600 * time.Millisecond
	if backoff < fastRecheckMinBackoff {
		select {
		case <-r.ctx.Done():
		case <-time.After(backoff):
		}
		return
	}

	// Fire the test-observability hook (see SetFastRecheckWaitStartHook) right
	// as this wait qualifies for fast-recheck, before the deadline timer and
	// attempts loop below start -- this is the earliest point at which a test
	// can act (e.g. kill a session) and be guaranteed the resulting change is
	// still visible to this wait's own fast-recheck attempts.
	r.fireFastRecheckWaitStartHook(backoff)

	// See the ponytail: comment on the fastRecheckAttempts/fastRecheckSyncTimeout/
	// fastRecheckInterval declaration above for the 700ms ceiling this loop
	// enforces and why it's decoupled from backoff.
	deadline := time.NewTimer(backoff)
	defer deadline.Stop()

	// failedAttempts is logged once after the loop, not per-attempt: this
	// helper runs on every backoff-wait cycle, so a per-attempt log line
	// during a real tmux-server outage becomes a hot loop logging concern
	// (pre-mortem.md failure #2) — batching to one summary line per
	// waitBackoffWithFastRecheck call keeps volume proportional to
	// reconnect cycles, not fast-recheck attempts.
	var failedAttempts int
	var lastErr error
	for i := 0; i < fastRecheckAttempts; i++ {
		select {
		case <-r.ctx.Done():
			return
		case <-deadline.C:
			return
		default:
		}

		if err := r.syncSessionsFastRecheck(r.ctx, fastRecheckSyncTimeout); err != nil {
			failedAttempts++
			lastErr = err
		}

		select {
		case <-r.ctx.Done():
			return
		case <-deadline.C:
			return
		case <-time.After(fastRecheckInterval):
		}
	}
	if failedAttempts > 0 {
		log.Debug("[registry] fast-recheck sync failed", "failedAttempts", failedAttempts, "of", fastRecheckAttempts, "lastErr", lastErr)
	}

	select {
	case <-r.ctx.Done():
	case <-deadline.C:
	}
}

// debounce state for %sessions-changed handling.
var (
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	debounceDelay = 50 * time.Millisecond
)

// readLines processes control-mode event lines from scanner until the scanner
// returns false (process exited) or the registry context is cancelled.
func (r *TmuxServerRegistry) readLines(scanner *bufio.Scanner) {
	for scanner.Scan() {
		select {
		case <-r.ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		r.handleEvent(line)
	}
}

// handleEvent parses a single control-mode notification line and updates state.
func (r *TmuxServerRegistry) handleEvent(line string) {
	switch {
	case strings.HasPrefix(line, "%session-created "):
		// %session-created $ID <name>
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[2]
			r.mu.Lock()
			r.sessions[name] = true
			delete(r.closedAt, name)
			r.mu.Unlock()
			log.Info("[registry] session created", "session", name)
		}

	case strings.HasPrefix(line, "%session-closed "):
		// %session-closed $ID <name>
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := parts[2]
			r.mu.Lock()
			delete(r.sessions, name)
			r.closedAt[name] = time.Now()
			r.mu.Unlock()
			log.Info("[registry] session closed", "session", name)
			r.firePaneExit(name)
		}

	case strings.HasPrefix(line, "%sessions-changed"):
		// Debounce: wait 50ms then sync from list-sessions.
		debounceMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounceDelay, func() {
			if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {
				log.Warn("[registry] sync after sessions-changed failed", "err", err)
			}
		})
		debounceMu.Unlock()

	case strings.HasPrefix(line, "%pane-exited "):
		// %pane-exited ... -t <session-name>
		// Parse the target from the line; look for the token after "-t".
		parts := strings.Fields(line)
		for i, part := range parts {
			if part == "-t" && i+1 < len(parts) {
				target := parts[i+1]
				// Target may be "session:window.pane" — extract session name.
				if idx := strings.Index(target, ":"); idx >= 0 {
					target = target[:idx]
				}
				r.firePaneExit(target)
				break
			}
		}

	case strings.HasPrefix(line, "%exit"):
		// Server is going away.
		r.healthMu.Lock()
		r.healthy = false
		r.healthMu.Unlock()

	default:
		// Unknown event — ignore, no panic.
	}
}

// GetServerRegistry returns the singleton TmuxServerRegistry for the given
// socket. Creates and starts the registry on first call for each socket.
// Never call from init().
func GetServerRegistry(socket string) *TmuxServerRegistry {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()

	if r, ok := globalRegistries[socket]; ok {
		return r
	}

	r := NewTmuxServerRegistry(socket)
	_ = r.Start(context.Background())
	globalRegistries[socket] = r
	return r
}

// StopServerRegistry stops and removes the registry for the given socket.
// Safe to call even if no registry was ever created for the socket.
// After this call, GetServerRegistry(socket) will create a fresh registry.
// Intended for test cleanup to prevent reconnectLoop from restarting a
// tmux server after it has been killed.
func StopServerRegistry(socket string) {
	globalRegistryMu.Lock()
	r, ok := globalRegistries[socket]
	if ok {
		delete(globalRegistries, socket)
	}
	globalRegistryMu.Unlock()
	if r != nil {
		r.Stop()
	}
}

// globalRegistryMu guards globalRegistries.
var globalRegistryMu sync.Mutex

// globalRegistries holds one registry per socket string.
var globalRegistries = make(map[string]*TmuxServerRegistry)

// RemoveServerRegistry stops and removes the TmuxServerRegistry for the given socket.
// This is primarily used in tests to clean up ephemeral registries.
func RemoveServerRegistry(socket string) {
	globalRegistryMu.Lock()
	r, ok := globalRegistries[socket]
	if ok {
		delete(globalRegistries, socket)
	}
	globalRegistryMu.Unlock()

	if ok {
		r.Stop()
	}
}
