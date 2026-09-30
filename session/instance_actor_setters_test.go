package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAutoApprove_should_NotRestart_When_StatusPaused(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Paused

	persisted := false
	err := inst.SetAutoApprove(true, func() error {
		persisted = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, inst.AutoApprove)
	assert.True(t, persisted, "persist must run even when no restart occurs")
}

func TestSetAutoApprove_should_ReturnErrorAndSkipRestart_When_PersistFails(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Active
	inst.started.Store(true)
	persistErr := errors.New("disk full")

	err := inst.SetAutoApprove(true, func() error {
		return persistErr
	})

	require.ErrorIs(t, err, persistErr)
	// Matches the documented ordering (set -> persist -> restart): the in-memory
	// field is already flipped by the time persist runs, but a persist failure
	// must short-circuit before any restart is attempted.
	assert.True(t, inst.AutoApprove)
}

// TestSetAutoApprove_RestartError_PersistAlreadyRan mirrors
// TestSwitchProgram_RestartError_PersistAlreadyRan (session/instance_program_test.go):
// a restart failure must not roll back or hide the already-persisted field change, so
// the caller (UpdateSession) and the persisted store agree on the new value even though
// the session didn't actually restart.
func TestSetAutoApprove_RestartError_PersistAlreadyRan(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Status = Active
	inst.started.Store(true)
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	persistedValue := false
	persistRan := false
	err := inst.SetAutoApprove(true, func() error {
		persistRan = true
		persistedValue = inst.AutoApprove
		return nil
	})

	require.Error(t, err, "Restart should fail with no working directory configured")
	assert.True(t, persistRan, "persist callback must run before the restart attempt")
	assert.True(t, persistedValue, "persist callback must observe the new value, i.e. run after the field is set")
	assert.True(t, inst.AutoApprove, "AutoApprove field must reflect the change even though restart failed")
}

// TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls is the AC7
// regression guard: SetAutoApprove and SwitchProgram must serialize through the shared
// restartTriggerMu so a racing program-switch and auto-approve toggle can't both observe
// Status == Active and double-restart the same tmux session. Proven deterministically (not
// just via -race) by blocking SetAutoApprove mid-persist and asserting a concurrent
// SwitchProgram call cannot proceed until the lock is released.
func TestSetAutoApprove_SerializesWithSwitchProgram_When_ConcurrentCalls(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Status = Active
	inst.started.Store(true)
	inst.Path = "" // forces Restart() to fail deterministically, no real tmux involved

	entered := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = inst.SetAutoApprove(true, func() error {
			close(entered)
			<-release
			return nil
		})
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("SetAutoApprove never reached persist callback")
	}

	done := make(chan struct{})
	go func() {
		_, _, _ = inst.SwitchProgram(context.Background(), "aider", nil)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("SwitchProgram completed while SetAutoApprove held restartTriggerMu — the two setters are not sharing a lock")
	case <-time.After(100 * time.Millisecond):
		// Expected: SwitchProgram is blocked waiting on restartTriggerMu.
	}

	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SwitchProgram never completed after SetAutoApprove released restartTriggerMu")
	}
}

func TestInstance_SetPinned_should_SetPinnedTrue_When_ToggledOn(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)

	require.NoError(t, inst.SetPinned(true))
	assert.True(t, inst.Snapshot().Pinned)

	require.NoError(t, inst.SetPinned(true), "pinning twice must be idempotent")
	require.NoError(t, inst.SetPinned(false))
	require.NoError(t, inst.SetPinned(false), "unpinning twice must be idempotent")
	assert.False(t, inst.Snapshot().Pinned)
}

func TestInstance_SetPinned_should_RejectPin_When_SessionIsArchived(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	now := time.Now()
	inst.SetArchivedAt(&now)

	err := inst.SetPinned(true)

	assert.ErrorIs(t, err, ErrCannotPinArchivedSession)
	assert.False(t, inst.Snapshot().Pinned)
}

func TestInstance_SetArchivedAt_should_ClearPinned_When_Archiving(t *testing.T) {
	t.Parallel()
	inst := minimalInstance(t)
	require.NoError(t, inst.SetPinned(true))

	now := time.Now()
	inst.SetArchivedAt(&now)
	assert.False(t, inst.Snapshot().Pinned)

	inst.SetArchivedAt(nil)
	assert.False(t, inst.Snapshot().Pinned, "unarchiving must not re-pin")
}
