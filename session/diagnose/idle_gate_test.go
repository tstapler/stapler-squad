package diagnose

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/detection"
)

// claudeReadlinePromptContext is one of SafeIdleStatusContexts' three pinned
// descriptions (also mirrored in server/services/session_service.go) -- used
// here instead of re-deriving the allowlist so this test breaks loudly (not
// silently) if that allowlist's wording ever changes underneath it.
const claudeReadlinePromptContext = "Claude Code readline input prompt"

// commandPromptContext is a raw-shell-prompt description that reports
// detection.StatusIdle but is NOT on SafeIdleStatusContexts -- the exact
// "flickers off the allowlist" scenario the settle window resets on.
const commandPromptContext = "command_prompt"

func newTestIdleGate(t *testing.T, windowSeconds int) *IdleGate {
	t.Helper()
	gate, _ := newTestIdleGateWithLog(t, windowSeconds) // second value is a log buffer, not an error
	return gate
}

// newTestIdleGateWithLog builds an IdleGate bound to a private *slog.Logger
// (never the global slog.Default()) so the settle-reset log assertions below
// are immune to the global-slog-swap race documented in
// docs/bugs/open/BUG-087-captureLogs-global-slog-swap-races-under-t-parallel.md.
func newTestIdleGateWithLog(t *testing.T, windowSeconds int) (*IdleGate, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	return NewIdleGate(windowSeconds, logger), buf
}

func TestIdleGate_Evaluate_ShouldPass_WhenSafeIdleStatusSustainedForEntireWindow(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	ok, reason := gate.Evaluate(base, "claude", detection.StatusIdle, claudeReadlinePromptContext)
	require.False(t, ok, "should not pass before any time has elapsed within the window")
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)

	// AC: sustained continuously for 45s against a 30s window passes.
	ok, reason = gate.Evaluate(base.Add(45*time.Second), "claude", detection.StatusIdle, claudeReadlinePromptContext)
	assert.True(t, ok, "45s sustained against a 30s window must pass")
	assert.Nil(t, reason)
}

func TestIdleGate_Evaluate_ShouldFailAtExactWindowBoundaryMinusOneNanosecond(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	gate.Evaluate(base, "claude", detection.StatusIdle, claudeReadlinePromptContext)
	ok, reason := gate.Evaluate(base.Add(30*time.Second-time.Nanosecond), "claude", detection.StatusIdle, claudeReadlinePromptContext)
	assert.False(t, ok, "one nanosecond short of the window must not pass")
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)

	ok, reason = gate.Evaluate(base.Add(30*time.Second), "claude", detection.StatusIdle, claudeReadlinePromptContext)
	assert.True(t, ok, "exactly the window duration must pass")
	assert.Nil(t, reason)
}

func TestIdleGate_Evaluate_ShouldResetSettleTimer_WhenStatusLeavesAllowlistMidWindow(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	// Enters the settle window at the base timestamp.
	gate.Evaluate(base, "claude", detection.StatusIdle, claudeReadlinePromptContext)

	// Five seconds later it flickers off the allowlist for a single poll,
	// which must reset the settle timer rather than merely pausing it.
	ok, reason := gate.Evaluate(base.Add(5*time.Second), "claude", detection.StatusIdle, commandPromptContext)
	assert.False(t, ok)
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)

	// Thirty more seconds pass (35s after the base timestamp). This is the
	// plan's literal AC: the gate still fails here, because the settle timer
	// restarted at the five-second reset rather than crediting the progress
	// accrued before it.
	ok, reason = gate.Evaluate(base.Add(35*time.Second), "claude", detection.StatusIdle, claudeReadlinePromptContext)
	assert.False(t, ok, "settle timer must have restarted at the reset, not credited pre-reset progress")
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)
}

func TestIdleGate_Evaluate_ShouldRejectNonClaudeCodeProgram_RegardlessOfStatusOrElapsedTime(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	// A raw shell/editor program reporting an otherwise-safe idle status and
	// context must never pass, even after the full window has elapsed.
	ok, reason := gate.Evaluate(base, "vim", detection.StatusIdle, claudeReadlinePromptContext)
	assert.False(t, ok)
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)

	ok, reason = gate.Evaluate(base.Add(60*time.Second), "vim", detection.StatusIdle, claudeReadlinePromptContext)
	assert.False(t, ok, "non-Claude-Code program must never pass regardless of elapsed time")
	require.NotNil(t, reason)
	assert.Equal(t, SafetyGateReasonNotIdle, *reason)
}

func TestIdleGate_Evaluate_ShouldNeverTreatStatusReadyOrStatusSuccessAsIdle(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	for _, status := range []detection.DetectedStatus{detection.StatusReady, detection.StatusSuccess} {
		ok, reason := gate.Evaluate(base.Add(60*time.Second), "claude", status, claudeReadlinePromptContext)
		assert.False(t, ok, "status %v must never be treated as idle by this gate", status)
		require.NotNil(t, reason)
		assert.Equal(t, SafetyGateReasonNotIdle, *reason)
	}
}

func TestIdleGate_Evaluate_ShouldLogSettleReset_WithDisqualifyingContextAndSecondsLost(t *testing.T) {
	gate, buf := newTestIdleGateWithLog(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	// Sustains 25s of a 30s window, then reports an off-allowlist context.
	gate.Evaluate(base, "claude", detection.StatusIdle, claudeReadlinePromptContext)
	gate.Evaluate(base.Add(25*time.Second), "claude", detection.StatusIdle, commandPromptContext)

	logLine := findLogLine(t, buf, "diagnose.idle.settle_reset")
	assert.Equal(t, commandPromptContext, logLine["status_context"])
	assert.InDelta(t, 25, logLine["seconds_lost"], 0.001)
}

func TestIdleGate_Evaluate_ShouldNotLog_WhenResetOccursWithNoPriorAccruedProgress(t *testing.T) {
	gate, buf := newTestIdleGateWithLog(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	// The very first poll is already off the allowlist, so nothing was ever
	// accrued -- there is no "progress lost" to report.
	gate.Evaluate(base, "claude", detection.StatusIdle, commandPromptContext)

	assert.NotContains(t, buf.String(), "diagnose.idle.settle_reset", "a reset with zero prior progress must not log")
}

// TestIdleGate_Evaluate_ShouldNotRace_WhenCalledConcurrentlyForSameSession
// regression-tests the fix for the unsynchronized read-modify-write of
// settleStart: idleGateRegistry (server/mcp/diagnose_gate_wiring.go) hands
// the SAME *IdleGate pointer to steer_session/write_to_session/resume_session
// for the same target session UUID, and those MCP handlers can invoke
// Evaluate concurrently. Must be run with -race to be meaningful.
func TestIdleGate_Evaluate_ShouldNotRace_WhenCalledConcurrentlyForSameSession(t *testing.T) {
	gate := newTestIdleGate(t, 30)
	base := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

	const goroutines = 20
	const iterationsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				now := base.Add(time.Duration(g*iterationsPerGoroutine+i) * time.Second)
				gate.Evaluate(now, "claude", detection.StatusIdle, concurrentPollContext(i))
			}
		}(g)
	}
	wg.Wait()
}

// concurrentPollContext alternates between an allowlisted and a
// non-allowlisted status context, so the concurrency test above exercises
// both Evaluate's settle-accrual path and its reset path under contention.
func concurrentPollContext(i int) string {
	if i%2 == 0 {
		return claudeReadlinePromptContext
	}
	return commandPromptContext
}

// TestIsClaudeCodeProgram_ExactMatchOnEmptyOrClaude regression-tests this
// package's necessarily-duplicated isClaudeCodeProgram (see idle_gate.go's
// doc comment: session/diagnose cannot import server/services to reuse the
// original, even from a test file, per `go test`'s own "import cycle not
// allowed in test" -- server/services already imports session/diagnose
// transitively via session/tmux). There is no automated cross-package check
// against server/services/backlog_service_pr_fix_steer.go's original; keeping
// the two in sync is a code-review responsibility, noted on both copies.
func TestIsClaudeCodeProgram_ExactMatchOnEmptyOrClaude(t *testing.T) {
	for _, tc := range []struct {
		program string
		want    bool
	}{
		{"", true},
		{"claude", true},
		{"vim", false},
		{"bash", false},
		{"proxy-claude", false},
		{"aider", false},
	} {
		assert.Equal(t, tc.want, isClaudeCodeProgram(tc.program), "program %q", tc.program)
	}
}

// findLogLine parses buf (newline-delimited JSON log records) and returns the
// fields of the first record whose "msg" matches want, failing the test if
// none is found.
func findLogLine(t *testing.T, buf *bytes.Buffer, want string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &fields))
		if fields["msg"] == want {
			return fields
		}
	}
	t.Fatalf("no log line with msg %q found in:\n%s", want, buf.String())
	return nil
}
