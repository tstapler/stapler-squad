package session

import "testing"

// TestRollbackFailedResume_DoesNotClobberLaterTransition verifies AC1: a
// failed async hibernation-resume must not overwrite a status that already
// moved on legitimately (e.g. Hibernated->Active->Stopped, where the Stopped
// transition lands before the resume goroutine's failure-path rollback
// runs) -- see rollbackFailedResume's doc comment.
func TestRollbackFailedResume_DoesNotClobberLaterTransition(t *testing.T) {
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
	inst := &Instance{Title: "test-rollback-normal", Status: Active}
	inst.snapshot.Store(buildSnapshot(inst))

	rollbackFailedResume(&instanceState{inst: inst})

	if got := inst.Snapshot().Status; got != Hibernated {
		t.Errorf("rollbackFailedResume did not revert: got %s, want Hibernated", got)
	}
}
