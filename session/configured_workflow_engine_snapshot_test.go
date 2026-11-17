package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/stagecfg"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// newDeletedStageItemFixture creates a real item (via the EntRepository write
// path) transitioned into a custom stage with one outgoing transition,
// captures its ADR-004 snapshot fields at write time, then deletes that stage
// (cascading away its StageTransition row) and invalidates engine's cache —
// the end-to-end precondition
// TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition
// asserts against. Returns the engine, the item's (from,to) BacklogStatus
// pair, and the item reloaded post-deletion (for BuildStageConfigSnapshotFallback).
func newDeletedStageItemFixture(t *testing.T) (engine *workflowengine.ConfiguredWorkflowEngine, from, to domain.BacklogStatus, reloaded *backlogdto.BacklogItemData) {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.Client

	fromStage, err := client.BacklogStage.Create().SetSlug("e2e-from").SetName("E2E From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("e2e-to").SetName("E2E To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	stageRepo := stagecfg.NewEntStageConfigRepository(client)
	gateSatisfactionRepo := stagecfg.NewEntGateSatisfactionRepository(client)
	engine, err = workflowengine.NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)

	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:  "e2e item entering a stage that will be deleted",
		Status: string(domain.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatus(fromStage.Slug), nil, instance.TriggeredByUser)
	require.NoError(t, err)

	require.NoError(t, client.BacklogStage.DeleteOneID(fromStage.ID).Exec(ctx))
	require.NoError(t, engine.InvalidateCache(ctx))

	reloaded, err = repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	return engine, domain.BacklogStatus(fromStage.Slug), domain.BacklogStatus(toStage.Slug), reloaded
}

// TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition
// is ADR-004's end-to-end acceptance test: given a real item transitioned
// into a custom stage that is later deleted, reconstruct the fallback via
// BuildStageConfigSnapshotFallback from the reloaded item, and confirm
// AllowedTransitions/CanTransition/PendingGates all behave per the frozen
// snapshot when called with that fallback — instead of the fail-open/empty
// answers they give when called without one.
func TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition(t *testing.T) {
	t.Parallel()
	engine, from, to, reloaded := newDeletedStageItemFixture(t)

	fallback := backlogdto.BuildStageConfigSnapshotFallback(reloaded)
	require.NotNil(t, fallback, "expected a reconstructed snapshot from the item's own status-event history")
	require.Equal(t, "E2E From", fallback.StageName)
	require.Equal(t, []domain.BacklogStatus{to}, fallback.AllowedTransitions)

	// Without the fallback: the deleted stage reports empty/false/nil, the
	// fail-open behavior ADR-004's fallback plumbing exists to avoid.
	assert.False(t, engine.CanTransition(from, to, nil), "sanity: without a fallback, the deleted stage reports false")
	assert.Empty(t, engine.AllowedTransitions(from, nil), "sanity: without a fallback, the deleted stage reports empty")
	gatesNoFallback, err := engine.PendingGates(domain.BacklogItemTransitionInput{Status: from}, to, nil)
	require.NoError(t, err)
	assert.Empty(t, gatesNoFallback, "sanity: without a fallback, PendingGates reports zero pending gates")

	// With the reconstructed fallback: all three answer per the frozen snapshot.
	assert.True(t, engine.CanTransition(from, to, fallback), "CanTransition must honor the captured snapshot")
	assert.Equal(t, []domain.BacklogStatus{to}, engine.AllowedTransitions(from, fallback), "AllowedTransitions must honor the captured snapshot")

	gates, err := engine.PendingGates(domain.BacklogItemTransitionInput{Status: from}, to, fallback)
	require.NoError(t, err)
	require.Len(t, gates, 1, "PendingGates must report the synthetic blocking gate, not silently pass the item through")
	assert.Equal(t, domain.StageConfigUnresolvableGateID, gates[0].GateID)
	assert.False(t, gates[0].Satisfied)
}
