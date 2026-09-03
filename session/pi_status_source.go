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

	// status is the currently-inferred detection.DetectedStatus, stored as
	// int32 so CurrentStatus() -- called from the hot GetStatus() path --
	// needs no lock. Defaults to detection.StatusReady in the constructor
	// (the int32 zero value is detection.StatusUnknown, not StatusReady).
	status atomic.Int32

	// mu guards pendingTools and idleTimer, both touched only from
	// handleEvent (single reader goroutine per subprocess generation, but
	// Stop() can race a relaunch's idle-timer cancellation).
	mu           sync.Mutex
	pendingTools map[string]struct{}
	idleTimer    *time.Timer

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
		// Session header: informational only (log the protocol version),
		// no status change.
		log.Debug("pi status subprocess session header", "session", p.sessionTitle, "pi_session_id", ev.ID, "version", ev.Version)
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
// treated as fatal -- Epic 6.1's structured metrics point is out of scope
// for this file.
func (p *PiStatusSource) readLoop(stdout io.ReadCloser) {
	defer p.wg.Done()
	reader := NewPiEventReader(stdout)
	for {
		event, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				var unrecognized *piUnrecognizedTypeError
				if ok := isPiUnrecognizedTypeError(err, &unrecognized); ok {
					log.Warn("pi status subprocess emitted unrecognized event type", "session", p.sessionTitle, "err", err)
					continue
				}
				log.Warn("pi status subprocess event stream read error", "session", p.sessionTitle, "err", err)
			}
			return
		}
		p.handleEvent(event)
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

	time.AfterFunc(backoff, func() {
		if p.stopped.Load() {
			return
		}
		if err := p.launch(); err != nil {
			log.Error("pi status subprocess relaunch failed", "session", p.sessionTitle, "attempt", attempt, "err", err)
			p.handleProcessExit()
		}
	})
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

	p.cmdMu.Lock()
	cmd := p.cmd
	p.cmdMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	p.wg.Wait()
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
