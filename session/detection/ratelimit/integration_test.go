package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type stubBuffer struct{}

func (s *stubBuffer) GetRecentOutput(_ int) []byte { return nil }

// blockingBuffer's GetRecentOutput blocks until told to proceed via release,
// closing entered on first call. This lets a test hold pollLoop deterministically
// inside its notifyCh-processing branch — not yet re-selecting on ctx.Done() —
// instead of racing a timing-based assertion against a poll loop that exits
// essentially instantly on cancellation.
type blockingBuffer struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingBuffer() *blockingBuffer {
	return &blockingBuffer{entered: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingBuffer) GetRecentOutput(_ int) []byte {
	close(b.entered)
	<-b.release
	return nil
}

// TestPTYConsumer_StartStop_Concurrent verifies that Start() and Stop() are
// safe to call concurrently. The race detector catches unsynchronised access
// to shared fields (e.g. the old stopCh replacement pattern).
//
// Must fail against pre-fix code (stopCh chan struct{} replaced in Stop while
// pollLoop reads it) because the race detector fires when run with -race.
func TestPTYConsumer_StartStop_Concurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 10
	const iterations = 50

	// nil manager is safe: pollLoop only calls manager.ProcessOutput when
	// buffer returns non-empty data, and stubBuffer always returns nil.
	pc := NewPTYConsumer(&stubBuffer{}, nil)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				pc.Start()
				pc.Stop()
			}
		}()
	}
	wg.Wait()
}

// TestPTYConsumer_Stop_UnlocksBeforeWaitingOnPollLoop covers backlog item
// 1cea70ed-3127-48db-8e68-f02eac685510 AC1/AC5: Stop() must release pc.mu
// before blocking on pc.doneCh, or any future pollLoop change that takes
// pc.mu would deadlock against Stop() (pre-mortem.md failure #4).
//
// Racing a TryLock poll against a live Stop() call doesn't reliably catch
// this: there's no guarantee the polling goroutine even runs before Stop()
// finishes, so a prior version of this test passed against a deliberately
// reintroduced `defer pc.mu.Unlock()` bug regardless of which was correct.
// Instead: wedge pollLoop (via blockingBuffer) so it can never close doneCh,
// forcing Stop() through the full stopJoinTimeout wait every time, then
// TryLock partway through that window. A correct Stop() releases pc.mu in
// microseconds, long before the halfway point; a buggy deferred unlock would
// still hold it there, since Stop() can't return until the timeout elapses.
func TestPTYConsumer_Stop_UnlocksBeforeWaitingOnPollLoop(t *testing.T) {
	prevTimeout := stopJoinTimeout
	stopJoinTimeout = 300 * time.Millisecond
	t.Cleanup(func() { stopJoinTimeout = prevTimeout })

	buf := newBlockingBuffer()
	t.Cleanup(func() { close(buf.release) })
	pc := NewPTYConsumer(buf, nil)
	pc.Start()
	pc.NotifyOutput()
	<-buf.entered // pollLoop is now wedged inside GetRecentOutput.

	stopDone := make(chan struct{})
	go func() {
		pc.Stop()
		close(stopDone)
	}()

	time.Sleep(stopJoinTimeout / 2)
	locked := pc.mu.TryLock()
	if locked {
		pc.mu.Unlock()
	}

	// Join the spawned Stop() call before any assertion, regardless of the
	// lock check's outcome — otherwise a t.Fatal here exits via
	// runtime.Goexit() while Stop() (and its read of stopJoinTimeout) is
	// still in flight, racing a later test's mutation of that shared var.
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return")
	}

	if !locked {
		t.Fatal("pc.mu was still held partway through Stop()'s wait on a wedged pollLoop")
	}
}

// TestPTYConsumer_Stop_ReturnsOnTimeout_When_PollLoopNeverExits covers the
// stopJoinTimeout escape hatch (AC5): if pollLoop never observes ctx.Done()
// (e.g. permanently wedged), Stop() must still return within stopJoinTimeout
// rather than blocking forever. Shrinks stopJoinTimeout for the duration of
// the test so this doesn't cost a real 10s wait.
func TestPTYConsumer_Stop_ReturnsOnTimeout_When_PollLoopNeverExits(t *testing.T) {
	prevTimeout := stopJoinTimeout
	stopJoinTimeout = 20 * time.Millisecond
	t.Cleanup(func() { stopJoinTimeout = prevTimeout })

	buf := newBlockingBuffer()
	// pollLoop is left wedged inside GetRecentOutput for the test's duration
	// (that's the scenario under test), so release it on cleanup rather than
	// never, or the goroutine leaks into a later test's goleak snapshot —
	// exactly the bug class this whole item exists to fix.
	t.Cleanup(func() { close(buf.release) })
	pc := NewPTYConsumer(buf, nil)
	pc.Start()
	pc.NotifyOutput()
	<-buf.entered // pollLoop is now wedged inside GetRecentOutput.

	stopDone := make(chan struct{})
	go func() {
		pc.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Expected: Stop() returned via the timeout branch, not via <-done.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within a bounded time despite a wedged pollLoop and a short stopJoinTimeout")
	}
}
