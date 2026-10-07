package session

import "sync"

// ReviewSpawnGuard is the process-wide check-then-reserve guard that keeps at
// most one review spawn per backlog item in flight, across every entry point
// (onSessionExited, TriggerReviewForSession, ReconcileStuckItems, and the
// services-package headless re-review paths). A single mutex covers the whole
// map so the check and the reservation are one atomic step; a per-item lock
// would not stop two entry points racing past each other's check.
type ReviewSpawnGuard struct {
	mu       sync.Mutex
	reserved map[string]struct{}
}

// NewReviewSpawnGuard returns an empty guard.
func NewReviewSpawnGuard() *ReviewSpawnGuard {
	return &ReviewSpawnGuard{reserved: make(map[string]struct{})}
}

// TryReserve atomically reserves itemID unless it is already reserved or
// blocked reports true. blocked runs under the guard's lock, so it must be
// brief (one storage read). The returned release is idempotent and must be
// called on every exit path (defer it).
func (g *ReviewSpawnGuard) TryReserve(itemID string, blocked func() bool) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, held := g.reserved[itemID]; held {
		return nil, false
	}
	if blocked != nil && blocked() {
		return nil, false
	}
	g.reserved[itemID] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.reserved, itemID)
		})
	}, true
}

// InFlight reports whether itemID currently holds a reservation.
func (g *ReviewSpawnGuard) InFlight(itemID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, held := g.reserved[itemID]
	return held
}
