// Package syncutil provides small synchronization helpers shared across
// packages that don't otherwise have a natural common dependency.
package syncutil

import (
	"sync"
	"time"
)

// WaitWithTimeout waits for wg to reach zero, returning true if it did so
// within timeout. Returns false if timeout elapses first; the spawned
// goroutine remains running and reports into wg after this function returns,
// but its result is discarded (sync.WaitGroup has no cancelable Wait).
func WaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
