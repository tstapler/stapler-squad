package session

import (
	"sync"
	"time"
)

// conflictRetryInterval is the minimum gap between conflict-only PR fix spawns.
const conflictRetryInterval = time.Hour

// conflictRetryLedger remembers when a conflict-only fix was last granted per
// item. In-memory: a restart grants one extra attempt, which the rework cap bounds.
type conflictRetryLedger struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// take reports whether interval has elapsed since the last grant for itemID,
// recording now as the new grant time when it has.
func (g *conflictRetryLedger) take(itemID string, now time.Time, interval time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, ok := g.last[itemID]; ok && now.Sub(last) < interval {
		return false
	}
	if g.last == nil {
		g.last = map[string]time.Time{}
	}
	g.last[itemID] = now
	return true
}

func (g *conflictRetryLedger) clear(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.last, itemID)
}
