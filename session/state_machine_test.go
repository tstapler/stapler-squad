package session

import (
	"context"
	"errors"
	"testing"
)

// allStatuses lists every Status constant in the new 5-state model.
var allStatuses = []Status{Creating, Active, Paused, Stopped, Hibernated}

// validTransitionSet is the ground truth for TestCanTransition_ExhaustiveMatrix.
// Any pair not listed here must return false from CanTransition.
var validTransitionSet = map[[2]Status]bool{
	// Creating
	{Creating, Active}:   true,
	{Creating, Stopped}:  true,
	// Active
	{Active, Paused}:     true,
	{Active, Stopped}:    true,
	{Active, Hibernated}: true,
	// Paused
	{Paused, Active}:  true,
	{Paused, Stopped}: true,
	// Stopped — recoverable: Stopped → Active is allowed for session revival
	{Stopped, Active}: true,
	// Hibernated
	{Hibernated, Active}:  true,
	{Hibernated, Stopped}: true,
}

// TestCanTransition_ExhaustiveMatrix verifies every pair of known statuses against
// the ground-truth validTransitionSet. This is the single source of truth: adding
// a new transition to transitionDefs without also updating validTransitionSet
// (or vice-versa) will cause this test to fail.
func TestCanTransition_ExhaustiveMatrix(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			pair := [2]Status{from, to}
			wantValid := validTransitionSet[pair]
			got := CanTransition(from, to)
			if got != wantValid {
				if wantValid {
					t.Errorf("CanTransition(%s, %s) = false, want true", from, to)
				} else {
					t.Errorf("CanTransition(%s, %s) = true, want false (should be invalid)", from, to)
				}
			}
		}
	}
}

// TestCanTransition_ValidTransitions is a human-readable complement to the matrix
// test — it names each valid transition explicitly for documentation value.
func TestCanTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
	}{
		// Creating transitions
		{"Creating -> Active", Creating, Active},
		{"Creating -> Stopped", Creating, Stopped},
		// Active transitions
		{"Active -> Paused", Active, Paused},
		{"Active -> Stopped", Active, Stopped},
		{"Active -> Hibernated", Active, Hibernated},
		// Paused transitions
		{"Paused -> Active", Paused, Active},
		{"Paused -> Stopped", Paused, Stopped},
		// Stopped recovery
		{"Stopped -> Active", Stopped, Active},
		// Hibernated transitions
		{"Hibernated -> Active", Hibernated, Active},
		{"Hibernated -> Stopped", Hibernated, Stopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !CanTransition(tt.from, tt.to) {
				t.Errorf("CanTransition(%s, %s) = false, want true", tt.from, tt.to)
			}
		})
	}
}

// TestCanTransition_InvalidTransitions spot-checks transitions that must never be
// allowed. The exhaustive matrix test above already catches all of them, but having
// named cases here makes failures easier to diagnose.
func TestCanTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
	}{
		// Stopped allows only Active (recovery)
		{"Stopped -> Paused", Stopped, Paused},
		{"Stopped -> Hibernated", Stopped, Hibernated},
		{"Stopped -> Creating", Stopped, Creating},
		// Self-transitions are not allowed
		{"Active -> Active", Active, Active},
		{"Paused -> Paused", Paused, Paused},
		{"Stopped -> Stopped", Stopped, Stopped},
		{"Hibernated -> Hibernated", Hibernated, Hibernated},
		{"Creating -> Creating", Creating, Creating},
		// Creating cannot go to Paused or Hibernated
		{"Creating -> Paused", Creating, Paused},
		{"Creating -> Hibernated", Creating, Hibernated},
		// Paused cannot hibernate directly (must go Active first)
		{"Paused -> Hibernated", Paused, Hibernated},
		// Hibernated cannot go back to Creating
		{"Hibernated -> Creating", Hibernated, Creating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanTransition(tt.from, tt.to) {
				t.Errorf("CanTransition(%s, %s) = true, want false", tt.from, tt.to)
			}
		})
	}
}

func TestCanTransition_UnknownStatus(t *testing.T) {
	unknownStatus := Status(999)
	if CanTransition(unknownStatus, Active) {
		t.Error("CanTransition with unknown from status should return false")
	}
	if CanTransition(Active, unknownStatus) {
		t.Error("CanTransition with unknown to status should return false")
	}
}

func TestErrInvalidTransition(t *testing.T) {
	err := ErrInvalidTransition{From: Paused, To: Hibernated}

	expected := "invalid transition: Paused -> Hibernated"
	if err.Error() != expected {
		t.Errorf("ErrInvalidTransition.Error() = %q, want %q", err.Error(), expected)
	}

	var target ErrInvalidTransition
	if !errors.As(err, &target) {
		t.Error("errors.As should match ErrInvalidTransition")
	}
	if target.From != Paused || target.To != Hibernated {
		t.Errorf("errors.As target = {%s, %s}, want {Paused, Hibernated}", target.From, target.To)
	}
}

// TestTransitionDefs_StoppedRecovery verifies Stopped can reach Active.
func TestTransitionDefs_StoppedRecovery(t *testing.T) {
	if !CanTransition(Stopped, Active) {
		t.Error("Stopped should be able to transition to Active (session revival)")
	}
}

// TestTransitionDefs_AllStatusesCovered verifies every status has at least one outgoing transition.
func TestTransitionDefs_AllStatusesCovered(t *testing.T) {
	for _, s := range allStatuses {
		if s == Stopped {
			continue // Stopped → Active is covered above
		}
		hasOut := false
		for _, to := range allStatuses {
			if CanTransition(s, to) {
				hasOut = true
				break
			}
		}
		if !hasOut {
			t.Errorf("Status %s has no outgoing transitions", s)
		}
	}
}

// TestTransitionDefs_StoppedReachableFromEveryState verifies that every
// non-terminal state can reach Stopped in at most one hop.
func TestTransitionDefs_StoppedReachableFromEveryState(t *testing.T) {
	for _, s := range allStatuses {
		if s == Stopped {
			continue
		}
		if !CanTransition(s, Stopped) {
			t.Errorf("State %s cannot transition directly to Stopped — all states must be stoppable", s)
		}
	}
}

// TestCanTransition_NewStates verifies Epic 1 acceptance criteria:
// - CanTransition(Active, Hibernated) returns true.
// - CanTransition(Hibernated, Active) returns true.
// - CanTransition(Active, Status(NeedsApprovalOldValue=4)) returns false (removed).
func TestCanTransition_NewStates(t *testing.T) {
	if !CanTransition(Active, Hibernated) {
		t.Error("CanTransition(Active, Hibernated) = false, want true")
	}
	if !CanTransition(Hibernated, Active) {
		t.Error("CanTransition(Hibernated, Active) = false, want true")
	}
	// Old NeedsApproval value was 4; Hibernated is now 4. Transitioning
	// Active → Hibernated (4) must succeed, but Active → old NeedsApproval
	// semantics are gone. We verify via the named alias behavior.
	// Running and Ready are aliases for Active (value=1), so
	// CanTransition(Running, Ready) == CanTransition(Active, Active) == false.
	if CanTransition(Running, Ready) {
		t.Error("CanTransition(Running, Ready) = true (self-transition), want false")
	}
}

// TestTransitionTo_ValidTransitions verifies that Instance.transitionTo updates
// Status and returns nil for every allowed transition.
func TestTransitionTo_ValidTransitions(t *testing.T) {
	ctx := context.Background()
	for pair := range validTransitionSet {
		from, to := pair[0], pair[1]
		t.Run(from.String()+"->"+to.String(), func(t *testing.T) {
			inst := &Instance{Title: "test", Status: from}
			err := inst.transitionTo(ctx, to)
			if err != nil {
				t.Errorf("transitionTo(%s) from %s: unexpected error %v", to, from, err)
			}
			if inst.Status != to {
				t.Errorf("after transitionTo(%s): Status = %s, want %s", to, inst.Status, to)
			}
		})
	}
}

// TestTransitionTo_InvalidTransitions verifies that Instance.transitionTo returns
// ErrInvalidTransition and leaves Status unchanged for every disallowed transition.
func TestTransitionTo_InvalidTransitions(t *testing.T) {
	ctx := context.Background()
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			if validTransitionSet[[2]Status{from, to}] {
				continue // skip valid pairs
			}
			from, to := from, to // capture
			t.Run(from.String()+"->"+to.String(), func(t *testing.T) {
				inst := &Instance{Title: "test", Status: from}
				err := inst.transitionTo(ctx, to)
				if err == nil {
					t.Errorf("transitionTo(%s) from %s: expected error, got nil", to, from)
					return
				}
				var te ErrInvalidTransition
				if !errors.As(err, &te) {
					t.Errorf("transitionTo(%s) from %s: error is %T, want ErrInvalidTransition", to, from, err)
				}
				if inst.Status != from {
					t.Errorf("transitionTo(%s) from %s: Status changed to %s (must be unchanged on error)", to, from, inst.Status)
				}
			})
		}
	}
}

// TestTransitionTo_ChainedTransitions verifies common multi-hop paths through
// the state machine work as a sequence of transitionTo calls.
func TestTransitionTo_ChainedTransitions(t *testing.T) {
	ctx := context.Background()
	type step struct{ to Status }

	chains := []struct {
		name  string
		start Status
		steps []step
	}{
		{
			name:  "new session lifecycle",
			start: Creating,
			steps: []step{{Active}, {Paused}, {Active}, {Stopped}},
		},
		{
			name:  "hibernate and resume",
			start: Active,
			steps: []step{{Hibernated}, {Active}, {Stopped}},
		},
		{
			name:  "stop and revive",
			start: Active,
			steps: []step{{Stopped}, {Active}, {Paused}, {Stopped}},
		},
	}

	for _, chain := range chains {
		t.Run(chain.name, func(t *testing.T) {
			inst := &Instance{Title: "test-chain", Status: chain.start}
			for i, step := range chain.steps {
				if err := inst.transitionTo(ctx, step.to); err != nil {
					t.Fatalf("step %d: transitionTo(%s) from %s: %v", i, step.to, inst.Status, err)
				}
			}
		})
	}
}
