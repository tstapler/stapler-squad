package session

import (
	"testing"
	"time"
)

// TestWireRateLimitCallbacks_NoDeadlock verifies that wireRateLimitCallbacks
// can be called while holding the instance write lock (i.mu). This prevents a
// self-deadlock that previously occurred because wireRateLimitCallbacks called
// GetController(), which attempted to acquire i.mu.RLock while i.mu.Lock() was
// already held on the same goroutine.
//
// After the stateMutex→mu rename + ControllerManager.mu split, GetController()
// uses controllerManager.mu instead of i.mu, so no self-deadlock is possible.
func TestWireRateLimitCallbacks_NoDeadlock(t *testing.T) {
	i := &Instance{}

	done := make(chan bool)
	go func() {
		i.mu.Lock()
		defer i.mu.Unlock()

		// Passing nil verifies the method itself doesn't attempt to re-acquire i.mu.
		i.wireRateLimitCallbacks(nil)
		done <- true
	}()

	select {
	case <-done:
		// Success: no deadlock
	case <-time.After(1 * time.Second):
		t.Fatal("Deadlock detected: wireRateLimitCallbacks re-acquires i.mu from within a locked context")
	}
}
