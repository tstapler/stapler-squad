package instance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

// TestCanTransition_ExhaustiveMatrix verifies every pair of known statuses against
// the ground-truth fixturetest.ValidTransitionSet. This is the single source of truth: adding
// a new transition to transitionDefs without also updating fixturetest.ValidTransitionSet
// (or vice-versa) will cause this test to fail.
func TestCanTransition_ExhaustiveMatrix(t *testing.T) {
	t.Parallel()
	for _, from := range fixturetest.AllStatuses {
		for _, to := range fixturetest.AllStatuses {
			pair := [2]domain.Status{from, to}
			wantValid := fixturetest.ValidTransitionSet[pair]
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
	t.Parallel()
	tests := []struct {
		name string
		from domain.Status
		to   domain.Status
	}{
		// Creating transitions
		{"Creating -> Active", domain.Creating, domain.Active},
		{"Creating -> Stopped", domain.Creating, domain.Stopped},
		// Active transitions
		{"Active -> Paused", domain.Active, domain.Paused},
		{"Active -> Stopped", domain.Active, domain.Stopped},
		{"Active -> Hibernated", domain.Active, domain.Hibernated},
		// Paused transitions
		{"Paused -> Active", domain.Paused, domain.Active},
		{"Paused -> Stopped", domain.Paused, domain.Stopped},
		// Stopped recovery
		{"Stopped -> Active", domain.Stopped, domain.Active},
		// Hibernated transitions
		{"Hibernated -> Active", domain.Hibernated, domain.Active},
		{"Hibernated -> Stopped", domain.Hibernated, domain.Stopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name string
		from domain.Status
		to   domain.Status
	}{
		// Stopped allows only Active (recovery)
		{"Stopped -> Paused", domain.Stopped, domain.Paused},
		{"Stopped -> Hibernated", domain.Stopped, domain.Hibernated},
		{"Stopped -> Creating", domain.Stopped, domain.Creating},
		// Self-transitions are not allowed
		{"Active -> Active", domain.Active, domain.Active},
		{"Paused -> Paused", domain.Paused, domain.Paused},
		{"Stopped -> Stopped", domain.Stopped, domain.Stopped},
		{"Hibernated -> Hibernated", domain.Hibernated, domain.Hibernated},
		{"Creating -> Creating", domain.Creating, domain.Creating},
		// Creating cannot go to Paused or Hibernated
		{"Creating -> Paused", domain.Creating, domain.Paused},
		{"Creating -> Hibernated", domain.Creating, domain.Hibernated},
		// Paused cannot hibernate directly (must go Active first)
		{"Paused -> Hibernated", domain.Paused, domain.Hibernated},
		// Hibernated cannot go back to Creating
		{"Hibernated -> Creating", domain.Hibernated, domain.Creating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if CanTransition(tt.from, tt.to) {
				t.Errorf("CanTransition(%s, %s) = true, want false", tt.from, tt.to)
			}
		})
	}
}

func TestCanTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
	unknownStatus := domain.Status(999)
	if CanTransition(unknownStatus, domain.Active) {
		t.Error("CanTransition with unknown from status should return false")
	}
	if CanTransition(domain.Active, unknownStatus) {
		t.Error("CanTransition with unknown to status should return false")
	}
}

func TestErrInvalidTransition(t *testing.T) {
	t.Parallel()
	err := domain.ErrInvalidTransition{From: domain.Paused, To: domain.Hibernated}

	expected := "invalid transition: Paused -> Hibernated"
	if err.Error() != expected {
		t.Errorf("ErrInvalidTransition.Error() = %q, want %q", err.Error(), expected)
	}

	var target domain.ErrInvalidTransition
	if !errors.As(err, &target) {
		t.Error("errors.As should match ErrInvalidTransition")
	}
	if target.From != domain.Paused || target.To != domain.Hibernated {
		t.Errorf("errors.As target = {%s, %s}, want {Paused, Hibernated}", target.From, target.To)
	}
}

// TestTransitionDefs_StoppedRecovery verifies Stopped can reach Active.
func TestTransitionDefs_StoppedRecovery(t *testing.T) {
	t.Parallel()
	if !CanTransition(domain.Stopped, domain.Active) {
		t.Error("Stopped should be able to transition to Active (session revival)")
	}
}

// TestTransitionDefs_AllStatusesCovered verifies every status has at least one outgoing transition.
func TestTransitionDefs_AllStatusesCovered(t *testing.T) {
	t.Parallel()
	for _, s := range fixturetest.AllStatuses {
		if s == domain.Stopped {
			continue // Stopped → Active is covered above
		}
		hasOut := false
		for _, to := range fixturetest.AllStatuses {
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
	t.Parallel()
	for _, s := range fixturetest.AllStatuses {
		if s == domain.Stopped {
			continue
		}
		if !CanTransition(s, domain.Stopped) {
			t.Errorf("State %s cannot transition directly to Stopped — all states must be stoppable", s)
		}
	}
}

// TestCanTransition_NewStates verifies Epic 1 acceptance criteria:
// - CanTransition(Active, Hibernated) returns true.
// - CanTransition(Hibernated, Active) returns true.
// - CanTransition(Active, Status(NeedsApprovalOldValue=4)) returns false (removed).
func TestCanTransition_NewStates(t *testing.T) {
	t.Parallel()
	if !CanTransition(domain.Active, domain.Hibernated) {
		t.Error("CanTransition(Active, Hibernated) = false, want true")
	}
	if !CanTransition(domain.Hibernated, domain.Active) {
		t.Error("CanTransition(Hibernated, Active) = false, want true")
	}
	// Old NeedsApproval value was 4; Hibernated is now 4. Transitioning
	// Active → Hibernated (4) must succeed, but Active → old NeedsApproval
	// semantics are gone. We verify via the named alias behavior.
	// Running and Ready are aliases for Active (value=1), so
	// CanTransition(Running, Ready) == CanTransition(Active, Active) == false.
	if CanTransition(domain.Active, domain.Active) {
		t.Error("CanTransition(Running, Ready) = true (self-transition), want false")
	}
}

// TestTransitionTo_ValidTransitions verifies that Instance.transitionTo updates
// Status and returns nil for every allowed transition.
func TestTransitionTo_ValidTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for pair := range fixturetest.ValidTransitionSet {
		from, to := pair[0], pair[1]
		t.Run(from.String()+"->"+to.String(), func(t *testing.T) {
			t.Parallel()
			// Path must be a real directory: Hibernated->Active performs a genuine
			// cold-restore Start() that now hard-fails on a missing workdir instead
			// of silently falling back to the process cwd (see ErrWorkDirMissing).
			inst := &Instance{Title: "test", Status: from, Path: t.TempDir()}
			t.Cleanup(func() {
				JoinHibernation(inst)
				JoinSessionDriver(inst)
			})
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

// TestTransitionTo_BumpsUpdatedAt verifies a real status transition advances
// Instance.UpdatedAt -- see touchUpdatedAt's doc comment for why this matters
// to the frontend.
func TestTransitionTo_BumpsUpdatedAt(t *testing.T) {
	ctx := context.Background()
	before := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "test", Status: domain.Active, Path: t.TempDir(), UpdatedAt: before}
	if err := inst.transitionTo(ctx, domain.Stopped); err != nil {
		t.Fatalf("transitionTo(Stopped): unexpected error %v", err)
	}
	if !inst.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt = %v, want a time after %v (transition must bump it)", inst.UpdatedAt, before)
	}
}

// TestTransitionToLocked_BumpsUpdatedAt covers the actor-mailbox path
// separately from TestTransitionTo_BumpsUpdatedAt: this is the function
// actually invoked by MarkExitedNormally/MarkCrashed/instanceOnExitCallback --
// the real trigger for the frontend-staleness bug touchUpdatedAt fixes -- so a
// regression here would slip past the transitionTo-only test above.
func TestTransitionToLocked_BumpsUpdatedAt(t *testing.T) {
	ctx := context.Background()
	before := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "test", Status: domain.Active, Path: t.TempDir(), UpdatedAt: before}
	s := &instanceState{inst: inst}
	if err := transitionToLocked(s, ctx, domain.Stopped); err != nil {
		t.Fatalf("transitionToLocked(Stopped): unexpected error %v", err)
	}
	if !inst.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt = %v, want a time after %v (transition must bump it)", inst.UpdatedAt, before)
	}
}

// TestForceStatus_BumpsUpdatedAt covers ForceStatus (used by health.go's
// markStartFailed), a second real bypass-the-state-machine path sharing the
// same touchUpdatedAt fix.
func TestForceStatus_BumpsUpdatedAt(t *testing.T) {
	before := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "test", Status: domain.Active, UpdatedAt: before}
	inst.ForceStatus(domain.Stopped)
	if !inst.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt = %v, want a time after %v (ForceStatus must bump it)", inst.UpdatedAt, before)
	}
}

// TestRecoverFromStopped_BumpsUpdatedAt covers RecoverFromStopped, the third
// bypass-the-state-machine path sharing the same touchUpdatedAt fix.
func TestRecoverFromStopped_BumpsUpdatedAt(t *testing.T) {
	before := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "test", Status: domain.Stopped, UpdatedAt: before}
	inst.RecoverFromStopped()
	if !inst.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt = %v, want a time after %v (RecoverFromStopped must bump it)", inst.UpdatedAt, before)
	}
}

// TestTransitionTo_InvalidTransitions verifies that Instance.transitionTo returns
// ErrInvalidTransition and leaves Status unchanged for every disallowed transition.
func TestTransitionTo_InvalidTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, from := range fixturetest.AllStatuses {
		for _, to := range fixturetest.AllStatuses {
			if fixturetest.ValidTransitionSet[[2]domain.Status{from, to}] {
				continue // skip valid pairs
			}
			from, to := from, to // capture
			t.Run(from.String()+"->"+to.String(), func(t *testing.T) {
				t.Parallel()
				inst := &Instance{Title: "test", Status: from}
				err := inst.transitionTo(ctx, to)
				if err == nil {
					t.Errorf("transitionTo(%s) from %s: expected error, got nil", to, from)
					return
				}
				var te domain.ErrInvalidTransition
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
//
// This is the original reproduction case for the Instance.Status data race
// between transitionTo and the (now-removed) untracked hibernate/resume
// After-hook goroutines -- `go test -race ./session -run 'TestTransitionTo_'
// -count=50` must stay race-free.
func TestTransitionTo_ChainedTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	type step struct{ to domain.Status }

	chains := []struct {
		name  string
		start domain.Status
		steps []step
	}{
		{
			name:  "new session lifecycle",
			start: domain.Creating,
			steps: []step{{domain.Active}, {domain.Paused}, {domain.Active}, {domain.Stopped}},
		},
		{
			name:  "hibernate and resume",
			start: domain.Active,
			steps: []step{{domain.Hibernated}, {domain.Active}, {domain.Stopped}},
		},
		{
			name:  "stop and revive",
			start: domain.Active,
			steps: []step{{domain.Stopped}, {domain.Active}, {domain.Paused}, {domain.Stopped}},
		},
	}

	for _, chain := range chains {
		t.Run(chain.name, func(t *testing.T) {
			t.Parallel()
			// Path must be a real directory: Hibernated->Active performs a genuine
			// cold-restore Start() that now hard-fails on a missing workdir instead
			// of silently falling back to the process cwd (see ErrWorkDirMissing).
			inst := &Instance{Title: "test-chain", Status: chain.start, Path: t.TempDir()}
			t.Cleanup(func() {
				JoinHibernation(inst)
				JoinSessionDriver(inst)
			})
			for i, step := range chain.steps {
				if err := inst.transitionTo(ctx, step.to); err != nil {
					t.Fatalf("step %d: transitionTo(%s) from %s: %v", i, step.to, inst.Status, err)
				}
			}
		})
	}
}
