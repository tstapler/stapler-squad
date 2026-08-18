//go:build !windows

package tmux

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestStartZombieReaper_GoroutineFullyExits_When_WaitGroupIsJoined proves
// StartZombieReaper's goroutine has actually returned by the time wg.Wait()
// unblocks — not just that ctx was canceled (backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC2). logFn only fires when
// reapZombieChildren() > 0, which this test cannot guarantee (no controllable
// OS zombie state), so goroutine count before/after the join is used instead
// of a side-effect counter to prove the goroutine actually terminated rather
// than merely being signaled — mirroring zombie_detector_test.go's
// TestStartZombieWatcher_GoroutineFullyExits_When_WaitGroupIsJoined for the
// same reason.
func TestStartZombieReaper_GoroutineFullyExits_When_WaitGroupIsJoined(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	before := runtime.NumGoroutine()

	StartZombieReaper(ctx, time.Millisecond, func(string, ...any) {}, &wg)

	// Give the goroutine a moment to actually start running.
	time.Sleep(10 * time.Millisecond)
	cancel()

	joined := make(chan struct{})
	go func() {
		wg.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(10 * time.Second):
		t.Fatal("wg.Wait() did not return within 10s after cancel — goroutine leaked")
	}

	// Allow the runtime a brief window to reclaim the exited goroutine's stack
	// bookkeeping before sampling NumGoroutine again.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		t.Fatalf("goroutine count did not return to baseline after wg join (before=%d after=%d) — goroutine did not fully exit", before, got)
	}
}

// TestReapZombieChildren_ReturnsZero_When_NoZombieChildrenExist documents
// reapZombieChildren's behavior in the common case (no zombies present) so
// the goroutine-exit test above isn't the only coverage of this function.
func TestReapZombieChildren_ReturnsZero_When_NoZombieChildrenExist(t *testing.T) {
	if n := reapZombieChildren(); n != 0 {
		t.Fatalf("expected 0 zombies reaped when none exist, got %d", n)
	}
}

// TestStartZombieReaper_JoinsOnCtxCancel pins the regression this fix
// addresses: StartZombieReaper used to be signaled via ctx cancellation but
// never joined, so a caller had no way to know the goroutine had actually
// exited. A short interval drives multiple ticks, and goleak.VerifyNone after
// wg.Wait() confirms no goroutine survives cancellation.
func TestStartZombieReaper_JoinsOnCtxCancel(t *testing.T) {
	baseline := goleak.IgnoreCurrent()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	StartZombieReaper(ctx, time.Millisecond, func(string, ...any) {}, &wg)

	time.Sleep(20 * time.Millisecond) // let several ticks fire
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Mirrors the margin in zombie_detector_test.go: under heavy parallel
		// test-suite load, scheduling delays alone (even without a subprocess
		// call here) can push a tick past 1s without indicating a real hang.
		t.Fatal("wg.Wait() did not return within 5s of ctx cancellation")
	}

	goleak.VerifyNone(t, baseline)
}
