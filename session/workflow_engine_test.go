package session

import "testing"

func TestGuardedTransitionAllowed(t *testing.T) {
	t.Parallel()
	engine := NewDefaultWorkflowEngine()

	tests := []struct {
		name string
		item BacklogItemTransitionInput
		to   BacklogStatus
		want bool
	}{
		{
			name: "idea to archived is a valid edge with no gate",
			item: BacklogItemTransitionInput{Status: BacklogStatusIdea},
			to:   BacklogStatusArchived,
			want: true,
		},
		{
			name: "in_progress to archived has no such edge",
			item: BacklogItemTransitionInput{Status: BacklogStatusInProgress},
			to:   BacklogStatusArchived,
			want: false,
		},
		{
			name: "review to done with a passing verdict",
			item: BacklogItemTransitionInput{
				Status:         BacklogStatusReview,
				OverallOutcome: ReviewOutcomePass,
			},
			to:   BacklogStatusDone,
			want: true,
		},
		{
			name: "review to done with no verdict fails ErrVerdictRequired gate",
			item: BacklogItemTransitionInput{
				Status: BacklogStatusReview,
			},
			to:   BacklogStatusDone,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GuardedTransitionAllowed(engine, tt.item, tt.to)
			if got != tt.want {
				t.Errorf("GuardedTransitionAllowed(%+v, %v) = %v, want %v", tt.item, tt.to, got, tt.want)
			}
		})
	}
}

// TestDefaultWorkflowEngine_PendingGates_should_TranslateTransitionGuard_When_EdgeHasAGuard
// covers ADR-002's requirement that DefaultWorkflowEngine.PendingGates express
// every TransitionGuard branch as an equivalent GateStatus: a guarded edge
// (review->done with no verdict) must surface exactly one GateKindStructural
// entry with Satisfied: false, and an unguarded edge (idea->archived) must
// surface no entries at all.
func TestDefaultWorkflowEngine_PendingGates_should_TranslateTransitionGuard_When_EdgeHasAGuard(t *testing.T) {
	t.Parallel()
	engine := NewDefaultWorkflowEngine()

	statuses, err := engine.PendingGates(BacklogItemTransitionInput{Status: BacklogStatusReview}, BacklogStatusDone)
	if err != nil {
		t.Fatalf("PendingGates returned error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected exactly 1 GateStatus for review->done with no verdict, got %d", len(statuses))
	}
	if statuses[0].Satisfied {
		t.Errorf("expected review->done with no verdict to be unsatisfied")
	}
	if statuses[0].Kind != GateKindStructural {
		t.Errorf("expected GateKindStructural, got %v", statuses[0].Kind)
	}

	statuses, err = engine.PendingGates(BacklogItemTransitionInput{Status: BacklogStatusIdea}, BacklogStatusArchived)
	if err != nil {
		t.Fatalf("PendingGates returned error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected no gates for idea->archived (no TransitionGuard branch), got %d", len(statuses))
	}

	// review->done WITH a passing verdict must report Satisfied: true, and
	// ValidateGates (the thin wrapper) must agree.
	passingItem := BacklogItemTransitionInput{Status: BacklogStatusReview, OverallOutcome: ReviewOutcomePass}
	statuses, err = engine.PendingGates(passingItem, BacklogStatusDone)
	if err != nil {
		t.Fatalf("PendingGates returned error: %v", err)
	}
	if len(statuses) != 1 || !statuses[0].Satisfied {
		t.Fatalf("expected review->done with a passing verdict to be satisfied, got %+v", statuses)
	}
	if err := engine.ValidateGates(passingItem, BacklogStatusDone); err != nil {
		t.Errorf("ValidateGates should agree with PendingGates and return nil, got %v", err)
	}
}
