package services

import (
	"context"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
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

// TestDequeueNextQueuedItems_should_NotStartItem_When_PlanArtifactsPathDeletedAfterApproval
// is the defense-in-depth half of the fix for backlog item
// 1cfae441-8953-41e6-803d-959062119d4f: a plan_artifacts_path that was valid
// when the plan was approved can be deleted later (e.g. its triage worktree
// reaped by CleanupTerminalItem after the item went terminal and was then
// reopened). The dequeue sweep must leave the item queued rather than
// claiming it into in_progress with no readable plan content.
func TestDequeueNextQueuedItems_should_NotStartItem_When_PlanArtifactsPathDeletedAfterApproval(t *testing.T) {
	svc, creator, repoPath := newDequeueClaimService(t, &fakeClaimChecker{verdict: NewUnclaimedVerdict()})
	svc.claimDedupFlag = func() bool { return false }
	ctx := context.Background()

	planDir := t.TempDir()
	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "item with deleted plan artifacts",
		RepoPath:           repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	planArtifactsPath := planDir
	_, err = svc.storage.UpdateBacklogItem(ctx, itemID, session.BacklogItemUpdate{PlanArtifactsPath: &planArtifactsPath}, nil)
	require.NoError(t, err)
	_, err = svc.ApprovePlan(ctx, connect.NewRequest(&sessionv1.ApprovePlanRequest{ItemId: itemID}))
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: itemID, TargetStatus: "ready"}))
	require.NoError(t, err)
	item, err := svc.storage.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)
	_, err = svc.queueBacklogItem(ctx, item, false)
	require.NoError(t, err)

	// Simulate the triage worktree being reaped after the plan was approved.
	require.NoError(t, os.RemoveAll(planDir))

	require.NoError(t, svc.DequeueNextQueuedItems(ctx))

	assert.Equal(t, "queued", itemStatus(t, svc, itemID), "item with a now-missing plan artifacts path must not be dequeued into in_progress")
	assert.Empty(t, creator.calls, "no work session may be spawned when the plan can no longer be read")
}

func TestTransitionBacklogItemStatus_should_RejectInProgress_When_BlockerUnresolved(t *testing.T) {
	svc, _, _, dependentID := blockedDequeueFixture(t)

	_, err := svc.TransitionBacklogItemStatus(context.Background(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: dependentID, TargetStatus: "in_progress", ExpectedStatus: "queued",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
