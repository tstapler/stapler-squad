package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// Story 1.5.5d: the operator RPC writers decide through the transition gate.

func gateEdge(t *testing.T, repo *session.EntStageConfigRepository, cfg *session.ConfiguredWorkflowEngine, from, to session.BacklogStatus) session.TransitionData {
	t.Helper()
	edges, err := repo.ListAllTransitions(t.Context(), string(from))
	require.NoError(t, err)
	for _, e := range edges {
		if e.ToStageSlug == string(to) {
			return *e
		}
	}
	require.Failf(t, "edge not seeded", "%s->%s", from, to)
	return session.TransitionData{}
}

// addApprovalGate puts an unsatisfied human_approval gate on a seeded edge.
func addApprovalGate(t *testing.T, repo *session.EntStageConfigRepository, cfg *session.ConfiguredWorkflowEngine, from, to session.BacklogStatus) {
	t.Helper()
	edge := gateEdge(t, repo, cfg, from, to)
	_, err := repo.CreateGate(t.Context(), session.GateCreateInput{
		TransitionID: edge.ID, Kind: string(session.GateKindHumanApproval), Config: map[string]interface{}{}, Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(t.Context()))
}

func storedStatus(t *testing.T, storage *session.Storage, id string) string {
	t.Helper()
	item, err := storage.GetBacklogItem(t.Context(), id)
	require.NoError(t, err)
	return item.Status
}

func newItemAt(t *testing.T, storage *session.Storage, status session.BacklogStatus) *session.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "gate item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func TestTransitionHandler_ResolvesCallerClassFromContextActor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  context.Context
		want session.TransitionCaller
		cls  session.CallerClass
	}{
		{"absent actor is the operator RPC", context.Background(), session.CallerOperatorTransitionRPC, session.ClassOperator},
		{"user actor is the operator RPC", WithTransitionActor(context.Background(), session.TriggeredByUser), session.CallerOperatorTransitionRPC, session.ClassOperator},
		{"agent actor is the MCP delegate", WithTransitionActor(context.Background(), session.TriggeredByAgent), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"unknown actor fails closed to the delegate", WithTransitionActor(context.Background(), "robot"), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"notification action rides the user actor", WithTransitionCaller(context.Background(), session.CallerOperatorNotificationAction), session.CallerOperatorNotificationAction, session.ClassOperator},
		{"a caller hint never lifts an agent", WithTransitionCaller(WithTransitionActor(context.Background(), session.TriggeredByAgent), session.CallerOperatorNotificationAction), session.CallerMCPTransitionDelegate, session.ClassAgent},
		{"only the notification caller is honored", WithTransitionCaller(context.Background(), session.CallerOperatorArchiveRPC), session.CallerOperatorTransitionRPC, session.ClassOperator},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveTransitionCaller(tc.ctx)
			assert.Equal(t, tc.want, got)
			spec, ok := session.LookupTransitionCaller(got)
			require.True(t, ok)
			assert.Equal(t, tc.cls, spec.Class)
		})
	}
}

func TestTransitionHandler_Enforce_GraphEditAndGateDenyWithUnchangedCodes(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	transition := func(c context.Context, id string, to session.BacklogStatus, override, expected string) error {
		_, err := svc.TransitionBacklogItemStatus(c, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId: id, TargetStatus: string(to), OverrideReason: override, ExpectedStatus: expected,
		}))
		return err
	}

	// A graph edit: idea->refining disabled.
	edge := gateEdge(t, repo, cfg, session.BacklogStatusIdea, session.BacklogStatusRefining)
	disabled := false
	_, err := repo.UpdateTransition(ctx, edge.ID, session.TransitionUpdateInput{Enabled: &disabled})
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
	item := newItemAt(t, storage, session.BacklogStatusIdea)
	err = transition(ctx, item.ID, session.BacklogStatusRefining, "", "")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "graph denial")
	assert.Equal(t, string(session.BacklogStatusIdea), storedStatus(t, storage, item.ID))

	// A Default guard sentinel keeps FailedPrecondition.
	noAC := newItemAt(t, storage, session.BacklogStatusIdea)
	err = transition(ctx, noAC.ID, session.BacklogStatusReady, "", "")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "guard denial")

	// A lost compare-and-swap keeps Aborted.
	err = transition(ctx, item.ID, session.BacklogStatusArchived, "", string(session.BacklogStatusReady))
	assert.Equal(t, connect.CodeAborted, connect.CodeOf(err), "stale expected_status")
}

func TestTransitionHandler_Enforce_OperatorOverrideBypassesGateAgentDoesNot(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	addApprovalGate(t, repo, cfg, session.BacklogStatusReady, session.BacklogStatusRefining)
	req := func(id, override string) *connect.Request[sessionv1.TransitionBacklogItemStatusRequest] {
		return connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: id, TargetStatus: string(session.BacklogStatusRefining), OverrideReason: override})
	}

	item := newItemAt(t, storage, session.BacklogStatusReady)
	_, err := svc.TransitionBacklogItemStatus(ctx, req(item.ID, ""))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "an unsatisfied operator gate denies")
	assert.Empty(t, transitionDeniedRows(t, svc, item.ID), "an operator denial returns the error only")

	_, err = svc.TransitionBacklogItemStatus(WithTransitionActor(ctx, session.TriggeredByAgent), req(item.ID, "agent insists"))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "an agent never bypasses a Configured gate")
	assert.Equal(t, string(session.BacklogStatusReady), storedStatus(t, storage, item.ID))
	rows := transitionDeniedRows(t, svc, item.ID)
	require.Len(t, rows, 1, "an agent denial reaches the reporter")
	assert.Equal(t, string(session.CallerMCPTransitionDelegate), rows[0].DeniedCaller)

	_, err = svc.TransitionBacklogItemStatus(ctx, req(item.ID, "owner says so"))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))
}

func TestTransitionHandler_Off_MatchesLegacyAnswers(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, session.NewDefaultWorkflowEngine(), nil, nil)
	require.False(t, storage.TransitionGateWired(), "no gate is injected, so the answer is Off")
	item := newItemAt(t, storage, session.BacklogStatusIdea)

	_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: item.ID, TargetStatus: string(session.BacklogStatusInProgress)}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "idea->in_progress is not an edge")
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: item.ID, TargetStatus: string(session.BacklogStatusRefining)}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))
}

func TestArchiveBacklogItem_Enforce_AllowsFromAnyStatusAndDeniesOnDoneGate(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	archive := func(id string) error {
		_, err := svc.ArchiveBacklogItem(ctx, connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{ItemId: id}))
		return err
	}

	for _, from := range []session.BacklogStatus{session.BacklogStatusInProgress, session.BacklogStatusReview, session.BacklogStatusPRPending} {
		item := newItemAt(t, storage, from)
		require.NoError(t, archive(item.ID), "archive from %s", from)
		assert.Equal(t, string(session.BacklogStatusArchived), storedStatus(t, storage, item.ID))
		require.NoError(t, archive(item.ID), "archived->archived is idempotently allowed")
	}

	addApprovalGate(t, repo, cfg, session.BacklogStatusDone, session.BacklogStatusArchived)
	done := newItemAt(t, storage, session.BacklogStatusDone)
	err := archive(done.ID)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, string(session.BacklogStatusDone), storedStatus(t, storage, done.ID), "a denied archive changes nothing")
	inProgress := newItemAt(t, storage, session.BacklogStatusInProgress)
	require.NoError(t, archive(inProgress.ID), "the gate on done->archived does not touch other statuses")
}

func TestUnarchiveBacklogItem_Enforce_DeniesNonArchivedItem(t *testing.T) {
	t.Parallel()
	svc, storage, _, _ := newEnforcedGateService(t)
	unarchive := func(id string) error {
		_, err := svc.UnarchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UnarchiveBacklogItemRequest{ItemId: id}))
		return err
	}
	archived := newItemAt(t, storage, session.BacklogStatusArchived)
	require.NoError(t, unarchive(archived.ID))
	assert.Equal(t, string(session.BacklogStatusIdea), storedStatus(t, storage, archived.ID))

	live := newItemAt(t, storage, session.BacklogStatusIdea)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(unarchive(live.ID)), "idea->idea is not an edge")
	assert.Equal(t, string(session.BacklogStatusIdea), storedStatus(t, storage, live.ID))
}

func TestSubmitManualReview_Enforce_GateOnReviewToDoneDeniesWithRecordedVerdict(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, session.BacklogStatusReview, session.BacklogStatusDone)
	item := newItemAt(t, storage, session.BacklogStatusReview)

	_, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId: item.ID, OverallOutcome: "PASS", Summary: "looks good",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, string(session.BacklogStatusReview), storedStatus(t, storage, item.ID), "the gate holds the item in review")
}

func TestSubmitManualReview_Enforce_PassGuardInputLetsUngatedReviewToDoneThrough(t *testing.T) {
	t.Parallel()
	svc, storage, _, _ := newEnforcedGateService(t)
	item := newItemAt(t, storage, session.BacklogStatusReview)

	resp, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId: item.ID, OverallOutcome: "PASS", Summary: "looks good",
	}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), resp.Msg.Item.Status, "the Default verdict guard sees the PASS the review just recorded")
}

func TestUpdateBacklogItem_Enforce_PRAssociationHonorsOperatorGate(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, session.BacklogStatusReview, session.BacklogStatusPRPending)
	item := newItemAt(t, storage, session.BacklogStatusReview)
	url, number := "https://github.com/acme/widgets/pull/7", int32(7)

	_, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: item.ID, PrUrl: &url, PrNumber: &number,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.ErrorContains(t, err, "transition gate not satisfied")
	assert.Equal(t, string(session.BacklogStatusReview), storedStatus(t, storage, item.ID))
}

func TestBacklogDecider_Transition_ReachesHandlerAsNotificationActionCaller(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, session.BacklogStatusReview, session.BacklogStatusReady)
	item := newItemAt(t, storage, session.BacklogStatusReview)
	before := session.GateCallsTotal(session.ClassOperator, session.CallerOperatorNotificationAction, session.ModeEnforce)

	decider := backlogDecider{BacklogDeciderDeps{Transitions: svc}}
	res, err := decider.transition(t.Context(), BacklogDecision{ItemID: item.ID, Reason: ""}, string(session.BacklogStatusReview), string(session.BacklogStatusReady), string(session.BacklogStatusReview))
	require.NoError(t, err)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, res.Outcome, "the operator gate denies the click")
	assert.Equal(t, before+1, session.GateCallsTotal(session.ClassOperator, session.CallerOperatorNotificationAction, session.ModeEnforce))
}
