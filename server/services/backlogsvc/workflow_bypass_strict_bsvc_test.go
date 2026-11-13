package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestWorkflowBypassStrict_GuardedTransitionCallerReturnsRegisteredCallers(t *testing.T) {
	statuses := []domain.BacklogStatus{
		domain.BacklogStatusIdea, domain.BacklogStatusRefining, domain.BacklogStatusReady, domain.BacklogStatusQueued,
		domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusDone,
		domain.BacklogStatusArchived,
	}
	for _, to := range statuses {
		for _, actor := range []string{session.TriggeredByUser, session.TriggeredByAgent, session.TriggeredBySystem, ""} {
			_, ok := domain.LookupTransitionCaller(guardedTransitionCaller(to, actor))
			assert.True(t, ok, "to=%s actor=%q", to, actor)
		}
	}
}
