package services

// backlog_service_gates_test.go — tests for GetPendingGates, the read-only
// preview RPC backing the item-detail "what's blocking this" checklist
// (Epic 2.10). Added alongside the checklist UI once it surfaced that no RPC
// exposed WorkflowEngine.PendingGates to the frontend at all.

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func TestGetPendingGates_should_ReturnNotFound_When_ItemDoesNotExist(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   "00000000-0000-0000-0000-000000000000",
		ToStatus: string(session.BacklogStatusRefining),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetPendingGates_should_ReturnEmpty_When_TransitionHasNoConfiguredGates(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()
	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, storage.GetEntClient()))

	repo := session.NewEntStageConfigRepository(storage.GetEntClient())
	gateSatisfactionRepo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	engine, err := session.NewConfiguredWorkflowEngine(repo, gateSatisfactionRepo, nil)
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, engine, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "an item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	resp, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   item.ID,
		ToStatus: string(session.BacklogStatusRefining),
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Gates, "the built-in idea->refining edge has no configured gates")
}

// newGatesTestService builds a *BacklogService wired with a real
// ConfiguredWorkflowEngine, plus its backing StageCRUDRepository — so a test
// can add a custom stage/transition/gate directly, mirroring
// newStageCRUDTestService's shape but wiring the engine into svc.engine (not
// just svc.stageConfigEngine), which is what GetPendingGates actually reads.
func newGatesTestService(t *testing.T) (*BacklogService, session.StageCRUDRepository, *session.ConfiguredWorkflowEngine, *session.Storage) {
	t.Helper()
	storage := createTestStorage(t)
	ctx := t.Context()
	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, storage.GetEntClient()))

	repo := session.NewEntStageConfigRepository(storage.GetEntClient())
	gateSatisfactionRepo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	engine, err := session.NewConfiguredWorkflowEngine(repo, gateSatisfactionRepo, nil)
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, engine, nil, nil)
	return svc, repo, engine, storage
}

// TestGetPendingGates_should_ReturnPopulatedGateStatus_When_TransitionHasConfiguredGates
// covers the primary success path GetPendingGates previously had no test for:
// a real configured structural gate must come back as a populated GateStatus
// entry with the correct GateId/Kind/Satisfied/Description, not just the
// not-found/empty edge cases the two tests above already covered.
func TestGetPendingGates_should_ReturnPopulatedGateStatus_When_TransitionHasConfiguredGates(t *testing.T) {
	t.Parallel()
	svc, repo, engine, storage := newGatesTestService(t)
	ctx := t.Context()

	fromStage, err := repo.CreateStage(ctx, session.StageCreateInput{Slug: "design-review", Name: "Design Review", Enabled: true})
	require.NoError(t, err)
	toStage, err := repo.CreateStage(ctx, session.StageCreateInput{Slug: "design-approved", Name: "Design Approved", Enabled: true})
	require.NoError(t, err)
	transition, err := repo.CreateTransition(ctx, session.TransitionCreateInput{FromStageSlug: fromStage.Slug, ToStageSlug: toStage.Slug, Enabled: true})
	require.NoError(t, err)
	gate, err := repo.CreateGate(ctx, session.GateCreateInput{
		TransitionID: transition.ID,
		Kind:         string(session.GateKindStructural),
		Config:       map[string]interface{}{"check_id": session.StructuralCheckACComplete},
		Enabled:      true,
	})
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	criteria, err := session.SerializeAcCriteria([]session.AcCriterion{
		{Index: 0, Text: "criterion one", Status: session.AcStatusPending},
	})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:              "item with an unfinished AC",
		Status:             fromStage.Slug,
		AcceptanceCriteria: criteria,
	})
	require.NoError(t, err)

	resp, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   item.ID,
		ToStatus: toStage.Slug,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Gates, 1, "expected exactly the one configured structural gate")

	got := resp.Msg.Gates[0]
	assert.Equal(t, gate.ID.String(), got.GateId)
	assert.Equal(t, string(session.GateKindStructural), got.Kind)
	assert.False(t, got.Satisfied, "the AC criterion is still pending, so the gate must not report satisfied")
	assert.Equal(t, "1 of 1 acceptance criteria incomplete", got.Description)
	assert.Empty(t, got.ConfigError, "a correctly configured gate must not carry a ConfigError")
}

// TestGetPendingGates_should_SetConfigError_When_CustomGateReferencesUnregisteredSkill
// is the regression test for ADR-006 Part B's entire reason for existing: a
// GateKindCustom gate whose configured skill is no longer in the registered
// allowlist must surface a non-empty ConfigError all the way through
// gateStatusToProto to the RPC response, not just fail silently or report a
// misleading "unsatisfied" with no explanation.
func TestGetPendingGates_should_SetConfigError_When_CustomGateReferencesUnregisteredSkill(t *testing.T) {
	t.Parallel()
	svc, repo, engine, storage := newGatesTestService(t)
	ctx := t.Context()

	fromStage, err := repo.CreateStage(ctx, session.StageCreateInput{Slug: "needs-review", Name: "Needs Review", Enabled: true})
	require.NoError(t, err)
	toStage, err := repo.CreateStage(ctx, session.StageCreateInput{Slug: "reviewed", Name: "Reviewed", Enabled: true})
	require.NoError(t, err)
	transition, err := repo.CreateTransition(ctx, session.TransitionCreateInput{FromStageSlug: fromStage.Slug, ToStageSlug: toStage.Slug, Enabled: true})
	require.NoError(t, err)
	const unregisteredSkill = "no-such-skill"
	_, err = repo.CreateGate(ctx, session.GateCreateInput{
		TransitionID: transition.ID,
		Kind:         string(session.GateKindCustom),
		Config:       map[string]interface{}{"skill": unregisteredSkill},
		Enabled:      true,
	})
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item behind an unregistered custom gate",
		Status: fromStage.Slug,
	})
	require.NoError(t, err)

	resp, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   item.ID,
		ToStatus: toStage.Slug,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Gates, 1)

	got := resp.Msg.Gates[0]
	assert.False(t, got.Satisfied, "a config-error gate must always report unsatisfied")
	require.NotEmpty(t, got.ConfigError, "ConfigError must survive end-to-end through gateStatusToProto to the RPC response")
	assert.Contains(t, got.ConfigError, unregisteredSkill)
	assert.Contains(t, got.ConfigError, "no longer registered")
}
