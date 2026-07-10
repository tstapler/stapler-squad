package push

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// dedupWindow is the time window within which a duplicate notification tag is
// suppressed. Also used as the retention window for lastSent sweeping (see
// sweepExpiredSent) so stale dedup entries don't accumulate forever.
const dedupWindow = 2 * time.Second

// statusSweepInterval controls how often the delivery subscriber prunes
// lastSent entries older than dedupWindow and lastStatus entries older than
// statusEntryTTL, bounding both maps' memory growth over the lifetime of a
// long-running server process.
const statusSweepInterval = 1 * time.Minute

// statusEntryTTL bounds how long a session's last-known status is retained in
// lastStatus after its last update. EventSessionDeleted only fires on hard
// delete; sessions that are archived instead (the more common path) never
// emit it, so without this TTL sweep the map would grow unbounded for the
// life of the process — mirrors server/analytics/subscriber.go's
// statusEntryTTL for its analogous lastStatusByID map.
const statusEntryTTL = 24 * time.Hour

// statusEntry pairs a session's last-known status with the time it was last
// observed, so stale entries can be evicted by the periodic sweep.
type statusEntry struct {
	status   session.Status
	lastSeen time.Time
}

// deliveryState holds the delivery subscriber's mutable, shared-across-events
// state: the dedup window's lastSent timestamps and the last observed status
// per session. lastStatus exists because EventSessionUpdated carries no
// old/new status pair now that the dedicated SessionStatusChangedEvent was
// merged into it — without tracking the previous status ourselves, any
// unrelated update to an already-Stopped session (e.g. a rename, or PR URL
// discovery) would look identical to a genuine Active→Stopped transition and
// spuriously re-fire the "Session Completed" notification.
type deliveryState struct {
	mu         sync.Mutex
	lastSent   map[string]time.Time
	lastStatus map[string]statusEntry
}

func newDeliveryState() *deliveryState {
	return &deliveryState{
		lastSent:   make(map[string]time.Time),
		lastStatus: make(map[string]statusEntry),
	}
}

// forgetSession clears any tracked status for sessionID. Called on
// EventSessionDeleted so the map doesn't retain entries for sessions that no
// longer exist, mirroring the cleanup server/analytics/subscriber.go performs
// on its analogous lastStatusByID map.
func (s *deliveryState) forgetSession(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	delete(s.lastStatus, sessionID)
	s.mu.Unlock()
}

// sweepExpiredSent deletes lastSent entries whose dedup window has already
// elapsed as of now. Extracted as a pure function (operating on a plain map)
// so it can be exercised in tests deterministically, independent of the
// ticker's real-time interval.
func sweepExpiredSent(lastSent map[string]time.Time, now time.Time, window time.Duration) {
	for tag, sentAt := range lastSent {
		if now.Sub(sentAt) >= window {
			delete(lastSent, tag)
		}
	}
}

// sweepStaleLastStatus deletes lastStatus entries whose lastSeen is older
// than ttl. This bounds map growth for sessions that are archived (rather
// than hard-deleted) and therefore never publish EventSessionDeleted.
// Extracted as a pure function so it can be exercised in tests
// deterministically, independent of the ticker's real-time interval.
func sweepStaleLastStatus(lastStatus map[string]statusEntry, now time.Time, ttl time.Duration) {
	cutoff := now.Add(-ttl)
	for id, entry := range lastStatus {
		if entry.lastSeen.Before(cutoff) {
			delete(lastStatus, id)
		}
	}
}

// StartDeliverySubscriber subscribes to the EventBus and fans push notifications
// out to all provided Notifiers. It exits when ctx is cancelled.
// A single failing Notifier does not prevent delivery to the others.
// The returned channel is closed when the subscriber goroutine has fully exited.
func StartDeliverySubscriber(ctx context.Context, bus *events.EventBus, notifiers []Notifier) <-chan struct{} {
	done := make(chan struct{})
	if bus == nil {
		log.Warn("DeliverySubscriber EventBus is nil, not starting")
		close(done)
		return done
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		defer close(done)
		log.Info("DeliverySubscriber started", "notifiers", len(notifiers))
		defer log.Info("DeliverySubscriber stopped")

		state := newDeliveryState()

		sweepTicker := time.NewTicker(statusSweepInterval)
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

				if event.Type == events.EventSessionDeleted {
					state.forgetSession(event.SessionID)
					continue
				}

				dn, ok := state.buildDeliveryNotification(event)
				if !ok {
					continue
				}

				// Dedup: skip if the same tag was sent within the dedup window.
				state.mu.Lock()
				if last, seen := state.lastSent[dn.Tag]; seen && time.Since(last) < dedupWindow {
					state.mu.Unlock()
					continue
				}
				state.lastSent[dn.Tag] = time.Now()
				state.mu.Unlock()

				fanout(ctx, notifiers, dn)

			case <-sweepTicker.C:
				state.mu.Lock()
				now := time.Now()
				sweepExpiredSent(state.lastSent, now, dedupWindow)
				sweepStaleLastStatus(state.lastStatus, now, statusEntryTTL)
				state.mu.Unlock()

			case <-ctx.Done():
				return
			}
		}
	}()
	return done
}

// StartPushSubscriber is the legacy entry-point. New code should use
// StartDeliverySubscriber with an explicit []Notifier slice.
func StartPushSubscriber(ctx context.Context, bus *events.EventBus, pushService *services.PushService) {
	if pushService == nil {
		log.Warn("PushSubscriber push service is nil, not starting")
		return
	}
	StartDeliverySubscriber(ctx, bus, []Notifier{NewWebPushNotifier(pushService)})
}

// shouldNotify returns true when the event/priority/type combination warrants a
// push notification. Extracted as a pure function for easy table-driven testing.
func shouldNotify(
	eventType events.EventType,
	priority int32,
	notificationType int32,
	newStatus session.Status,
) bool {
	switch eventType {
	case events.EventSessionUpdated:
		return newStatus == session.Stopped
	case events.EventNotification:
		if priority >= priorityHigh {
			return true
		}
		if notificationType == typeApproval {
			return true
		}
		return false
	default:
		return false
	}
}

// buildDeliveryNotification converts a raw Event into a DeliveryNotification.
// Returns (dn, true) when the event should be delivered; (zero, false) otherwise.
func (s *deliveryState) buildDeliveryNotification(event *events.Event) (DeliveryNotification, bool) {
	switch event.Type {
	case events.EventSessionUpdated:
		return s.buildStatusChangeNotification(event)
	case events.EventNotification:
		return buildInlineNotification(event)
	default:
		return DeliveryNotification{}, false
	}
}

// buildStatusChangeNotification builds the "Session Completed" notification,
// but only when the session's status has just transitioned TO Stopped. It
// records the session's current status in s.lastStatus on every call so a
// later, unrelated EventSessionUpdated for the same already-Stopped session
// (e.g. a title rename, or RunOneShot republishing after discovering a
// github_pr_url) is recognized as a non-transition and does not re-fire the
// notification.
func (s *deliveryState) buildStatusChangeNotification(event *events.Event) (DeliveryNotification, bool) {
	sess := event.Session
	if sess == nil {
		return DeliveryNotification{}, false
	}
	id := stableID(sess)
	// Read via the locked accessor, not the raw field: Status is written under
	// Instance.stateMutex (see transitionTo), and this runs on the EventBus
	// subscriber goroutine, concurrently with the instance's own goroutine.
	newStatus := session.Status(sess.GetStatus())

	s.mu.Lock()
	prev, seen := s.lastStatus[id]
	oldStatus := prev.status
	s.lastStatus[id] = statusEntry{status: newStatus, lastSeen: time.Now()}
	s.mu.Unlock()

	if newStatus != session.Stopped {
		return DeliveryNotification{}, false
	}
	if seen && oldStatus == session.Stopped {
		// Already recorded as Stopped — this event didn't carry a genuine
		// Active→Stopped transition, so don't re-notify.
		return DeliveryNotification{}, false
	}

	title := "Session Completed"
	// Read via the locked accessor for the same race-safety reason as Status above.
	body := fmt.Sprintf("Session '%s' has completed", sess.GetTitle())
	tag := "session-completed-" + id
	data := buildDataMap(sess, "SESSION_COMPLETE", false)

	return DeliveryNotification{
		Title:              title,
		Body:               body,
		Icon:               "/icons/icon-192.png",
		Tag:                tag,
		Data:               data,
		RequireInteraction: false,
		Renotify:           false,
	}, true
}

func buildInlineNotification(event *events.Event) (DeliveryNotification, bool) {
	if !shouldNotify(event.Type, event.NotificationPriority, event.NotificationType, 0) {
		return DeliveryNotification{}, false
	}
	if event.NotificationTitle == "" || event.NotificationMessage == "" {
		return DeliveryNotification{}, false
	}

	requireInteraction := event.NotificationType == typeApproval
	renotify := event.NotificationType == typeApproval
	tag := "notification-" + event.NotificationID

	var data map[string]interface{}
	if event.SessionID != "" {
		data = map[string]interface{}{
			"sessionId":        event.SessionID,
			"notificationType": notificationTypeName(event.NotificationType),
			"timestamp":        time.Now().Unix(),
			"url":              buildSessionURL(event.SessionID),
		}
	}

	return DeliveryNotification{
		Title:              event.NotificationTitle,
		Body:               event.NotificationMessage,
		Icon:               "/icons/icon-192.png",
		Tag:                tag,
		Data:               data,
		RequireInteraction: requireInteraction,
		Renotify:           renotify,
	}, true
}

// buildNotificationForSession constructs a DeliveryNotification for a specific
// session event type. Used by tests and helper callers.
func buildNotificationForSession(sess *session.Instance, eventType events.EventType) DeliveryNotification {
	switch eventType {
	case events.EventSessionUpdated:
		return buildApprovalNotification(sess)
	default:
		return buildCompletedNotification(sess)
	}
}

// buildApprovalNotification constructs an approval-required notification for sess.
func buildApprovalNotification(sess *session.Instance) DeliveryNotification {
	return DeliveryNotification{
		Title:              "Approval Required",
		Body:               fmt.Sprintf("Session '%s' requires approval", sess.GetTitle()),
		Icon:               "/icons/icon-192.png",
		Tag:                "approval-required-" + stableID(sess),
		Data:               buildDataMap(sess, "APPROVAL_NEEDED", true),
		RequireInteraction: true,
		Renotify:           true,
	}
}

// buildCompletedNotification constructs a session-completed notification for sess.
func buildCompletedNotification(sess *session.Instance) DeliveryNotification {
	return DeliveryNotification{
		Title:              "Session Completed",
		Body:               fmt.Sprintf("Session '%s' has completed", sess.GetTitle()),
		Icon:               "/icons/icon-192.png",
		Tag:                "session-completed-" + stableID(sess),
		Data:               buildDataMap(sess, "SESSION_COMPLETE", false),
		RequireInteraction: false,
		Renotify:           false,
	}
}

// stableID returns the stable identifier for a session: ID when non-empty, Title otherwise.
// ID is set once at construction and never mutated, so it's safe to read directly; Title can
// be changed via Rename() under Instance.stateMutex, so it's read via the locked accessor.
func stableID(sess *session.Instance) string {
	if sess.ID != "" {
		return sess.ID
	}
	return sess.GetTitle()
}

// buildDataMap builds the FCM-compatible data map for a notification.
func buildDataMap(sess *session.Instance, notifType string, isApproval bool) map[string]interface{} {
	id := stableID(sess)
	data := map[string]interface{}{
		"sessionId":        id,
		"sessionTitle":     sess.GetTitle(),
		"notificationType": notifType,
		"timestamp":        time.Now().Unix(),
		"url":              buildSessionURL(id),
	}
	if isApproval {
		data["actions"] = []map[string]string{
			{"action": "review", "title": "Review"},
			{"action": "later", "title": "Later"},
		}
	}
	return data
}

// buildSessionURL returns the deep-link URL for a session, using the stable ID.
func buildSessionURL(sessionID string) string {
	return "/?session=" + url.QueryEscape(sessionID) + "&tab=terminal"
}

// notificationTypeName maps a proto NotificationType int32 to a string.
func notificationTypeName(t int32) string {
	switch t {
	case typeApproval:
		return "APPROVAL_NEEDED"
	default:
		return "GENERIC"
	}
}
