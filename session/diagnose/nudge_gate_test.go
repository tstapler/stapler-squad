package diagnose

import (
	"context"
	"errors"
	"testing"
)

// spyCheck wraps a NudgeGateCheck and counts how many times it was invoked,
// so tests can assert a later check in the pipeline was never called after
// an earlier one short-circuited.
type spyCheck struct {
	calls  int
	ok     bool
	reason SafetyGateReason
}

func (s *spyCheck) check(_ context.Context, _ GateInput) (bool, SafetyGateReason) {
	s.calls++
	return s.ok, s.reason
}

func alwaysPass(_ context.Context, _ GateInput) (bool, SafetyGateReason) {
	return true, ""
}

func TestNudgeGate_Evaluate_ShouldReturnTrue_WhenFlagIdleIdentityAndCapChecksAllPass(t *testing.T) {
	flag := &spyCheck{ok: true}
	idle := &spyCheck{ok: true}
	identity := &spyCheck{ok: true}
	cap := &spyCheck{ok: true}

	gate := NewNudgeGate(flag.check, idle.check, identity.check, cap.check)

	ok, reason := gate.Evaluate(context.Background(), GateInput{ItemID: "item-1", SessionUUID: "sess-1"})

	if !ok {
		t.Fatalf("Evaluate() ok = false, want true")
	}
	if reason != nil {
		t.Fatalf("Evaluate() reason = %v, want nil", *reason)
	}
	for name, spy := range map[string]*spyCheck{"flag": flag, "idle": idle, "identity": identity, "cap": cap} {
		if spy.calls != 1 {
			t.Errorf("%s check calls = %d, want 1", name, spy.calls)
		}
	}
}

func TestNudgeGate_Evaluate_ShouldReturnNudgeExecutionDisabled_AndSkipIdleIdentityCap_When_FlagCheckFails(t *testing.T) {
	flag := &spyCheck{ok: false, reason: SafetyGateReasonNudgeExecutionDisabled}
	idle := &spyCheck{ok: true}
	identity := &spyCheck{ok: true}
	cap := &spyCheck{ok: true}

	gate := NewNudgeGate(flag.check, idle.check, identity.check, cap.check)

	ok, reason := gate.Evaluate(context.Background(), GateInput{ItemID: "item-1", SessionUUID: "sess-1"})

	if ok {
		t.Fatalf("Evaluate() ok = true, want false")
	}
	if reason == nil || *reason != SafetyGateReasonNudgeExecutionDisabled {
		t.Fatalf("Evaluate() reason = %v, want %v", reason, SafetyGateReasonNudgeExecutionDisabled)
	}
	if flag.calls != 1 {
		t.Errorf("flag check calls = %d, want 1", flag.calls)
	}
	for name, spy := range map[string]*spyCheck{"idle": idle, "identity": identity, "cap": cap} {
		if spy.calls != 0 {
			t.Errorf("%s check calls = %d, want 0 (must not be evaluated after flag check fails)", name, spy.calls)
		}
	}
}

// TestNudgeGate_Evaluate_ShouldAbortOnFirstFailingCheck_AndNeverEvaluateSubsequentChecks
// is table-driven across all four pipeline positions (Task 3.4.1b): for each
// position, that check fails and every check after it in pipeline order must
// record zero invocations, while every check before it (and the failing
// check itself) records exactly one.
func TestNudgeGate_Evaluate_ShouldAbortOnFirstFailingCheck_AndNeverEvaluateSubsequentChecks(t *testing.T) {
	positions := []struct {
		name         string
		failIndex    int
		failedReason SafetyGateReason
	}{
		{"flag", 0, SafetyGateReasonNudgeExecutionDisabled},
		{"idle", 1, SafetyGateReasonNotIdle},
		{"identity", 2, SafetyGateReasonIdentityMismatchInstance},
		{"cap", 3, SafetyGateReasonNudgeCapReached},
	}

	for _, tc := range positions {
		t.Run(tc.name, func(t *testing.T) {
			assertFailAtPositionShortCircuits(t, tc.failIndex, tc.failedReason)
		})
	}
}

// assertFailAtPositionShortCircuits builds a 4-check pipeline where only the
// check at failIndex fails, evaluates it, and asserts: the pipeline reports
// (false, failedReason); every check up to and including failIndex ran
// exactly once; every check after failIndex never ran.
func assertFailAtPositionShortCircuits(t *testing.T, failIndex int, failedReason SafetyGateReason) {
	t.Helper()

	spies := []*spyCheck{{ok: true}, {ok: true}, {ok: true}, {ok: true}}
	spies[failIndex].ok = false
	spies[failIndex].reason = failedReason

	gate := NewNudgeGate(spies[0].check, spies[1].check, spies[2].check, spies[3].check)
	ok, reason := gate.Evaluate(context.Background(), GateInput{ItemID: "item-1", SessionUUID: "sess-1"})

	if ok {
		t.Fatalf("Evaluate() ok = true, want false")
	}
	if reason == nil || *reason != failedReason {
		t.Fatalf("Evaluate() reason = %v, want %v", reason, failedReason)
	}
	assertSpyCallCounts(t, spies, failIndex)
}

// assertSpyCallCounts asserts every spy up to and including failIndex was
// called exactly once, and every spy after it was never called.
func assertSpyCallCounts(t *testing.T, spies []*spyCheck, failIndex int) {
	t.Helper()

	for i, spy := range spies {
		want := 1
		if i > failIndex {
			want = 0
		}
		if spy.calls != want {
			t.Errorf("check[%d] calls = %d, want %d", i, spy.calls, want)
		}
	}
}

// TestNudgeGate_Evaluate_ShouldTreatCheckErrorAsFailure_NeverDefaultAllow_WhenACheckReturnsAnAmbiguousError
// models a check that internally hits an ambiguous error -- e.g. the tmux
// marker read failing with a transport error -- and, per NudgeGateCheck's
// documented contract, resolves that ambiguity to (false, reason) rather
// than propagating an error out through the signature (there is no error
// return to propagate through). The pipeline must abort exactly like any
// other false result, never fall through to (true, nil).
func TestNudgeGate_Evaluate_ShouldTreatCheckErrorAsFailure_NeverDefaultAllow_WhenACheckReturnsAnAmbiguousError(t *testing.T) {
	transportErr := errors.New("tmux marker read: transport error")

	// identityCheck simulates a real identity check that encountered an
	// ambiguous internal error (transportErr) while re-verifying identity,
	// and -- per contract -- fails closed rather than propagating it.
	identityCallCount := 0
	identityCheck := func(_ context.Context, _ GateInput) (bool, SafetyGateReason) {
		identityCallCount++
		if transportErr != nil {
			return false, SafetyGateReasonIdentityMismatchTmuxMarker
		}
		return true, ""
	}

	capCallCount := 0
	capCheck := func(_ context.Context, _ GateInput) (bool, SafetyGateReason) {
		capCallCount++
		return true, ""
	}

	gate := NewNudgeGate(alwaysPass, alwaysPass, identityCheck, capCheck)

	ok, reason := gate.Evaluate(context.Background(), GateInput{ItemID: "item-1", SessionUUID: "sess-1"})

	if ok {
		t.Fatalf("Evaluate() ok = true, want false (ambiguous error must fail closed)")
	}
	if reason == nil || *reason != SafetyGateReasonIdentityMismatchTmuxMarker {
		t.Fatalf("Evaluate() reason = %v, want %v", reason, SafetyGateReasonIdentityMismatchTmuxMarker)
	}
	if identityCallCount != 1 {
		t.Errorf("identity check calls = %d, want 1", identityCallCount)
	}
	if capCallCount != 0 {
		t.Errorf("cap check calls = %d, want 0 (must not be evaluated after identity check fails)", capCallCount)
	}
}
