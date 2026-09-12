package session

// backlog_status_event_test.go covers Epic 2.5, Story 2.5.1's acceptance
// criterion: a BacklogStatusEvent row created for a transition into a
// since-deleted custom stage still renders the stage's original name in the
// item-detail history (via BacklogItemData.StatusEvents, the same eager-load
// GetBacklogItem uses to back the item-detail RPC).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBacklogStatusEvent_should_RenderOriginalStageName_When_ReferencedCustomStageIsLaterDeleted
// covers validation.md's traceability mapping for Story 2.5.1: transitioning
// an item into a custom stage freezes that stage's Name onto the resulting
// BacklogStatusEvent row (StageNameSnapshot). Deleting the BacklogStage row
// afterwards must not blank out or crash GetBacklogItem's history rendering —
// the event must keep reporting the original name, "Design Review".
func TestBacklogStatusEvent_should_RenderOriginalStageName_When_ReferencedCustomStageIsLaterDeleted(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	customStage, err := client.BacklogStage.Create().
		SetSlug("design-review").
		SetName("Design Review").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "item entering a custom stage",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatus("design-review"), nil, TriggeredByUser)
	require.NoError(t, err)

	// The custom stage is later deleted — e.g. an operator removing a
	// no-longer-needed workflow stage via the Epic 2.7 CRUD RPC.
	require.NoError(t, client.BacklogStage.DeleteOneID(customStage.ID).Exec(ctx))

	reloaded, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	var found *BacklogStatusEventData
	for i := range reloaded.StatusEvents {
		if reloaded.StatusEvents[i].ToStatus == "design-review" {
			found = &reloaded.StatusEvents[i]
			break
		}
	}
	require.NotNil(t, found, "expected a status event for the transition into design-review")
	require.NotNil(t, found.StageNameSnapshot, "StageNameSnapshot must survive the stage row's deletion")
	require.Equal(t, "Design Review", *found.StageNameSnapshot)
}

// TestResolveAllowedTransitionsSnapshot_should_ReturnSortedEnabledDestinationSlugs_When_StageHasOutgoingTransitions
// covers ADR-004's write-time query: only enabled StageTransition rows
// pointing at enabled destination stages are captured, sorted, with a
// disabled edge excluded.
func TestResolveAllowedTransitionsSnapshot_should_ReturnSortedEnabledDestinationSlugs_When_StageHasOutgoingTransitions(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("triage").SetName("Triage").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toB, err := client.BacklogStage.Create().SetSlug("triage-b").SetName("B").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toA, err := client.BacklogStage.Create().SetSlug("triage-a").SetName("A").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toDisabledEdge, err := client.BacklogStage.Create().SetSlug("triage-disabled-edge").SetName("Disabled Edge").SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toB.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toA.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	// A disabled edge must not appear in the snapshot even though its
	// destination stage is itself enabled.
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toDisabledEdge.ID).SetEnabled(false).Save(ctx)
	require.NoError(t, err)

	got := resolveAllowedTransitionsSnapshot(ctx, client.BacklogStage, client.StageTransition, BacklogStatus("triage"))
	require.Equal(t, []string{"triage-a", "triage-b"}, got)
}

// TestResolveAllowedTransitionsSnapshot_should_ReturnEmptyNonNilSlice_When_StageHasZeroEnabledOutgoingTransitions
// distinguishes "stage exists but is a dead end" (empty, non-nil) from "no
// matching stage row at all" (nil) — see the next test.
func TestResolveAllowedTransitionsSnapshot_should_ReturnEmptyNonNilSlice_When_StageHasZeroEnabledOutgoingTransitions(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	_, err := client.BacklogStage.Create().SetSlug("dead-end").SetName("Dead End").SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	got := resolveAllowedTransitionsSnapshot(ctx, client.BacklogStage, client.StageTransition, BacklogStatus("dead-end"))
	require.NotNil(t, got, "a stage with zero outgoing transitions must still resolve to an empty slice, not nil")
	require.Empty(t, got)
}

// TestResolveAllowedTransitionsSnapshot_should_ReturnNil_When_NoMatchingBacklogStageRowExists
// covers the resolution-miss case (e.g. the seed migration hasn't run yet) —
// must never block the write, mirroring resolveStageNameSnapshot's own
// best-effort discipline.
func TestResolveAllowedTransitionsSnapshot_should_ReturnNil_When_NoMatchingBacklogStageRowExists(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	got := resolveAllowedTransitionsSnapshot(ctx, client.BacklogStage, client.StageTransition, BacklogStatus("no-such-slug"))
	require.Nil(t, got)
}

// TestBuildStageConfigSnapshotFallback_should_ReconstructSnapshotFromMostRecentMatchingEvent_When_ItemHasTransitionedIntoCurrentStage
// covers ADR-004's happy path: an item transitioned into a custom stage
// reconstructs a StageConfigSnapshot carrying that stage's captured name and
// allowed-transitions snapshot.
func TestBuildStageConfigSnapshotFallback_should_ReconstructSnapshotFromMostRecentMatchingEvent_When_ItemHasTransitionedIntoCurrentStage(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("review-a").SetName("Review A").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("review-b").SetName("Review B").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "item entering review-a",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatus("review-a"), nil, TriggeredByUser)
	require.NoError(t, err)

	reloaded, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	fallback := BuildStageConfigSnapshotFallback(reloaded)
	require.NotNil(t, fallback)
	require.Equal(t, "Review A", fallback.StageName)
	require.Equal(t, []BacklogStatus{BacklogStatus("review-b")}, fallback.AllowedTransitions)
}

// TestBuildStageConfigSnapshotFallback_should_ReturnNil_When_NoStatusEventMatchesCurrentStatus
// covers the no-matching-event case: an item still sitting in its entry stage
// (created directly into it, never the destination of a recorded transition)
// has no BacklogStatusEvent to reconstruct from — must degrade to nil (already
// ConfiguredWorkflowEngine's "no fallback" case), not a crash.
func TestBuildStageConfigSnapshotFallback_should_ReturnNil_When_NoStatusEventMatchesCurrentStatus(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "fresh item, never transitioned",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	reloaded, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	require.Nil(t, BuildStageConfigSnapshotFallback(reloaded))
}
