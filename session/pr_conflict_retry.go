package session

import (
	"fmt"
	"sync"
	"time"
)

// conflictRetryInterval is the minimum gap between conflict-only PR fix spawns.
const conflictRetryInterval = time.Hour

// conflictRetryMaxAttempts bounds consecutive conflict-only grants per item. The rework
// cap does not: ReworkCapOverride == 0 means uncapped, so without this a conflict no
// session can fix would respawn hourly forever.
const conflictRetryMaxAttempts = 6

// conflictRetryMaxRefunds bounds how many failed-to-start attempts per item are refunded;
// later failures spend the budget, so a spawn that can never succeed exhausts and notifies.
const conflictRetryMaxRefunds = 3

type conflictRetryDecision int

const (
	conflictRetryGranted conflictRetryDecision = iota
	conflictRetryThrottled
	conflictRetryExhausted
)

type conflictRetryState struct {
	last              time.Time
	grants            int
	refunds           int // consecutive failed starts refunded; reset by confirmStarted
	failedSpent       int // failed starts past the refund cap, whose grants stayed spent
	startFailures     int // all failed starts, refunded or not
	exhaustedNotified bool
	rowAttempts       int32 // the pr_needs_fix row's attempt counter at the last take
}

// conflictRetryLedger remembers conflict-only fix grants per item. Deliberately
// in-memory: a restart re-arms the budget, at most conflictRetryMaxRefunds+
// conflictRetryMaxAttempts spawns per process lifetime, and a conflict-only grant
// overrides RemediationDue's parked state. Revisit if a live incident shows >12 conflict
// spawns for one item; the persistent option is a conflict_grants int column on
// BacklogStuckState (an ent schema change; Reset must zero it with the attempt counter).
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
	case !s.last.IsZero() && now.Sub(s.last) < interval:
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
// (keyed on last, not grants, so refunding the only grant cannot lift it). At most
// conflictRetryMaxRefunds consecutive failed starts are returned per item (confirmStarted
// ends the run); later ones keep their grant spent. An attempt that returns nil without
// spawning cannot be told apart from one that did, so it is not refunded.
func (g *conflictRetryLedger) refund(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.items[itemID]
	if s == nil || s.grants == 0 {
		return
	}
	s.startFailures++
	if s.refunds < conflictRetryMaxRefunds {
		s.refunds++
		s.grants--
		return
	}
	s.failedSpent++
}

// confirmStarted records that the last grant's fix attempt did not fail to start, which
// ends the run of consecutive failed starts the refund cap bounds.
func (g *conflictRetryLedger) confirmStarted(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s := g.items[itemID]; s != nil {
		s.refunds = 0
	}
}

// sessionsRan is how many spent grants actually started a fix session.
func (g *conflictRetryLedger) sessionsRan(itemID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s := g.items[itemID]; s != nil {
		return s.grants - s.failedSpent
	}
	return 0
}

// failedStarts is how many fix attempts failed to start since the item was last cleared.
func (g *conflictRetryLedger) failedStarts(itemID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s := g.items[itemID]; s != nil {
		return s.startFailures
	}
	return 0
}

func (g *conflictRetryLedger) clear(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.items, itemID)
}

// conflictExhaustionMessage words the "needs manual attention" notice from what actually
// happened: ran fix sessions that did not clear the conflict, failedStarts that never started.
func conflictExhaustionMessage(title string, ran, failedStarts int) string {
	const tail = "It now needs manual attention; use Reset to try again automatically."
	switch {
	case ran == 0:
		return fmt.Sprintf("%s — the merge conflict could not be fixed automatically: %d automated fix attempts could not be started. %s", title, failedStarts, tail)
	case failedStarts == 0:
		return fmt.Sprintf("%s — the merge conflict survived %d automated fix sessions. %s", title, ran, tail)
	default:
		return fmt.Sprintf("%s — the merge conflict survived %d automated fix sessions, and %d further attempts could not be started. %s", title, ran, failedStarts, tail)
	}
}
