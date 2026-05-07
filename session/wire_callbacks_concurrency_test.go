package session

import (
	"testing"
	"time"
)

// TestWireRateLimitCallbacks_NoDeadlock verifies that wireRateLimitCallbacks
// can be called while holding the stateMutex. This prevents a self-deadlock
// that previously occurred because wireRateLimitCallbacks called GetController(),
// which attempted to acquire an RLock while the same goroutine held the Lock.
func TestWireRateLimitCallbacks_NoDeadlock(t *testing.T) {
	i := &Instance{}

	// Pre-fix, this would deadlock because GetController() (called by wireRateLimitCallbacks)
	// tries to acquire RLock while stateMutex.Lock() is held.

	done := make(chan bool)
	go func() {
		i.stateMutex.Lock()
		defer i.stateMutex.Unlock()

		// The fix is to pass the controller directly to avoid re-acquiring the lock.
		// Passing nil is sufficient to verify that the method itself doesn't lock anymore.
		i.wireRateLimitCallbacks(nil)
		done <- true
	}()

	select {
	case <-done:
		// Success: no deadlock
	case <-time.After(1 * time.Second):
		t.Fatal("Deadlock detected: wireRateLimitCallbacks still attempts to acquire a lock, causing a self-deadlock when called from a locked context (e.g., StartController)")
	}
}
