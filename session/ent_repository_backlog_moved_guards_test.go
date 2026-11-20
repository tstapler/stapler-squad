package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestUpdateBacklogItem_should_RejectWithoutWritingOrPublishing_When_ExpectedStatusMismatches(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "keep", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	published := captureChanges(repo)

	_, err = repo.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{Title: ptrTo("changed")},
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusDone)})

	require.ErrorIs(t, err, domain.ErrPreconditionFailed)
	got, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "keep", got.Title)
	assert.Empty(t, published(), "a refused update must not publish")
}

func TestAcCriterionWriters_should_RejectOutOfRangeIndex_When_IndexEqualsLenOrNegative(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	ac, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "only", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "r", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: ac})
	require.NoError(t, err)

	for _, idx := range []int{1, -1} {
		err := repo.UpdateAcCriterionStatus(ctx, item.ID, idx, string(domain.AcStatusDone), "n")
		assert.ErrorIs(t, err, backlogdto.ErrAcCriterionOutOfRange, "update idx %d", idx)
		err = repo.WaiveAcCriterion(ctx, item.ID, idx, "why")
		assert.ErrorIs(t, err, backlogdto.ErrAcCriterionOutOfRange, "waive idx %d", idx)
	}
}

func TestListBacklogItems_should_ExcludeTerminalStatusesOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	for _, st := range []domain.BacklogStatus{domain.BacklogStatusIdea, domain.BacklogStatusDone, domain.BacklogStatusArchived} {
		_, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: string(st), Status: string(st)})
		require.NoError(t, err)
	}
	titles := func(f backlogdto.BacklogItemFilter) []string {
		list, err := repo.ListBacklogItems(ctx, f)
		require.NoError(t, err)
		out := make([]string, 0, len(list))
		for _, it := range list {
			out = append(out, it.Title)
		}
		return out
	}

	assert.ElementsMatch(t, []string{"idea", "done", "archived"}, titles(backlogdto.BacklogItemFilter{}))
	assert.ElementsMatch(t, []string{"idea", "archived"}, titles(backlogdto.BacklogItemFilter{ExcludeDone: true}))
	assert.ElementsMatch(t, []string{"idea", "done"}, titles(backlogdto.BacklogItemFilter{ExcludeArchived: true}))
	assert.ElementsMatch(t, []string{"idea"}, titles(backlogdto.BacklogItemFilter{ExcludeDone: true, ExcludeArchived: true}))
	assert.ElementsMatch(t, []string{"done"}, titles(backlogdto.BacklogItemFilter{Statuses: []string{"done"}, ExcludeDone: true}),
		"an explicit Statuses whitelist wins over the exclusion flags")
}

// The 200 is pinned as a literal on purpose: the cap bounds the heap cost of a publish on an item
// stuck in a transition crash-loop, so raising it must be a deliberate edit of this test too.
func TestPublishItemChanged_should_CarryExactlyTheNewest200StatusEvents(t *testing.T) {
	t.Parallel()
	require.Equal(t, 200, backlogstore.MaxPublishedStatusEvents)
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "loop", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	itemID, err := repo.ResolveBacklogItemLookup(ctx, item.ID)
	require.NoError(t, err)
	const total = 205
	for i := 0; i < total; i++ {
		_, err := repo.Client.BacklogStatusEvent.Create().SetItemID(itemID).
			SetFromStatus(string(domain.BacklogStatusIdea)).SetToStatus(string(domain.BacklogStatusIdea)).
			SetTriggeredBy(instance.TriggeredBySystem).SetNote(fmt.Sprintf("n%d", i)).Save(ctx)
		require.NoError(t, err)
	}
	published := captureChanges(repo)

	repo.PublishItemChanged(ctx, &backlogdto.BacklogItemData{ID: item.ID}, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})

	require.Len(t, published(), 1)
	evs := published()[0].item.StatusEvents
	require.Len(t, evs, 200)
	require.NotNil(t, evs[0].Note)
	require.NotNil(t, evs[199].Note)
	assert.Equal(t, "n5", *evs[0].Note, "the five oldest events are dropped")
	assert.Equal(t, "n204", *evs[199].Note, "the newest event is kept, oldest-first order")
}
