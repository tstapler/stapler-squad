package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

const dequeueCtx = domain.DequeueSpawnFailedContextPrefix + "could not spawn (boom)"

func openRow(t *testing.T, er *EntRepository, itemID string, reason domain.StuckReason) (OpenStuckStateData, bool) {
	t.Helper()
	rows, err := er.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	for _, r := range rows {
		if r.ItemID == itemID && r.Reason == reason {
			return r, true
		}
	}
	return OpenStuckStateData{}, false
}

// A dequeue-kind spawn_failed row anchors at ready/queued (and in_progress, the
// transient claim state): once the item sits anywhere else the sweep resolves it.
func TestSelfHealStuck_DequeueSpawnFailedRow_AnchoredToReadyQueuedInProgress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status   BacklogStatus
		wantOpen bool
	}{
		{BacklogStatusReady, true},
		{BacklogStatusQueued, true},
		{BacklogStatusInProgress, true}, // mid-claim: resolving here would erase the backoff count
		{BacklogStatusIdea, false},
		{BacklogStatusReview, false},
		{BacklogStatusPRPending, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			storage, cleanup := createTestStorage(t)
			defer cleanup()
			ctx := context.Background()
			item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "x", Status: string(tc.status)})
			require.NoError(t, err)
			applied, err := storage.repo.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, tc.status, dequeueCtx)
			require.NoError(t, err)
			require.True(t, applied)

			NewBacklogLifecycleListener(storage).selfHealStuck(ctx, storage.repo)

			_, open := openRow(t, storage.repo, item.ID, domain.StuckReasonSpawnFailed)
			assert.Equal(t, tc.wantOpen, open)
		})
	}
}

// The review-reopen use of spawn_failed (item stranded in_progress after a failed
// rollback) must keep its existing "stays open until an event site resolves it" semantics.
func TestSelfHealStuck_ReviewReopenSpawnFailedRow_IsNotAnchored(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "x", Status: string(BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = storage.repo.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, BacklogStatusInProgress,
		"a rework session failed to spawn (x) and the automatic rollback to review also failed")
	require.NoError(t, err)

	NewBacklogLifecycleListener(storage).selfHealStuck(ctx, storage.repo)

	_, open := openRow(t, storage.repo, item.ID, domain.StuckReasonSpawnFailed)
	assert.True(t, open)
}

// A later failure reopening a resolved spawn_failed row must start from a fresh
// attempt count, not the stale one the previous episode left behind.
func TestMarkStuck_ReopeningResolvedSpawnFailedRow_ResetsRemediationCounters(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "x", Status: string(BacklogStatusReady)})
	require.NoError(t, err)
	_, err = storage.repo.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, BacklogStatusReady, dequeueCtx)
	require.NoError(t, err)
	next := time.Now().Add(time.Hour)
	_, err = storage.repo.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonSpawnFailed, 4, &next)
	require.NoError(t, err)
	_, err = storage.repo.ResolveStuck(ctx, item.ID, domain.StuckReasonSpawnFailed) // e.g. selfHeal or manual fix
	require.NoError(t, err)

	_, err = storage.repo.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, BacklogStatusReady, dequeueCtx)
	require.NoError(t, err)

	row, ok := openRow(t, storage.repo, item.ID, domain.StuckReasonSpawnFailed)
	require.True(t, ok)
	assert.Zero(t, row.RemediationAttempts)
	assert.Nil(t, row.NextRemediationAt)
}

// RemediationBlocked is read from durable state, so a parked/backed-off item must
// stay blocked after the process restarts (DB closed and reopened).
func TestRemediationBlocked_SurvivesDBReopen(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "blocked-restart.db")
	ctx := context.Background()
	var itemID string
	func() {
		repo, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()
		storage, err := NewStorageWithRepository(repo)
		require.NoError(t, err)
		item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "x", Status: string(BacklogStatusReady)})
		require.NoError(t, err)
		itemID = item.ID
		_, err = storage.MarkStuck(ctx, itemID, domain.StuckReasonSpawnFailed, BacklogStatusReady, dequeueCtx)
		require.NoError(t, err)
		due, _, err := storage.RemediationDue(ctx, itemID, domain.StuckReasonSpawnFailed)
		require.NoError(t, err)
		require.True(t, due)
	}()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo.Close()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	blocked, err := storage.RemediationBlocked(ctx, itemID, domain.StuckReasonSpawnFailed)
	require.NoError(t, err)
	assert.True(t, blocked, "backoff deadline must outlive a restart")
	batch, err := storage.RemediationBlockedItems(ctx, domain.StuckReasonSpawnFailed)
	require.NoError(t, err)
	assert.True(t, batch[itemID])
}

func TestRemediationBlockedItems_ReturnsOnlyBlockedRowsOfThatReason(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	mk := func(reason domain.StuckReason, consume bool) string {
		item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "x", Status: string(BacklogStatusReady)})
		require.NoError(t, err)
		_, err = storage.MarkStuck(ctx, item.ID, reason, BacklogStatusReady, "c")
		require.NoError(t, err)
		if consume {
			_, _, err = storage.RemediationDue(ctx, item.ID, reason)
			require.NoError(t, err)
		}
		return item.ID
	}
	blocked := mk(domain.StuckReasonSpawnFailed, true)
	fresh := mk(domain.StuckReasonSpawnFailed, false)
	otherReason := mk(domain.StuckReasonPushFailed, true)

	got, err := storage.RemediationBlockedItems(ctx, domain.StuckReasonSpawnFailed)
	require.NoError(t, err)
	assert.True(t, got[blocked])
	assert.False(t, got[fresh])
	assert.False(t, got[otherReason])
}

func TestIsDequeueSpawnFailedContext(t *testing.T) {
	t.Parallel()
	assert.True(t, domain.IsDequeueSpawnFailedContext(dequeueCtx))
	assert.False(t, domain.IsDequeueSpawnFailedContext("a rework session failed to spawn"))
}
