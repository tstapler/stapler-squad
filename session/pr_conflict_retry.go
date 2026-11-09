package session

import (
	"fmt"
	"sync"
	"time"
)

// ConflictRetryInterval is the minimum gap between conflict-only PR fix spawns.
const ConflictRetryInterval = time.Hour

// ConflictRetryMaxAttempts bounds consecutive conflict-only grants per item. The rework
// cap does not: ReworkCapOverride == 0 means uncapped, so without this a conflict no
// session can fix would respawn hourly forever.
const ConflictRetryMaxAttempts = 6

// ConflictRetryMaxRefunds bounds how many failed-to-start attempts per item are refunded;
// later failures spend the budget, so a spawn that can never succeed exhausts and notifies.
const ConflictRetryMaxRefunds = 3

type conflictRetryDecision int

const (
	ConflictRetryGranted conflictRetryDecision = iota
	ConflictRetryThrottled
	ConflictRetryExhausted
)

type conflictRetryState struct {
	last              time.Time
	Grants            int
	refunds           int // consecutive failed starts refunded; reset by confirmStarted
	failedSpent       int // failed starts past the refund cap, whose grants stayed spent
	startFailures     int // all failed starts, refunded or not
	exhaustedNotified bool
	rowAttempts       int32 // the pr_needs_fix row's attempt counter at the last take
}

// ConflictRetryLedger remembers conflict-only fix grants per item. Deliberately
// in-memory: a restart re-arms the budget, at most ConflictRetryMaxRefunds+
// ConflictRetryMaxAttempts spawns per process lifetime, and a conflict-only grant
// overrides RemediationDue's parked state. Revisit if a live incident shows >12 conflict
// spawns for one item; the persistent option is a conflict_grants int column on
// BacklogStuckState (an ent schema change; Reset must zero it with the attempt counter).
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
	case !s.last.IsZero() && now.Sub(s.last) < interval:
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
// (keyed on last, not grants, so refunding the only grant cannot lift it). At most
// ConflictRetryMaxRefunds consecutive failed starts are returned per item (confirmStarted
// ends the run); later ones keep their grant spent. An attempt that returns nil without
// spawning cannot be told apart from one that did, so it is not refunded.
func (g *ConflictRetryLedger) Refund(itemID string) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	s := g.Items[itemID]
	if s == nil || s.Grants == 0 {
		return
	}
	s.startFailures++
	if s.refunds < ConflictRetryMaxRefunds {
		s.refunds++
		s.Grants--
		return
	}
	s.failedSpent++
}

// confirmStarted records that the last grant's fix attempt did not fail to start, which
// ends the run of consecutive failed starts the refund cap bounds.
func (g *ConflictRetryLedger) ConfirmStarted(itemID string) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if s := g.Items[itemID]; s != nil {
		s.refunds = 0
	}
}

// sessionsRan is how many spent grants actually started a fix session.
func (g *ConflictRetryLedger) SessionsRan(itemID string) int {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if s := g.Items[itemID]; s != nil {
		return s.Grants - s.failedSpent
	}
	return 0
}

// failedStarts is how many fix attempts failed to start since the item was last cleared.
func (g *ConflictRetryLedger) FailedStarts(itemID string) int {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if s := g.Items[itemID]; s != nil {
		return s.startFailures
	}
	return 0
}

func (g *ConflictRetryLedger) Clear(itemID string) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	delete(g.Items, itemID)
}

// conflictExhaustionMessage words the "needs manual attention" notice from what actually
// happened: ran fix sessions that did not clear the conflict, failedStarts that never started.
func ConflictExhaustionMessage(title string, ran, failedStarts int) string {
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

// Counters reports an item's net grants and refunded failed starts (diagnostics and tests).
func (g *ConflictRetryLedger) Counters(itemID string) (grants, refunds int) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	if s := g.Items[itemID]; s != nil {
		return s.Grants, s.refunds
	}
	return 0, 0
}
