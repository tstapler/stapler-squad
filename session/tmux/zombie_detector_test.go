package tmux

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestStartZombieWatcher_JoinsOnCtxCancel pins the regression this fix
// addresses: StartZombieWatcher used to be signaled via ctx cancellation but
// never joined, so a caller had no way to know the goroutine had actually
// exited. A short interval drives multiple ticks, and goleak.VerifyNone after
// wg.Wait() confirms no goroutine survives cancellation.
//
// scanZombiesFn is swapped for a fake here rather than exercising the real
// ScanZombies (which forks a "ps" subprocess): a real fork/exec/wait has
// unbounded latency under system load (observed exceeding 5s under `-race`
// alongside this package's other subprocess-heavy tmux tests), which made
// any fixed wall-clock assertion around it inherently flaky. The fake proves
// the same join behavior deterministically.
func TestStartZombieWatcher_JoinsOnCtxCancel(t *testing.T) {
	baseline := goleak.IgnoreCurrent()

	origScan := scanZombiesFn
	scanZombiesFn = func() ([]ZombieInfo, error) { return nil, nil }
	defer func() { scanZombiesFn = origScan }()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	StartZombieWatcher(ctx, time.Millisecond, func(string, ...any) {}, &wg)

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
		t.Fatal("wg.Wait() did not return within 5s of ctx cancellation")
	}

	goleak.VerifyNone(t, baseline)
}
