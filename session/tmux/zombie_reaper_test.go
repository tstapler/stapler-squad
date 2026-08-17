//go:build !windows

package tmux

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

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
