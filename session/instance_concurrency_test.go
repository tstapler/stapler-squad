package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

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
