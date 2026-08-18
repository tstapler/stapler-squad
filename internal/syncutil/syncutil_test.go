package syncutil

import (
	"sync"
	"testing"
	"time"
)

// Backlog item 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC3: the join must be
// bounded by a timeout, not a bare Wait(). These cases were previously
// duplicated (byte-for-byte) across server/server_test.go and
// session/pty_discovery_test.go, one per copy of the waitGroupWithTimeout
// helper they now share via WaitWithTimeout.

// Test_WaitWithTimeout_should_ReturnTrue_When_WaitGroupReachesZeroBeforeTimeout
// covers the success path: wg.Done() is called well within the timeout, so
// WaitWithTimeout must return true promptly rather than waiting out the full
// timeout duration.
func Test_WaitWithTimeout_should_ReturnTrue_When_WaitGroupReachesZeroBeforeTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	start := time.Now()
	ok := WaitWithTimeout(&wg, 5*time.Second)
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("expected WaitWithTimeout to return true when the WaitGroup reaches zero before the timeout")
	}
	if elapsed > time.Second {
		t.Fatalf("WaitWithTimeout took %v to return after the WaitGroup reached zero; expected it to return promptly, not wait out the full timeout", elapsed)
	}
}

// Test_WaitWithTimeout_should_ReturnFalse_When_WaitGroupNeverReachesZero
// covers the timeout path: the WaitGroup counter never reaches zero (a
// goroutine that never calls Done()), so WaitWithTimeout must return false at
// approximately the timeout instead of blocking forever like a bare wg.Wait()
// would.
func Test_WaitWithTimeout_should_ReturnFalse_When_WaitGroupNeverReachesZero(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// Deliberately never call wg.Done() — simulates a stuck background task.

	const timeout = 100 * time.Millisecond
	start := time.Now()
	ok := WaitWithTimeout(&wg, timeout)
	elapsed := time.Since(start)

	if ok {
		t.Fatal("expected WaitWithTimeout to return false when the WaitGroup never reaches zero")
	}
	if elapsed < timeout {
		t.Fatalf("WaitWithTimeout returned after %v, before the %v timeout elapsed", elapsed, timeout)
	}
	if elapsed > timeout+2*time.Second {
		t.Fatalf("WaitWithTimeout took %v to return; expected it to return at approximately the %v timeout", elapsed, timeout)
	}
}

// TestWaitWithTimeout_TableDriven mirrors the sibling table-driven coverage
// that lived in session/pty_discovery_test.go (TestWaitGroupWithTimeout),
// pinning both branches directly.
func TestWaitWithTimeout_TableDriven(t *testing.T) {
	t.Run("returns true when the group finishes in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done() }()
		if !WaitWithTimeout(&wg, time.Second) {
			t.Error("WaitWithTimeout returned false, want true")
		}
	})

	t.Run("returns false when the group doesn't finish in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1) // deliberately never Done()
		if WaitWithTimeout(&wg, 10*time.Millisecond) {
			t.Error("WaitWithTimeout returned true, want false")
		}
	})
}
