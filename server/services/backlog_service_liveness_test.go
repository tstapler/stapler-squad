package services

// backlog_service_liveness_test.go — tests for the 5 LivenessDefinition CRUD
// RPCs (Epic 1.3, Story 1.3.2 of project_plans/backlog-custom-workflow-stages).
// Test names and scenarios are taken verbatim from
// project_plans/backlog-custom-workflow-stages/implementation/validation.md's
// "Story 1.3.2" rows. Structural pattern mirrors
// backlog_service_pipeline_mode_test.go's PipelineMode CRUD tests.

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// newLivenessTestService builds a *BacklogService wired with a real
// ent-backed LivenessRepository and a real CachingLivenessEngine constructed
// over that same repository — mirroring newPipelineModeTestService and how
// server/dependencies.go wires production (both share one instance).
func newLivenessTestService(t *testing.T) (*BacklogService, session.LivenessRepository, *session.CachingLivenessEngine) {
	t.Helper()
	storage := createTestStorage(t)
	repo := session.NewEntLivenessRepository(storage.GetEntClient())
	engine, err := session.NewCachingLivenessEngine(repo)
	require.NoError(t, err)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetLivenessRepository(repo)
	svc.SetLivenessEngine(engine)
	return svc, repo, engine
}

// ─── TestCreateLivenessDefinition ──────────────────────────────────────────

// TestCreateLivenessDefinition_should_ReturnLivenessDefinition_When_StageSlugAndPipelineModePairIsNew
// covers Story 1.3.2's happy path: a successful create returns the persisted
// row.
func TestCreateLivenessDefinition_should_ReturnLivenessDefinition_When_StageSlugAndPipelineModePairIsNew(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newLivenessTestService(t)
	ctx := t.Context()

	mode := "sdd"
	resp, err := svc.CreateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.CreateLivenessDefinitionRequest{
		StageSlug:          "idea",
		PipelineMode:       &mode,
		Kind:               "duration_budget",
		ExpectedDurationMs: int64(45 * 60 * 1000),
		StalenessMarginMs:  int64(10 * 60 * 1000),
		Enabled:            true,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	item := resp.Msg.Item
	assert.NotEmpty(t, item.Id)
	assert.Equal(t, "idea", item.StageSlug)
	require.NotNil(t, item.PipelineMode)
	assert.Equal(t, "sdd", *item.PipelineMode)
	assert.Equal(t, "duration_budget", item.Kind)
	assert.Equal(t, int64(45*60*1000), item.ExpectedDurationMs)
	assert.Equal(t, int64(10*60*1000), item.StalenessMarginMs)
	assert.True(t, item.Enabled)

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "idea", all[0].StageSlug)
}

// TestCreateLivenessDefinition_should_ReturnAlreadyExists_When_StageSlugAndPipelineModePairHasEnabledRow
// covers Story 1.3.2's error path: a second CreateLivenessDefinition call for
// the same (stage_slug, pipeline_mode) pair that already has an enabled row
// is rejected with connect.CodeAlreadyExists.
func TestCreateLivenessDefinition_should_ReturnAlreadyExists_When_StageSlugAndPipelineModePairHasEnabledRow(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newLivenessTestService(t)
	ctx := t.Context()

	mode := "sdd"
	_, err := svc.CreateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.CreateLivenessDefinitionRequest{
		StageSlug:          "idea",
		PipelineMode:       &mode,
		Kind:               "duration_budget",
		ExpectedDurationMs: int64(45 * 60 * 1000),
		StalenessMarginMs:  int64(10 * 60 * 1000),
		Enabled:            true,
	}))
	require.NoError(t, err)

	_, err = svc.CreateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.CreateLivenessDefinitionRequest{
		StageSlug:          "idea",
		PipelineMode:       &mode,
		Kind:               "duration_budget",
		ExpectedDurationMs: int64(60 * 60 * 1000),
		StalenessMarginMs:  int64(10 * 60 * 1000),
		Enabled:            true,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))

	all, listErr := repo.ListAll(ctx)
	require.NoError(t, listErr)
	assert.Len(t, all, 1, "the duplicate request must not write a second row")
}

// ─── TestUpdateLivenessDefinition ──────────────────────────────────────────

// TestUpdateLivenessDefinition_should_InvalidateCache_When_UpdateSucceeds
// (Integration, per validation.md) proves UpdateLivenessDefinition invalidates
// the live LivenessEngine's cache synchronously: given a cached ("idea","sdd")
// row with ExpectedDuration=45m, after Update changes it to 60m, the very
// next LivenessFor(BacklogStatusIdea, "sdd") call — no server restart, no
// explicit invalidate call from the test — reflects the new value.
func TestUpdateLivenessDefinition_should_InvalidateCache_When_UpdateSucceeds(t *testing.T) {
	t.Parallel()
	svc, _, engine := newLivenessTestService(t)
	ctx := t.Context()

	mode := "sdd"
	createResp, err := svc.CreateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.CreateLivenessDefinitionRequest{
		StageSlug:          "idea",
		PipelineMode:       &mode,
		Kind:               "duration_budget",
		ExpectedDurationMs: int64(45 * 60 * 1000),
		StalenessMarginMs:  int64(10 * 60 * 1000),
		Enabled:            true,
	}))
	require.NoError(t, err)
	id := createResp.Msg.Item.Id

	before, err := engine.LivenessFor(session.BacklogStatusIdea, session.PipelineMode("sdd"))
	require.NoError(t, err)
	assert.Equal(t, 45*time.Minute, before.ExpectedDuration)

	newExpectedMs := int64(60 * 60 * 1000)
	updateResp, err := svc.UpdateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.UpdateLivenessDefinitionRequest{
		Id:                 id,
		ExpectedDurationMs: &newExpectedMs,
	}))
	require.NoError(t, err)
	assert.Equal(t, newExpectedMs, updateResp.Msg.Item.ExpectedDurationMs)

	after, err := engine.LivenessFor(session.BacklogStatusIdea, session.PipelineMode("sdd"))
	require.NoError(t, err)
	assert.Equal(t, 60*time.Minute, after.ExpectedDuration,
		"expected the next LivenessFor call to reflect the updated value with no server restart")
	assert.NotEqual(t, before.ExpectedDuration, after.ExpectedDuration)
}

// ─── TestGetLivenessDefinition / TestListLivenessDefinitions / TestDeleteLivenessDefinition ───

// TestLivenessDefinitionCRUD_should_RoundTripCreateGetUpdateDelete_When_CalledSequentially
// exercises the full Create->Get->Update->Delete sequence against a real
// (ent-backed) test DB — not a named validation.md row, but the same
// round-trip coverage TestPipelineModeCRUD_should_RoundTrip... provides for
// PipelineMode.
func TestLivenessDefinitionCRUD_should_RoundTripCreateGetUpdateDelete_When_CalledSequentially(t *testing.T) {
	t.Parallel()
	svc, _, _ := newLivenessTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.CreateLivenessDefinitionRequest{
		StageSlug:               "in_progress",
		Kind:                    "heartbeat",
		MaxNoProgressDurationMs: int64(2 * 60 * 60 * 1000),
		Enabled:                 true,
	}))
	require.NoError(t, err)
	created := createResp.Msg.Item
	require.NotEmpty(t, created.Id)
	assert.Equal(t, "in_progress", created.StageSlug)
	assert.Nil(t, created.PipelineMode, "a mode-less row must round-trip with PipelineMode unset")

	getResp, err := svc.GetLivenessDefinition(ctx, connect.NewRequest(&sessionv1.GetLivenessDefinitionRequest{
		StageSlug: "in_progress",
	}))
	require.NoError(t, err)
	assert.Equal(t, created.Id, getResp.Msg.Item.Id)

	newMax := int64(3 * 60 * 60 * 1000)
	updateResp, err := svc.UpdateLivenessDefinition(ctx, connect.NewRequest(&sessionv1.UpdateLivenessDefinitionRequest{
		Id:                      created.Id,
		MaxNoProgressDurationMs: &newMax,
	}))
	require.NoError(t, err)
	assert.Equal(t, newMax, updateResp.Msg.Item.MaxNoProgressDurationMs)

	listResp, err := svc.ListLivenessDefinitions(ctx, connect.NewRequest(&sessionv1.ListLivenessDefinitionsRequest{}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.Items, 1)

	_, err = svc.DeleteLivenessDefinition(ctx, connect.NewRequest(&sessionv1.DeleteLivenessDefinitionRequest{Id: created.Id}))
	require.NoError(t, err)

	_, err = svc.GetLivenessDefinition(ctx, connect.NewRequest(&sessionv1.GetLivenessDefinitionRequest{StageSlug: "in_progress"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
