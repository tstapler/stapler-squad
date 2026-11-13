package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestWorkflowBypassStrict_GuardedTransitionCallerReturnsRegisteredCallers(t *testing.T) {
	statuses := []session.BacklogStatus{
		session.BacklogStatusIdea, session.BacklogStatusRefining, session.BacklogStatusReady, session.BacklogStatusQueued,
		session.BacklogStatusInProgress, session.BacklogStatusReview, session.BacklogStatusPRPending, session.BacklogStatusDone,
		session.BacklogStatusArchived,
	}
	for _, to := range statuses {
		for _, actor := range []string{session.TriggeredByUser, session.TriggeredByAgent, session.TriggeredBySystem, ""} {
			_, ok := domain.LookupTransitionCaller(guardedTransitionCaller(to, actor))
			assert.True(t, ok, "to=%s actor=%q", to, actor)
		}
	}
}
