package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransitionBacklogItemStatus_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview
// reproduces the live 2026-07-20 incident on backlog item 0fd4a940 ("Backlog
// Rich text", PR #176): AutoReopenAfterFailedReview reads the item (capturing
// its UpdatedAt) while it is still "review", then — delayed behind other DB
// work, exactly what happened in production — issues its review->in_progress
// CAS write only after the item has, in the meantime, already legitimately
// shipped all the way to "done" via a completely different call path. The
// precondition must reject the stale write rather than silently bouncing an
// already-shipped item back into rework.
//
// Before this fix, TransitionBacklogItemStatus checked the precondition
// against a separately-fetched row and then issued an unconditional
// UpdateOneID().Save() — a read-then-write gap wide enough for exactly this
// staleness to slip through unnoticed.
func TestTransitionBacklogItemStatus_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "already-shipped item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	// Simulate AutoReopenAfterFailedReview's read at the top of the function:
	// it captures item.UpdatedAt while status is still "review", well before
	// its eventual (delayed) CAS write below.
	staleUpdatedAt := item.UpdatedAt

	// Meanwhile, the item's real review session's PASS verdict ships it all
	// the way to "done" — a different call path than the stale reopen in
	// flight, with no knowledge of it.
	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, &BacklogItemPrecondition{
		ExpectedStatus: string(BacklogStatusReview),
	}, TriggeredBySystem)
	require.NoError(t, err)
	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, &BacklogItemPrecondition{
		ExpectedStatus: string(BacklogStatusPRPending),
	}, TriggeredBySystem)
	require.NoError(t, err)

	// The stale reopen attempt finally executes, using the precondition it
	// captured before any of the above happened. It must be rejected — not
	// silently applied on top of the now-shipped item.
	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, &BacklogItemPrecondition{
		ExpectedStatus:    string(BacklogStatusReview),
		ExpectedUpdatedAt: &staleUpdatedAt,
		Note:              "auto-reopened after failed review verdict",
	}, TriggeredBySystem)
	require.ErrorIs(t, err, ErrPreconditionFailed)

	final, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), final.Status, "item must remain done, not bounce back to in_progress")
}

// TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently
// exercises the same precondition under genuine goroutine concurrency: two
// callers both read the item while it is "review" and race to transition it
// — one to "done" (a legitimate ship), one to "in_progress" (a stale
// reopen). Exactly one may win; the loser must get ErrPreconditionFailed,
// and the item must land in a single consistent status — never a status
// that neither writer's precondition actually authorized (the "bounce
// through terminal and back" shape of the live incident).
func TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "racing writers item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	results := make(chan error, 2)

	race := func(toStatus BacklogStatus) {
		defer wg.Done()
		startBarrier.Wait()
		_, txErr := repo.TransitionBacklogItemStatus(ctx, item.ID, toStatus, &BacklogItemPrecondition{
			ExpectedStatus: string(BacklogStatusReview),
		}, TriggeredBySystem)
		results <- txErr
	}

	wg.Add(2)
	go race(BacklogStatusDone)
	go race(BacklogStatusInProgress)
	startBarrier.Done()
	wg.Wait()
	close(results)

	var successes, failures int
	for txErr := range results {
		switch {
		case txErr == nil:
			successes++
		case errors.Is(txErr, ErrPreconditionFailed):
			failures++
		default:
			t.Fatalf("unexpected error: %v", txErr)
		}
	}
	assert.Equal(t, 1, successes, "exactly one writer should win the race")
	assert.Equal(t, 1, failures, "the loser must see ErrPreconditionFailed, not a silent overwrite")

	final, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Contains(t, []string{string(BacklogStatusDone), string(BacklogStatusInProgress)}, final.Status,
		"final status must be exactly one writer's intended toStatus, not a third bounced state")
}

// TestBacklogItem_UpdatedAt_should_BeStoredInUTC_When_CreatedOrTransitioned is
// the regression test for a real bug found live via this repo's manual-
// override e2e test (backlog-manual-override.spec.ts): mattn/go-sqlite3
// binds a time.Time CAS-comparison value by formatting it as TEXT in the
// value's own Location (sqlite3.go's statementBind: `case time.Time: b :=
// []byte(v.Format(SQLiteTimestampFormats[0]))`), so a stored Local-zoned
// updated_at (schema previously used the bare time.Now default, which
// returns Local) can never byte-match a `WHERE updated_at = ?` comparison
// built from a protobuf Timestamp — timestamppb.Timestamp.AsTime() always
// returns UTC — even for the exact same instant (time.Now().Equal(time.Now().UTC())
// is true, but their RFC3339Nano-formatted strings differ, e.g.
// "...-07:00" vs "...Z"). Every RPC caller supplying expected_updated_at
// (TransitionBacklogItemStatusRequest, used by the manual-override UI) would
// see this CAS check fail unconditionally, regardless of correctness. Fixed
// by normalizing BacklogItem.updated_at's schema Default/UpdateDefault to
// time.Now().UTC().
func TestBacklogItem_UpdatedAt_should_BeStoredInUTC_When_CreatedOrTransitioned(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "utc timestamp regression item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	assert.Equal(t, time.UTC, item.UpdatedAt.Location(), "a freshly created item's updated_at must be stored in UTC")

	// The exact scenario that broke: a caller builds a CAS precondition from
	// a protobuf-Timestamp round trip (always UTC, e.g. timestamppb.New(t).AsTime()).
	utcPrecondition := item.UpdatedAt.UTC()
	updated, err := repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, &BacklogItemPrecondition{
		ExpectedStatus:    string(BacklogStatusReview),
		ExpectedUpdatedAt: &utcPrecondition,
	}, TriggeredByUser)
	require.NoError(t, err, "a UTC-derived expected_updated_at must match the stored value's CAS check")
	assert.Equal(t, time.UTC, updated.UpdatedAt.Location(), "updated_at must remain UTC after a transition")
}
