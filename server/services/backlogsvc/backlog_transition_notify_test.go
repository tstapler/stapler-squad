package backlogsvc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// enforceEveryClass is a class-aware mode source that enforces every class.
type enforceEveryClass struct{}

func (enforceEveryClass) Mode() session.EngineMode                      { return session.ModeEnforce }
func (enforceEveryClass) ModeFor(domain.CallerClass) session.EngineMode { return session.ModeEnforce }

func TestTransitionDeniedReporter_should_RecordOnceAndReportDistinctCallersSeparately_When_GateDeniesAgentAndSystem(t *testing.T) {
	svc := NewBacklogService(svctest.CreateTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, enforceEveryClass{})
	require.NoError(t, svc.storage.SetTransitionGate(sel, svc.TransitionDeniedReporter()))
	bus := events.NewEventBus(16)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	ctx := context.Background()

	item, err := svc.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "denied", Status: string(domain.BacklogStatusReady)})
	require.NoError(t, err)
	denied := func(caller domain.TransitionCaller) {
		t.Helper()
		_, gateErr := svc.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: caller, ItemID: item.ID, To: domain.BacklogStatusReview}) // ready->review has no edge
		require.ErrorIs(t, gateErr, session.ErrTransitionNotAllowed)
	}
	note := func(caller domain.TransitionCaller) string {
		return fmt.Sprintf("automatic %s denied: from=ready to=review reason=config_graph", caller)
	}

	denied(domain.CallerMCPResumeWork)
	rows := transitionDeniedRows(t, svc, item.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "mcp_resume_work", rows[0].DeniedCaller)
	assert.Equal(t, "ready", rows[0].DeniedFrom)
	assert.Equal(t, "review", rows[0].DeniedTo)
	assert.Equal(t, "config_graph", rows[0].DeniedReason)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(domain.CallerMCPResumeWork)))
	assert.Equal(t, 1, countDeniedNotifications(ch))

	denied(domain.CallerMCPResumeWork)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(domain.CallerMCPResumeWork)), "an identical denial adds no comment")
	assert.Equal(t, 0, countDeniedNotifications(ch), "an identical denial adds no notification")

	denied(domain.CallerAttachSessionToItem)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(domain.CallerAttachSessionToItem)), "another caller on the same edge reports separately")
	assert.Equal(t, 1, countDeniedNotifications(ch))

	_, err = svc.storage.GatedTransition(ctx, backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusReview})
	require.ErrorIs(t, err, session.ErrTransitionNotAllowed)
	assert.Equal(t, 0, countDeniedNotes(t, svc, item.ID, note(domain.CallerOperatorTransitionRPC)), "an operator denial returns the error only")
	assert.Equal(t, 0, countDeniedNotifications(ch))
}
