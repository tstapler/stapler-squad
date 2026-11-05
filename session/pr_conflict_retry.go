package session

import (
	"sync"
	"time"
)

// conflictRetryInterval is the minimum gap between conflict-only PR fix spawns.
const ConflictRetryInterval = time.Hour

// conflictRetryMaxAttempts bounds consecutive conflict-only grants per item. The rework
// cap does not: ReworkCapOverride == 0 means uncapped, so without this a conflict no
// session can fix would respawn hourly forever.
const ConflictRetryMaxAttempts = 6

type conflictRetryDecision int

const (
	ConflictRetryGranted conflictRetryDecision = iota
	ConflictRetryThrottled
	ConflictRetryExhausted
)

type conflictRetryState struct {
	last              time.Time
	Grants            int
	exhaustedNotified bool
	rowAttempts       int32 // the pr_needs_fix row's attempt counter at the last take
}

// conflictRetryLedger remembers conflict-only fix grants per item. In-memory: a
// restart grants up to conflictRetryMaxAttempts more.
type ConflictRetryLedger struct {
	Mu    sync.Mutex
	Items map[string]*conflictRetryState
}

// take decides whether a conflict-only fix may spawn for itemID now, recording the
// grant when it may. rowAttempts is the open pr_needs_fix row's attempt counter: it only
// grows until the Reset tool zeroes it, so a drop re-arms the override.
func (g *ConflictRetryLedger) Take(itemID string, now time.Time, interval time.Duration, rowAttempts int32) conflictRetryDecision {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if g.Items == nil {
		g.Items = map[string]*conflictRetryState{}
	}
	s := g.Items[itemID]
	if s == nil {
		s = &conflictRetryState{}
		g.Items[itemID] = s
	}
	if rowAttempts < s.rowAttempts {
		*s = conflictRetryState{}
	}
	s.rowAttempts = rowAttempts
	switch {
	case s.Grants >= ConflictRetryMaxAttempts:
		return ConflictRetryExhausted
	case s.Grants > 0 && now.Sub(s.last) < interval:
		return ConflictRetryThrottled
	}
	s.Grants++
	s.last = now
	return ConflictRetryGranted
}

// markExhaustedNotified reports true exactly once per exhaustion, so the
// "needs manual attention" notice is not repeated every tick.
func (g *ConflictRetryLedger) MarkExhaustedNotified(itemID string) bool {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	s := g.Items[itemID]
	if s == nil || s.exhaustedNotified {
		return false
	}
	s.exhaustedNotified = true
	return true
}

// refund returns a grant whose fix attempt failed to start, keeping the hourly throttle
// so the next try waits for the next interval. An attempt that returns nil without
// spawning cannot be told apart from one that did, so it is not refunded.
func (g *ConflictRetryLedger) Refund(itemID string) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if s := g.Items[itemID]; s != nil && s.Grants > 0 {
		s.Grants--
	}
}

func (g *ConflictRetryLedger) Clear(itemID string) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	delete(g.Items, itemID)
}
