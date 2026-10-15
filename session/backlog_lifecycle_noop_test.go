package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// newPassItemWithNoopSessions creates a review-status item with a PASS verdict
// and n ended work sessions that made no commits.
func newPassItemWithNoopSessions(t *testing.T, storage *Storage, n int) *BacklogItemData {
	t.Helper()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PASS item that loops",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		is, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID: item.ID, SessionUUID: "work-" + uuid.New().String(), SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))
	}
	rs, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID: item.ID, SessionUUID: "review-" + uuid.New().String(), SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID: item.ID, SessionUUID: "reviewv-" + uuid.New().String(), SessionRole: SessionRoleReview,
	}, ReviewVerdictData{OverallOutcome: ReviewOutcomePass, PerCriterion: `[]`, Summary: "ok"})
	require.NoError(t, err)
	_ = rs
	return item
}

func TestReconcileRepeatedNoopDispatch_should_flagPassItem_When_ThresholdReached(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo
	item := newPassItemWithNoopSessions(t, storage, 3)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileRepeatedNoopDispatch(ctx, er)

	blocked, err := storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	assert.True(t, blocked, "dispatcher gate reads this row")
}

func TestReconcileRepeatedNoopDispatch_should_notFlag_When_BelowThreshold(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item := newPassItemWithNoopSessions(t, storage, 2)

	NewBacklogLifecycleListener(storage).reconcileRepeatedNoopDispatch(ctx, storage.repo)

	blocked, err := storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	assert.False(t, blocked)
}

func TestReconcileRepeatedNoopDispatch_should_honorConfiguredThreshold(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	item := newPassItemWithNoopSessions(t, storage, 2)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNoopDispatchThresholdFn(func() int { return 2 })
	listener.reconcileRepeatedNoopDispatch(ctx, storage.repo)

	blocked, err := storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	assert.True(t, blocked)
}

func TestSelfHealStuck_should_resolveRepeatedNoopDispatch_When_ItemLeavesReviewAndInProgress(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo
	item := newPassItemWithNoopSessions(t, storage, 3)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileRepeatedNoopDispatch(ctx, er)
	listener.selfHealStuck(ctx, er)
	blocked, err := storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	require.True(t, blocked, "row stays open while item is still in review")

	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusArchived,
		&BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReview)}, TriggeredBySystem)
	require.NoError(t, err)
	listener.selfHealStuck(ctx, er)

	blocked, err = storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	assert.False(t, blocked, "archived duplicate must clear the row")
}

func TestReconcileRepeatedNoopDispatch_should_resolve_When_NewCommitLands(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo
	item := newPassItemWithNoopSessions(t, storage, 3)
	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileRepeatedNoopDispatch(ctx, er)

	// A newer work session that committed breaks the no-op run.
	waitForClockDelta(t, 10*time.Millisecond)
	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID: item.ID, SessionUUID: "work-" + uuid.New().String(), SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, er.UpdateItemSessionGitActivity(ctx, is.ID, "sha1", "msg", time.Now(), 1))
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	listener.reconcileRepeatedNoopDispatch(ctx, er)
	blocked, err := storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch)
	require.NoError(t, err)
	assert.False(t, blocked)
}
