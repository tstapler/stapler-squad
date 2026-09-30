package wait

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestPollEventually_should_SucceedImmediately_When_ConditionAlreadyTrue
// covers the fast path most callers hit: no ticker wait needed at all.
func TestPollEventually_should_SucceedImmediately_When_ConditionAlreadyTrue(t *testing.T) {
	ok, _ := pollEventually(func() bool { return true }, 50*time.Millisecond, 5*time.Millisecond, nil)
	if !ok {
		t.Fatal("pollEventually returned false for an always-true condition")
	}
}

// TestPollEventually_should_SucceedAfterPolling_When_ConditionBecomesTrueLater
// verifies the actual polling loop, not just the immediate-check fast path.
func TestPollEventually_should_SucceedAfterPolling_When_ConditionBecomesTrueLater(t *testing.T) {
	var calls int32
	condition := func() bool {
		return atomic.AddInt32(&calls, 1) >= 3
	}
	ok, _ := pollEventually(condition, 500*time.Millisecond, 5*time.Millisecond, nil)
	if !ok {
		t.Fatal("pollEventually returned false for a condition that becomes true after a few polls")
	}
}

// TestPollEventually_should_ReportFailure_When_ConditionNeverBecomesTrue is
// the must-fail-against-pre-fix-code case: without a real bound, this would
// hang forever instead of returning false.
func TestPollEventually_should_ReportFailure_When_ConditionNeverBecomesTrue(t *testing.T) {
	freezeLoadFactor(t, 1.0) // deterministic bound, independent of real machine load

	ok, tr := pollEventually(func() bool { return false }, 30*time.Millisecond, 5*time.Millisecond, nil)
	if ok {
		t.Fatal("pollEventually returned true for an always-false condition")
	}
	if tr.err() == nil {
		t.Fatal("expected a non-nil timeout error")
	}
}

// TestPollEventually_should_ExtendDeadline_When_EnvScaleOverrideIsSet proves
// ScaleTimeout's effect actually reaches the polling loop: a condition that
// only becomes true after the *unscaled* timeout would have elapsed must
// still succeed once STAPLER_SQUAD_TEST_TIMEOUT_SCALE stretches the bound.
func TestPollEventually_should_ExtendDeadline_When_EnvScaleOverrideIsSet(t *testing.T) {
	freezeLoadFactor(t, 1.0)
	t.Setenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE", "10")

	start := time.Now()
	becomesTrueAt := start.Add(60 * time.Millisecond) // well past the unscaled 30ms base timeout
	condition := func() bool { return time.Now().After(becomesTrueAt) }

	ok, _ := pollEventually(condition, 30*time.Millisecond, 5*time.Millisecond, nil)
	if !ok {
		t.Fatal("pollEventually failed within the scaled (10x) timeout for a condition that becomes true at 2x the base timeout")
	}
}

// TestRequireEventually_should_NotFailTest_When_ConditionSatisfied is a
// smoke test of the public testing.TB-facing entry point.
func TestRequireEventually_should_NotFailTest_When_ConditionSatisfied(t *testing.T) {
	RequireEventually(t, func() bool { return true }, 100*time.Millisecond, 5*time.Millisecond, "should never fire")
}

// fakeTB implements just enough of testing.TB to observe RequireEventually's
// Fatalf call without actually failing the real test running it.
type fakeTB struct {
	testing.TB
	fatalCalls int
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatalCalls++
	_ = fmt.Sprintf(format, args...) // exercise formatting; not asserted on directly
}
func (f *fakeTB) Logf(format string, args ...any) {}

// TestRequireEventually_should_CallFatalf_When_ConditionNeverSatisfied is the
// must-fail-against-pre-fix-code case for the public API: a real
// require.Eventually-shaped caller relies on Fatalf actually being invoked.
func TestRequireEventually_should_CallFatalf_When_ConditionNeverSatisfied(t *testing.T) {
	freezeLoadFactor(t, 1.0)

	ft := &fakeTB{}
	RequireEventually(ft, func() bool { return false }, 20*time.Millisecond, 5*time.Millisecond, "custom message")
	if ft.fatalCalls != 1 {
		t.Fatalf("expected exactly one Fatalf call, got %d", ft.fatalCalls)
	}
}

func TestFormatMsgAndArgs_should_HandleEmptyAndFormatted(t *testing.T) {
	if got := formatMsgAndArgs(nil); got != "condition never satisfied" {
		t.Errorf("formatMsgAndArgs(nil) = %q, want default message", got)
	}
	if got := formatMsgAndArgs([]any{"plain message"}); got != "plain message" {
		t.Errorf("formatMsgAndArgs([]any{\"plain message\"}) = %q", got)
	}
	if got := formatMsgAndArgs([]any{"got %d, want %d", 1, 2}); got != "got 1, want 2" {
		t.Errorf("formatMsgAndArgs with format args = %q", got)
	}
}
