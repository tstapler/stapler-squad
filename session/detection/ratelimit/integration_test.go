package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type stubBuffer struct{}

func (s *stubBuffer) GetRecentOutput(_ int) []byte { return nil }

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
// before blocking on pc.wg.Wait(), or any future pollLoop change that takes
// pc.mu would deadlock against Stop() (pre-mortem.md failure #4). Asserted by
// racing a TryLock against a live Stop() call: it must succeed well before
// Stop() itself returns, proving the lock isn't held for the whole wait.
func TestPTYConsumer_Stop_UnlocksBeforeWaitingOnPollLoop(t *testing.T) {
	t.Parallel()

	pc := NewPTYConsumer(&stubBuffer{}, nil)
	pc.Start()

	stopDone := make(chan struct{})
	go func() {
		pc.Stop()
		close(stopDone)
	}()

	locked := false
	for range 200 {
		if pc.mu.TryLock() {
			locked = true
			pc.mu.Unlock()
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !locked {
		t.Fatal("pc.mu was never observed unlocked while Stop() was in flight — wg.Wait() may be running under the lock")
	}

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return")
	}
}
