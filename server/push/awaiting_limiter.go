package push

import (
	"fmt"
	"sync"
	"time"
)

// Volume bounds for awaiting_direction pushes (ADR-007 decision 3). Tray records are never capped.
const (
	awaitingSessionInterval = 5 * time.Minute
	awaitingGlobalWindow    = 15 * time.Minute
	awaitingGlobalMax       = 6
	// awaitingDigestDelay coalesces a burst of overflow into one digest push.
	awaitingDigestDelay = 30 * time.Second
	// awaitingDigestTag is stable so each digest replaces the previous one.
	awaitingDigestTag = "awaiting-direction-digest"
)

// Gate reasons for notifications_suppressed_total.
const (
	GateReasonAwaitingPushLimit = "awaiting_direction_push_limit"
	GateReasonAwaitingGlobalCap = "awaiting_direction_global_cap"
)

// PushCounters is where the limiter reports its suppressions and digests.
type PushCounters interface {
	Suppressed(gateReason string)
	Digest()
}

type noopPushCounters struct{}

func (noopPushCounters) Suppressed(string) {}
func (noopPushCounters) Digest()           {}

type admitVerdict int

const (
	admitPush admitVerdict = iota
	admitSessionLimited
	admitGlobalCapped
)

// awaitingLimiter bounds awaiting_direction web pushes per session and across sessions.
// Only awaiting_direction pushes pass through it; prose_question never does.
type awaitingLimiter struct {
	now      func() time.Time
	counters PushCounters

	mu          sync.Mutex
	lastBySess  map[string]time.Time
	sends       []time.Time          // individual pushes inside the rolling window
	overflow    map[string]time.Time // sessions capped inside the window, by time capped
	pendingFrom time.Time            // when the first not-yet-digested overflow arrived; zero = none
}

func newAwaitingLimiter(now func() time.Time, c PushCounters) *awaitingLimiter {
	if now == nil {
		now = time.Now
	}
	if c == nil {
		c = noopPushCounters{}
	}
	return &awaitingLimiter{
		now: now, counters: c,
		lastBySess: map[string]time.Time{},
		overflow:   map[string]time.Time{},
	}
}

// admit decides one awaiting_direction push for sessionID.
func (l *awaitingLimiter) admit(sessionID string) admitVerdict {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(now)

	if last, ok := l.lastBySess[sessionID]; ok && now.Sub(last) < awaitingSessionInterval {
		l.counters.Suppressed(GateReasonAwaitingPushLimit)
		return admitSessionLimited
	}
	if len(l.sends) >= awaitingGlobalMax {
		l.counters.Suppressed(GateReasonAwaitingGlobalCap)
		if _, seen := l.overflow[sessionID]; !seen && l.pendingFrom.IsZero() {
			l.pendingFrom = now
		}
		l.overflow[sessionID] = now
		return admitGlobalCapped
	}
	l.sends = append(l.sends, now)
	l.lastBySess[sessionID] = now
	return admitPush
}

// prune drops state older than its window. Must hold l.mu.
func (l *awaitingLimiter) prune(now time.Time) {
	cut := 0
	for cut < len(l.sends) && now.Sub(l.sends[cut]) >= awaitingGlobalWindow {
		cut++
	}
	l.sends = l.sends[cut:]
	for id, at := range l.overflow {
		if now.Sub(at) >= awaitingGlobalWindow {
			delete(l.overflow, id)
		}
	}
	for id, at := range l.lastBySess {
		if now.Sub(at) >= awaitingSessionInterval {
			delete(l.lastBySess, id)
		}
	}
	if len(l.overflow) == 0 {
		l.pendingFrom = time.Time{}
	}
}

// digestDue returns the digest body once overflow has waited awaitingDigestDelay,
// and marks it sent so the same overflow is not announced twice.
func (l *awaitingLimiter) digestDue() (body string, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(now)
	if l.pendingFrom.IsZero() || now.Sub(l.pendingFrom) < awaitingDigestDelay {
		return "", false
	}
	l.pendingFrom = time.Time{}
	l.counters.Digest()
	n := len(l.overflow)
	noun := "sessions"
	if n == 1 {
		noun = "session"
	}
	return fmt.Sprintf("%d more %s finished and are waiting", n, noun), true
}
