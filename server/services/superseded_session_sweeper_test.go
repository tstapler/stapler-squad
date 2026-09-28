package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// timePtr addresses t -- Go can't take &now.Add(-x) directly.
func timePtr(t time.Time) *time.Time { return &t }

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
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{})
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
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{})
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
		// Recent, not -2h: Story 6.1.1's findIdleStaleSessions now also runs
		// on this same tick with a 2h default staleness threshold, and this
		// round must stay "current" (superseded-only, not idle-stale) for
		// this test's own AC — a -2h "current" round would sit right on that
		// threshold's edge and could be archived by the wrong code path.
		now.Add(-2 * time.Minute),
	})
	current := uuids[len(uuids)-1]

	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{})
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
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{})

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

// --- Story 6.1.1: findIdleStaleSessions ---

// TestFindIdleStaleSessions_ShouldReturnSession_WhenIdleSustainedWithNoNewerRound
// is the validation.md-named test: a 2h-idle session with no newer round for
// its item+role is returned.
func TestFindIdleStaleSessions_ShouldReturnSession_WhenIdleSustainedWithNoNewerRound(t *testing.T) {
	t.Parallel()
	now := time.Now()

	idleWork := session.ItemSessionSummary{
		SessionUUID:    "work-1",
		Role:           session.SessionRoleWork,
		CreatedAt:      now.Add(-3 * time.Hour),
		LastProgressAt: timePtr(now.Add(-2 * time.Hour)),
	}

	got := findIdleStaleSessions([]session.ItemSessionSummary{idleWork}, time.Hour, now)
	require.Len(t, got, 1)
	assert.Equal(t, "work-1", got[0].SessionUUID)
}

// TestFindIdleStaleSessions_ShouldReturnNothing_WhenNewerRoundExists covers
// the no-double-handling AC: once a newer round exists, the older one is
// findSupersededSessions' job, not this predicate's.
func TestFindIdleStaleSessions_ShouldReturnNothing_WhenNewerRoundExists(t *testing.T) {
	t.Parallel()
	now := time.Now()

	older := session.ItemSessionSummary{
		SessionUUID:    "work-1",
		Role:           session.SessionRoleWork,
		CreatedAt:      now.Add(-3 * time.Hour),
		LastProgressAt: timePtr(now.Add(-2 * time.Hour)),
	}
	newer := session.ItemSessionSummary{
		SessionUUID: "work-2",
		Role:        session.SessionRoleWork,
		CreatedAt:   now.Add(-30 * time.Minute),
	}

	got := findIdleStaleSessions([]session.ItemSessionSummary{older, newer}, time.Hour, now)
	assert.Empty(t, got)
}

// TestFindIdleStaleSessions_ShouldReturnNothing_WhenWithinThreshold covers a
// current round that simply hasn't been idle long enough yet.
func TestFindIdleStaleSessions_ShouldReturnNothing_WhenWithinThreshold(t *testing.T) {
	t.Parallel()
	now := time.Now()

	fresh := session.ItemSessionSummary{
		SessionUUID:    "work-1",
		Role:           session.SessionRoleWork,
		CreatedAt:      now.Add(-3 * time.Hour),
		LastProgressAt: timePtr(now.Add(-10 * time.Minute)),
	}

	got := findIdleStaleSessions([]session.ItemSessionSummary{fresh}, time.Hour, now)
	assert.Empty(t, got)
}

// TestFindIdleStaleSessions_ShouldReturnNothing_WhenSessionAlreadyEnded covers
// a session that finished normally (EndedAt set) -- not this sweep's concern.
func TestFindIdleStaleSessions_ShouldReturnNothing_WhenSessionAlreadyEnded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	endedAt := now.Add(-2 * time.Hour)

	ended := session.ItemSessionSummary{
		SessionUUID: "work-1",
		Role:        session.SessionRoleWork,
		CreatedAt:   now.Add(-3 * time.Hour),
		EndedAt:     &endedAt,
	}

	got := findIdleStaleSessions([]session.ItemSessionSummary{ended}, time.Hour, now)
	assert.Empty(t, got)
}

// TestSupersededSessionSweeper_should_ArchiveIdleStaleSession_When_NoNewerRoundExists
// exercises Story 6.1.1's wiring end-to-end through sweep(): with no handoff
// generator/instance lookup wired, a findIdleStaleSessions match is archived
// directly.
func TestSupersededSessionSweeper_should_ArchiveIdleStaleSession_When_NoNewerRoundExists(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "idle stale item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "idle-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, session.TestBackdateItemSessionCreatedAt(t, storage, is.ID, time.Now().Add(-3*time.Hour)))

	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{StaleThreshold: time.Hour})
	sweeper.sweep(t.Context())

	assert.Contains(t, stopper.archivedUUIDs, "idle-uuid")
}

// TestSupersededSessionSweeper_should_SkipLiveIdleStaleSession_When_SessionStillAttached
// mirrors AC4's superseded-session guard for the idle-stale path: a
// confirmed-live session is never archived out from under a user.
func TestSupersededSessionSweeper_should_SkipLiveIdleStaleSession_When_SessionStillAttached(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "idle stale item, live",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "idle-uuid-live",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, session.TestBackdateItemSessionCreatedAt(t, storage, is.ID, time.Now().Add(-3*time.Hour)))

	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"idle-uuid-live": true}}
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{StaleThreshold: time.Hour})
	sweeper.sweep(t.Context())

	assert.NotContains(t, stopper.archivedUUIDs, "idle-uuid-live")
}

// --- Story 6.1.5: findStalledDiagnoseDispatches ---

// TestFindStalledDiagnoseDispatches_ShouldReturnDispatch_WhenPastThresholdAndSessionDead
// covers the AC's core case.
func TestFindStalledDiagnoseDispatches_ShouldReturnDispatch_WhenPastThresholdAndSessionDead(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d := DiagnoseDispatchRecord{ID: "d1", DiagnosticSessionUUID: "sess-1", CreatedAt: now.Add(-45 * time.Minute)}
	isLive := func(string) bool { return false }

	got := findStalledDiagnoseDispatches([]DiagnoseDispatchRecord{d}, isLive, 30*time.Minute, now)
	require.Len(t, got, 1)
	assert.Equal(t, "d1", got[0].ID)
}

// TestFindStalledDiagnoseDispatches_ShouldReturnNothing_WhenSessionStillLive
// covers a genuinely long-running investigation: still-live means never
// stalled, regardless of age.
func TestFindStalledDiagnoseDispatches_ShouldReturnNothing_WhenSessionStillLive(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d := DiagnoseDispatchRecord{ID: "d1", DiagnosticSessionUUID: "sess-1", CreatedAt: now.Add(-45 * time.Minute)}
	isLive := func(string) bool { return true }

	got := findStalledDiagnoseDispatches([]DiagnoseDispatchRecord{d}, isLive, 30*time.Minute, now)
	assert.Empty(t, got)
}

// TestFindStalledDiagnoseDispatches_ShouldReturnNothing_WhenWithinThreshold
// covers a dispatch too young to distinguish "stalled" from "a session that
// hasn't started yet."
func TestFindStalledDiagnoseDispatches_ShouldReturnNothing_WhenWithinThreshold(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d := DiagnoseDispatchRecord{ID: "d1", DiagnosticSessionUUID: "sess-1", CreatedAt: now.Add(-5 * time.Minute)}
	isLive := func(string) bool { return false }

	got := findStalledDiagnoseDispatches([]DiagnoseDispatchRecord{d}, isLive, 30*time.Minute, now)
	assert.Empty(t, got)
}

// TestSupersededSessionSweeper_should_MarkDispatchStalled_When_PastThresholdAndSessionDead
// exercises Story 6.1.5's wiring end-to-end through sweep(): MarkStalled is
// persisted and a live notification is published exactly once.
func TestSupersededSessionSweeper_should_MarkDispatchStalled_When_PastThresholdAndSessionDead(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	dispatchStore := NewDiagnoseDispatchStore(storage)

	_, err := dispatchStore.Record(t.Context(), DiagnoseDispatchRequest{
		ItemID:                "item-stalled-1",
		TargetSessionUUID:     "target-uuid",
		DiagnosticSessionUUID: "diagnostic-uuid-1",
	})
	require.NoError(t, err)

	notifier := &fakeDiagnoseNotifier{}
	stopper := &mockSessionStopper{}
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{
		DispatchStore: dispatchStore,
		Notifier:      notifier,
		// Effectively zero without triggering NewSupersededSessionSweeper's
		// "<=0 means use the default" fallback -- any dispatch is already
		// older than 1ns by the time sweep() reads it back.
		StalledThreshold: time.Nanosecond,
	})

	sweeper.sweep(t.Context())

	rows, err := dispatchStore.ListByItem(t.Context(), "item-stalled-1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusStalled, rows[0].Status)
	assert.Nil(t, rows[0].Outcome)
	require.NotNil(t, rows[0].CompletedAt)
	assert.Equal(t, 1, notifier.callCount())
}

// TestSupersededSessionSweeper_should_LeaveDispatchAlone_When_SessionStillLive
// covers the live-session guard through the full sweep wiring.
func TestSupersededSessionSweeper_should_LeaveDispatchAlone_When_SessionStillLive(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	dispatchStore := NewDiagnoseDispatchStore(storage)

	_, err := dispatchStore.Record(t.Context(), DiagnoseDispatchRequest{
		ItemID:                "item-stalled-2",
		TargetSessionUUID:     "target-uuid",
		DiagnosticSessionUUID: "diagnostic-uuid-2",
	})
	require.NoError(t, err)

	notifier := &fakeDiagnoseNotifier{}
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"diagnostic-uuid-2": true}}
	sweeper := NewSupersededSessionSweeper(storage, stopper, SupersededSessionSweeperDeps{
		DispatchStore:    dispatchStore,
		Notifier:         notifier,
		StalledThreshold: time.Nanosecond,
	})

	sweeper.sweep(t.Context())

	rows, err := dispatchStore.ListByItem(t.Context(), "item-stalled-2")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusPending, rows[0].Status, "a still-live diagnostic session must never be flagged stalled")
	assert.Equal(t, 0, notifier.callCount())
}
