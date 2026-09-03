package session

import (
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/detection"
)

// TestPiStatusSource_ReadyBeforeAnyEvent covers Story 5.2.1's third AC
// bullet: a freshly constructed PiStatusSource reports StatusReady before
// any event has been fed to it.
func TestPiStatusSource_ReadyBeforeAnyEvent(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	if got := src.CurrentStatus(); got != detection.StatusReady {
		t.Errorf("CurrentStatus() = %v, want StatusReady", got)
	}
}

// TestPiStatusSource_ToolExecutionStartWithoutEnd covers Story 5.2.1's first
// AC bullet: a tool_execution_start with no matching tool_execution_end yet
// reports StatusExecuting.
func TestPiStatusSource_ToolExecutionStartWithoutEnd(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	src.handleEvent(PiAgentStartEvent{Type: "agent_start"})
	src.handleEvent(PiToolExecutionStartEvent{
		Type:       "tool_execution_start",
		ToolCallID: "call-1",
		ToolName:   "bash",
	})

	if got := src.CurrentStatus(); got != detection.StatusExecuting {
		t.Errorf("CurrentStatus() = %v, want StatusExecuting", got)
	}
}

// TestPiStatusSource_ToolExecutionEndReturnsToProcessing verifies that once
// every outstanding tool call has a matching end event, status moves off
// StatusExecuting (back to StatusProcessing -- the turn is still in
// progress, agent_end hasn't fired yet).
func TestPiStatusSource_ToolExecutionEndReturnsToProcessing(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	src.handleEvent(PiToolExecutionStartEvent{Type: "tool_execution_start", ToolCallID: "call-1", ToolName: "bash"})
	src.handleEvent(PiToolExecutionEndEvent{Type: "tool_execution_end", ToolCallID: "call-1", ToolName: "bash"})

	if got := src.CurrentStatus(); got != detection.StatusProcessing {
		t.Errorf("CurrentStatus() = %v, want StatusProcessing", got)
	}
}

// TestPiStatusSource_MultipleOutstandingToolCalls verifies that closing one
// of two outstanding tool calls leaves status at StatusExecuting (the other
// call is still outstanding).
func TestPiStatusSource_MultipleOutstandingToolCalls(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	src.handleEvent(PiToolExecutionStartEvent{Type: "tool_execution_start", ToolCallID: "call-1", ToolName: "bash"})
	src.handleEvent(PiToolExecutionStartEvent{Type: "tool_execution_start", ToolCallID: "call-2", ToolName: "read"})
	src.handleEvent(PiToolExecutionEndEvent{Type: "tool_execution_end", ToolCallID: "call-1", ToolName: "bash"})

	if got := src.CurrentStatus(); got != detection.StatusExecuting {
		t.Errorf("CurrentStatus() = %v, want StatusExecuting (call-2 still outstanding)", got)
	}
}

// TestPiStatusSource_IdleAfterAgentEndAndGracePeriod covers Story 5.2.1's
// second AC bullet: after agent_end and piIdleGracePeriod elapses with no
// further events, status becomes StatusIdle.
func TestPiStatusSource_IdleAfterAgentEndAndGracePeriod(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	src.handleEvent(PiAgentStartEvent{Type: "agent_start"})
	src.handleEvent(PiAgentEndEvent{Type: "agent_end"})

	// Immediately after agent_end, the grace period has not elapsed yet.
	if got := src.CurrentStatus(); got == detection.StatusIdle {
		t.Errorf("CurrentStatus() = %v immediately after agent_end, want NOT StatusIdle yet", got)
	}

	deadline := time.Now().Add(piIdleGracePeriod + 2*time.Second)
	for time.Now().Before(deadline) {
		if src.CurrentStatus() == detection.StatusIdle {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("CurrentStatus() never became StatusIdle within grace period + margin, got %v", src.CurrentStatus())
}

// TestPiStatusSource_NewEventCancelsPendingIdleTransition verifies that a
// new event arriving before the grace period elapses cancels the pending
// idle transition, per Story 5.2.1's AC.
func TestPiStatusSource_NewEventCancelsPendingIdleTransition(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)
	src.handleEvent(PiAgentEndEvent{Type: "agent_end"})
	// A new turn starts before the grace period elapses.
	src.handleEvent(PiAgentStartEvent{Type: "agent_start"})

	time.Sleep(piIdleGracePeriod + 500*time.Millisecond)

	if got := src.CurrentStatus(); got == detection.StatusIdle {
		t.Errorf("CurrentStatus() = %v, want the pending idle transition to have been canceled by the new agent_start", got)
	}
}

// --- Story 5.2.3: subprocess death detection and bounded relaunch ---

// sleeperCmd returns a piCommandFactory that launches a plain `sleep`
// subprocess as a controllable stand-in for the real pi binary: unlike pi,
// it's guaranteed to be present in any Unix CI environment and its exit can
// be forced deterministically via cmd.Process.Kill().
func sleeperCmd() piCommandFactory {
	return func() *exec.Cmd {
		return safeexec.CommandContext(context.Background(), "sleep", "100")
	}
}

func TestPiStatusSource_DetectsSubprocessDeath(t *testing.T) {
	src := NewPiStatusSource("test-session", sleeperCmd())
	if err := src.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer src.Stop()

	src.cmdMu.Lock()
	proc := src.cmd.Process
	src.cmdMu.Unlock()
	if err := proc.Kill(); err != nil {
		t.Fatalf("failed to kill subprocess: %v", err)
	}

	// One relaunch attempt's worth of backoff, plus margin, is enough for
	// the wait goroutine to observe the exit and bump the retry counter.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if src.retryCount.Load() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("retryCount never incremented after killing the subprocess")
}

func TestPiStatusSource_SuccessfulRelaunchResumesAndResetsRetryCounter(t *testing.T) {
	attempt := 0
	factory := func() *exec.Cmd {
		attempt++
		if attempt == 1 {
			// First launch: die immediately (empty command exits fast).
			return safeexec.CommandContext(context.Background(), "/bin/sh", "-c", "exit 1")
		}
		// Every subsequent launch: emit an event and stay alive.
		return safeexec.CommandContext(context.Background(), "/bin/sh", "-c", `echo '{"type":"agent_start"}'; sleep 100`)
	}

	src := NewPiStatusSource("test-session", factory)
	if err := src.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer src.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// The relaunch's echoed agent_start event both resumes normal
		// inference (StatusProcessing, via handleEvent's default case) and
		// resets the retry counter (handleEvent's unconditional reset).
		if src.CurrentStatus() == detection.StatusProcessing && src.retryCount.Load() == 0 && !src.unavailable.Load() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("relaunch did not resume normal inference / reset retry counter in time (attempts=%d, retryCount=%d, unavailable=%v, status=%v)", attempt, src.retryCount.Load(), src.unavailable.Load(), src.CurrentStatus())
}

// --- Task 2.2.1e: propagating the real pi session ID back to the owning Instance ---

// TestPiStatusSource_SessionEventFiresOnSessionIDCallbackOnce verifies that
// observing a "session" header event invokes the onSessionID callback with
// the event's ID, and that a repeated event carrying the same ID does not
// fire the callback again (handleEvent dedupes against lastSessionID).
func TestPiStatusSource_SessionEventFiresOnSessionIDCallbackOnce(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)

	var mu sync.Mutex
	var gotIDs []string
	src.SetOnSessionIDCallback(func(id string) {
		mu.Lock()
		defer mu.Unlock()
		gotIDs = append(gotIDs, id)
	})

	src.handleEvent(PiSessionEvent{Type: "session", ID: "abc-123", Version: 1})
	// A duplicate of the same header (e.g. re-sent after a relaunch) must
	// not fire the callback again.
	src.handleEvent(PiSessionEvent{Type: "session", ID: "abc-123", Version: 1})

	mu.Lock()
	defer mu.Unlock()
	if len(gotIDs) != 1 {
		t.Fatalf("onSessionID called %d times, want exactly 1: %v", len(gotIDs), gotIDs)
	}
	if gotIDs[0] != "abc-123" {
		t.Errorf("onSessionID called with %q, want %q", gotIDs[0], "abc-123")
	}
}

// TestPiStatusSource_SessionEventWithEmptyIDDoesNotFireCallback verifies a
// "session" event with no ID (shouldn't happen per the pi protocol, but
// defensively) never invokes the callback.
func TestPiStatusSource_SessionEventWithEmptyIDDoesNotFireCallback(t *testing.T) {
	src := NewPiStatusSource("test-session", nil)

	called := false
	src.SetOnSessionIDCallback(func(id string) { called = true })

	src.handleEvent(PiSessionEvent{Type: "session", ID: "", Version: 1})

	if called {
		t.Errorf("onSessionID fired for an empty session ID, want no-op")
	}
}

// TestPiStatusSource_SessionEventPropagatesToInstance is the Instance-level
// companion to the callback tests above: it wires PiStatusSource's
// onSessionID callback to a real Instance.SetPiSessionID (as
// startPiStatusSource does in instance_pi_status.go) and asserts the
// Instance's piSession.SessionID reflects the observed ID.
func TestPiStatusSource_SessionEventPropagatesToInstance(t *testing.T) {
	inst := &Instance{Title: "pi-session-propagation-test", UUID: "sess-pi-session-propagation"}

	src := NewPiStatusSource(inst.Title, nil)
	src.SetOnSessionIDCallback(inst.SetPiSessionID)

	src.handleEvent(PiSessionEvent{Type: "session", ID: "real-pi-uuid-1", Version: 1})

	inst.piSessionMu.Lock()
	got := inst.piSession
	inst.piSessionMu.Unlock()

	if got == nil || got.SessionID != "real-pi-uuid-1" {
		t.Fatalf("inst.piSession = %+v, want SessionID %q", got, "real-pi-uuid-1")
	}
	if got.LastAttached.IsZero() {
		t.Errorf("inst.piSession.LastAttached was not set")
	}
}

// TestPiStatusSource_StopWaitsOutPendingRelaunchAndPreventsIt is a regression
// test for the BLOCKER finding that Stop() didn't wait for or cancel a
// pending relaunch timer: handleProcessExit's time.AfterFunc was never added
// to p.wg (so Stop()'s wg.Wait() could return before it fired) and never
// stored anywhere Stop() could cancel, so a relaunch racing a concurrent
// Stop() could call launch() -- and its p.wg.Add(2) -- after Stop()'s Wait()
// already returned, leaking a subprocess/goroutines nothing would ever stop
// again.
//
// This test kills the subprocess to force handleProcessExit to schedule a
// relaunch, then immediately calls Stop() (racing the pending backoff timer)
// and asserts that launch() (observed via the counting factory) is never
// called again after Stop() returns -- true whether Stop() won the race
// (cancelled the timer) or the timer had already fired and lost to the
// stopped flag, because Stop()'s wg.Wait() now blocks until that resolution
// either way.
func TestPiStatusSource_StopWaitsOutPendingRelaunchAndPreventsIt(t *testing.T) {
	var launches atomic.Int32
	factory := func() *exec.Cmd {
		launches.Add(1)
		return safeexec.CommandContext(context.Background(), "sleep", "100")
	}

	src := NewPiStatusSource("test-session", factory)
	if err := src.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	src.cmdMu.Lock()
	proc := src.cmd.Process
	src.cmdMu.Unlock()
	if err := proc.Kill(); err != nil {
		t.Fatalf("failed to kill subprocess: %v", err)
	}

	// Wait for handleProcessExit to observe the exit and schedule the
	// relaunch timer (retryCount bumps synchronously with scheduling).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && src.retryCount.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if src.retryCount.Load() == 0 {
		t.Fatal("relaunch was never scheduled after killing the subprocess")
	}

	launchesBeforeStop := launches.Load()

	// Race the pending backoff timer: Stop() must block until the relaunch
	// attempt (whether cancelled or already in flight) has fully resolved.
	src.Stop()

	// Give any would-be leaked relaunch (the pre-fix bug) time to fire --
	// well past the backoff and then some margin.
	time.Sleep(piRelaunchBackoff*time.Duration(piMaxRelaunchAttempts) + 500*time.Millisecond)

	if got := launches.Load(); got != launchesBeforeStop {
		t.Errorf("launch() was called again after Stop() returned (launches before=%d, after=%d) -- pending relaunch timer was not cancelled/joined by Stop()", launchesBeforeStop, got)
	}
}

func TestPiStatusSource_ExhaustedRetriesReportUnavailable(t *testing.T) {
	factory := func() *exec.Cmd {
		// Every launch dies immediately -- retries never succeed.
		return safeexec.CommandContext(context.Background(), "/bin/sh", "-c", "exit 1")
	}

	src := NewPiStatusSource("test-session", factory)
	if err := src.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer src.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if src.unavailable.Load() {
			if got := src.CurrentStatus(); got != detection.StatusError {
				t.Errorf("CurrentStatus() = %v once unavailable, want StatusError", got)
			}
			if src.StatusContext() == "" {
				t.Errorf("StatusContext() empty once unavailable, want a non-empty message")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("subprocess never reported unavailable after exhausting %d retries (retryCount=%d)", piMaxRelaunchAttempts, src.retryCount.Load())
}

// TestPiStatusSource_StopConcurrentWithRelaunchStress is a stress/fuzz-style
// substitute for a fully deterministic reproduction of Blocker 1's narrow
// race (independent review pass on PR #685): the relaunch timer callback's
// stopped.Load() check racing Stop()'s CompareAndSwap on the same flag, such
// that Stop() could snapshot-and-kill the OLD subprocess while the callback
// was still inside launch() spawning a NEW one that nothing then kills.
// Forcing that exact interleaving deterministically would need a test-only
// hook inside the callback, which doesn't exist -- a fully deterministic
// repro is impractical here, so this instead forces a subprocess crash and
// immediately races Stop() against the resulting relaunch attempt (with no
// synchronization between the two) across many iterations under -race,
// giving the scheduler repeated chances to hit both possible
// lifecycleMu-acquisition orderings.
//
// It asserts two things any leaked subprocess would violate: (1) Stop()
// always returns promptly rather than hanging on wg.Wait() for an orphaned
// process that nothing killed, and (2) every subprocess the factory ever
// spawned has actually exited (verified via signal(0)) by the time all
// iterations finish -- a leaked "sleep 100" would still be alive and
// answer signal(0) successfully.
func TestPiStatusSource_StopConcurrentWithRelaunchStress(t *testing.T) {
	const iterations = 200

	var mu sync.Mutex
	var spawned []*exec.Cmd

	for iter := 0; iter < iterations; iter++ {
		factory := func() *exec.Cmd {
			cmd := safeexec.CommandContext(context.Background(), "sleep", "5")
			mu.Lock()
			spawned = append(spawned, cmd)
			mu.Unlock()
			return cmd
		}

		src := NewPiStatusSource("test-session", factory)
		if err := src.Start(); err != nil {
			t.Fatalf("iteration %d: Start() failed: %v", iter, err)
		}

		src.cmdMu.Lock()
		proc := src.cmd.Process
		src.cmdMu.Unlock()
		if err := proc.Kill(); err != nil {
			t.Fatalf("iteration %d: failed to kill subprocess: %v", iter, err)
		}

		// Race Stop() against handleProcessExit's relaunch timer callback
		// with no synchronization -- whichever of Stop()'s
		// CompareAndSwap-then-lifecycleMu sequence and the callback's
		// lifecycleMu-then-stopped.Load() sequence gets there first should
		// still leave nothing orphaned (see lifecycleMu's doc comment on
		// the PiStatusSource struct).
		done := make(chan struct{})
		go func() {
			src.Stop()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("iteration %d: Stop() did not return within 3s -- suspected hang on a leaked relaunch", iter)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for i, cmd := range spawned {
		if cmd.Process == nil {
			continue
		}
		// signal(0) succeeds iff the process still exists. A process
		// already reaped by its own PiStatusSource waitLoop's cmd.Wait()
		// call returns ESRCH here, so this only flags a subprocess that is
		// genuinely still running and unaccounted for.
		if err := cmd.Process.Signal(syscall.Signal(0)); err == nil {
			t.Errorf("spawned process %d (pid %d) still appears to exist after all Stop() calls returned -- leaked subprocess", i, cmd.Process.Pid)
		}
	}
}
