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
	rowAttempts       int32 // the pr_needs_fix row's attempt counter at the last take
}

// conflictRetryLedger remembers conflict-only fix grants per item. In-memory: a
// restart grants up to conflictRetryMaxAttempts more.
type conflictRetryLedger struct {
	mu    sync.Mutex
	items map[string]*conflictRetryState
}

// take decides whether a conflict-only fix may spawn for itemID now, recording the
// grant when it may. rowAttempts is the open pr_needs_fix row's attempt counter: it only
// grows until the Reset tool zeroes it, so a drop re-arms the override.
func (g *conflictRetryLedger) take(itemID string, now time.Time, interval time.Duration, rowAttempts int32) conflictRetryDecision {
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
	if rowAttempts < s.rowAttempts {
		*s = conflictRetryState{}
	}
	s.rowAttempts = rowAttempts
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

// refund returns a grant whose fix attempt failed to start, keeping the hourly throttle
// so the next try waits for the next interval. An attempt that returns nil without
// spawning cannot be told apart from one that did, so it is not refunded.
func (g *conflictRetryLedger) refund(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s := g.items[itemID]; s != nil && s.grants > 0 {
		s.grants--
	}
}

func (g *conflictRetryLedger) clear(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.items, itemID)
}
