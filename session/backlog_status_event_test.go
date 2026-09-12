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
