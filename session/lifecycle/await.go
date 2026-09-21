package lifecycle

import "time"

// AwaitBounded waits for done to close, returning true if it does before
// wait elapses and false otherwise. Generalizes the bounded-wait-then-
// abandon idiom needed when tearing down a generation whose goroutine may
// be blocked on a read Go cannot forcibly interrupt.
func AwaitBounded(done <-chan struct{}, wait time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(wait):
		return false
	}
}
