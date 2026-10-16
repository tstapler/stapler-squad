package session

import "sync/atomic"

// scrollLease is a per-Instance, single-flight exclusive claim held by the
// one client currently forwarding a scroll gesture for a session. It
// serializes a single client's own overlapping ForwardScroll calls (e.g.
// rapid repeat scroll gestures) -- distinct from streamhub's
// ScrollForwardAttachBarrier, which instead guards new subscribers attaching
// mid-forward. Transient orchestration state, not observable session state,
// so it lives as an unexported Instance field, not in InstanceSnapshot.
type scrollLease struct {
	held atomic.Bool
	// lastCaptured is the most recently captured pane content for this
	// lease's lifetime, compared against a new RedrawQuiescence capture to
	// compute AtTop vs. Delivered. Unguarded by a mutex, but safe: every
	// access is sequenced between a tryAcquire (CompareAndSwap) and the
	// matching release (Store) on held, which establishes happens-before
	// under the Go memory model -- a lock-free spinlock, not a bare race.
	lastCaptured []byte
}

// tryAcquire claims the lease for the calling goroutine, returning false if
// another ForwardScroll call already holds it.
func (l *scrollLease) tryAcquire() bool {
	return l.held.CompareAndSwap(false, true)
}

// release relinquishes the lease. Safe to call even if not held (e.g. a
// defer running after tryAcquire already failed) -- it simply leaves held
// false.
func (l *scrollLease) release() {
	l.held.Store(false)
}
