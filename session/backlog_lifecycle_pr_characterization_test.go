package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git"
)

// Characterization tests for the single-PR scalar paths (T5.1.1). They pin
// today's behavior with every multi-PR scope off so Epic 5 cannot change it.

func TestReconcilePRPending_ShouldMoveToDone_WhenSingleMergedPRFlagOff(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 21)
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/char-merged", "")

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/char-merged")

	listener.ReconcilePRPending(ctx, storage.repo)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status)
	assert.Equal(t, 21, fetched.PrNumber, "the scalar stays as recorded")
}

func TestReconcilePRPending_ShouldStayPending_WhenSinglePRNotMergedFlagOff(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 21)
	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: false, status: &git.PRStatus{}})

	listener.ReconcilePRPending(ctx, storage.repo)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
}

func TestFindPRPendingItems_ShouldReturnOnlyPRPendingWithPositiveNumber(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	withPR := newPRPendingTestItem(t, storage, 21)
	noPR, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title: "pr_pending, no number", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, BacklogItemData{
		Title: "review", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(BacklogStatusReview), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)

	items, err := storage.repo.FindPRPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, withPR.ID, items[0].ID.String())
	assert.NotEqual(t, noPR.ID, items[0].ID.String())
}

func TestReconcilePRPendingWithoutPRItems_ShouldFlagStuck_WhenNumberZero(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title: "pr_pending, no number", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	listener.reconcilePRPendingWithoutPRItems(ctx, storage.repo)

	rows, err := storage.repo.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(rows, item.ID, "pr_pending_no_pr")
	assert.True(t, ok, "empty set with pr_number=0 raises StuckReasonPRPendingNoPR")
}
