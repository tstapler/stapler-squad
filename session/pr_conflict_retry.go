package session

import (
	"sync"
	"time"
)

// conflictRetryInterval is the minimum gap between conflict-only PR fix spawns.
const conflictRetryInterval = time.Hour

// conflictRetryMaxAttempts bounds consecutive conflict-only grants per item. The rework
// cap does not: ReworkCapOverride == 0 means uncapped, so without this a conflict no
// session can fix would respawn hourly forever.
const conflictRetryMaxAttempts = 6

type conflictRetryDecision int

const (
	conflictRetryGranted conflictRetryDecision = iota
	conflictRetryThrottled
	conflictRetryExhausted
)

type conflictRetryState struct {
	last              time.Time
	grants            int
	exhaustedNotified bool
}

// conflictRetryLedger remembers conflict-only fix grants per item. In-memory: a
// restart grants up to conflictRetryMaxAttempts more.
type conflictRetryLedger struct {
	mu    sync.Mutex
	items map[string]*conflictRetryState
}

// take decides whether a conflict-only fix may spawn for itemID now, recording the
// grant when it may.
func (g *conflictRetryLedger) take(itemID string, now time.Time, interval time.Duration) conflictRetryDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.items == nil {
		g.items = map[string]*conflictRetryState{}
	}
	s := g.items[itemID]
	if s == nil {
		s = &conflictRetryState{}
		g.items[itemID] = s
	}
	switch {
	case s.grants >= conflictRetryMaxAttempts:
		return conflictRetryExhausted
	case s.grants > 0 && now.Sub(s.last) < interval:
		return conflictRetryThrottled
	}
	s.grants++
	s.last = now
	return conflictRetryGranted
}

// markExhaustedNotified reports true exactly once per exhaustion, so the
// "needs manual attention" notice is not repeated every tick.
func (g *conflictRetryLedger) markExhaustedNotified(itemID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.items[itemID]
	if s == nil || s.exhaustedNotified {
		return false
	}
	s.exhaustedNotified = true
	return true
}

func (g *conflictRetryLedger) clear(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.items, itemID)
}
