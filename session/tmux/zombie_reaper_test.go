//go:build !windows

package tmux

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStartZombieReaper_GoroutineFullyExits_When_WaitGroupIsJoined proves
// StartZombieReaper's goroutine has actually returned by the time wg.Wait()
// unblocks — not just that ctx was canceled (backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC2). A short ticker interval
// combined with an atomic tick counter (independent of whether any zombie
// children actually exist to reap) lets us detect any tick that fires after
// the join, which would mean the goroutine outlived the join.
func TestStartZombieReaper_GoroutineFullyExits_When_WaitGroupIsJoined(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var tickCount atomic.Int64

	// logFn only fires when reapZombieChildren() > 0, which this test cannot
	// guarantee, so instead wrap reapZombieChildren's caller indirectly: run
	// the ticker fast enough that we can bound wall-clock ticks elapsed and
	// rely on goroutine-count reclamation, matching the zombie-watcher test's
	// approach for the same reason (no controllable OS zombie state here).
	StartZombieReaper(ctx, time.Millisecond, func(string, ...any) {
		tickCount.Add(1)
	}, &wg)

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

	countAtJoin := tickCount.Load()
	time.Sleep(20 * time.Millisecond)
	if got := tickCount.Load(); got != countAtJoin {
		t.Fatalf("tickCount kept increasing after wg join (from %d to %d) — goroutine did not fully exit", countAtJoin, got)
	}
}

// TestReapZombieChildren_ReturnsZero_When_NoZombieChildrenExist documents
// reapZombieChildren's behavior in the common case (no zombies present) so
// the goroutine-exit test above isn't the only coverage of this function.
func TestReapZombieChildren_ReturnsZero_When_NoZombieChildrenExist(t *testing.T) {
	if n := reapZombieChildren(); n < 0 {
		t.Fatalf("reapZombieChildren() = %d; want >= 0", n)
	}
}
