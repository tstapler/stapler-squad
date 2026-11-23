package itemlink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestComputeBackgroundOutcome(t *testing.T) {
	t.Parallel()
	verdict := &backlogdto.ReviewVerdictSummary{OverallOutcome: "PASS"}
	tests := []struct {
		name   string
		status domain.Status
		is     *backlogdto.ItemSessionSummary
		want   BackgroundOutcome
	}{
		{"ComputeBackgroundOutcome_should_returnCompleted_When_reviewVerdictRecorded", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview, ReviewVerdict: verdict}, BackgroundOutcomeCompleted},
		{"completed_When_triageResultRecorded", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleTriage, TriageResult: `{"summary":"x"}`}, BackgroundOutcomeCompleted},
		{"ComputeBackgroundOutcome_should_returnFailed_When_statusCrashed", domain.Crashed,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview}, BackgroundOutcomeFailed},
		{"failed_When_crashedAndNoItemSession", domain.Crashed, nil, BackgroundOutcomeFailed},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_endReasonShutdown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview, EndReason: "shutdown"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_diagnoseRole", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleDiagnose}, BackgroundOutcomeUnknown},
		{"unknown_When_diagnoseRoleEvenWithFailureReason", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleDiagnose, EndReason: "timeout"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_noItemSession", domain.Stopped, nil, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_treatTimeoutAsFailed", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview, EndReason: "timeout"}, BackgroundOutcomeFailed},
		{"_JulesCompletedAsUnknown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleJulesWork, EndReason: "jules_completed"}, BackgroundOutcomeUnknown},
		{"_UnlistedReasonAsUnknown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview, EndReason: "some_future_reason"}, BackgroundOutcomeUnknown},
		{"_VerdictBeatsFailureReason", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: instance.SessionRoleReview, ReviewVerdict: verdict, EndReason: "timeout"}, BackgroundOutcomeCompleted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ComputeBackgroundOutcome(tc.status, tc.is))
		})
	}
}
