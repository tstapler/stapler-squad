package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Dequeue spawn-failure backoff. A claim whose spawn fails is rolled back to
// ready/queued, and without a gate the next sweep re-claimed it immediately (one
// ready<->in_progress flip pair per minute). Real spawn failures ride the shared
// remediation gate on a durable spawn_failed row (so the deadline survives restarts and a
// persistently failing item is parked and visible): real gaps are 30m, 2h, 8h, 24h, then
// two cold retries 7d apart, after which retries stop until Reset.
//
// Not every refused spawn is a failure of the item. Gate refusals and a systemic outage
// are held briefly in memory instead of spending the durable budget (see classifySpawnFailure
// and dequeueSpawnGuard); that hold is lost on restart, costing one extra attempt per item.

const (
	// dequeueRefusalHold bounds flip churn for a refusal that resolves on its own.
	dequeueRefusalHold = 10 * time.Minute
	// dequeueSystemicHold spaces retries while the spawn path looks down for everyone.
	dequeueSystemicHold = 30 * time.Minute
	// dequeueSystemicItems is how many distinct items must fail with no success in
	// between before the failures are attributed to the spawn path, not the items.
	dequeueSystemicItems = 3
	// dequeueStreakWindow is how long a failure counts toward the streak. Without decay,
	// unrelated items failing days apart would add up to a phantom outage.
	dequeueStreakWindow = time.Hour
	// dequeueSystemicMaxHolds bounds how often one item is excused as an outage victim. A
	// real outage ends within a few holds; an item still failing after that is broken
	// itself, and must climb the durable ladder so it parks and the operator is told.
	dequeueSystemicMaxHolds = 2
	// dequeueRefusalEscalateAfter is how many consecutive refusals (about an hour at the
	// 10-minute hold) mean the cause is persistent, so the item takes the durable path.
	dequeueRefusalEscalateAfter = 6
)

type spawnFailureKind int

const (
	// spawnFailureReal is a spawn that was attempted and failed: counts against the item.
	spawnFailureReal spawnFailureKind = iota
	// spawnFailureRefusal is a gate that declined to spawn (missing repo_path/plan,
	// repeated_noop_dispatch, an active work session, SessionCreator unwired).
	spawnFailureRefusal
	// spawnFailureCancelled is our own context ending (shutdown).
	spawnFailureCancelled
)

func classifySpawnFailure(ctx context.Context, err error) spawnFailureKind {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return spawnFailureCancelled
	}
	switch connect.CodeOf(err) {
	case connect.CodeFailedPrecondition, connect.CodeAlreadyExists, connect.CodeUnimplemented:
		return spawnFailureRefusal
	}
	return spawnFailureReal
}

// dequeueSpawnGuard is the in-memory half of the backoff: per-item holds for failures that
// must not spend the durable budget, and the cross-item streak that detects a systemic
// outage. The zero value is ready to use; now is overridable for tests.
type dequeueSpawnGuard struct {
	mu      sync.Mutex
	now     func() time.Time
	holds   map[string]time.Time
	streak  map[string]time.Time // items with a real failure inside the window, since the last success
	alerted bool
	// sysHolds counts systemic holds per item; deliberately survives other items' successes.
	sysHolds map[string]sysHoldEntry
	// refusals counts consecutive refusals per item.
	refusals map[string]int
}

type sysHoldEntry struct {
	n    int
	last time.Time
}

func (g *dequeueSpawnGuard) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func (g *dequeueSpawnGuard) held(itemID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.holds[itemID]
	if ok && !g.clock().Before(until) {
		delete(g.holds, itemID)
		return false
	}
	return ok
}

func (g *dequeueSpawnGuard) hold(itemID string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.holds == nil {
		g.holds = map[string]time.Time{}
	}
	g.holds[itemID] = g.clock().Add(d)
}

// noteRealFailure adds itemID to the failure streak. systemic reports that enough distinct
// items are failing together and this item is still excused from the durable budget;
// firstAlert is true once per streak so the operator is told once.
func (g *dequeueSpawnGuard) noteRealFailure(itemID string) (systemic, firstAlert bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	if g.streak == nil {
		g.streak = map[string]time.Time{}
	}
	for id, at := range g.streak {
		if now.Sub(at) > dequeueStreakWindow {
			delete(g.streak, id)
		}
	}
	g.streak[itemID] = now
	if len(g.streak) < dequeueSystemicItems {
		return false, false
	}
	if !g.excuseSystemicLocked(itemID, now) {
		return false, false
	}
	if !g.alerted {
		g.alerted, firstAlert = true, true
	}
	return true, firstAlert
}

// excuseSystemicLocked spends one of itemID's bounded systemic holds; false when exhausted.
func (g *dequeueSpawnGuard) excuseSystemicLocked(itemID string, now time.Time) bool {
	if g.sysHolds == nil {
		g.sysHolds = map[string]sysHoldEntry{}
	}
	e := g.sysHolds[itemID]
	if now.Sub(e.last) > dequeueStreakWindow {
		e.n = 0
	}
	if e.n >= dequeueSystemicMaxHolds {
		return false
	}
	g.sysHolds[itemID] = sysHoldEntry{n: e.n + 1, last: now}
	return true
}

// noteRefusal counts a consecutive refusal and reports whether it has persisted long
// enough to be charged durably.
func (g *dequeueSpawnGuard) noteRefusal(itemID string) (escalate bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.refusals == nil {
		g.refusals = map[string]int{}
	}
	g.refusals[itemID]++
	return g.refusals[itemID] >= dequeueRefusalEscalateAfter
}

// noteSuccess ends the streak: the spawn path works, so failures are the items' own again.
// The succeeding item's own counters are cleared; other items keep their exemption debt.
func (g *dequeueSpawnGuard) noteSuccess(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.streak, g.alerted = nil, false
	delete(g.holds, itemID)
	delete(g.sysHolds, itemID)
	delete(g.refusals, itemID)
}

// forget drops all of itemID's in-memory state (the operator changed the item).
func (g *dequeueSpawnGuard) forget(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.holds, itemID)
	delete(g.streak, itemID)
	delete(g.sysHolds, itemID)
	delete(g.refusals, itemID)
}

// dequeueBlockedSet reads, once per sweep, which items an earlier spawn failure's backoff
// still blocks. A lookup error fails open so a storage hiccup cannot strand the queue.
func (s *BacklogService) dequeueBlockedSet(ctx context.Context) map[string]bool {
	blocked, err := s.storage.RemediationBlockedItems(ctx, domain.StuckReasonSpawnFailed)
	if err != nil {
		log.Warn("[DequeueNextQueuedItems] spawn backoff lookup failed, proceeding", "error", err)
		return nil
	}
	return blocked
}

// dequeueSpawnBlocks reports whether item must sit out this sweep: an in-memory hold or an
// unexpired durable backoff (or park). Read-only.
func (s *BacklogService) dequeueSpawnBlocks(itemID string, blocked map[string]bool) bool {
	switch {
	case s.spawnGuard.held(itemID):
		log.Debug("[DequeueNextQueuedItems] skipping item: spawn hold active", "item", itemID)
		return true
	case blocked[itemID]:
		log.Debug("[DequeueNextQueuedItems] skipping item: spawn_failed backoff not elapsed or parked", "item", itemID)
		return true
	}
	return false
}

// recordDequeueSpawnFailure handles a failed spawn (item already rolled back to fromStatus).
// A real failure opens or refreshes the spawn_failed row and spends one remediation attempt,
// which sets the next deadline; the operator is notified when the budget is exhausted.
func (s *BacklogService) recordDequeueSpawnFailure(ctx context.Context, item session.BacklogItemData, fromStatus session.BacklogStatus, spawnErr error) {
	switch classifySpawnFailure(ctx, spawnErr) {
	case spawnFailureCancelled:
		return
	case spawnFailureRefusal:
		s.spawnGuard.hold(item.ID, dequeueRefusalHold)
		if s.spawnGuard.noteRefusal(item.ID) {
			// Persistent refusal: the durable ladder parks the item and notifies.
			s.chargeDequeueSpawnFailure(ctx, item, fromStatus, spawnErr)
		}
		return
	}
	if systemic, firstAlert := s.spawnGuard.noteRealFailure(item.ID); systemic {
		s.spawnGuard.hold(item.ID, dequeueSystemicHold)
		if firstAlert {
			s.notifyDequeueSpawnSystemic(item.ID, spawnErr)
		}
		return
	}
	s.chargeDequeueSpawnFailure(ctx, item, fromStatus, spawnErr)
}

// chargeDequeueSpawnFailure opens or refreshes the durable spawn_failed row and spends an attempt.
func (s *BacklogService) chargeDequeueSpawnFailure(ctx context.Context, item session.BacklogItemData, fromStatus session.BacklogStatus, spawnErr error) {
	msg := fmt.Sprintf("%scould not spawn a work session (%v); retrying with backoff", domain.DequeueSpawnFailedContextPrefix, spawnErr)
	applied, err := s.storage.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, fromStatus, msg)
	if err != nil {
		log.Warn("[DequeueNextQueuedItems] MarkStuck(spawn_failed) failed", "item", item.ID, "error", err)
		s.spawnGuard.hold(item.ID, dequeueRefusalHold) // no durable row, so don't re-claim every sweep
		return
	}
	if !applied {
		log.Info("[DequeueNextQueuedItems] item moved on before its spawn failure could be recorded", "item", item.ID, "expected", fromStatus)
		return
	}
	_, justParked, err := s.storage.RemediationDue(ctx, item.ID, domain.StuckReasonSpawnFailed)
	if err != nil {
		log.Warn("[DequeueNextQueuedItems] recording spawn failure attempt failed", "item", item.ID, "error", err)
		return
	}
	if justParked {
		s.notifyDequeueSpawnParked(ctx, item, spawnErr)
	}
}

func (s *BacklogService) notifyDequeueSpawnParked(ctx context.Context, item session.BacklogItemData, spawnErr error) {
	if s.eventBus == nil {
		return
	}
	next := "Automatic retries are limited; fix the cause or use Reset to retry sooner."
	if row, ok, err := s.storage.OpenStuckState(ctx, item.ID, domain.StuckReasonSpawnFailed); err == nil && ok {
		if row.RemediationAttempts >= session.MaxRemediationAttempts+session.MaxRemediationColdRetries {
			next = "Automatic retries have now stopped; fix the cause, then use Reset to retry."
		} else {
			next = fmt.Sprintf("The fast backoff is exhausted; up to %d more retries run a week apart, then they stop. Fix the cause or use Reset to retry sooner.", session.MaxRemediationColdRetries)
		}
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		item.ID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
		derivePriority(true, true),
		"Backlog spawn keeps failing",
		fmt.Sprintf("%s — spawning a work session failed repeatedly (last error: %v). %s", item.Title, spawnErr, next),
		map[string]string{"item_id": item.ID},
	))
}

func (s *BacklogService) notifyDequeueSpawnSystemic(itemID string, spawnErr error) {
	log.Warn("[DequeueNextQueuedItems] spawns failing across several items; pausing retries instead of spending per-item backoff",
		"items", dequeueSystemicItems, "hold", dequeueSystemicHold, "error", spawnErr)
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
		derivePriority(true, true),
		"Backlog spawns failing across items",
		fmt.Sprintf("Spawning work sessions failed for %d different items in a row (last error: %v). This looks like a problem with the spawn path, not the items; retries are spaced %v apart and items are not being penalised until a spawn succeeds.", dequeueSystemicItems, spawnErr, dequeueSystemicHold),
		map[string]string{"item_id": itemID},
	))
}

// clearDequeueSpawnFailure forgets an item's spawn failures — on a successful spawn, or when
// the operator changed the item (repo_path fix, manual transition or spawn) — so a later
// failure starts from the first backoff step instead of inheriting a stale count.
func (s *BacklogService) clearDequeueSpawnFailure(ctx context.Context, itemID string) {
	s.spawnGuard.forget(itemID)
	if _, err := s.storage.ResetStuckRemediation(ctx, itemID, domain.StuckReasonSpawnFailed); err != nil {
		log.Warn("[DequeueNextQueuedItems] reset spawn_failed counters failed", "item", itemID, "error", err)
	}
	if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonSpawnFailed); err != nil {
		log.Warn("[DequeueNextQueuedItems] resolve spawn_failed failed", "item", itemID, "error", err)
	}
}

// onDequeueSpawnSucceeded clears the item and ends the systemic-failure streak.
func (s *BacklogService) onDequeueSpawnSucceeded(ctx context.Context, itemID string) {
	s.spawnGuard.noteSuccess(itemID)
	s.clearDequeueSpawnFailure(ctx, itemID)
}
