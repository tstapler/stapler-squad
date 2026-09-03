package session

import (
	"os/exec"
	"testing"
	"time"

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
		return exec.Command("sleep", "100")
	}
}

// echoOneEventThenSleepCmd returns a piCommandFactory whose subprocess
// emits exactly one valid pi JSONL event on stdout and then sleeps, so
// PiStatusSource's real readLoop/waitLoop machinery can be exercised without
// depending on a real pi binary.
func echoOneEventThenSleepCmd() piCommandFactory {
	return func() *exec.Cmd {
		return exec.Command("/bin/sh", "-c", `echo '{"type":"agent_start"}'; sleep 100`)
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
			return exec.Command("/bin/sh", "-c", "exit 1")
		}
		// Every subsequent launch: emit an event and stay alive.
		return exec.Command("/bin/sh", "-c", `echo '{"type":"agent_start"}'; sleep 100`)
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

func TestPiStatusSource_ExhaustedRetriesReportUnavailable(t *testing.T) {
	factory := func() *exec.Cmd {
		// Every launch dies immediately -- retries never succeed.
		return exec.Command("/bin/sh", "-c", "exit 1")
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
