package services

// Story 1.5.6: system-class writers in server/services (and the two operator
// triage writers) go through the transition gate. Each test wires a real
// ConfiguredWorkflowEngine under Enforce for every class and adds an operator
// gate or disables an edge, then drives the production function.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// systemGateHarness is the storage's gate, backed by a Configured engine whose
// stage config a test can edit.
type systemGateHarness struct {
	repo   session.StageCRUDRepository
	engine *session.ConfiguredWorkflowEngine
}

// wireEnforcedGate injects a gate that enforces every caller class on storage,
// reporting denials through svc.
func wireEnforcedGate(t *testing.T, svc *BacklogService) *systemGateHarness {
	t.Helper()
	ctx := context.Background()
	client := svc.storage.GetEntClient()
	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, client))
	repo := session.NewEntStageConfigRepository(client)
	engine, err := session.NewConfiguredWorkflowEngine(repo, session.NewEntGateSatisfactionRepository(client), nil)
	require.NoError(t, err)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), engine, enforceEveryClass{})
	require.NoError(t, svc.storage.SetTransitionGate(sel, svc.TransitionDeniedReporter()))
	return &systemGateHarness{repo: repo, engine: engine}
}

func (h *systemGateHarness) transitionID(t *testing.T, from, to session.BacklogStatus) *session.TransitionData {
	t.Helper()
	ts, err := h.repo.ListAllTransitions(context.Background(), string(from))
	require.NoError(t, err)
	for _, tr := range ts {
		if tr.ToStageSlug == string(to) {
			return tr
		}
	}
	require.FailNowf(t, "no transition", "%s->%s is not seeded", from, to)
	return nil
}

// blockWithGate adds an unsatisfiable human_approval gate to from->to and
// returns the function that removes it.
func (h *systemGateHarness) blockWithGate(t *testing.T, from, to session.BacklogStatus) (remove func()) {
	t.Helper()
	ctx := context.Background()
	gate, err := h.repo.CreateGate(ctx, session.GateCreateInput{
		TransitionID: h.transitionID(t, from, to).ID,
		Kind:         string(session.GateKindHumanApproval),
		Enabled:      true,
	})
	require.NoError(t, err)
	require.NoError(t, h.engine.InvalidateCache(ctx))
	return func() {
		require.NoError(t, h.repo.DeleteGate(ctx, gate.ID))
		require.NoError(t, h.engine.InvalidateCache(ctx))
	}
}

func systemItemAt(t *testing.T, svc *BacklogService, status session.BacklogStatus) *session.BacklogItemData {
	t.Helper()
	item, err := svc.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "gate item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func deniedCallers(t *testing.T, svc *BacklogService, itemID string) []string {
	t.Helper()
	rows := transitionDeniedRows(t, svc, itemID)
	callers := make([]string, 0, len(rows))
	for _, row := range rows {
		callers = append(callers, row.DeniedCaller)
	}
	return callers
}

func TestGuardedTransitionCaller_should_NameQueueClaimAndJulesDispatch_When_GivenTheirTargetAndActor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		to          session.BacklogStatus
		triggeredBy string
		want        session.TransitionCaller
	}{
		{"queue", session.BacklogStatusQueued, session.TriggeredByUser, session.CallerQueueBacklogItem},
		{"queue autonomous", session.BacklogStatusQueued, session.TriggeredBySystem, session.CallerQueueBacklogItem},
		{"dequeue claim", session.BacklogStatusInProgress, session.TriggeredBySystem, session.CallerClaimAndSpawn},
		{"jules dispatch", session.BacklogStatusInProgress, session.TriggeredByUser, session.CallerJulesDispatchStart},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, guardedTransitionCaller(tc.to, tc.triggeredBy), tc.name)
	}
}

func TestForceResetItem_should_ReturnDenialAndLeaveStatus_When_OperatorGateBlocksReviewToInProgress(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	h := wireEnforcedGate(t, svc)
	item := systemItemAt(t, svc, session.BacklogStatusReview)
	remove := h.blockWithGate(t, session.BacklogStatusReview, session.BacklogStatusInProgress)

	_, err := svc.forceResetItem(context.Background(), item, session.TriggeredByUser)

	require.ErrorIs(t, err, session.ErrGateNotSatisfied)
	assert.Equal(t, "review", itemStatus(t, svc, item.ID))
	assert.Empty(t, transitionDeniedRows(t, svc, item.ID), "an operator denial returns the error; it raises no stuck row")

	remove()
	updated, err := svc.forceResetItem(context.Background(), item, session.TriggeredByUser)
	require.NoError(t, err)
	assert.Equal(t, "in_progress", updated.Status)
}

func TestForceResetItem_should_ReDecideAgainstTheTrueStatus_When_ItemLeftReviewAfterTheRead(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	wireEnforcedGate(t, svc)
	stale := systemItemAt(t, svc, session.BacklogStatusReview)
	_, err := svc.storage.RawArchiveBacklogItem(context.Background(), stale.ID, nil, session.TriggeredByUser, "")
	require.NoError(t, err)

	_, err = svc.forceResetItem(context.Background(), stale, session.TriggeredByUser)

	require.ErrorIs(t, err, session.ErrTransitionNotAllowed, "the unconditional retry re-reads archived and finds no archived->in_progress edge")
	assert.Equal(t, "archived", itemStatus(t, svc, stale.ID))
}

func TestMaybeTriggerTriage_should_ReportOnceUnderItsOwnCallerAndStartNoTriage_When_GateDeniesTheSkipRuleAdvance(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	svc.SetHeadlessPool(&fakeHeadlessPool{})
	h := wireEnforcedGate(t, svc)
	h.blockWithGate(t, session.BacklogStatusIdea, session.BacklogStatusReady)
	item, err := svc.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title: "has a plan already", Status: string(session.BacklogStatusIdea), RepoPath: t.TempDir(), PlanApproved: true,
	})
	require.NoError(t, err)

	for range 2 {
		assert.False(t, svc.MaybeTriggerTriage(context.Background(), item.ID, false, item.RepoPath))
	}

	require.NotEmpty(t, newItemTriageSkipReason(item), "the fixture must hit a skip rule")
	assert.Equal(t, "idea", itemStatus(t, svc, item.ID))
	assert.Equal(t, []string{string(session.CallerMaybeTriggerTriage)}, deniedCallers(t, svc, item.ID), "one row for the repeated denial")
}

func TestAttachSessionToItem_should_ReportDenialAndKeepStatus_When_SystemGateBlocksReadyToInProgress(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	h := wireEnforcedGate(t, svc)
	h.blockWithGate(t, session.BacklogStatusReady, session.BacklogStatusInProgress)
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "attach behind a gate",
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true,
		SkipPlanning:       true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	_, err = svc.storage.RawTransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusReady, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	_, err = svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{ItemId: itemID, SessionUuid: "attach-behind-gate"}))

	require.NoError(t, err, "the attach itself succeeds; only the follow-on transition is denied")
	assert.Equal(t, "ready", itemStatus(t, svc, itemID))
	assert.Equal(t, []string{string(session.CallerAttachSessionToItem)}, deniedCallers(t, svc, itemID))
}

func TestRollbackFailedReopen_should_RestoreReviewEvenWhenAGateBlocksInProgressToReview(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	h := wireEnforcedGate(t, svc)
	item := systemItemAt(t, svc, session.BacklogStatusReview)
	inProgress, err := svc.storage.GatedTransition(context.Background(), session.TransitionRequest{
		Caller: session.CallerAutoReopenFailedReview, ItemID: item.ID, To: session.BacklogStatusInProgress, TriggeredBy: session.TriggeredBySystem,
	})
	require.NoError(t, err)
	h.blockWithGate(t, session.BacklogStatusInProgress, session.BacklogStatusReview)
	before := session.EngineBypassCount(session.CallerRollback)

	svc.rollbackFailedReopen(context.Background(), item, inProgress, errors.New("spawn failed"))

	assert.Equal(t, "review", itemStatus(t, svc, item.ID), "a rollback is never engine-denied")
	assert.Equal(t, before+1, session.EngineBypassCount(session.CallerRollback))
}

func TestRollbackFailedPRFixReopen_should_RestorePRPendingAndNotOverwriteAConcurrentMover(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	wireEnforcedGate(t, svc)
	item := systemItemAt(t, svc, session.BacklogStatusPRPending)
	_, err := svc.storage.GatedTransition(context.Background(), session.TransitionRequest{
		Caller: session.CallerAutoReopenForPRFix, ItemID: item.ID, To: session.BacklogStatusInProgress, TriggeredBy: session.TriggeredBySystem,
	})
	require.NoError(t, err)

	svc.rollbackFailedPRFixReopen(context.Background(), item, errors.New("spawn failed"))
	assert.Equal(t, "pr_pending", itemStatus(t, svc, item.ID))

	_, err = svc.storage.RawTransitionBacklogItemStatus(context.Background(), item.ID, session.BacklogStatusDone, nil, session.TriggeredBySystem)
	require.NoError(t, err)
	svc.rollbackFailedPRFixReopen(context.Background(), item, errors.New("spawn failed"))
	assert.Equal(t, "done", itemStatus(t, svc, item.ID), "the status-only CAS keeps the rollback off an item that moved on")
}

func TestDebugMutateHandler_should_WriteAnyStatusAndCountDebug_When_EnforceAndGraphWouldDeny(t *testing.T) {
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	wireEnforcedGate(t, svc)
	handler := NewBacklogDebugMutateHandler(svc.storage)
	item := systemItemAt(t, svc, session.BacklogStatusIdea)
	before := session.EngineBypassCount(session.CallerDebug)

	post := func(fn http.HandlerFunc, body string) int {
		rec := httptest.NewRecorder()
		fn(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		return rec.Code
	}
	assert.Equal(t, http.StatusNoContent, post(handler.handleTransition, `{"itemId":"`+item.ID+`","targetStatus":"done"}`), "idea->done is not an edge")
	assert.Equal(t, "done", itemStatus(t, svc, item.ID))
	assert.Equal(t, http.StatusNoContent, post(handler.handleArchive, `{"itemId":"`+item.ID+`"}`))
	assert.Equal(t, "archived", itemStatus(t, svc, item.ID))
	assert.Equal(t, before+2, session.EngineBypassCount(session.CallerDebug))
	assert.Empty(t, transitionDeniedRows(t, svc, item.ID))
}
