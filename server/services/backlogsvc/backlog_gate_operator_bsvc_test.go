package backlogsvc

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func gateEdge(t *testing.T, repo *session.EntStageConfigRepository, cfg *session.ConfiguredWorkflowEngine, from, to domain.BacklogStatus) session.TransitionData {
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
func addApprovalGate(t *testing.T, repo *session.EntStageConfigRepository, cfg *session.ConfiguredWorkflowEngine, from, to domain.BacklogStatus) {
	t.Helper()
	edge := gateEdge(t, repo, cfg, from, to)
	_, err := repo.CreateGate(t.Context(), session.GateCreateInput{
		TransitionID: edge.ID, Kind: string(domain.GateKindHumanApproval), Config: map[string]interface{}{}, Enabled: true,
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

func newItemAt(t *testing.T, storage *session.Storage, status domain.BacklogStatus) *backlogdto.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{Title: "gate item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func TestTransitionHandler_Enforce_GraphEditAndGateDenyWithUnchangedCodes(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	transition := func(c context.Context, id string, to domain.BacklogStatus, override, expected string) error {
		_, err := svc.TransitionBacklogItemStatus(c, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId: id, TargetStatus: string(to), OverrideReason: override, ExpectedStatus: expected,
		}))
		return err
	}

	// A graph edit: idea->refining disabled.
	edge := gateEdge(t, repo, cfg, domain.BacklogStatusIdea, domain.BacklogStatusRefining)
	disabled := false
	_, err := repo.UpdateTransition(ctx, edge.ID, session.TransitionUpdateInput{Enabled: &disabled})
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
	item := newItemAt(t, storage, domain.BacklogStatusIdea)
	err = transition(ctx, item.ID, domain.BacklogStatusRefining, "", "")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "graph denial")
	assert.Equal(t, string(domain.BacklogStatusIdea), storedStatus(t, storage, item.ID))

	// A Default guard sentinel keeps FailedPrecondition.
	noAC := newItemAt(t, storage, domain.BacklogStatusIdea)
	err = transition(ctx, noAC.ID, domain.BacklogStatusReady, "", "")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "guard denial")

	// A lost compare-and-swap keeps Aborted.
	err = transition(ctx, item.ID, domain.BacklogStatusArchived, "", string(domain.BacklogStatusReady))
	assert.Equal(t, connect.CodeAborted, connect.CodeOf(err), "stale expected_status")
}

func TestTransitionHandler_Enforce_OperatorOverrideBypassesGateAgentDoesNot(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	addApprovalGate(t, repo, cfg, domain.BacklogStatusReady, domain.BacklogStatusRefining)
	req := func(id, override string) *connect.Request[sessionv1.TransitionBacklogItemStatusRequest] {
		return connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: id, TargetStatus: string(domain.BacklogStatusRefining), OverrideReason: override})
	}

	item := newItemAt(t, storage, domain.BacklogStatusReady)
	_, err := svc.TransitionBacklogItemStatus(ctx, req(item.ID, ""))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "an unsatisfied operator gate denies")
	assert.Empty(t, transitionDeniedRows(t, svc, item.ID), "an operator denial returns the error only")

	_, err = svc.TransitionBacklogItemStatus(services.WithTransitionActor(ctx, session.TriggeredByAgent), req(item.ID, "agent insists"))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "an agent never bypasses a Configured gate")
	assert.Equal(t, string(domain.BacklogStatusReady), storedStatus(t, storage, item.ID))
	rows := transitionDeniedRows(t, svc, item.ID)
	require.Len(t, rows, 1, "an agent denial reaches the reporter")
	assert.Equal(t, string(domain.CallerMCPTransitionDelegate), rows[0].DeniedCaller)

	_, err = svc.TransitionBacklogItemStatus(ctx, req(item.ID, "owner says so"))
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusRefining), storedStatus(t, storage, item.ID))
}

func TestTransitionHandler_Off_MatchesLegacyAnswers(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, session.NewDefaultWorkflowEngine(), nil, nil)
	require.False(t, storage.TransitionGateWired(), "no gate is injected, so the answer is Off")
	item := newItemAt(t, storage, domain.BacklogStatusIdea)

	_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: item.ID, TargetStatus: string(domain.BacklogStatusInProgress)}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "idea->in_progress is not an edge")
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: item.ID, TargetStatus: string(domain.BacklogStatusRefining)}))
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusRefining), storedStatus(t, storage, item.ID))
}

func TestArchiveBacklogItem_Enforce_AllowsFromAnyStatusAndDeniesOnDoneGate(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	ctx := t.Context()
	archive := func(id string) error {
		_, err := svc.ArchiveBacklogItem(ctx, connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{ItemId: id}))
		return err
	}

	for _, from := range []domain.BacklogStatus{domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending} {
		item := newItemAt(t, storage, from)
		require.NoError(t, archive(item.ID), "archive from %s", from)
		assert.Equal(t, string(domain.BacklogStatusArchived), storedStatus(t, storage, item.ID))
		require.NoError(t, archive(item.ID), "archived->archived is idempotently allowed")
	}

	addApprovalGate(t, repo, cfg, domain.BacklogStatusDone, domain.BacklogStatusArchived)
	done := newItemAt(t, storage, domain.BacklogStatusDone)
	err := archive(done.ID)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, string(domain.BacklogStatusDone), storedStatus(t, storage, done.ID), "a denied archive changes nothing")
	inProgress := newItemAt(t, storage, domain.BacklogStatusInProgress)
	require.NoError(t, archive(inProgress.ID), "the gate on done->archived does not touch other statuses")
}

func TestUnarchiveBacklogItem_Enforce_DeniesNonArchivedItem(t *testing.T) {
	t.Parallel()
	svc, storage, _, _ := newEnforcedGateService(t)
	unarchive := func(id string) error {
		_, err := svc.UnarchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UnarchiveBacklogItemRequest{ItemId: id}))
		return err
	}
	archived := newItemAt(t, storage, domain.BacklogStatusArchived)
	require.NoError(t, unarchive(archived.ID))
	assert.Equal(t, string(domain.BacklogStatusIdea), storedStatus(t, storage, archived.ID))

	live := newItemAt(t, storage, domain.BacklogStatusIdea)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(unarchive(live.ID)), "idea->idea is not an edge")
	assert.Equal(t, string(domain.BacklogStatusIdea), storedStatus(t, storage, live.ID))
}

func TestSubmitManualReview_Enforce_GateOnReviewToDoneDeniesWithRecordedVerdict(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, domain.BacklogStatusReview, domain.BacklogStatusDone)
	item := newItemAt(t, storage, domain.BacklogStatusReview)

	_, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId: item.ID, OverallOutcome: "PASS", Summary: "looks good",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Equal(t, string(domain.BacklogStatusReview), storedStatus(t, storage, item.ID), "the gate holds the item in review")
}

func TestSubmitManualReview_Enforce_PassGuardInputLetsUngatedReviewToDoneThrough(t *testing.T) {
	t.Parallel()
	svc, storage, _, _ := newEnforcedGateService(t)
	item := newItemAt(t, storage, domain.BacklogStatusReview)

	resp, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId: item.ID, OverallOutcome: "PASS", Summary: "looks good",
	}))
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusDone), resp.Msg.Item.Status, "the Default verdict guard sees the PASS the review just recorded")
}

func TestUpdateBacklogItem_Enforce_PRAssociationHonorsOperatorGate(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, domain.BacklogStatusReview, domain.BacklogStatusPRPending)
	item := newItemAt(t, storage, domain.BacklogStatusReview)
	url, number := "https://github.com/acme/widgets/pull/7", int32(7)

	_, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: item.ID, PrUrl: &url, PrNumber: &number,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.ErrorContains(t, err, "transition gate not satisfied")
	assert.Equal(t, string(domain.BacklogStatusReview), storedStatus(t, storage, item.ID))
}

func TestBacklogDecider_Transition_ReachesHandlerAsNotificationActionCaller(t *testing.T) {
	t.Parallel()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	addApprovalGate(t, repo, cfg, domain.BacklogStatusReview, domain.BacklogStatusReady)
	item := newItemAt(t, storage, domain.BacklogStatusReview)
	before := session.GateCallsTotal(domain.ClassOperator, domain.CallerOperatorNotificationAction, session.ModeEnforce)

	// "rework" is Review -> Ready under the same operator caller, through the exported entry point.
	decider := services.NewBacklogDecider(services.BacklogDeciderDeps{Status: services.NewItemStatusReader(storage), Transitions: svc})
	res, err := decider.Decide(t.Context(), services.BacklogDecision{ItemID: item.ID, Choice: services.ChoiceRework})
	require.NoError(t, err)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, res.Outcome, "the operator gate denies the click")
	assert.Equal(t, before+1, session.GateCallsTotal(domain.ClassOperator, domain.CallerOperatorNotificationAction, session.ModeEnforce))
}
