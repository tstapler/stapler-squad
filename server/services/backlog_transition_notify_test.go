package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// enforceEveryClass is a class-aware mode source that enforces every class.
type enforceEveryClass struct{}

func (enforceEveryClass) Mode() session.EngineMode                       { return session.ModeEnforce }
func (enforceEveryClass) ModeFor(session.CallerClass) session.EngineMode { return session.ModeEnforce }

func TestTransitionDeniedReporter_should_RecordOnceAndReportDistinctCallersSeparately_When_GateDeniesAgentAndSystem(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, enforceEveryClass{})
	require.NoError(t, svc.storage.SetTransitionGate(sel, svc.TransitionDeniedReporter()))
	bus := events.NewEventBus(16)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	ctx := context.Background()

	item, err := svc.storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "denied", Status: string(session.BacklogStatusReady)})
	require.NoError(t, err)
	denied := func(caller session.TransitionCaller) {
		t.Helper()
		_, gateErr := svc.storage.GatedTransition(ctx, session.TransitionRequest{Caller: caller, ItemID: item.ID, To: session.BacklogStatusReview}) // ready->review has no edge
		require.ErrorIs(t, gateErr, session.ErrTransitionNotAllowed)
	}
	note := func(caller session.TransitionCaller) string {
		return fmt.Sprintf("automatic %s denied: from=ready to=review reason=config_graph", caller)
	}

	denied(session.CallerMCPResumeWork)
	rows := transitionDeniedRows(t, svc, item.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, "mcp_resume_work", rows[0].DeniedCaller)
	assert.Equal(t, "ready", rows[0].DeniedFrom)
	assert.Equal(t, "review", rows[0].DeniedTo)
	assert.Equal(t, "config_graph", rows[0].DeniedReason)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(session.CallerMCPResumeWork)))
	assert.Equal(t, 1, countDeniedNotifications(ch))

	denied(session.CallerMCPResumeWork)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(session.CallerMCPResumeWork)), "an identical denial adds no comment")
	assert.Equal(t, 0, countDeniedNotifications(ch), "an identical denial adds no notification")

	denied(session.CallerAttachSessionToItem)
	assert.Equal(t, 1, countDeniedNotes(t, svc, item.ID, note(session.CallerAttachSessionToItem)), "another caller on the same edge reports separately")
	assert.Equal(t, 1, countDeniedNotifications(ch))

	_, err = svc.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerOperatorTransitionRPC, ItemID: item.ID, To: session.BacklogStatusReview})
	require.ErrorIs(t, err, session.ErrTransitionNotAllowed)
	assert.Equal(t, 0, countDeniedNotes(t, svc, item.ID, note(session.CallerOperatorTransitionRPC)), "an operator denial returns the error only")
	assert.Equal(t, 0, countDeniedNotifications(ch))
}
