package session

// pi_status_source.go implements Epic 5.2: a PiStatusSource owns a dedicated
// `pi --mode json` subprocess and maps its event stream (session/pi_adapter.go)
// onto the same detection.DetectedStatus vocabulary Claude's PTY-scraping
// detector produces, so pi sessions render in the session list the same way
// Claude sessions do (see session/instance_status.go's GetStatus, which
// consults this as a fallback -- Pattern Decision: parallel map, not a
// shared ProgramStatusController interface).

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

const (
	// piIdleGracePeriod is how long after agent_end with no further events
	// before a PiStatusSource reports detection.StatusIdle. Not empirically
	// measurable from the Phase 1 spike (`--mode json --print` exits
	// immediately once the turn settles, so there is no natural inactivity
	// gap to measure against) -- set to match the existing Claude idle
	// threshold precedent (session/detection/idle.go's
	// DefaultIdleDetectorConfig.IdleThreshold) instead of a pi-specific
	// measured value. Revisit if real interactive/RPC-mode usage shows this
	// is wrong.
	piIdleGracePeriod = 5 * time.Second

	// piMaxRelaunchAttempts bounds how many times PiStatusSource will
	// automatically relaunch a dead status subprocess before giving up and
	// reporting the confirmed-dead/unavailable state (Story 5.2.3).
	piMaxRelaunchAttempts = 3

	// piRelaunchBackoff is the base backoff between relaunch attempts,
	// scaled linearly by attempt number (attempt 1 waits 1x, attempt 2
	// waits 2x, ...).
	piRelaunchBackoff = 250 * time.Millisecond
)

// piCommandFactory constructs a fresh *exec.Cmd for the status-only pi
// subprocess. It is a field on PiStatusSource (rather than a hardcoded
// exec.Command call) for two reasons: it is invoked again on every
// relaunch attempt (Story 5.2.3), and tests substitute a lightweight
// stand-in process instead of spawning the real pi binary.
type piCommandFactory func() *exec.Cmd

// PiStatusSource owns a dedicated `pi --mode json` subprocess (started
// alongside the tmux-launched interactive pi process -- pi's `--mode json`
// output mode is a separate, non-interactive invocation, not something that
// can attach to an already-running interactive session, so a pi session
// costs two live pi processes while this feature is active) and infers a
// detection.DetectedStatus from its event stream.
//
// Unlike ClaudeController, PiStatusSource does not scrape PTY output --pi's
// `--mode json` transcript is a structured, typed event feed (see
// session/pi_adapter.go), so status inference here is a straightforward
// event-to-status mapping rather than regex-based PTY pattern matching.
type PiStatusSource struct {
	sessionTitle string
	newCmd       piCommandFactory

	// onSessionID, if set via SetOnSessionIDCallback before Start(), is
	// invoked with the real pi session UUID the first time it is observed in
	// a "session" header event (Task 2.2.1e) -- this is how the owning
	// Instance's piSession field gets populated from a live pi run, so a
	// later Restart's buildLaunchCommand call can inject `--session <id>`
	// and actually resume the conversation. A callback (rather than an
	// *Instance back-reference) keeps PiStatusSource testable in isolation
	// and avoids a circular session <-> PiStatusSource dependency.
	onSessionID func(id string)
	// lastSessionID is the most recently observed non-empty session ID,
	// guarded by mu. Deduplicates onSessionID calls: a repeated "session"
	// event carrying the same ID (e.g. a relaunch re-sending its header)
	// fires the callback only once, not once per event.
	lastSessionID string

	// status is the currently-inferred detection.DetectedStatus, stored as
	// int32 so CurrentStatus() -- called from the hot GetStatus() path --
	// needs no lock. Defaults to detection.StatusReady in the constructor
	// (the int32 zero value is detection.StatusUnknown, not StatusReady).
	status atomic.Int32

	// mu guards pendingTools, idleTimer, and relaunchTimer, all touched only
	// from handleEvent/handleProcessExit (single reader/wait goroutine pair
	// per subprocess generation, but Stop() can race a relaunch's idle-timer
	// or relaunch-timer cancellation).
	mu           sync.Mutex
	pendingTools map[string]struct{}
	idleTimer    *time.Timer
	// relaunchTimer holds the pending time.AfterFunc scheduled by
	// handleProcessExit to retry a dead subprocess, so Stop() can proactively
	// cancel it (an optimization -- avoids waiting out the full backoff on a
	// clean shutdown). The correctness guarantee against the Bug 1 race
	// (Stop()'s wg.Wait() returning before a scheduled relaunch fires and
	// calls launch(), which would Add to an already-Wait()-ed WaitGroup) does
	// NOT come from this field -- it comes from wg.Add(1)/Done() bracketing
	// the timer callback below, so wg.Wait() always blocks until any pending
	// relaunch attempt has resolved one way or another.
	relaunchTimer *time.Timer

	// unavailable is true only once relaunch retries are exhausted --
	// confirmed-dead, per Story 5.2.3's three-state rigor (known-good /
	// stale-unknown / confirmed-dead, mirroring PiExtensionHealth). Any
	// event received afterward (a later relaunch succeeding) clears it.
	unavailable atomic.Bool
	// retryCount is the number of consecutive relaunch attempts since the
	// last successfully-observed event. Reset to 0 the moment any event is
	// handled.
	retryCount atomic.Int32

	cmdMu   sync.Mutex
	cmd     *exec.Cmd
	stopCh  chan struct{}
	stopped atomic.Bool
	wg      sync.WaitGroup

	// lifecycleMu serializes launch() (called from Start() and from the
	// relaunch timer callback in handleProcessExit, including that
	// callback's stopped.Load() check) against Stop()'s
	// "snapshot p.cmd -> kill -> wg.Wait()" sequence.
	//
	// Without this, Stop() and a just-fired relaunch callback can interleave
	// so that Stop() snapshots and kills the OLD (already-dead) p.cmd before
	// the callback's own stopped.Load() check runs, sees stopped == false
	// (Stop()'s CompareAndSwap on p.stopped races the callback's read), and
	// proceeds to launch() a NEW subprocess + reader/wait goroutine pair
	// AFTER Stop() has already taken its kill snapshot. Stop()'s wg.Wait()
	// then blocks until those new goroutines exit on their own -- i.e. until
	// the orphaned new subprocess exits naturally, since nothing kills it.
	//
	// Holding lifecycleMu across "check stopped -> launch()" in the
	// callback and across "snapshot cmd -> kill" in Stop() makes whichever
	// side acquires the mutex first fully determine the outcome: if Stop()
	// wins, the callback's later stopped.Load() (now true, since the
	// CompareAndSwap in Stop() happens-before the lock acquisition) sees
	// stopped and returns without launching anything. If the callback wins,
	// it may launch() a new subprocess before releasing the mutex; Stop()
	// then acquires it, snapshots the NOW-current p.cmd (the new one), and
	// kills that one correctly. Either way nothing is orphaned. wg.Wait()
	// itself stays OUTSIDE the mutex in Stop() -- holding it during Wait()
	// would deadlock against a callback blocked trying to acquire
	// lifecycleMu (whose own deferred wg.Done() only fires after it returns
	// from launch()/the callback body).
	lifecycleMu sync.Mutex
}

// NewPiStatusSource constructs a PiStatusSource for the given instance
// title. newCmd is called once per launch/relaunch to obtain a fresh
// *exec.Cmd (a spent *exec.Cmd cannot be re-started). The source starts in
// detection.StatusReady, per Story 5.2.1's AC, until Start is called and the
// subprocess emits its first event.
func NewPiStatusSource(sessionTitle string, newCmd piCommandFactory) *PiStatusSource {
	p := &PiStatusSource{
		sessionTitle: sessionTitle,
		newCmd:       newCmd,
		pendingTools: make(map[string]struct{}),
		stopCh:       make(chan struct{}),
	}
	p.status.Store(int32(detection.StatusReady))
	return p
}

// SetOnSessionIDCallback registers fn to be invoked (with the real pi
// session UUID) the first time a "session" header event with a non-empty ID
// is observed. Must be called before Start() -- handleEvent reads
// p.onSessionID without a lock, on the assumption that it is only ever
// written during construction/wiring, before the reader goroutine starts.
func (p *PiStatusSource) SetOnSessionIDCallback(fn func(id string)) {
	p.onSessionID = fn
}

// CurrentStatus returns the currently-inferred status. Once the subprocess
// is confirmed dead and relaunch retries are exhausted, this returns
// detection.StatusError rather than a frozen copy of the last inferred
// status -- StatusError is reused instead of a new detection.DetectedStatus
// value (which would require proto/frontend changes -- see
// session/detection/detector.go's DetectedStatusToProto and
// proto/session/v1/types.proto's DetectedStatus enum) because the UI
// already renders StatusError distinctly from StatusIdle (see
// InstanceStatusInfo.GetStatusIcon/GetStatusDescription): the smaller-diff
// option named in plan.md's Story 5.2.3 AC. StatusContext carries the
// human-readable detail in this case.
func (p *PiStatusSource) CurrentStatus() detection.DetectedStatus {
	if p.unavailable.Load() {
		return detection.StatusError
	}
	return detection.DetectedStatus(p.status.Load())
}

// StatusContext returns a human-readable detail string for the current
// status, non-empty only while CurrentStatus() reports the confirmed-dead
// state.
func (p *PiStatusSource) StatusContext() string {
	if p.unavailable.Load() {
		return fmt.Sprintf("pi status subprocess unavailable after %d relaunch attempts", piMaxRelaunchAttempts)
	}
	return ""
}

func (p *PiStatusSource) setStatus(s detection.DetectedStatus) {
	p.status.Store(int32(s))
}

// cancelIdleTimer stops any pending agent_end -> StatusIdle transition.
// Called at the top of handling every event, per Story 5.2.1's AC that any
// new event cancels a pending idle transition.
func (p *PiStatusSource) cancelIdleTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.idleTimer != nil {
		p.idleTimer.Stop()
		p.idleTimer = nil
	}
}

// startIdleTimer schedules the agent_end -> StatusIdle transition after
// piIdleGracePeriod. Callers must call cancelIdleTimer first if a timer may
// already be pending (handleEvent does this for every event).
func (p *PiStatusSource) startIdleTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idleTimer = time.AfterFunc(piIdleGracePeriod, func() {
		p.setStatus(detection.StatusIdle)
	})
}

// handleEvent updates the inferred status from a single decoded pi event.
// It is exported-in-package (not exported outside session) so both the real
// subprocess reader loop (readLoop) and unit tests can drive it directly
// with synthetic events, without needing a real pi subprocess.
func (p *PiStatusSource) handleEvent(event any) {
	// Receiving any event at all proves the subprocess is alive and
	// producing output, so clear a stale confirmed-dead flag and reset the
	// relaunch counter -- this is what "a successful relaunch that emits at
	// least one event resumes normal inference and resets the retry
	// counter" (Story 5.2.3 AC) reduces to.
	if p.unavailable.CompareAndSwap(true, false) {
		log.Info("pi status subprocess recovered", "session", p.sessionTitle)
	}
	p.retryCount.Store(0)

	// Every event cancels any pending idle transition (Story 5.2.1 AC).
	p.cancelIdleTimer()

	switch ev := event.(type) {
	case PiToolExecutionStartEvent:
		p.mu.Lock()
		p.pendingTools[ev.ToolCallID] = struct{}{}
		p.mu.Unlock()
		p.setStatus(detection.StatusExecuting)
	case PiToolExecutionEndEvent:
		p.mu.Lock()
		delete(p.pendingTools, ev.ToolCallID)
		remaining := len(p.pendingTools)
		p.mu.Unlock()
		if remaining > 0 {
			p.setStatus(detection.StatusExecuting)
		} else {
			p.setStatus(detection.StatusProcessing)
		}
	case PiAgentEndEvent:
		// Don't flip status yet -- schedule the idle transition per the
		// grace period; CurrentStatus() keeps returning whatever status was
		// last inferred until the timer fires.
		p.startIdleTimer()
	case PiSessionEvent:
		// Session header: no status change, but the ID is the real pi
		// session UUID (Task 2.2.1e) -- propagate it to the owning Instance
		// via onSessionID so a later restart can resume with --session.
		// Deduped against lastSessionID so a repeated header (e.g. after a
		// relaunch) fires the callback once, not every time.
		log.Debug("pi status subprocess session header", "session", p.sessionTitle, "pi_session_id", ev.ID, "version", ev.Version)
		if ev.ID != "" {
			p.mu.Lock()
			changed := ev.ID != p.lastSessionID
			if changed {
				p.lastSessionID = ev.ID
			}
			p.mu.Unlock()
			if changed && p.onSessionID != nil {
				p.onSessionID(ev.ID)
			}
		}
	default:
		// agent_start, agent_settled, turn_start, turn_end, message_start,
		// message_end, message_update: all indicate the agent is actively
		// working on the turn.
		p.setStatus(detection.StatusProcessing)
	}
}

// Start launches the status-only pi subprocess and begins consuming its
// event stream. Safe to call once; relaunches after a detected crash are
// handled internally (see handleProcessExit).
func (p *PiStatusSource) Start() error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	return p.launch()
}

func (p *PiStatusSource) launch() error {
	cmd := p.newCmd()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pi_status_source: failed to open stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pi_status_source: failed to start subprocess: %w", err)
	}

	p.cmdMu.Lock()
	p.cmd = cmd
	p.cmdMu.Unlock()

	p.wg.Add(2)
	go p.readLoop(stdout)
	go p.waitLoop(cmd)

	log.Info("pi status subprocess started", "session", p.sessionTitle, "pid", cmd.Process.Pid)
	return nil
}

// readLoop consumes decoded events from the subprocess's stdout until EOF or
// a fatal read error, dispatching each to handleEvent. An unrecognized event
// type is logged and skipped (per PiEventReader.Next's contract) rather than
// treated as fatal. Every event -- recognized or not -- increments
// pi_status_source_events_total{type} (session/pi_status_source_metrics.go,
// Epic 6.1 Story 6.1.1) before it's dispatched, so the counter observes
// throughput even for events handleEvent's own default case would otherwise
// treat identically to a recognized "processing" event.
func (p *PiStatusSource) readLoop(stdout io.ReadCloser) {
	defer p.wg.Done()
	reader := NewPiEventReader(stdout)
	for {
		event, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				var unrecognized *piUnrecognizedTypeError
				if ok := isPiUnrecognizedTypeError(err, &unrecognized); ok {
					recordPiStatusSourceEvent(piEventTypeUnrecognized)
					log.Warn("pi status subprocess emitted unrecognized event type", "session", p.sessionTitle, "err", err)
					continue
				}
				log.Warn("pi status subprocess event stream read error", "session", p.sessionTitle, "err", err)
			}
			return
		}
		recordPiStatusSourceEvent(piEventType(event))
		p.handleEvent(event)
	}
}

// piEventType returns the JSON "type" discriminator carried by a decoded pi
// event, for pi_status_source_events_total's label only -- it is not part of
// handleEvent's status-inference dispatch.
func piEventType(event any) string {
	switch ev := event.(type) {
	case PiSessionEvent:
		return ev.Type
	case PiAgentStartEvent:
		return ev.Type
	case PiAgentSettledEvent:
		return ev.Type
	case PiTurnStartEvent:
		return ev.Type
	case PiTurnEndEvent:
		return ev.Type
	case PiMessageStartEvent:
		return ev.Type
	case PiMessageEndEvent:
		return ev.Type
	case PiMessageUpdateEvent:
		return ev.Type
	case PiToolExecutionStartEvent:
		return ev.Type
	case PiToolExecutionEndEvent:
		return ev.Type
	case PiAgentEndEvent:
		return ev.Type
	default:
		return piEventTypeUnrecognized
	}
}

// waitLoop blocks on cmd.Wait(), then -- unless Stop() was the cause --
// treats the exit as a crash and hands off to the bounded-relaunch logic
// (Story 5.2.3).
func (p *PiStatusSource) waitLoop(cmd *exec.Cmd) {
	defer p.wg.Done()
	err := cmd.Wait()

	select {
	case <-p.stopCh:
		// Stop() killed the process intentionally; not a crash.
		return
	default:
	}

	log.Warn("pi status subprocess exited unexpectedly", "session", p.sessionTitle, "err", err)
	p.handleProcessExit()
}

// handleProcessExit implements the bounded relaunch policy: up to
// piMaxRelaunchAttempts, with a linearly increasing backoff, before flipping
// to the confirmed-dead/unavailable state. A successful relaunch that goes
// on to emit an event resets the counter (in handleEvent), so this counter
// only tracks *consecutive* failures since the last observed event.
func (p *PiStatusSource) handleProcessExit() {
	if p.stopped.Load() {
		return
	}

	attempt := p.retryCount.Add(1)
	if attempt > piMaxRelaunchAttempts {
		p.unavailable.Store(true)
		log.Error("pi status subprocess relaunch attempts exhausted; reporting unavailable", "session", p.sessionTitle, "attempts", piMaxRelaunchAttempts)
		return
	}

	backoff := piRelaunchBackoff * time.Duration(attempt)
	log.Warn("pi status subprocess died; scheduling relaunch", "session", p.sessionTitle, "attempt", attempt, "max_attempts", piMaxRelaunchAttempts, "backoff", backoff)

	// Bug 1 fix: Add(1) now, Done() inside the callback no matter which
	// branch it takes (relaunch, no-op because Stop() won, or a failed
	// relaunch that recurses into handleProcessExit again). This makes
	// Stop()'s existing wg.Wait() block until this pending relaunch attempt
	// has fully resolved, closing the "Wait() returns zero, then the timer
	// fires and calls launch(), which Add()s to an already-waited-on
	// WaitGroup" race described in the review finding.
	p.wg.Add(1)
	timer := time.AfterFunc(backoff, func() {
		// lifecycleMu (Blocker 1 fix): hold it across both the stopped.Load()
		// check AND the launch() call so this callback and a concurrent
		// Stop() can never interleave into "Stop() kills the old process,
		// then this callback launches a new, unkilled one" -- see
		// lifecycleMu's doc comment on the struct. wg.Done() is deferred
		// outermost so it still fires exactly once on every return path, but
		// after lifecycleMu is released (defers run LIFO).
		defer p.wg.Done()
		p.lifecycleMu.Lock()
		defer p.lifecycleMu.Unlock()

		if p.stopped.Load() {
			return
		}
		if err := p.launch(); err != nil {
			log.Error("pi status subprocess relaunch failed", "session", p.sessionTitle, "attempt", attempt, "err", err)
			p.handleProcessExit()
		}
	})

	p.mu.Lock()
	p.relaunchTimer = timer
	p.mu.Unlock()
}

// Stop terminates the subprocess (if running), cancels any pending idle
// timer, and waits for the reader/wait goroutines to finish. Safe to call
// multiple times or on a source that was never started.
func (p *PiStatusSource) Stop() {
	if !p.stopped.CompareAndSwap(false, true) {
		return
	}
	close(p.stopCh)
	p.cancelIdleTimer()
	p.cancelRelaunchTimer()

	// lifecycleMu (Blocker 1 fix): hold it across the snapshot-and-kill
	// sequence so it can never interleave with a concurrent relaunch
	// callback's own stopped.Load()-check-then-launch() sequence -- see
	// lifecycleMu's doc comment on the struct for the two possible
	// orderings and why both are safe. wg.Wait() is deliberately OUTSIDE
	// the lock: holding lifecycleMu during Wait() would deadlock against a
	// callback blocked trying to acquire it.
	p.lifecycleMu.Lock()
	p.cmdMu.Lock()
	cmd := p.cmd
	p.cmdMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	p.lifecycleMu.Unlock()

	p.wg.Wait()
}

// cancelRelaunchTimer proactively cancels a pending relaunch timer scheduled
// by handleProcessExit, if one exists. This is purely an optimization (avoids
// Stop() waiting out the full backoff on a clean shutdown) -- the actual
// correctness guarantee is the wg.Add(1)/Done() pairing around the timer
// callback in handleProcessExit. If timer.Stop() successfully cancels the
// timer (it returns true), the callback will never run and its deferred
// wg.Done() will never fire, so this must call wg.Done() itself to balance
// the Add(1). If Stop() returns false, the callback has already fired or is
// already running concurrently -- it will call wg.Done() itself, so this must
// NOT double-Done.
func (p *PiStatusSource) cancelRelaunchTimer() {
	p.mu.Lock()
	timer := p.relaunchTimer
	p.relaunchTimer = nil
	p.mu.Unlock()

	if timer == nil {
		return
	}
	if timer.Stop() {
		p.wg.Done()
	}
}

// isPiUnrecognizedTypeError is a small errors.As wrapper kept local to this
// file so readLoop doesn't need to import "errors" just for this one check,
// and so the unexported piUnrecognizedTypeError type (session/pi_adapter.go)
// stays unexported.
func isPiUnrecognizedTypeError(err error, target **piUnrecognizedTypeError) bool {
	//nolint:errorlint // simple unwrap of a single-level, package-local sentinel-ish error type
	if e, ok := err.(*piUnrecognizedTypeError); ok {
		*target = e
		return true
	}
	return false
}
