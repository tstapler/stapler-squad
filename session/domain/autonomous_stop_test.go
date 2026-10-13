package domain

import (
	"fmt"
	"testing"
)

func TestStopReason_Class_CoversEveryValue(t *testing.T) {
	t.Parallel()

	want := map[StopReason]OutcomeClass{
		StopReasonDone:               OutcomeClassDone,
		StopReasonMaxTurns:           OutcomeClassStall,
		StopReasonStartupTimeout:     OutcomeClassStall,
		StopReasonOrchestratorError:  OutcomeClassStall,
		StopReasonSubmitFailed:       OutcomeClassStall,
		StopReasonLeaseTimeout:       OutcomeClassStall,
		StopReasonRateLimitExhausted: OutcomeClassStall,
		StopReasonCancelledExternal:  OutcomeClassCancelled,
		StopReasonCancelledShutdown:  OutcomeClassCancelled,
		StopReasonPanic:              OutcomeClassStall,
		StopReason(""):               OutcomeClassStall,
	}

	for _, r := range append([]StopReason{""}, AllStopReasons...) {
		r := r
		t.Run(fmt.Sprintf("%q", string(r)), func(t *testing.T) {
			t.Parallel()
			expected, ok := want[r]
			if !ok {
				t.Fatalf("StopReason %q has no expected class row; add it to this table", r)
			}
			if got := r.Class(); got != expected {
				t.Errorf("Class() = %q, want %q", got, expected)
			}
			if got, wantStall := r.IsStall(), expected == OutcomeClassStall; got != wantStall {
				t.Errorf("IsStall() = %v, want %v", got, wantStall)
			}
			if r.IsDone() && r.IsStall() {
				t.Errorf("IsDone() and IsStall() both true")
			}
			if r.String() != string(r) {
				t.Errorf("String() = %q, want %q", r.String(), string(r))
			}
		})
	}

	if StopReason("bogus").IsValid() {
		t.Error(`IsValid("bogus") = true, want false`)
	}
	if StopReason("").IsValid() {
		t.Error(`IsValid("") = true, want false (legacy value is not a defined reason)`)
	}
	for _, r := range AllStopReasons {
		if !r.IsValid() {
			t.Errorf("IsValid(%q) = false, want true", r)
		}
	}
	if !StopReason("bogus").IsStall() {
		t.Error(`unknown reason must be a stall`)
	}
}

func TestStopReason_AllValuesListed_ExhaustiveSwitch(t *testing.T) {
	t.Parallel()

	expected := map[StopReason]bool{
		StopReasonDone: true, StopReasonMaxTurns: true, StopReasonStartupTimeout: true,
		StopReasonOrchestratorError: true, StopReasonSubmitFailed: true, StopReasonLeaseTimeout: true,
		StopReasonRateLimitExhausted: true, StopReasonCancelledExternal: true,
		StopReasonCancelledShutdown: true, StopReasonPanic: true,
	}
	if len(AllStopReasons) != len(expected) {
		t.Fatalf("AllStopReasons has %d entries, hand-listed set has %d", len(AllStopReasons), len(expected))
	}
	seen := map[StopReason]bool{}
	for _, r := range AllStopReasons {
		if !expected[r] {
			t.Errorf("AllStopReasons contains unexpected %q", r)
		}
		if seen[r] {
			t.Errorf("AllStopReasons lists %q twice", r)
		}
		seen[r] = true
	}
}

func TestClassifyObserved_Table(t *testing.T) {
	t.Parallel()

	alive := func(b StatusBucket) StopSnapshot {
		return StopSnapshot{Bucket: b, TmuxAlive: true, ControllerActive: true}
	}
	cases := []struct {
		name string
		snap StopSnapshot
		want ObservedState
	}{
		{"safe_idle alive is idle_prompt", alive(BucketSafeIdle), ObservedIdlePrompt},
		{"needs_input is needs_approval", alive(BucketNeedsInput), ObservedNeedsApproval},
		{"error bucket is error", alive(BucketError), ObservedError},
		{"active is working", alive(BucketActive), ObservedWorking},
		{"idle is idle", alive(BucketIdle), ObservedIdle},
		{"unknown bucket is unknown", alive(BucketUnknown), ObservedUnknown},
		{"empty bucket is unknown", alive(""), ObservedUnknown},
		{"unrecognised bucket is unknown", alive("bogus"), ObservedUnknown},
		{
			"pane dead exit 0 is exited",
			StopSnapshot{Bucket: BucketSafeIdle, TmuxAlive: true, ControllerActive: true, PaneDead: true, ExitCode: 0},
			ObservedExited,
		},
		{
			"pane dead exit 1 with active bucket is crashed (liveness beats bucket)",
			StopSnapshot{Bucket: BucketActive, TmuxAlive: true, ControllerActive: true, PaneDead: true, ExitCode: 1},
			ObservedCrashed,
		},
		{
			"tmux not alive is exited",
			StopSnapshot{Bucket: BucketActive, TmuxAlive: false, ControllerActive: true},
			ObservedExited,
		},
		{
			"no controller is exited",
			StopSnapshot{Bucket: BucketActive, TmuxAlive: true, ControllerActive: false},
			ObservedExited,
		},
		{
			"pane death outranks missing controller (exit 2 is crashed)",
			StopSnapshot{Bucket: BucketIdle, TmuxAlive: true, ControllerActive: false, PaneDead: true, ExitCode: 2},
			ObservedCrashed,
		},
		{
			"pane death outranks tmux not alive (exit 137 is crashed)",
			StopSnapshot{Bucket: BucketIdle, TmuxAlive: false, ControllerActive: true, PaneDead: true, ExitCode: 137},
			ObservedCrashed,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyObserved(tc.snap); got != tc.want {
				t.Errorf("ClassifyObserved() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClassifyObserved_EveryBucketMapsToDefinedState(t *testing.T) {
	t.Parallel()

	defined := map[ObservedState]bool{}
	for _, s := range AllObservedStates {
		defined[s] = true
	}
	for _, b := range AllStatusBuckets {
		got := ClassifyObserved(StopSnapshot{Bucket: b, TmuxAlive: true, ControllerActive: true})
		if !defined[got] {
			t.Errorf("bucket %q classified to %q which is not in AllObservedStates", b, got)
		}
	}
}

// ruleCase builds a fully-enabled work-role input for the named-row tests.
func ruleCase(reason StopReason, state ObservedState, mutate func(*StallDecisionInput)) StallDecisionInput {
	in := StallDecisionInput{
		Reason:             reason,
		State:              state,
		NudgeEnabled:       true,
		NudgeAllowed:       true,
		ResuperviseAllowed: true,
		ItemInWorkStatus:   true,
	}
	if mutate != nil {
		mutate(&in)
	}
	return in
}

func TestDecideStallAction_RuleTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   StallDecisionInput
		want StallAction
	}{
		{"row1 done idle_prompt", ruleCase(StopReasonDone, ObservedIdlePrompt, nil), StallActionNone},
		{"row1 cancelled_external working", ruleCase(StopReasonCancelledExternal, ObservedWorking, nil), StallActionNone},
		{"row1 cancelled_shutdown idle_prompt", ruleCase(StopReasonCancelledShutdown, ObservedIdlePrompt, nil), StallActionNone},
		{"row2 archived", ruleCase(StopReasonOrchestratorError, ObservedIdlePrompt, func(i *StallDecisionInput) { i.Archived = true }), StallActionNone},
		{"row3 retry pending", ruleCase(StopReasonOrchestratorError, ObservedIdlePrompt, func(i *StallDecisionInput) { i.RetryPending = true }), StallActionNone},
		{"row4 max_turns idle_prompt never nudges", ruleCase(StopReasonMaxTurns, ObservedIdlePrompt, nil), StallActionRespawn},
		{"row4 max_turns working never resupervises", ruleCase(StopReasonMaxTurns, ObservedWorking, nil), StallActionRespawn},
		{"row4 panic idle_prompt", ruleCase(StopReasonPanic, ObservedIdlePrompt, nil), StallActionRespawn},
		{"row4 panic working", ruleCase(StopReasonPanic, ObservedWorking, nil), StallActionRespawn},
		{"row5 working with headroom resupervises", ruleCase(StopReasonStartupTimeout, ObservedWorking, nil), StallActionResupervise},
		{"row6 working no headroom respawns", ruleCase(StopReasonStartupTimeout, ObservedWorking, func(i *StallDecisionInput) { i.ResuperviseAllowed = false }), StallActionRespawn},
		{"row6 working item not in work status respawns", ruleCase(StopReasonStartupTimeout, ObservedWorking, func(i *StallDecisionInput) { i.ItemInWorkStatus = false }), StallActionRespawn},
		{"row7 idle_prompt nudges", ruleCase(StopReasonOrchestratorError, ObservedIdlePrompt, nil), StallActionNudge},
		{"row7 idle_prompt item not in work status respawns", ruleCase(StopReasonOrchestratorError, ObservedIdlePrompt, func(i *StallDecisionInput) { i.ItemInWorkStatus = false }), StallActionRespawn},
		{"row8 needs_approval", ruleCase(StopReasonOrchestratorError, ObservedNeedsApproval, nil), StallActionRespawn},
		{"row8 error", ruleCase(StopReasonOrchestratorError, ObservedError, nil), StallActionRespawn},
		{"row8 exited", ruleCase(StopReasonOrchestratorError, ObservedExited, nil), StallActionRespawn},
		{"row8 crashed", ruleCase(StopReasonOrchestratorError, ObservedCrashed, nil), StallActionRespawn},
		{"row8 idle", ruleCase(StopReasonOrchestratorError, ObservedIdle, nil), StallActionRespawn},
		{"row8 unknown", ruleCase(StopReasonOrchestratorError, ObservedUnknown, nil), StallActionRespawn},
		{"legacy empty reason is a stall and nudges when waiting", ruleCase("", ObservedIdlePrompt, nil), StallActionNudge},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DecideStallAction(tc.in); got != tc.want {
				t.Errorf("DecideStallAction(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDecideStallAction_RowsOneToThreeForEveryState(t *testing.T) {
	t.Parallel()

	for _, state := range AllObservedStates {
		for _, r := range []StopReason{StopReasonDone, StopReasonCancelledExternal, StopReasonCancelledShutdown} {
			if got := DecideStallAction(ruleCase(r, state, nil)); got != StallActionNone {
				t.Errorf("reason %q state %q = %q, want none", r, state, got)
			}
		}
		for _, mut := range []func(*StallDecisionInput){
			func(i *StallDecisionInput) { i.Archived = true },
			func(i *StallDecisionInput) { i.RetryPending = true },
		} {
			if got := DecideStallAction(ruleCase(StopReasonOrchestratorError, state, mut)); got != StallActionNone {
				t.Errorf("archived/retry-pending state %q = %q, want none", state, got)
			}
		}
	}
}

// expectedAction is an independent oracle written as plain boolean algebra
// (the ADR-001 invariants), not as the first-match switch under test.
func expectedAction(in StallDecisionInput) StallAction {
	stall := in.Reason.Class() == OutcomeClassStall
	budgetExempt := in.Reason == StopReasonMaxTurns || in.Reason == StopReasonPanic
	switch {
	case !stall || in.Archived || in.RetryPending:
		return StallActionNone
	case !budgetExempt && in.ItemInWorkStatus && in.State == ObservedIdlePrompt && in.NudgeEnabled && in.NudgeAllowed:
		return StallActionNudge
	case !budgetExempt && in.ItemInWorkStatus && in.State == ObservedWorking && in.ResuperviseAllowed:
		return StallActionResupervise
	default:
		return StallActionRespawn
	}
}

func TestDecideStallAction_ExhaustiveInvariants(t *testing.T) {
	t.Parallel()

	reasons := append([]StopReason{""}, AllStopReasons...)
	const boolBits = 6
	combos := 0

	for _, reason := range reasons {
		for _, state := range AllObservedStates {
			for mask := 0; mask < 1<<boolBits; mask++ {
				combos++
				in := StallDecisionInput{
					Reason:             reason,
					State:              state,
					NudgeEnabled:       mask&1 != 0,
					NudgeAllowed:       mask&2 != 0,
					ResuperviseAllowed: mask&4 != 0,
					ItemInWorkStatus:   mask&8 != 0,
					Archived:           mask&16 != 0,
					RetryPending:       mask&32 != 0,
				}
				got := DecideStallAction(in)

				if want := expectedAction(in); got != want {
					t.Fatalf("DecideStallAction(%+v) = %q, oracle says %q", in, got, want)
				}
				if got == StallActionNudge && state != ObservedIdlePrompt {
					t.Fatalf("nudge outside idle_prompt: %+v", in)
				}
				if reason.Class() != OutcomeClassStall && got != StallActionNone {
					t.Fatalf("non-stall reason produced %q: %+v", got, in)
				}
				if reason.Class() == OutcomeClassStall && !in.Archived && !in.RetryPending && got == StallActionNone {
					t.Fatalf("recoverable stall produced none: %+v", in)
				}
				if (reason == StopReasonMaxTurns || reason == StopReasonPanic) &&
					(got == StallActionNudge || got == StallActionResupervise) {
					t.Fatalf("budget-exempt reason %q produced %q", reason, got)
				}
			}
		}
	}

	if want := len(reasons) * len(AllObservedStates) * (1 << boolBits); combos != want {
		t.Fatalf("covered %d combinations, want %d", combos, want)
	}
}

func TestDecideStallAction_NudgeNeverWhenNudgeDisabled(t *testing.T) {
	t.Parallel()

	in := ruleCase(StopReasonOrchestratorError, ObservedIdlePrompt, func(i *StallDecisionInput) {
		i.NudgeEnabled = false
		i.NudgeAllowed = true
	})
	if got := DecideStallAction(in); got != StallActionRespawn {
		t.Errorf("DecideStallAction(%+v) = %q, want respawn", in, got)
	}
}

func TestEnumSlices_NoDuplicatesAndNonEmpty(t *testing.T) {
	t.Parallel()

	check := func(name string, values []string) {
		seen := map[string]bool{}
		for _, v := range values {
			if v == "" {
				t.Errorf("%s contains an empty value", name)
			}
			if seen[v] {
				t.Errorf("%s lists %q twice", name, v)
			}
			seen[v] = true
		}
	}
	var states, buckets, actions []string
	for _, s := range AllObservedStates {
		states = append(states, s.String())
	}
	for _, b := range AllStatusBuckets {
		buckets = append(buckets, string(b))
	}
	for _, a := range AllStallActions {
		actions = append(actions, a.String())
	}
	check("AllObservedStates", states)
	check("AllStatusBuckets", buckets)
	check("AllStallActions", actions)
}
