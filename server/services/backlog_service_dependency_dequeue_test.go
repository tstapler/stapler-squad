package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// blockedDequeueFixture seeds a queued dependent whose blocker is a not-yet-started
// idea item, so the blocker stays unresolved until a test moves it.
func blockedDequeueFixture(t *testing.T) (svc *BacklogService, creator *mockSessionCreator, blockerID, dependentID string) {
	t.Helper()
	svc, creator, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: NewUnclaimedVerdict()})
	svc.claimDedupFlag = func() bool { return false }
	ctx := context.Background()

	blocker, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "blocker", SkipTriage: true,
	}))
	require.NoError(t, err)
	dependentID = createQueuedItemWithURL(t, svc, repoPath, "https://github.com/o/r/issues/1")
	_, err = svc.AddBacklogItemDependency(ctx, connect.NewRequest(&sessionv1.AddBacklogItemDependencyRequest{
		BlockerItemId: blocker.Msg.Item.Id,
		BlockedItemId: dependentID,
	}))
	require.NoError(t, err)
	return svc, creator, blocker.Msg.Item.Id, dependentID
}

func blockedByDependencyRows(t *testing.T, svc *BacklogService, itemID string) []*sessionv1.StuckBacklogItem {
	t.Helper()
	resp, err := svc.ListStuckBacklogItems(context.Background(), connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	var rows []*sessionv1.StuckBacklogItem
	for _, it := range resp.Msg.Items {
		if it.ItemId == itemID && it.Reason == sessionv1.StuckReason_STUCK_REASON_BLOCKED_BY_DEPENDENCY {
			rows = append(rows, it)
		}
	}
	return rows
}

func TestDequeueNextQueuedItems_should_NotStartItem_When_BlockerUnresolved(t *testing.T) {
	svc, creator, blockerID, dependentID := blockedDequeueFixture(t)

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))

	assert.Equal(t, "queued", itemStatus(t, svc, dependentID), "blocked item must stay queued")
	assert.Empty(t, creator.calls, "no work session may be spawned for a blocked item")

	rows := blockedByDependencyRows(t, svc, dependentID)
	require.Len(t, rows, 1, "exactly one open blocked_by_dependency stuck row")
	assert.Contains(t, rows[0].Context, blockerID, "stuck context must name the blocker id")

	require.NoError(t, svc.DequeueNextQueuedItems(context.Background()))
	assert.Len(t, blockedByDependencyRows(t, svc, dependentID), 1, "a second sweep must not duplicate the row")
}

func TestDequeueNextQueuedItems_should_StartItem_When_BlockerArchived(t *testing.T) {
	svc, creator, blockerID, dependentID := blockedDequeueFixture(t)
	ctx := context.Background()
	require.NoError(t, svc.DequeueNextQueuedItems(ctx))
	require.Equal(t, "queued", itemStatus(t, svc, dependentID))

	_, err := svc.ArchiveBacklogItem(ctx, connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{ItemId: blockerID}))
	require.NoError(t, err)
	require.NoError(t, svc.DequeueNextQueuedItems(ctx))

	assert.Equal(t, "in_progress", itemStatus(t, svc, dependentID), "archived blocker counts as resolved")
	assert.NotEmpty(t, creator.calls)
}

func TestDequeueNextQueuedItems_should_StartItem_When_BlockerDeleted(t *testing.T) {
	svc, _, blockerID, dependentID := blockedDequeueFixture(t)
	ctx := context.Background()
	require.NoError(t, svc.DequeueNextQueuedItems(ctx))
	require.Equal(t, "queued", itemStatus(t, svc, dependentID))

	_, err := svc.DeleteBacklogItem(ctx, connect.NewRequest(&sessionv1.DeleteBacklogItemRequest{ItemId: blockerID}))
	require.NoError(t, err)
	require.NoError(t, svc.DequeueNextQueuedItems(ctx))

	assert.Equal(t, "in_progress", itemStatus(t, svc, dependentID))
}

func TestTransitionBacklogItemStatus_should_RejectInProgress_When_BlockerUnresolved(t *testing.T) {
	svc, _, _, dependentID := blockedDequeueFixture(t)

	_, err := svc.TransitionBacklogItemStatus(context.Background(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: dependentID, TargetStatus: "in_progress", ExpectedStatus: "queued",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
