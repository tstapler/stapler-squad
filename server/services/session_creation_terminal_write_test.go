package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// addCreatingSession inserts a Creating-status instance directly into storage
// (via AddInstance) and returns the corresponding live *session.Instance so a
// test can additionally exercise the in-memory actor path (TryForceStatusIfEpoch).
func addCreatingSession(t *testing.T, storage *session.Storage, title, uuid string) *session.Instance {
	t.Helper()
	inst := &session.Instance{
		Title:     title,
		UUID:      uuid,
		Path:      "/tmp/test",
		Status:    session.Creating,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}

// TestCommitTerminalStatus_should_UpdatePersistedAndInMemory_When_EpochMatches
// covers Task 1.2.4c's happy path: a matching DB epoch updates both the
// persisted row and the in-memory actor state in one commitTerminalStatus call.
func TestCommitTerminalStatus_should_UpdatePersistedAndInMemory_When_EpochMatches(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	inst := addCreatingSession(t, storage, "commit-terminal-match", "uuid-commit-match")

	applied := commitTerminalStatus(context.Background(), storage, inst, 0, session.Active, "")

	assert.True(t, applied)
	assert.Equal(t, session.Active, inst.Status, "in-memory status must be synced")

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, session.Active, loaded[0].Status, "persisted row must be updated")
}

// TestCommitTerminalStatus_should_LeaveBothUntouched_When_EpochIsStale covers
// Task 1.2.4c's negative path: a stale captured epoch (DB-epoch-mismatch) must
// leave the persisted row AND the in-memory actor state untouched — the
// in-memory TryForceStatusIfEpoch call must never even be attempted.
func TestCommitTerminalStatus_should_LeaveBothUntouched_When_EpochIsStale(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	inst := addCreatingSession(t, storage, "commit-terminal-stale", "uuid-commit-stale")

	// The persisted row's creation_epoch defaults to 0; present a stale captured
	// value of 1 (as if a cancel/retry had already bumped the DB row past it).
	applied := commitTerminalStatus(context.Background(), storage, inst, 1, session.Active, "")

	assert.False(t, applied)
	assert.Equal(t, session.Creating, inst.Status, "in-memory status must be untouched when the durable write is rejected")

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, session.Creating, loaded[0].Status, "persisted row must be unchanged when epochs mismatch")
}

// TestCommitTerminalStatus_should_NeverProduceStaleLookingButLiveSession_When_CrashBetweenPersistAndInMemorySync
// is the regression test for pre-mortem.md failure #2 (P1), per Task 1.2.4d: it
// simulates a process crash between the durable persist and the in-memory sync
// by calling storage.UpdateInstanceIfEpoch directly (bypassing
// commitTerminalStatus entirely, modeling "durable write succeeded, process died
// before the in-memory call"), then reloads the instance from storage into a
// fresh registry (no live goroutine involved) and asserts its status is the
// correct terminal value, not Creating — i.e. the Stale-Creation Sweeper would
// never see this row as a stale-candidate at all.
func TestCommitTerminalStatus_should_NeverProduceStaleLookingButLiveSession_When_CrashBetweenPersistAndInMemorySync(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	addCreatingSession(t, storage, "commit-terminal-crash", "uuid-commit-crash")

	// Model the crash: the durable persist succeeds, but the process is killed
	// before commitTerminalStatus can call instance.TryForceStatusIfEpoch.
	applied, err := storage.UpdateInstanceIfEpoch(context.Background(), "uuid-commit-crash", 0, session.Active, "")
	require.NoError(t, err)
	require.True(t, applied, "the durable persist half of the write must succeed")

	// Simulate the process restart: reload from storage into a brand-new,
	// no-live-goroutine view of the world.
	reloaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)

	assert.Equal(t, session.Active, reloaded[0].Status,
		"a reloaded instance must reflect the durably-persisted terminal status, "+
			"not Creating — otherwise the Stale-Creation Sweeper would wrongly treat "+
			"a genuinely-successful session as an orphaned, still-in-progress one")
}
