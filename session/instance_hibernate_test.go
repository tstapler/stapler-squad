package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRollbackFailedResume_DoesNotClobberLaterTransition verifies AC1: a
// failed async hibernation-resume must not overwrite a status that already
// moved on legitimately (e.g. Hibernated->Active->Stopped, where the Stopped
// transition lands before the resume goroutine's failure-path rollback
// runs) -- see rollbackFailedResume's doc comment.
func TestRollbackFailedResume_DoesNotClobberLaterTransition(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-rollback-race", Status: Active}
	inst.snapshot.Store(buildSnapshot(inst))

	// Simulate a legitimate transition landing before the rollback runs.
	inst.mu.Lock()
	inst.Status = Stopped
	inst.snapshot.Store(buildSnapshot(inst))
	inst.mu.Unlock()

	rollbackFailedResume(&instanceState{inst: inst})

	if got := inst.Snapshot().Status; got != Stopped {
		t.Errorf("rollbackFailedResume clobbered later transition: got %s, want Stopped", got)
	}
}

// TestRollbackFailedResume_RevertsWhenStillActive covers the ordinary case:
// no intervening transition happened, so the failed resume correctly reverts
// Status back to Hibernated.
func TestRollbackFailedResume_RevertsWhenStillActive(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-rollback-normal", Status: Active}
	inst.snapshot.Store(buildSnapshot(inst))

	rollbackFailedResume(&instanceState{inst: inst})

	if got := inst.Snapshot().Status; got != Hibernated {
		t.Errorf("rollbackFailedResume did not revert: got %s, want Hibernated", got)
	}
}

// TestRollbackFailedResume_BumpsUpdatedAt covers rollbackFailedResume, a
// fourth bypass-the-state-machine path sharing the same touchUpdatedAt fix
// as TestForceStatus_BumpsUpdatedAt/TestRecoverFromStopped_BumpsUpdatedAt in
// state_machine_test.go -- without it, the frontend's upsertSession reducer
// silently drops the reverted status.
func TestRollbackFailedResume_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()
	before := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "test-rollback-updatedat", Status: Active, UpdatedAt: before}
	inst.snapshot.Store(buildSnapshot(inst))

	rollbackFailedResume(&instanceState{inst: inst})

	if got := inst.Snapshot().UpdatedAt; !got.After(before) {
		t.Errorf("UpdatedAt = %v, want a time after %v (rollbackFailedResume must bump it)", got, before)
	}
}

// TestResumeFromHibernation_FailedRollback_DoesNotClobberConcurrentStop
// exercises the same invariant as TestRollbackFailedResume_DoesNotClobberLaterTransition
// above, but through the real actor dispatch path instead of a direct
// synchronous call: rollbackFailedResume is sent via i.send, exactly as
// resumeFromHibernationLocked's failure path does, and races against a
// legitimate concurrent StopByUser() on a real *Instance wrapped by
// NewLiveInstance (construction pattern mirrors TestTransitionTo_ConcurrentApprove
// in instance_concurrency_test.go).
//
// Both possible actor-mailbox orderings converge on the same expected
// outcome: Active->Stopped, or (if the rollback wins the race first)
// Active->Hibernated->Stopped, since Hibernated->Stopped is itself a valid
// transition (state_machine.go). What must never happen is the reverse: the
// rollback clobbering an already-landed Stopped back to Hibernated -- the
// bug rollbackFailedResume's Status==Active guard exists to prevent.
// StopByUser's dispatch is synchronous (sendSyncErr), so by the time it
// returns the actor has fully applied its transition either way, making a
// direct post-wg.Wait() assertion deterministic without polling.
func TestResumeFromHibernation_FailedRollback_DoesNotClobberConcurrentStop(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "test-resume-rollback-race",
		Status:      Active,
		Permissions: GetManagedPermissions(),
	}
	inst.started.Store(true)
	li := NewLiveInstance(inst)
	defer li.Stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Mirrors resumeFromHibernationLocked's failure path in
		// instance_hibernate.go: a failed async resume dispatches the
		// rollback via i.send (fire-and-forget through the actor mailbox).
		inst.send(rollbackFailedResume)
	}()
	go func() {
		defer wg.Done()
		// A legitimate concurrent transition to a terminal status, landing
		// around the same time as the failed-resume rollback above.
		_ = inst.StopByUser()
	}()
	wg.Wait()

	require.Equal(t, Stopped, inst.Snapshot().Status,
		"a failed hibernation-resume rollback must not clobber a concurrent StopByUser transition")
}
