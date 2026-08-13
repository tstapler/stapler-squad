package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
)

// createStuckTestItem creates a bare backlog item in the given status, for use
// as the parent of BacklogStuckState rows under test.
func createStuckTestItem(t *testing.T, repo *EntRepository, ctx context.Context, status BacklogStatus) string {
	t.Helper()
	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "stuck-state test item",
		Status: string(status),
	})
	require.NoError(t, err)
	return item.ID
}

// TestMarkStuck_should_insertOpenRowWithNullNotified_When_NoExistingRow verifies
// a fresh MarkStuck call inserts exactly one row with first_detected_at/
// last_checked_at/context set and notified_at/resolved_at left null.
func TestMarkStuck_should_insertOpenRowWithNullNotified_When_NoExistingRow(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "PR #148 green & mergeable")
	require.NoError(t, err)
	assert.True(t, applied)

	row, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, itemID, row.ItemID.String())
	assert.Equal(t, string(domain.StuckReasonPRReadyUnmerged), row.Reason)
	assert.Equal(t, "PR #148 green & mergeable", row.Context)
	assert.Nil(t, row.NotifiedAt)
	assert.Nil(t, row.ResolvedAt)
	assert.WithinDuration(t, time.Now(), row.FirstDetectedAt, 5*time.Second)
	assert.WithinDuration(t, time.Now(), row.LastCheckedAt, 5*time.Second)
}

// TestMarkStuck_should_updateLastCheckedOnly_When_OpenRowExists verifies a
// second MarkStuck on the same still-open (item_id, reason) pair updates only
// last_checked_at (and context), leaving first_detected_at unchanged — the
// plain 2-column unique index guarantees no duplicate row is created.
func TestMarkStuck_should_updateLastCheckedOnly_When_OpenRowExists(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "first pass")
	require.NoError(t, err)
	require.True(t, applied)

	first, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	firstDetected := first.FirstDetectedAt

	// Force a measurable delta.
	time.Sleep(10 * time.Millisecond)

	applied, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "second pass")
	require.NoError(t, err)
	require.True(t, applied)

	count, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "no duplicate row should be created")

	second, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.True(t, second.FirstDetectedAt.Equal(firstDetected), "first_detected_at must not change on refresh")
	assert.True(t, second.LastCheckedAt.After(first.LastCheckedAt) || second.LastCheckedAt.Equal(first.LastCheckedAt))
	assert.Equal(t, "second pass", second.Context)
}

// TestMarkStuck_should_reopenRowInPlace_When_ExistingRowResolved verifies that
// re-marking a resolved (item_id, reason) pair reopens the SAME row: resolved_at
// and notified_at are cleared and first_detected_at is reset to the new onset
// time, never inserting a second row.
func TestMarkStuck_should_reopenRowInPlace_When_ExistingRowResolved(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing x3")
	require.NoError(t, err)
	require.True(t, applied)

	notified, err := repo.MarkStuckNotified(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	require.True(t, notified)

	resolved, err := repo.ResolveStuck(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	require.True(t, resolved)

	resolvedRow, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	originalID := resolvedRow.ID
	require.NotNil(t, resolvedRow.ResolvedAt)
	require.NotNil(t, resolvedRow.NotifiedAt)

	time.Sleep(10 * time.Millisecond)

	applied, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing again")
	require.NoError(t, err)
	require.True(t, applied)

	count, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one row per (item_id, reason) at all times")

	reopened, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalID, reopened.ID, "the same row must be reopened, not a new one")
	assert.Nil(t, reopened.ResolvedAt)
	assert.Nil(t, reopened.NotifiedAt)
	assert.True(t, reopened.FirstDetectedAt.After(resolvedRow.FirstDetectedAt), "first_detected_at must reset on reopen")
}

// TestMarkStuck_should_keepExactlyOneRow_When_ResolvedThenRecurs is a second,
// tick-shaped regression covering the same resolve-in-place invariant as
// above: resolved, then recurs, must never produce a 2nd row.
func TestMarkStuck_should_keepExactlyOneRow_When_ResolvedThenRecurs(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	for i := 0; i < 3; i++ {
		applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonStaleWork, BacklogStatusInProgress, "stale")
		require.NoError(t, err)
		require.True(t, applied)

		resolved, err := repo.ResolveStuck(ctx, itemID, domain.StuckReasonStaleWork)
		require.NoError(t, err)
		require.True(t, resolved)
	}

	count, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestMarkStuck_should_returnAppliedFalse_When_StatusPreconditionMismatch
// verifies MarkStuck's best-effort status precondition: a mismatched
// expectedStatus returns (false, nil) and writes nothing.
func TestMarkStuck_should_returnAppliedFalse_When_StatusPreconditionMismatch(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonReworkCap, BacklogStatusReview, "cap hit")
	require.NoError(t, err)
	assert.False(t, applied)

	count, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no row should be written on precondition mismatch")
}

// TestResolveStuck_should_setResolvedAtOnce_When_RowOpen verifies ResolveStuck
// is a single atomic UPDATE ... WHERE resolved_at IS NULL, and the resolved
// row drops out of FindOpenStuckStates.
func TestResolveStuck_should_setResolvedAtOnce_When_RowOpen(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	_, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "ready")
	require.NoError(t, err)

	resolved, err := repo.ResolveStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)
	assert.True(t, resolved)

	row, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, row.ResolvedAt)

	open, err := repo.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open)
}

// TestResolveStuck_should_beNoOpAndNotOverwrite_When_AlreadyResolved verifies
// a second ResolveStuck call is a no-op: affected-rows=0, no error, and the
// original resolved_at timestamp is preserved unchanged.
func TestResolveStuck_should_beNoOpAndNotOverwrite_When_AlreadyResolved(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	_, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "ready")
	require.NoError(t, err)

	resolved, err := repo.ResolveStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)
	require.True(t, resolved)

	first, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	firstResolvedAt := *first.ResolvedAt

	resolvedAgain, err := repo.ResolveStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)
	assert.False(t, resolvedAgain, "second resolve should affect 0 rows")

	second, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, second.ResolvedAt)
	assert.True(t, second.ResolvedAt.Equal(firstResolvedAt), "resolved_at must not be overwritten")
}

// TestFindOpenStuckStates_should_excludeResolvedAndSnoozed_When_Queried
// verifies the projection filter: of 6 open rows plus one snoozed-until-tomorrow
// and one resolved, only the 5 genuinely open+un-snoozed rows are returned,
// each carrying item title/status/pr_number/pr_url.
func TestFindOpenStuckStates_should_excludeResolvedAndSnoozed_When_Queried(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	// 6 open rows across distinct items/reasons.
	openItemIDs := make([]string, 0, 6)
	reasons := []domain.StuckReason{
		domain.StuckReasonPRReadyUnmerged,
		domain.StuckReasonReworkCap,
		domain.StuckReasonAbandonedReview,
		domain.StuckReasonStaleWork,
		domain.StuckReasonBouncing,
		domain.StuckReasonPushFailed,
	}
	statuses := []BacklogStatus{
		BacklogStatusPRPending,
		BacklogStatusReview,
		BacklogStatusReview,
		BacklogStatusInProgress,
		BacklogStatusInProgress,
		BacklogStatusReview,
	}
	for i, reason := range reasons {
		itemID := createStuckTestItem(t, repo, ctx, statuses[i])
		applied, err := repo.MarkStuck(ctx, itemID, reason, statuses[i], "context")
		require.NoError(t, err)
		require.True(t, applied)
		openItemIDs = append(openItemIDs, itemID)
	}

	// One additional row snoozed until tomorrow.
	snoozedItemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	applied, err := repo.MarkStuck(ctx, snoozedItemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "snoozed")
	require.NoError(t, err)
	require.True(t, applied)
	snoozedUUID, err := uuid.Parse(snoozedItemID)
	require.NoError(t, err)
	_, err = repo.client.BacklogStuckState.Update().
		Where(backlogstuckstate.ItemIDEQ(snoozedUUID)).
		SetSnoozedUntil(time.Now().Add(24 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	// One additional row resolved.
	resolvedItemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	applied, err = repo.MarkStuck(ctx, resolvedItemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "resolved")
	require.NoError(t, err)
	require.True(t, applied)
	_, err = repo.ResolveStuck(ctx, resolvedItemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)

	open, err := repo.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 6)

	seenItemIDs := make(map[string]bool, len(open))
	for _, row := range open {
		seenItemIDs[row.ItemID] = true
		assert.Equal(t, "stuck-state test item", row.ItemTitle)
		assert.NotEmpty(t, row.ItemStatus)
	}
	for _, id := range openItemIDs {
		assert.True(t, seenItemIDs[id], "expected open item %s to be present", id)
	}
	assert.False(t, seenItemIDs[snoozedItemID], "snoozed item must be excluded")
	assert.False(t, seenItemIDs[resolvedItemID], "resolved item must be excluded")
}

// TestMarkStuckNotified_should_setNotifiedAt_When_Null verifies
// MarkStuckNotified sets notified_at=now only where it was previously null.
func TestMarkStuckNotified_should_setNotifiedAt_When_Null(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	_, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "ready")
	require.NoError(t, err)

	row, err := repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.Nil(t, row.NotifiedAt)

	applied, err := repo.MarkStuckNotified(ctx, itemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)
	assert.True(t, applied)

	row, err = repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, row.NotifiedAt)
	first := *row.NotifiedAt

	// Second call is a no-op — notified_at is only set where null.
	applied, err = repo.MarkStuckNotified(ctx, itemID, domain.StuckReasonPRReadyUnmerged)
	require.NoError(t, err)
	assert.False(t, applied)

	row, err = repo.client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.True(t, row.NotifiedAt.Equal(first))
}

// TestFindOpenStuckStates_should_returnRowsAfterDBReopen_When_ProcessRestarts
// mirrors TestEntRepository_UUID_SurvivesDBReopen: open a stuck row, close and
// reopen the repository from the same DB file (simulating a server restart),
// and confirm the row is still open with first_detected_at preserved.
func TestFindOpenStuckStates_should_returnRowsAfterDBReopen_When_ProcessRestarts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-stuck-restart.db")

	var itemID string
	var firstDetected time.Time
	func() {
		repo, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()

		ctx := context.Background()
		itemID = createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

		applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "PR #148 green & mergeable")
		require.NoError(t, err)
		require.True(t, applied)

		row, err := repo.client.BacklogStuckState.Query().Only(ctx)
		require.NoError(t, err)
		firstDetected = row.FirstDetectedAt
	}()

	repo2, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	open, err := repo2.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, itemID, open[0].ItemID)
	assert.True(t, open[0].FirstDetectedAt.Equal(firstDetected),
		"first_detected_at must survive DB close/reopen (simulated server restart)")
}
