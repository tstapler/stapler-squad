package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPause_should_skipGitOps_When_IsWorktreeIsFalse verifies that Pause() on a
// non-worktree session does not call git operations. If the IsWorktree guard were
// absent, IsDirty() on an uninitialized GitWorktreeManager would return
// "git worktree not initialized", which would propagate into the error chain.
func TestPause_should_skipGitOps_When_IsWorktreeIsFalse(t *testing.T) {
	inst := &Instance{
		Title:      "test-non-worktree",
		Status:     Active,
		started:    true,
		IsWorktree: false,
		// gitManager left as zero value — IsDirty() returns error if called
	}

	err := inst.Pause()

	// Pause must succeed: no tmux session (DetachSafely is a no-op when session==nil),
	// no git operations, transitionTo(Paused) sets the status.
	require.NoError(t, err, "Pause() on a non-worktree session must not return an error")
	assert.Equal(t, Paused, inst.Status, "instance should be Paused after successful Pause()")
}

// TestPause_should_returnGitError_When_IsWorktreeIsTrueAndGitUninitialized verifies
// that the IsWorktree=true path does attempt git operations. An uninitialized
// GitWorktreeManager returns an error, confirming the guard condition is honored.
func TestPause_should_returnGitError_When_IsWorktreeIsTrueAndGitUninitialized(t *testing.T) {
	inst := &Instance{
		Title:      "test-worktree-uninit",
		Status:     Active,
		started:    true,
		IsWorktree: true,
		// gitManager.worktree == nil → IsDirty returns "git worktree not initialized"
	}

	err := inst.Pause()

	// With IsWorktree=true, IsDirty is called. Since gitManager.worktree is nil,
	// IsDirty returns an error that is appended to errs. combineErrors returns it.
	assert.Error(t, err, "Pause() on an uninitialized worktree session should propagate git error")
	assert.Contains(t, err.Error(), "worktree", "error should mention the worktree problem")
}

func TestTransitionTo_ConcurrentPause(t *testing.T) {
	// Create an instance in Active status.
	// Launch 10 goroutines all trying to transition to Paused simultaneously.
	// Exactly one should succeed; the rest should get ErrInvalidTransition
	// (because after the first successful transition, the status is Paused
	// and Paused->Paused is not a valid transition).
	inst := &Instance{
		Title:   "test-concurrent",
		Status:  Active,
		started: true,
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	var successCount int32
	var failCount int32

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Use the public-facing mutex pattern matching Approve/Deny
			inst.stateMutex.Lock()
			err := inst.transitionTo(context.Background(), Paused)
			inst.stateMutex.Unlock()

			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful transition, got %d", successCount)
	}
	if failCount != int32(numGoroutines-1) {
		t.Errorf("expected %d failed transitions, got %d", numGoroutines-1, failCount)
	}
	if inst.Status != Paused {
		t.Errorf("expected final status Paused, got %s", inst.Status)
	}
}

func TestTransitionTo_ConcurrentApprove(t *testing.T) {
	// Start from Paused — Paused→Active is valid.
	// Launch goroutines all calling Approve() simultaneously.
	// Exactly one should succeed; after that, Active→Active is invalid.
	inst := &Instance{
		Title:   "test-concurrent-approve",
		Status:  Paused,
		started: true,
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	var successCount int32

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := inst.Approve()
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	// Paused→Active should succeed exactly once.
	// After that, Active→Active is invalid, so subsequent Approve calls fail.
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful Approve, got %d", successCount)
	}
	if inst.Status != Active {
		t.Errorf("expected final status Active, got %s", inst.Status)
	}
}

func TestTransitionTo_ConcurrentMixed(t *testing.T) {
	// Multiple goroutines concurrently calling Approve and Deny.
	// Starting from Paused:
	//   Approve = Paused→Active (valid once, then Active→Active invalid)
	//   Deny    = Paused→Paused (invalid self-transition while in Paused;
	//             Active→Paused valid while in Active)
	// Because Active→Paused and Paused→Active are both valid,
	// the state can bounce between them. The key guarantees are:
	//   1. No data race (validated by -race flag)
	//   2. Final state is consistent (Active or Paused)
	//   3. At least one operation succeeds
	inst := &Instance{
		Title:   "test-concurrent-mixed",
		Status:  Paused,
		started: true,
	}

	const numGoroutines = 20
	var wg sync.WaitGroup
	var approveSuccess int32
	var denySuccess int32

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				if err := inst.Approve(); err == nil {
					atomic.AddInt32(&approveSuccess, 1)
				}
			} else {
				if err := inst.Deny(); err == nil {
					atomic.AddInt32(&denySuccess, 1)
				}
			}
		}(i)
	}
	wg.Wait()

	totalSuccess := approveSuccess + denySuccess
	if totalSuccess < 1 {
		t.Errorf("expected at least 1 successful transition, got %d (approve=%d, deny=%d)",
			totalSuccess, approveSuccess, denySuccess)
	}

	// The final status must be either Active or Paused (the only reachable
	// states via Approve/Deny cycles).
	if inst.Status != Active && inst.Status != Paused {
		t.Errorf("expected final status Active or Paused, got %s", inst.Status)
	}
}

// ─── U-GO-30: TestInstanceGetSetSessionGoal_threadSafe ───────────────────────
// Run with: go test -race ./session/ -run TestInstanceGetSetSessionGoal_threadSafe

func TestInstanceGetSetSessionGoal_threadSafe(t *testing.T) {
	inst := &Instance{
		Title: "concurrency-goal-test",
		UUID:  "test-uuid-concurrent-goal",
	}

	const goroutines = 20
	var wg sync.WaitGroup

	// Writers.
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := &SessionGoalData{
				UUID:        "test-uuid",
				SessionUUID: inst.UUID,
				Goal:        "concurrent goal",
				Status:      GoalStatusWorking,
			}
			inst.SetSessionGoalCached(g)
		}()
	}

	// Readers.
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = inst.GetSessionGoal()
		}()
	}

	wg.Wait()
	// No data race should occur (detected by -race flag).
}
