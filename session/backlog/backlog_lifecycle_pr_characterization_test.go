package backlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
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

	listener.ReconcilePRPending(ctx, storage.Repo)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), fetched.Status)
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

	listener.ReconcilePRPending(ctx, storage.Repo)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

func TestFindPRPendingItems_ShouldReturnOnlyPRPendingWithPositiveNumber(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	withPR := newPRPendingTestItem(t, storage, 21)
	noPR, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title: "pr_pending, no number", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(session.BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title: "review", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(session.BacklogStatusReview), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)

	items, err := storage.Repo.FindPRPendingItems(ctx)
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

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title: "pr_pending, no number", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(session.BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&sessiontest.FakeNotifier{})
	listener.reconcilePRPendingWithoutPRItems(ctx, storage.Repo)

	rows, err := storage.Repo.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := session.FindOpenStuckStateFor(rows, item.ID, "pr_pending_no_pr")
	assert.True(t, ok, "empty set with pr_number=0 raises StuckReasonPRPendingNoPR")
}

func TestStuckDetector_ShouldRaisePRPendingNoPR_WhenEmptySetAndNumberZero(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{false, true} {
		on := on
		t.Run(map[bool]string{false: "flag off", true: "reader:backlog"}[on], func(t *testing.T) {
			t.Parallel()
			f := newItemPRFixture(t, 0)
			listener := NewBacklogLifecycleListener(f.storage)
			listener.backlogReaderOverride = func() bool { return on }
			listener.SetNotifier(&sessiontest.FakeNotifier{})
			listener.reconcilePRPendingWithoutPRItems(context.Background(), f.repo)
			rows, err := f.repo.FindOpenStuckStates(context.Background())
			require.NoError(t, err)
			_, ok := session.FindOpenStuckStateFor(rows, f.itemID, "pr_pending_no_pr")
			assert.True(t, ok)
		})
	}
}

func TestStuckDetector_ShouldNotRaise_WhenNumberZeroButSetHasItemDirectPR(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	f.link(session.ItemPROwner(f.itemID), 21, session.PRStateOpen)
	listener := NewBacklogLifecycleListener(f.storage)
	listener.backlogReaderOverride = func() bool { return true }
	listener.SetNotifier(&sessiontest.FakeNotifier{})
	listener.reconcilePRPendingWithoutPRItems(context.Background(), f.repo)
	rows, err := f.repo.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	_, ok := session.FindOpenStuckStateFor(rows, f.itemID, "pr_pending_no_pr")
	assert.False(t, ok, "the empty-set condition replaces pr_number==0 under the reader")
}

func TestStuckDetector_ShouldRaise_WhenNumberSetButSetEmptiedByDetach(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 30)
	s := f.session(session.SessionRoleWork)
	f.link(session.SessionPROwner(s), 30, session.PRStateOpen)
	f.detach(session.SessionPROwner(s), 30)
	listener := NewBacklogLifecycleListener(f.storage)
	listener.backlogReaderOverride = func() bool { return true }
	listener.SetNotifier(&sessiontest.FakeNotifier{})
	listener.reconcilePRPendingWithoutPRItems(context.Background(), f.repo)
	rows, err := f.repo.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	_, ok := session.FindOpenStuckStateFor(rows, f.itemID, "pr_pending_no_pr")
	assert.True(t, ok)
}

func TestReconcileBouncingItems_ShouldHoldDone_WhenTrackedPRMergedButAnotherPROpenUnderReader(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{false, true} {
		on := on
		t.Run(map[bool]string{false: "flag off", true: "reader:backlog"}[on], func(t *testing.T) {
			t.Parallel()
			f := newItemPRFixture(t, 172)
			ctx := context.Background()
			_, err := f.repo.Client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(session.BacklogStatusInProgress)).Save(ctx)
			require.NoError(t, err)
			newTrackedWorkSession(t, f.storage, f.itemID, "/tmp/fake-repo", "backlog/bounce-hold", "")
			f.link(session.ItemPROwner(f.itemID), 172, session.PRStateMerged)
			f.link(session.ItemPROwner(f.itemID), 173, session.PRStateOpen)
			for i := 0; i < 3; i++ {
				_, err = f.storage.RawTransitionBacklogItemStatus(ctx, f.itemID, session.BacklogStatusReview, nil, session.TriggeredBySystem)
				require.NoError(t, err)
				_, err = f.storage.RawTransitionBacklogItemStatus(ctx, f.itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
				require.NoError(t, err)
			}
			listener := NewBacklogLifecycleListener(f.storage)
			listener.backlogReaderOverride = func() bool { return on }
			overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
			stubMatchingPRByNumberFinder(listener, "backlog/bounce-hold")
			listener.SetNotifier(&sessiontest.FakeNotifier{})

			listener.reconcileBouncingItems(ctx, f.repo)

			fetched, err := f.storage.GetBacklogItem(ctx, f.itemID)
			require.NoError(t, err)
			if on {
				assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status, "#173 is open: not done")
			} else {
				assert.Equal(t, string(session.BacklogStatusDone), fetched.Status, "flag off: today's behavior")
			}
		})
	}
}
