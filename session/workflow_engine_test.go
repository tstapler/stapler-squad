package session

import "testing"

func TestGuardedTransitionAllowed(t *testing.T) {
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
			got := GuardedTransitionAllowed(engine, tt.item, tt.to)
			if got != tt.want {
				t.Errorf("GuardedTransitionAllowed(%+v, %v) = %v, want %v", tt.item, tt.to, got, tt.want)
			}
		})
	}
}
