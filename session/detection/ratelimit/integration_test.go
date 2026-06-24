package ratelimit

import (
	"sync"
	"testing"
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
