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

// If this fails, a caller on a request goroutine can be parked indefinitely by
// one busy actor: sendSyncErr (which plain SetArchivedAtIfNil uses) has no
// deadline at all, so ArchiveWorkflowSessions' per-instance loop would stall the
// whole RPC on a single instance mid-Start.
func TestSetArchivedAtIfNilCtx_should_ReturnCtxError_When_ActorIsBusy(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "archive-ctx-busy", Status: Stopped}
	li := NewLiveInstance(inst)
	t.Cleanup(li.cancel)

	// Occupy the actor goroutine so no further command can be executed.
	occupied := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	inst.send(func(*instanceState) {
		close(occupied)
		<-release
	})
	<-occupied

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := inst.SetArchivedAtIfNilCtx(ctx, time.Now())
		errCh <- err
	}()
	cancel() // stands in for the caller's deadline firing

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("SetArchivedAtIfNilCtx did not return once its context was cancelled")
	}
}

// If this fails, the bounded variant has diverged from SetArchivedAtIfNil's CAS
// semantics on the normal (uncontended) path.
func TestSetArchivedAtIfNilCtx_should_ApplyCASOnce_When_ActorIsIdle(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "archive-ctx-idle", Status: Stopped}
	li := NewLiveInstance(inst)
	t.Cleanup(li.cancel)

	ctx := context.Background()
	set, err := inst.SetArchivedAtIfNilCtx(ctx, time.Now())
	require.NoError(t, err)
	assert.True(t, set, "first archive should apply (CAS)")
	assert.True(t, inst.IsArchived(), "IsArchived() must see the published snapshot")

	set, err = inst.SetArchivedAtIfNilCtx(ctx, time.Now())
	require.NoError(t, err)
	assert.False(t, set, "second archive is a no-op (CAS)")
}
