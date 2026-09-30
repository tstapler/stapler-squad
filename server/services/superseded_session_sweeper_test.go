package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session"
)

// seedItemWithRounds creates an in_progress backlog item plus one work-role
// ItemSession per entry in createdAts (in the given order), returning the
// item ID and the created sessions' UUIDs in the same order. Mirrors seeding
// storage directly rather than going through the spawn path -- AC5's
// regression shape: rows persisted before a sweeper ever ran.
func seedItemWithRounds(t *testing.T, storage *session.Storage, createdAts []time.Time) (itemID string, sessionUUIDs []string) {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "multi-round item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	for i, createdAt := range createdAts {
		uuid := "round-uuid-" + time.Duration(i).String()
		is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: uuid,
			SessionRole: session.SessionRoleWork,
		})
		require.NoError(t, err)
		require.NoError(t, session.TestBackdateItemSessionCreatedAt(t, storage, is.ID, createdAt))
		sessionUUIDs = append(sessionUUIDs, uuid)
	}
	return item.ID, sessionUUIDs
}

// TestSupersededSessionSweeper_should_ArchiveOlderRound_When_NewerRoundExists
// is AC3's core case: two work-role rounds for one in_progress item, only the
// older must be archived.
func TestSupersededSessionSweeper_should_ArchiveOlderRound_When_NewerRoundExists(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)

	now := time.Now()
	itemID, uuids := seedItemWithRounds(t, storage, []time.Time{now.Add(-2 * time.Hour), now.Add(-1 * time.Hour)})
	older, newer := uuids[0], uuids[1]

	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper)
	sweeper.sweep(t.Context())

	assert.Contains(t, stopper.archivedUUIDs, older, "the older round must be archived")
	assert.NotContains(t, stopper.archivedUUIDs, newer, "the current round must be left untouched")

	_ = itemID
}

// TestSupersededSessionSweeper_should_SkipLiveSuperseded_When_SessionStillAttached
// is AC4: a superseded round the SessionStopper reports as confirmed-live must
// never be archived out from under a user still attached to it.
func TestSupersededSessionSweeper_should_SkipLiveSuperseded_When_SessionStillAttached(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)

	now := time.Now()
	_, uuids := seedItemWithRounds(t, storage, []time.Time{now.Add(-2 * time.Hour), now.Add(-1 * time.Hour)})
	older, newer := uuids[0], uuids[1]

	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{older: true}}
	sweeper := NewSupersededSessionSweeper(storage, stopper)
	sweeper.sweep(t.Context())

	assert.NotContains(t, stopper.archivedUUIDs, older, "a live/attached superseded session must not be archived")
	assert.NotContains(t, stopper.archivedUUIDs, newer)
}

// TestSupersededSessionSweeper_should_ConvergeOnPreExistingStaleRows is AC5's
// regression test for the 2026-09-14 incident shape: rows seeded directly
// into storage (bypassing the spawn path entirely, as if persisted before
// this sweeper ever ran) must still converge to "only the latest round
// active" within a single sweep tick.
func TestSupersededSessionSweeper_should_ConvergeOnPreExistingStaleRows(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)

	now := time.Now()
	_, uuids := seedItemWithRounds(t, storage, []time.Time{
		now.Add(-10 * time.Hour),
		now.Add(-8 * time.Hour),
		now.Add(-6 * time.Hour),
		now.Add(-4 * time.Hour),
		now.Add(-2 * time.Hour),
	})
	current := uuids[len(uuids)-1]

	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper)
	sweeper.sweep(t.Context()) // one tick

	for _, uuid := range uuids[:len(uuids)-1] {
		assert.Contains(t, stopper.archivedUUIDs, uuid, "every pre-existing stale round must converge within one tick")
	}
	assert.NotContains(t, stopper.archivedUUIDs, current)
}

// TestSupersededSessionSweeper_should_JoinCleanly_When_ContextCancelled is
// AC8: the sweeper's own background goroutine must exit promptly once its
// context is cancelled, leaking nothing (BUG-089's failure class).
func TestSupersededSessionSweeper_should_JoinCleanly_When_ContextCancelled(t *testing.T) {
	// Not t.Parallel(): goleak's baseline must be captured after
	// createTestStorage, whose own DB-connection-pool goroutines
	// (connectionOpener/connectionCleaner) would otherwise be misattributed
	// to the sweeper as a leak -- same ordering rationale as
	// TestShutdown_WaitsForDeleteSessionCleanup_LiveInstanceNil in
	// session_service_test.go.
	storage := createTestStorage(t)
	baseline := goleak.IgnoreCurrent()

	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}

	goleak.VerifyNone(t, baseline)
}
