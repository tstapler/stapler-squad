package streamhub

import (
	"sync"
	"time"
)

// A resize holds back live output for as long as it applies, so its budget should be as short as
// the daemon allows. It is derived from how long recent resizes actually took instead of a
// blind constant: fast daemons stay at the floor, a loaded one earns more room (up to the
// ceiling) rather than timing out into a stale screen on every resize.
const (
	resizeBudgetFloor    = 2 * time.Second
	resizeBudgetCeiling  = 5 * time.Second // the pre-#1478 fixed budget
	resizeBudgetHeadroom = 2               // multiple of the slowest recent resize
	resizeBudgetSamples  = 8
)

// resizeBudget keeps the last resizeBudgetSamples apply durations. The zero value is ready and
// adaptive; a non-zero fixed pins the budget (tests, explicit overrides).
type resizeBudget struct {
	fixed time.Duration

	mu     sync.Mutex
	recent [resizeBudgetSamples]time.Duration
	next   int
}

// observe records one resize's apply time. A resize that hit its budget should report the
// budget itself (a lower bound on the real time), which doubles the next budget up to the ceiling.
func (b *resizeBudget) observe(took time.Duration) {
	b.mu.Lock()
	b.recent[b.next] = took
	b.next = (b.next + 1) % resizeBudgetSamples
	b.mu.Unlock()
}

// current is the budget for the next resize.
func (b *resizeBudget) current() time.Duration {
	if b.fixed > 0 {
		return b.fixed
	}
	b.mu.Lock()
	var slowest time.Duration
	for _, d := range b.recent {
		slowest = max(slowest, d)
	}
	b.mu.Unlock()
	return min(max(resizeBudgetHeadroom*slowest, resizeBudgetFloor), resizeBudgetCeiling)
}

// retry is the budget for the single capture retry after the first one timed out; the resize
// apply lock is held across it, so it is half the budget and never above the ceiling's half.
func (b *resizeBudget) retry() time.Duration {
	if b.fixed > 0 {
		return min(captureRetryTimeout, b.fixed)
	}
	return min(max(b.current()/2, captureRetryTimeout), resizeBudgetCeiling/2)
}
