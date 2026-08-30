package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
)

// fakeLivenessProcessManager reuses stuckDialogProcessManager's full
// ProcessManager implementation (session_driver_test.go) and overrides only
// IsAlive, so classifyFailureReason's tests don't need to hand-roll every
// interface method.
type fakeLivenessProcessManager struct {
	stuckDialogProcessManager
	alive bool
}

func (m *fakeLivenessProcessManager) IsAlive() bool { return m.alive }

func TestBackoffDelay_should_DoubleEachAttempt_When_BelowMaxDelayCap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 10 * time.Second},
		{1, 20 * time.Second},
		{2, 40 * time.Second},
	}
	for _, tc := range cases {
		got := backoffDelay(tc.attempt, 10*time.Second, 300*time.Second, 0)
		if got != tc.want {
			t.Errorf("backoffDelay(%d, 10s, 300s, 0) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoffDelay_should_CapAtMaxDelay_When_AttemptCountCausesShiftOverflow(t *testing.T) {
	t.Parallel()
	cases := []int{10, 63, 64, 1000}
	for _, attempt := range cases {
		got := backoffDelay(attempt, 10*time.Second, 300*time.Second, 0)
		if got != 300*time.Second {
			t.Errorf("backoffDelay(%d, 10s, 300s, 0) = %v, want exactly 300s (cap boundary, not overflowed/negative)", attempt, got)
		}
		if got < 0 {
			t.Errorf("backoffDelay(%d, ...) returned negative duration %v", attempt, got)
		}
	}
}

func TestBackoffDelay_should_ApplyFloor_When_InitialDelayIsZero(t *testing.T) {
	t.Parallel()
	// AC7's default policy keeps InitialDelaySeconds at 0; without a floor,
	// 0*2^0 stays 0 and same-tick shared-cause failures would restart in
	// lockstep even with jitter (pre-mortem Failure #1).
	got := backoffDelay(0, 0, 300*time.Second, 0)
	if got != minAttemptZeroFloor {
		t.Errorf("backoffDelay(0, 0, 300s, 0) = %v, want the minAttemptZeroFloor %v", got, minAttemptZeroFloor)
	}
}

func TestBackoffDelay_should_VaryRunToRun_When_JitterFractionNonzero(t *testing.T) {
	t.Parallel()
	seen := map[time.Duration]bool{}
	for range 50 {
		d := backoffDelay(2, 10*time.Second, 300*time.Second, defaultJitterFraction)
		if d < 0 {
			t.Fatalf("backoffDelay returned negative duration %v", d)
		}
		upper := time.Duration(float64(300*time.Second) * (1 + defaultJitterFraction))
		if d > upper {
			t.Fatalf("backoffDelay = %v exceeds max*(1+jitterFraction) = %v", d, upper)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Error("expected backoffDelay with nonzero jitterFraction to vary run-to-run, got identical values every time — jitter is not actually being applied at this call site")
	}
}

func TestClassifyFailureReason_should_ReturnTmuxExited_When_ProcessManagerNotAlive(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", processManager: &fakeLivenessProcessManager{alive: false}}
	if got := classifyFailureReason(inst); got != "tmux_exited" {
		t.Errorf("classifyFailureReason = %q, want tmux_exited", got)
	}
}

func TestClassifyFailureReason_should_ReturnCrashed_When_ProcessManagerAlive(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", processManager: &fakeLivenessProcessManager{alive: true}}
	if got := classifyFailureReason(inst); got != "crashed" {
		t.Errorf("classifyFailureReason = %q, want crashed", got)
	}
}

func TestEvaluateSessionRetry_should_ReturnNotEligible_When_ReasonAbsentFromRetryOn(t *testing.T) {
	t.Parallel()
	rs := RetryState{RetryAttempt: 2, RetryMaxAttempts: 3}
	policy := RetryPolicy{Enabled: true, RetryOn: []string{"crashed"}}
	now := time.Now()
	bootTime := now.Add(-time.Hour) // long past any restart-grace window

	got := evaluateSessionRetry(rs, policy, "tmux_exited", now, bootTime)
	if got != retryDecisionNotEligible {
		t.Errorf("evaluateSessionRetry(...) = %v, want notEligible (AC2: tmux_exited absent from retry_on)", got)
	}
}

func TestEvaluateSessionRetry_should_ReturnExhausted_When_AttemptAtCap(t *testing.T) {
	t.Parallel()
	rs := RetryState{RetryAttempt: 3, RetryMaxAttempts: 3}
	policy := RetryPolicy{Enabled: true, RetryOn: []string{"crashed"}}
	now := time.Now()
	bootTime := now.Add(-time.Hour)

	got := evaluateSessionRetry(rs, policy, "crashed", now, bootTime)
	if got != retryDecisionExhausted {
		t.Errorf("evaluateSessionRetry(...) = %v, want exhausted (AC1's terminal transition)", got)
	}
}

func TestEvaluateSessionRetry_should_ReturnScheduled_When_BelowCapAndEligible(t *testing.T) {
	t.Parallel()
	rs := RetryState{RetryAttempt: 1, RetryMaxAttempts: 3}
	policy := RetryPolicy{Enabled: true, RetryOn: []string{"crashed"}}
	now := time.Now()
	bootTime := now.Add(-time.Hour)

	got := evaluateSessionRetry(rs, policy, "crashed", now, bootTime)
	if got != retryDecisionScheduled {
		t.Errorf("evaluateSessionRetry(...) = %v, want scheduled", got)
	}
}

func TestEvaluateSessionRetry_should_ReturnRestartGrace_When_TmuxExitedWithinBootWindow(t *testing.T) {
	t.Parallel()
	rs := RetryState{RetryAttempt: 0, RetryMaxAttempts: 1}
	policy := RetryPolicy{Enabled: true, RetryOn: []string{"crashed", "stalled", "tmux_exited"}}
	bootTime := time.Now()
	now := bootTime.Add(5 * time.Second)

	got := evaluateSessionRetry(rs, policy, "tmux_exited", now, bootTime)
	if got != retryDecisionRestartGrace {
		t.Errorf("evaluateSessionRetry(...) = %v, want restartGrace", got)
	}
}

func TestEvaluateSessionRetry_should_ReturnNotEligible_When_PolicyDisabled(t *testing.T) {
	t.Parallel()
	rs := RetryState{RetryAttempt: 0, RetryMaxAttempts: 3}
	policy := RetryPolicy{Enabled: false, RetryOn: []string{"crashed"}}
	now := time.Now()

	got := evaluateSessionRetry(rs, policy, "crashed", now, now.Add(-time.Hour))
	if got != retryDecisionNotEligible {
		t.Errorf("evaluateSessionRetry(...) = %v, want notEligible when policy disabled", got)
	}
}

func TestResolveRetryPolicy_should_PreferOverride_When_PerSessionOverrideSet(t *testing.T) {
	t.Parallel()
	global := config.RetryPolicyConfig{MaxAttempts: 1}
	override := &config.RetryPolicyConfig{MaxAttempts: 5}

	got := resolveRetryPolicy(global, override)
	if got.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5 (override wins)", got.MaxAttempts)
	}
}

func TestResolveRetryPolicy_should_FallBackToGlobal_When_OverrideNil(t *testing.T) {
	t.Parallel()
	global := config.RetryPolicyConfig{MaxAttempts: 2}

	got := resolveRetryPolicy(global, nil)
	if got.MaxAttempts != 2 {
		t.Errorf("MaxAttempts = %d, want 2 (inherited from global)", got.MaxAttempts)
	}
}

func TestResolveRetryPolicy_should_PreferPerFieldOverride_When_OnlySomeFieldsSet(t *testing.T) {
	t.Parallel()
	// A narrowed global RetryOn plus an override that only sets MaxAttempts
	// must NOT silently widen RetryOn back to the all-three fallback that
	// RetryOnOrDefault() would produce from an empty override field taken in
	// isolation (architecture-review.md Concerns / adversarial-review.md
	// Blocker 4).
	global := config.RetryPolicyConfig{RetryOn: []string{"crashed"}, MaxAttempts: 1}
	override := &config.RetryPolicyConfig{MaxAttempts: 5}

	got := resolveRetryPolicy(global, override)
	if got.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", got.MaxAttempts)
	}
	if len(got.RetryOn) != 1 || got.RetryOn[0] != "crashed" {
		t.Errorf("RetryOn = %v, want [crashed] (must not widen to the all-three fallback)", got.RetryOn)
	}
}

func TestRetryExhausted_should_ReturnTrue_When_AttemptAtOrAboveMax(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t"}
	inst.RetryAttempt = 2
	inst.RetryMaxAttempts = 2
	if !inst.retryExhausted() {
		t.Error("retryExhausted() = false, want true when RetryAttempt >= RetryMaxAttempts")
	}
}

func TestRetryExhausted_should_ReturnFalse_When_MaxAttemptsUnset(t *testing.T) {
	t.Parallel()
	// A fresh Instance (driver never started, RetryMaxAttempts still its
	// zero value) must not report "exhausted" — that would incorrectly
	// short-circuit every one-shot/no-policy code path.
	inst := &Instance{Title: "t"}
	if inst.retryExhausted() {
		t.Error("retryExhausted() = true, want false when RetryMaxAttempts is unset (zero)")
	}
}

func TestRetryNow_should_ResetRetryStateAndTransitionToActive_When_CalledFromPermanentlyFailed(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:          "t",
		Status:         PermanentlyFailed,
		processManager: &fakeLivenessProcessManager{alive: true},
	}
	inst.RetryAttempt = 3
	inst.RetryMaxAttempts = 3
	inst.LastFailureReason = "crashed"

	// Restart() on a bare Instance with no real tmux plumbing is expected to
	// error (no session to restart) — RetryNow still resets state and
	// attempts the restart; the resulting error is not this test's concern
	// (restartForRetry's own failure handling is covered separately).
	_ = inst.RetryNow("/tmp")

	if inst.RetryAttempt != 0 {
		t.Errorf("RetryAttempt = %d, want 0 after RetryNow", inst.RetryAttempt)
	}
	if !inst.NextRetryAt.IsZero() {
		t.Error("NextRetryAt should be cleared after RetryNow")
	}
}

func TestRetryNow_should_ReturnErrRetryInFlight_When_RetryInFlightAlreadyClaimed(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:          "t",
		Status:         PermanentlyFailed,
		processManager: &fakeLivenessProcessManager{alive: true},
	}
	inst.retryInFlight.Store(true) // simulate an in-flight restart

	err := inst.RetryNow("/tmp")
	if !errors.Is(err, ErrRetryInFlight) {
		t.Errorf("RetryNow() err = %v, want ErrRetryInFlight", err)
	}
}

func TestRestartForRetry_should_PreventConcurrentRestart_When_TwoCallersRaceRetryInFlight(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:          "t",
		Status:         Stopped,
		processManager: &fakeLivenessProcessManager{alive: true},
	}
	policy := RetryPolicy{Enabled: true, MaxAttempts: 3, RetryOn: []string{"crashed"}}

	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			results <- restartForRetry(inst, "/tmp", "continue", policy, make(chan struct{}))
		}()
	}
	close(start)

	var inFlightCount, otherCount int
	for range 2 {
		err := <-results
		if errors.Is(err, ErrRetryInFlight) {
			inFlightCount++
		} else {
			otherCount++
		}
	}
	if inFlightCount != 1 {
		t.Errorf("expected exactly 1 of 2 concurrent restartForRetry calls to observe ErrRetryInFlight, got %d (other=%d)", inFlightCount, otherCount)
	}
}

func TestIsRetryPending_should_ReturnTrue_When_NextRetryAtSet(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t"}
	inst.NextRetryAt = time.Now().Add(time.Minute)
	if !inst.IsRetryPending() {
		t.Error("IsRetryPending() = false, want true when NextRetryAt is set")
	}
}

func TestBuildRetryContinuationPrompt_should_PrependFailureReasonAndWorktreeHint_When_ReasonGiven(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", InitialPrompt: "do the task"}
	inst.RetryAttempt = 2

	got := buildRetryContinuationPrompt(inst, "tmux_exited")

	if !containsAll(got, "Previous attempt failed due to tmux_exited.", "retry attempt 2", "worktree itself may be the problem", "do the task") {
		t.Errorf("buildRetryContinuationPrompt() = %q, missing expected failure-reason prefix, attempt number, worktree-integrity hint, or fallback prompt text", got)
	}
}

func TestBuildRetryContinuationPrompt_should_ApplyPrefix_When_JSONLContinuationUnavailable(t *testing.T) {
	t.Parallel()
	// No HistoryFilePath set -> buildContinuationPrompt falls back to the
	// generic message, then to InitialPrompt (AC3's fallback path).
	inst := &Instance{Title: "t", InitialPrompt: "resume the workflow"}

	got := buildRetryContinuationPrompt(inst, "crashed")

	if !containsAll(got, "Previous attempt failed due to crashed.", "resume the workflow") {
		t.Errorf("buildRetryContinuationPrompt() = %q, want failure-reason prefix applied even on the fallback prompt path", got)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestIsRetryPending_should_ReturnFalse_When_NoRetryScheduledOrInFlight(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t"}
	if inst.IsRetryPending() {
		t.Error("IsRetryPending() = true, want false for a fresh instance")
	}
}
