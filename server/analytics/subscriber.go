package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// statusEntryTTL bounds how long a session's last-known status is retained
	// in lastStatusByID after its last update. EventSessionDeleted only fires on
	// hard delete (DeleteSession RPC); sessions that are archived instead (the
	// more common path, e.g. ArchiveSession/UnarchiveSession/maybeAutoArchive)
	// never emit that event, so without this TTL sweep the map would grow
	// unbounded for the life of the process. 24h comfortably exceeds how long
	// this subscriber needs to remember a session's last status for transition
	// detection.
	statusEntryTTL = 24 * time.Hour

	// statusSweepInterval is how often the stale-entry sweep runs.
	statusSweepInterval = 10 * time.Minute
)

// statusEntry pairs a session's last-known status with the time it was last
// observed, so stale entries can be evicted by the periodic sweep.
type statusEntry struct {
	status   session.Status
	lastSeen time.Time
}

// analyticsSubscriber holds subscriber state including per-session last-known status
// for detecting transitions from EventSessionUpdated (which has no old_status field).
type analyticsSubscriber struct {
	provider       AnalyticsProvider
	mu             sync.Mutex
	lastStatusByID map[string]statusEntry
}

func newAnalyticsSubscriber(provider AnalyticsProvider) *analyticsSubscriber {
	return &analyticsSubscriber{
		provider:       provider,
		lastStatusByID: make(map[string]statusEntry),
	}
}

// sweepStaleStatusEntries deletes lastStatusByID entries whose lastSeen is
// older than statusEntryTTL. This bounds map growth for sessions that are
// archived (rather than hard-deleted) and therefore never publish
// EventSessionDeleted.
func (s *analyticsSubscriber) sweepStaleStatusEntries() {
	cutoff := time.Now().Add(-statusEntryTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, entry := range s.lastStatusByID {
		if entry.lastSeen.Before(cutoff) {
			delete(s.lastStatusByID, id)
		}
	}
}

// StartAnalyticsSubscriber subscribes to the EventBus and records analytics
// events for session lifecycle changes. It runs a goroutine that exits when ctx
// is cancelled. Unknown event types are logged and skipped.
//
// Mapping:
//
//	session.created          → event_name="session.created",      category="user_action", session_id=session.ID
//	session.deleted          → event_name="session.deleted",      category="user_action", session_id=event.SessionID
//	session.updated          → event_name="session.status_changed" when status transitions, category="user_action", labels={"old_status","new_status"}
//	session.user_interaction → event_name="session.user_interaction", category="user_action"
func StartAnalyticsSubscriber(ctx context.Context, bus *events.EventBus, provider AnalyticsProvider) {
	if bus == nil || provider == nil {
		log.Warn("analytics/subscriber EventBus or provider is nil, not starting subscriber")
		return
	}

	sub := newAnalyticsSubscriber(provider)
	ch, _ := bus.Subscribe(ctx)

	sweepTicker := time.NewTicker(statusSweepInterval)

	go func() {
		log.Info("analytics/subscriber started listening for session events")
		defer log.Info("analytics/subscriber stopped")
		defer sweepTicker.Stop()
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				if event == nil {
					continue
				}
				sub.recordFromEvent(ctx, event)

			case <-sweepTicker.C:
				sub.sweepStaleStatusEntries()

			case <-ctx.Done():
				return
			}
		}
	}()
}

// recordFromEvent maps an events.Event to an analytics.Event and records it.
// Unknown event types are logged and skipped without returning an error.
func (s *analyticsSubscriber) recordFromEvent(ctx context.Context, event *events.Event) {
	var ae Event

	switch event.Type {
	case events.EventSessionCreated:
		sessionID := ""
		if event.Session != nil {
			sessionID = event.Session.GetStableID()
		}
		ae = Event{
			EventName:     "session.created",
			EventCategory: "user_action",
			SessionID:     sessionID,
		}

	case events.EventSessionDeleted:
		sessionID := event.SessionID
		s.mu.Lock()
		delete(s.lastStatusByID, sessionID)
		s.mu.Unlock()
		ae = Event{
			EventName:     "session.deleted",
			EventCategory: "user_action",
			SessionID:     sessionID,
		}

	case events.EventSessionUpdated:
		if event.Session == nil {
			log.Debug("analytics/subscriber skipping session.updated with nil session")
			return
		}
		sess := event.Session
		sessionID := sess.GetStableID()
		// Read via the locked accessor, not the raw field: Status is written
		// under Instance.stateMutex from the instance's own goroutine,
		// concurrently with this EventBus subscriber goroutine.
		newStatus := session.Status(sess.GetStatus())

		s.mu.Lock()
		prev, seen := s.lastStatusByID[sessionID]
		oldStatus := prev.status
		s.lastStatusByID[sessionID] = statusEntry{status: newStatus, lastSeen: time.Now()}
		s.mu.Unlock()

		if !seen {
			// First event for this session — record initial status, no transition.
			return
		}
		if newStatus == oldStatus {
			// No status change — nothing to record.
			return
		}
		ae = Event{
			EventName:     "session.status_changed",
			EventCategory: "user_action",
			SessionID:     sessionID,
			Labels: map[string]string{
				"old_status": oldStatus.String(),
				"new_status": newStatus.String(),
			},
		}

	case events.EventUserInteraction:
		ae = Event{
			EventName:     "session.user_interaction",
			EventCategory: "user_action",
			SessionID:     event.SessionID,
		}

	default:
		log.Debug("analytics/subscriber skipping untracked event type", "type", event.Type)
		return
	}

	if err := s.provider.Record(ctx, ae); err != nil {
		log.Error("analytics/subscriber record error", "event", ae.EventName, "err", err)
	}
}
