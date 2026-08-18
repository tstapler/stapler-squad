package tmux

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestStartZombieWatcher_GoroutineFullyExits_When_WaitGroupIsJoined proves
// StartZombieWatcher's goroutine has actually returned by the time wg.Wait()
// unblocks — not just that ctx was canceled (backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC1). warnFn's invocation depends on
// real OS zombie state (ScanZombies shells out to `ps`), which a unit test
// cannot control deterministically, so goroutine count before/after the join
// is used instead of a side-effect counter to prove the goroutine actually
// terminated rather than merely being signaled.
func TestStartZombieWatcher_GoroutineFullyExits_When_WaitGroupIsJoined(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	before := runtime.NumGoroutine()

	StartZombieWatcher(ctx, time.Millisecond, func(string, ...any) {}, &wg)

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
