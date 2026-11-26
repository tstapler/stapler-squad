package notify

import (
	"strconv"
	"sync"
	"time"
)

// slackReplayGuardMaxEntries bounds the guard's memory. Only signature-verified
// requests are recorded, so real click volume stays far below it.
const slackReplayGuardMaxEntries = 10_000

type slackClaim int

const (
	slackClaimGranted slackClaim = iota
	slackClaimInFlight
	slackClaimProcessed
	slackClaimExpired
	slackClaimFull
)

type slackRequestKey struct{ timestamp, signature string }

type slackRequestEntry struct {
	signedAt  int64
	processed bool
}

// slackReplayGuard remembers signature-verified requests for as long as their
// signature stays acceptable, so one signed request is acted on once. State is
// in memory only: it does not survive a restart.
type slackReplayGuard struct {
	mu         sync.Mutex
	entries    map[slackRequestKey]slackRequestEntry
	maxEntries int
	now        func() time.Time
}

func newSlackReplayGuard(now func() time.Time, maxEntries int) *slackReplayGuard {
	return &slackReplayGuard{entries: map[slackRequestKey]slackRequestEntry{}, maxEntries: maxEntries, now: now}
}

// claim reserves a verified request. A granted claim must be followed by
// commit or release.
func (g *slackReplayGuard) claim(key slackRequestKey) slackClaim {
	signedAt, err := strconv.ParseInt(key.timestamp, 10, 64)
	if err != nil {
		return slackClaimExpired
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// The clock is read under the lock so that a request whose entry was just
	// evicted is also seen as expired here.
	oldest := g.now().Unix() - int64(slackSignatureReplayWindow/time.Second)
	if signedAt < oldest {
		return slackClaimExpired
	}
	for k, e := range g.entries {
		if e.signedAt < oldest {
			delete(g.entries, k)
		}
	}

	if e, ok := g.entries[key]; ok {
		if e.processed {
			return slackClaimProcessed
		}
		return slackClaimInFlight
	}
	if len(g.entries) >= g.maxEntries {
		return slackClaimFull
	}
	g.entries[key] = slackRequestEntry{signedAt: signedAt}
	return slackClaimGranted
}

// commit marks a granted claim as processed. It never re-adds an evicted entry.
func (g *slackReplayGuard) commit(key slackRequestKey) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if e, ok := g.entries[key]; ok {
		e.processed = true
		g.entries[key] = e
	}
}

// release forgets a failed request, so that a retry of it is processed again.
func (g *slackReplayGuard) release(key slackRequestKey) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.entries, key)
}
