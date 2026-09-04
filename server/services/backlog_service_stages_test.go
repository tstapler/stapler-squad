package services

// backlog_service_stages_test.go — tests for the 5 BacklogStage CRUD RPCs
// (Story 2.7.1 of backlog-custom-workflow-stages). Test names for the two
// rows validation.md lists for this story are taken verbatim from
// project_plans/backlog-custom-workflow-stages/implementation/validation.md.

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// newStageCRUDTestService builds a *BacklogService wired with a real
// ent-backed StageCRUDRepository, seeded with the 9 built-in workflow
// stages/transitions, plus a real ConfiguredWorkflowEngine sharing that same
// repository instance for cache-invalidation assertions — mirroring
// newPipelineModeTestService's shape in backlog_service_pipeline_mode_test.go.
func newStageCRUDTestService(t *testing.T) (*BacklogService, session.StageCRUDRepository, *session.ConfiguredWorkflowEngine, *session.Storage) {
	t.Helper()
	storage := createTestStorage(t)
	ctx := t.Context()

	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, storage.GetEntClient()))

	repo := session.NewEntStageConfigRepository(storage.GetEntClient())
	gateSatisfactionRepo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	engine, err := session.NewConfiguredWorkflowEngine(repo, gateSatisfactionRepo)
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetStageCRUDRepository(repo)
	svc.SetStageConfigEngine(engine)
	return svc, repo, engine, storage
}

// ─── TestCreateStage / TestGetStage / TestListStages ───────────────────────

func TestCreateStage_should_PersistAndReturnStage_When_ValidInput(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	resp, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug:    "triage",
		Name:    "Triage",
		Enabled: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "triage", resp.Msg.Item.Slug)
	assert.Equal(t, "Triage", resp.Msg.Item.Name)
	assert.True(t, resp.Msg.Item.Enabled)
	assert.NotEmpty(t, resp.Msg.Item.Id)

	getResp, err := svc.GetStage(ctx, connect.NewRequest(&sessionv1.GetStageRequest{Slug: "triage"}))
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.Item.Id, getResp.Msg.Item.Id)
}

func TestCreateStage_should_ReturnAlreadyExists_When_SlugIsDuplicate(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: "triage", Name: "Triage"}))
	require.NoError(t, err)

	_, err = svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: "triage", Name: "Triage Again"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
}

func TestListStages_should_ReturnAllNineBuiltInStages_When_DatabaseIsFreshlySeeded(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	resp, err := svc.ListStages(ctx, connect.NewRequest(&sessionv1.ListStagesRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Items, 9)
}

// ─── TestDeleteStage ────────────────────────────────────────────────────────

// TestDeleteStage_should_ReturnFailedPrecondition_When_StageHasLiveItems
// (validation.md, Story 2.7.1) proves a stage with >=1 live item is rejected
// with CodeFailedPrecondition naming the item count, and the stage survives.
func TestDeleteStage_should_ReturnFailedPrecondition_When_StageHasLiveItems(t *testing.T) {
	t.Parallel()
	svc, repo, _, storage := newStageCRUDTestService(t)
	ctx := t.Context()

	reviewStage, err := repo.GetStageBySlug(ctx, string(session.BacklogStatusReview))
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item 1", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item 2", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item 3", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.DeleteStage(ctx, connect.NewRequest(&sessionv1.DeleteStageRequest{Id: reviewStage.ID.String()}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "3")
	assert.Contains(t, err.Error(), string(session.BacklogStatusReview))

	// The stage must still exist — no partial write.
	_, err = svc.GetStage(ctx, connect.NewRequest(&sessionv1.GetStageRequest{Slug: string(session.BacklogStatusReview)}))
	require.NoError(t, err)
}

// TestDeleteStage_should_Succeed_When_ForceIsTrueDespiteLiveItems proves the
// force=true escape hatch bypasses the live-item guard.
func TestDeleteStage_should_Succeed_When_ForceIsTrueDespiteLiveItems(t *testing.T) {
	t.Parallel()
	svc, repo, _, storage := newStageCRUDTestService(t)
	ctx := t.Context()

	reviewStage, err := repo.GetStageBySlug(ctx, string(session.BacklogStatusReview))
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item 1", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.DeleteStage(ctx, connect.NewRequest(&sessionv1.DeleteStageRequest{Id: reviewStage.ID.String(), Force: true}))
	require.NoError(t, err)

	_, err = svc.GetStage(ctx, connect.NewRequest(&sessionv1.GetStageRequest{Slug: string(session.BacklogStatusReview)}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestDeleteStage_should_Succeed_When_StageHasZeroLiveItems proves the guard
// never fires for a stage nothing is currently sitting on.
func TestDeleteStage_should_Succeed_When_StageHasZeroLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: "scratch", Name: "Scratch"}))
	require.NoError(t, err)

	_, err = svc.DeleteStage(ctx, connect.NewRequest(&sessionv1.DeleteStageRequest{Id: createResp.Msg.Item.Id}))
	require.NoError(t, err)
}

// ─── TestUpdateStage ────────────────────────────────────────────────────────

// TestUpdateStage_should_InvalidateStageConfigCache_When_UpdateSucceeds
// (validation.md, Story 2.7.1) proves UpdateStage invalidates
// ConfiguredWorkflowEngine's stageConfigCache synchronously before
// returning: disabling a stage that is the "from" endpoint of a built-in
// edge makes that edge disappear (stageConfigCache.refresh skips any edge
// whose endpoint stage is disabled) with no restart and no explicit
// invalidate call from the test.
func TestUpdateStage_should_InvalidateStageConfigCache_When_UpdateSucceeds(t *testing.T) {
	t.Parallel()
	svc, repo, engine, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	require.True(t, engine.CanTransition(session.BacklogStatusIdea, session.BacklogStatusRefining),
		"precondition: idea -> refining is a built-in enabled edge")

	refiningStage, err := repo.GetStageBySlug(ctx, string(session.BacklogStatusRefining))
	require.NoError(t, err)

	_, err = svc.UpdateStage(ctx, connect.NewRequest(&sessionv1.UpdateStageRequest{
		Id:      refiningStage.ID.String(),
		Enabled: boolPtr(false),
	}))
	require.NoError(t, err)

	assert.False(t, engine.CanTransition(session.BacklogStatusIdea, session.BacklogStatusRefining),
		"expected the cache to be invalidated synchronously so the disabled stage's edge disappears immediately")
}

func TestUpdateStage_should_ReturnNotFound_When_IdDoesNotExist(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.UpdateStage(ctx, connect.NewRequest(&sessionv1.UpdateStageRequest{
		Id:   "00000000-0000-0000-0000-000000000000",
		Name: strPtr("new name"),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
