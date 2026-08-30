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
// interface method. restoreDelay, when set, is slept inside
// RestoreWithWorkDir() -- see TestRestartForRetry_..._TwoCallersRaceRetryInFlight
// for why a real concurrency test needs this.
type fakeLivenessProcessManager struct {
	stuckDialogProcessManager
	alive        bool
	restoreDelay time.Duration
}

func (m *fakeLivenessProcessManager) IsAlive() bool { return m.alive }

func (m *fakeLivenessProcessManager) RestoreWithWorkDir(dir string) error {
	if m.restoreDelay > 0 {
		time.Sleep(m.restoreDelay)
	}
	return m.stuckDialogProcessManager.RestoreWithWorkDir(dir)
}

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

func TestResolveRetryPolicy_should_FallBackToGlobal_When_OverrideRetryOnIsAllUnknownReasons(t *testing.T) {
	t.Parallel()
	// A narrowed global RetryOn plus an override RetryOn made entirely of
	// typo'd/unknown reasons must fall back to the resolved *global* value,
	// not RetryOnOrDefault()'s own empty-after-filter fallback to all three --
	// that would silently widen a deliberately narrow override.
	global := config.RetryPolicyConfig{RetryOn: []string{"crashed"}}
	override := &config.RetryPolicyConfig{RetryOn: []string{"totally-bogus-reason"}}

	got := resolveRetryPolicy(global, override)
	if len(got.RetryOn) != 1 || got.RetryOn[0] != "crashed" {
		t.Errorf("RetryOn = %v, want [crashed] (fall back to global, not widen to all three)", got.RetryOn)
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

	// A live (alive:true) process manager takes restartForRetry's cold-recovery
	// branch's "restore existing tmux session" sub-path (RecoverFromStopped +
	// StopController + Start(false), see TestRetryNow_should_TakeRecoverFromStoppedAndStartPath_...
	// below for the branch-selection assertion itself); Start(false) on a bare
	// Instance with a fake ProcessManager succeeds end-to-end.
	err := inst.RetryNow("/tmp")
	if err != nil {
		t.Errorf("RetryNow() err = %v, want nil", err)
	}

	if inst.RetryAttempt != 0 {
		t.Errorf("RetryAttempt = %d, want 0 after RetryNow", inst.RetryAttempt)
	}
	if !inst.NextRetryAt.IsZero() {
		t.Error("NextRetryAt should be cleared after RetryNow")
	}
	if inst.Status != Active {
		t.Errorf("Status = %v, want Active after a successful RetryNow", inst.Status)
	}
}

// TestRetryNow_should_TakeRecoverFromStoppedAndStartPath_When_CalledFromPermanentlyFailedWithDeadSession
// replaces a prior version of this test that sidestepped the actual bug: it
// called RetryNow() from PermanentlyFailed and only asserted that RetryState
// was reset, explicitly declining to check which restart path ran ("Restart()
// ... is expected to error ... not this test's concern"). That let a real bug
// go untested — RetryNow() used to set Status = Active BEFORE calling
// restartForRetry, so restartForRetry's own GetEffectiveStatus() branch check
// always saw Active and always took the Restart()-in-place path, never the
// RecoverFromStopped()+Start() cold-recovery path meant for a
// PermanentlyFailed/Stopped session with a dead pane (session/retry_state.go's
// restartForRetry). This test proves the correct branch is taken by relying on
// a real, observable divergence between the two: Restart() immediately returns
// ErrCannotRestart when i.started is false (as it is here, simulating a
// process that crashed and was never marked started again) without touching
// Status at all, whereas RecoverFromStopped()+Start() resets Status to
// Creating and then advances it to Active on success — regardless of
// i.started's prior value.
func TestRetryNow_should_TakeRecoverFromStoppedAndStartPath_When_CalledFromPermanentlyFailedWithDeadSession(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "t",
		Status: PermanentlyFailed,
		// alive: false simulates a dead/no tmux session (the pane exited along
		// with the crash that led to PermanentlyFailed).
		processManager: &fakeLivenessProcessManager{alive: false},
	}
	inst.RetryAttempt = 3
	inst.RetryMaxAttempts = 3
	inst.LastFailureReason = "crashed"
	// started is left at its zero value (false): if the bug regresses and
	// Restart() is wrongly taken, Restart() bails out immediately with
	// ErrCannotRestart and Status is left untouched at PermanentlyFailed.

	err := inst.RetryNow("/tmp")
	if errors.Is(err, ErrCannotRestart) {
		t.Fatalf("RetryNow() err = %v: took the Restart()-in-place path instead of RecoverFromStopped()+Start() — the PermanentlyFailed/dead-session branch-selection bug has regressed", err)
	}
	if err != nil {
		t.Fatalf("RetryNow() err = %v, want nil (cold-recovery Start() should succeed against the fake ProcessManager)", err)
	}
	if inst.Status != Active {
		t.Errorf("Status = %v, want Active — RecoverFromStopped()+Start() should have advanced Creating→Active", inst.Status)
	}
	if !inst.Started() {
		t.Error("Started() = false, want true after a successful cold-recovery Start()")
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

// TestRestartForRetry_should_PreventConcurrentRestart_When_TwoCallersRaceRetryInFlight
// was flaky under full-package-suite load: restartForRetry's synchronous work
// (RecoverFromStopped/StopController/Start/pm().RestoreWithWorkDir, all
// near-instant with this mock) could complete -- including the deferred
// retryInFlight.Store(false) release -- before the scheduler ever ran the
// second goroutine's CompareAndSwap, especially when this test's two
// goroutines are competing for CPU time against dozens of other packages'
// parallel tests. That isn't a bug in the CAS guard (mutual exclusion is
// still correctly enforced whenever the two calls actually overlap); it's a
// test that needs its critical section to have nonzero, real duration to
// make that overlap reliable regardless of system load. restoreDelay widens
// the window pm().RestoreWithWorkDir spends holding retryInFlight so the
// second goroutine's CAS attempt reliably lands while the first still holds
// it, without relying on scheduler timing.
func TestRestartForRetry_should_PreventConcurrentRestart_When_TwoCallersRaceRetryInFlight(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:          "t",
		Status:         Stopped,
		processManager: &fakeLivenessProcessManager{alive: true, restoreDelay: 50 * time.Millisecond},
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

// TestSessionDriver_should_RetryThreeTimesWithIncreasingBackoffThenPermanentlyFail_When_SessionCrashesRepeatedly
// is AC1's full-assembly test: the per-function unit tests above (evaluateSessionRetry,
// backoffDelay, markSessionPermanentlyFailed) each pass in isolation, but nothing
// previously drove handleDriverFailure end-to-end across a whole crash-loop episode with
// a policy of max_attempts=3 to confirm the pieces compose correctly — that each
// successive crash actually schedules a longer backoff than the last (not just that
// backoffDelay(n) > backoffDelay(n-1) in the abstract), that the transition to
// PermanentlyFailed happens exactly on the attempt after the cap, and that the resulting
// notification fires exactly once even if the driver observes the already-failed session
// again on a later tick.
func TestSessionDriver_should_RetryThreeTimesWithIncreasingBackoffThenPermanentlyFail_When_SessionCrashesRepeatedly(t *testing.T) {
	t.Parallel()
	rq := NewReviewQueue()
	notifier := &fakeNotifier{}
	inst := &Instance{
		Title:       "crash-loop",
		UUID:        "test-uuid-crash-loop",
		reviewQueue: rq,
		Status:      Active,
	}
	inst.RetryMaxAttempts = 3
	inst.SetNotifier(notifier)

	policy := RetryPolicy{
		Enabled:      true,
		MaxAttempts:  3,
		RetryOn:      []string{"crashed"},
		InitialDelay: 30 * time.Second,
		MaxDelay:     300 * time.Second,
	}

	var prevDelay time.Duration
	for attempt := 1; attempt <= 3; attempt++ {
		before := time.Now()
		handleDriverFailure(inst, "/tmp", policy, "crashed", make(chan struct{}))

		if inst.RetryAttempt != attempt {
			t.Fatalf("after crash #%d: RetryAttempt = %d, want %d", attempt, inst.RetryAttempt, attempt)
		}
		if inst.Status == PermanentlyFailed {
			t.Fatalf("after crash #%d: session marked PermanentlyFailed too early (max_attempts=3)", attempt)
		}
		if len(inst.RetryHistory) != attempt {
			t.Fatalf("after crash #%d: len(RetryHistory) = %d, want %d", attempt, len(inst.RetryHistory), attempt)
		}

		delay := inst.NextRetryAt.Sub(before)
		base := policy.InitialDelay * time.Duration(int64(1)<<uint(attempt-1))
		if base > policy.MaxDelay {
			base = policy.MaxDelay
		}
		// backoffDelay applies +/-10% jitter (defaultJitterFraction); allow a
		// generous slop on top for wall-clock scheduling noise between
		// `before` and handleDriverFailure's own now.
		lower := time.Duration(float64(base)*0.9) - time.Second
		upper := time.Duration(float64(base)*1.1) + time.Second
		if delay < lower || delay > upper {
			t.Errorf("after crash #%d: backoff delay = %v, want in [%v, %v] (base %v)", attempt, delay, lower, upper, base)
		}
		if attempt > 1 && delay <= prevDelay/2 {
			t.Errorf("after crash #%d: backoff delay %v did not increase over previous attempt's %v", attempt, delay, prevDelay)
		}
		prevDelay = delay

		// The real driver poll loop clears NextRetryAt once it consumes a
		// pending retry and restarts the session (clearNextRetryAt, called
		// from the restart path) — simulate that here so the next simulated
		// crash starts from a clean pending-retry state.
		inst.NextRetryAt = time.Time{}
	}

	// A 4th crash arrives with RetryAttempt already at the resolved cap (3):
	// evaluateSessionRetry must now return retryDecisionExhausted rather than
	// scheduling a 4th backoff.
	handleDriverFailure(inst, "/tmp", policy, "crashed", make(chan struct{}))

	if inst.Status != PermanentlyFailed {
		t.Fatalf("Status = %v, want PermanentlyFailed after exhausting max_attempts", inst.Status)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want exactly 1", len(notifier.calls))
	}
	if _, found := rq.Get(inst.UUID); !found {
		t.Error("expected a ReviewQueue entry for the permanently-failed session")
	}

	// A later observation of the already-PermanentlyFailed session (e.g. the
	// driver loop ticking again before it stops) must not re-fire the
	// one-shot notification.
	handleDriverFailure(inst, "/tmp", policy, "crashed", make(chan struct{}))
	if len(notifier.calls) != 1 {
		t.Errorf("notifier calls after redundant failure observation = %d, want still 1 (edge-triggered, no re-fire)", len(notifier.calls))
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
