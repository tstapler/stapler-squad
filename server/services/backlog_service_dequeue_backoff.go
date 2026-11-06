package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Dequeue spawn-failure backoff. A claim whose spawn fails is rolled back to
// ready/queued, and without a gate the next sweep re-claimed it immediately (one
// ready<->in_progress flip pair per minute). Failures now ride the shared remediation
// gate (30m/2h/8h/24h/72h, then bounded cold retries) on a durable spawn_failed row, so
// the deadline survives restarts and a persistently failing item is parked and visible.

// dequeueSpawnBackoffBlocks reports whether item must sit out a sweep because an earlier
// spawn failure's backoff has not elapsed (or the item is parked). Read-only; a lookup
// error fails open so a storage hiccup cannot strand the queue.
func (s *BacklogService) dequeueSpawnBackoffBlocks(ctx context.Context, itemID string) bool {
	blocked, err := s.storage.RemediationBlocked(ctx, itemID, domain.StuckReasonSpawnFailed)
	if err != nil {
		log.Warn("[DequeueNextQueuedItems] spawn backoff lookup failed, proceeding", "item", itemID, "error", err)
		return false
	}
	return blocked
}

// recordDequeueSpawnFailure opens or refreshes the spawn_failed row (item already rolled
// back to fromStatus) and spends one remediation attempt, which sets the next deadline.
// The operator is notified once, when the attempt budget is exhausted.
func (s *BacklogService) recordDequeueSpawnFailure(ctx context.Context, item session.BacklogItemData, fromStatus session.BacklogStatus, spawnErr error) {
	msg := fmt.Sprintf("dequeue could not spawn a work session (%v); retrying with backoff", spawnErr)
	if _, err := s.storage.MarkStuck(ctx, item.ID, domain.StuckReasonSpawnFailed, fromStatus, msg); err != nil {
		log.Warn("[DequeueNextQueuedItems] MarkStuck(spawn_failed) failed", "item", item.ID, "error", err)
		return
	}
	_, justParked, err := s.storage.RemediationDue(ctx, item.ID, domain.StuckReasonSpawnFailed)
	if err != nil {
		log.Warn("[DequeueNextQueuedItems] recording spawn failure attempt failed", "item", item.ID, "error", err)
		return
	}
	if justParked {
		s.notifyDequeueSpawnParked(item, spawnErr)
	}
}

func (s *BacklogService) notifyDequeueSpawnParked(item session.BacklogItemData, spawnErr error) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		item.ID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
		derivePriority(true, true),
		"Backlog spawn keeps failing",
		fmt.Sprintf("%s — spawning a work session failed repeatedly (last error: %v). Automatic retries are now spaced a week apart; fix the cause or use Reset to retry sooner.", item.Title, spawnErr),
		map[string]string{"item_id": item.ID},
	))
}

// clearDequeueSpawnFailure forgets prior failures once a spawn succeeds, so a later
// failure starts from the first backoff step instead of inheriting a stale count.
func (s *BacklogService) clearDequeueSpawnFailure(ctx context.Context, itemID string) {
	if _, err := s.storage.ResetStuckRemediation(ctx, itemID, domain.StuckReasonSpawnFailed); err != nil {
		log.Warn("[DequeueNextQueuedItems] reset spawn_failed counters failed", "item", itemID, "error", err)
	}
	if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonSpawnFailed); err != nil {
		log.Warn("[DequeueNextQueuedItems] resolve spawn_failed failed", "item", itemID, "error", err)
	}
}
