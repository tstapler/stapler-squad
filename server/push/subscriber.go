package push

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

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

	// Subscribe's second return is a subscriber ID for manual early unsubscribe;
	// unneeded here since Subscribe already auto-unsubscribes on ctx cancellation
	// (see its doc comment), which is this goroutine's own lifetime.
	ch, _ := bus.Subscribe(ctx)
	dedup := newDedupTracker(dedupWindow)

	go runDeliveryLoop(ctx, ch, dedup, notifiers, done)
	return done
}

// runDeliveryLoop drains ch until it closes or ctx is cancelled, delivering
// each event via deliverEvent, then closes done. Run as its own goroutine by
// StartDeliverySubscriber.
func runDeliveryLoop(ctx context.Context, ch <-chan *events.Event, dedup *dedupTracker, notifiers []Notifier, done chan<- struct{}) {
	defer close(done)
	log.Info("DeliverySubscriber started", "notifiers", len(notifiers))
	defer log.Info("DeliverySubscriber stopped")

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			deliverEvent(ctx, event, dedup, notifiers)
		case <-ctx.Done():
			return
		}
	}
}

// deliverEvent converts event into a DeliveryNotification and fans it out to
// notifiers, dropping the event entirely if it doesn't warrant a notification
// or if dedup has already seen its tag within its window.
func deliverEvent(ctx context.Context, event *events.Event, dedup *dedupTracker, notifiers []Notifier) {
	if event == nil {
		return
	}
	dn, ok := buildDeliveryNotification(event)
	if !ok {
		return
	}
	if !dedup.allow(dn.Tag) {
		return
	}
	fanout(ctx, notifiers, dn)
}

// dedupTracker suppresses re-delivering a notification with the same tag
// within a short window of a prior delivery (e.g. two EventBus deliveries
// describing the same logical change arriving close together).
type dedupTracker struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
	window   time.Duration
}

func newDedupTracker(window time.Duration) *dedupTracker {
	return &dedupTracker{lastSent: make(map[string]time.Time), window: window}
}

// allow reports whether tag may be sent now, recording the send time if so.
func (d *dedupTracker) allow(tag string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, seen := d.lastSent[tag]; seen && time.Since(last) < d.window {
		return false
	}
	d.lastSent[tag] = time.Now()
	return true
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
//
// Push only fires for priority == URGENT (both the urgent and important axes true —
// see session.Notifier's doc comment), tightened down from the old ">= HIGH" threshold
// because HIGH also covered "important but not urgent" notifications that shouldn't
// interrupt the user. age additionally gates URGENT on urgentTTL: urgency is time-bound
// ("a 1-hour-old notification is no longer urgent") even when importance isn't, so an
// event older than urgentTTL by the time it's evaluated here no longer pushes on
// priority alone — only the notificationType == typeApproval override still fires
// regardless of age, since an approval request stays actionable until resolved.
func shouldNotify(
	eventType events.EventType,
	priority int32,
	notificationType int32,
	age time.Duration,
) bool {
	switch eventType {
	case events.EventNotification:
		if priority == priorityUrgent && age < urgentTTL {
			return true
		}
		if notificationType == typeApproval {
			return true
		}
		return false
	default:
		// EventSessionUpdated isn't handled here: buildDeliveryNotification routes
		// it straight to buildStatusChangeNotification instead (see there).
		return false
	}
}

// buildDeliveryNotification converts a raw Event into a DeliveryNotification.
// Returns (dn, true) when the event should be delivered; (zero, false) otherwise.
func buildDeliveryNotification(event *events.Event) (DeliveryNotification, bool) {
	switch event.Type {
	case events.EventSessionUpdated:
		return buildStatusChangeNotification(event)
	case events.EventNotification:
		return buildInlineNotification(event)
	default:
		return DeliveryNotification{}, false
	}
}

func buildStatusChangeNotification(event *events.Event) (DeliveryNotification, bool) {
	sess := event.Session
	if sess == nil {
		return DeliveryNotification{}, false
	}
	// Require "status" in UpdatedFields, not just a Stopped snapshot: without this,
	// any unrelated update to an already-Stopped session (a title rename, goal
	// change, checkpoint, PR-status sync, ...) re-fires this push, because the
	// check below only looks at current status, not whether this event is the
	// transition that produced it.
	if !slices.Contains(event.UpdatedFields, events.FieldStatus) {
		return DeliveryNotification{}, false
	}
	// Read via the locked accessor, not the raw field: Status is written under
	// Instance.stateMutex (see transitionTo), and this runs on the EventBus
	// subscriber goroutine, concurrently with the instance's own goroutine.
	if session.Status(sess.GetStatus()) != session.Stopped {
		return DeliveryNotification{}, false
	}

	title := "Session Completed"
	body := fmt.Sprintf("Session '%s' has completed", sess.GetTitle())
	tag := "session-completed-" + stableID(sess)
	data := buildDataMap(sess, "SESSION_COMPLETE")

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
	if !shouldNotify(event.Type, event.NotificationPriority, event.NotificationType, time.Since(event.Timestamp)) {
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
		Data:               buildApprovalDataMap(sess),
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
		Data:               buildDataMap(sess, "SESSION_COMPLETE"),
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

// buildDataMap builds the FCM-compatible data map for a non-approval notification.
func buildDataMap(sess *session.Instance, notifType string) map[string]interface{} {
	return baseDataMap(sess, notifType)
}

// buildApprovalDataMap builds the FCM-compatible data map for an
// approval-required notification, adding the actionable review/later buttons
// a plain buildDataMap notification doesn't need.
func buildApprovalDataMap(sess *session.Instance) map[string]interface{} {
	data := baseDataMap(sess, "APPROVAL_NEEDED")
	data["actions"] = []map[string]string{
		{"action": "review", "title": "Review"},
		{"action": "later", "title": "Later"},
	}
	return data
}

func baseDataMap(sess *session.Instance, notifType string) map[string]interface{} {
	id := stableID(sess)
	return map[string]interface{}{
		"sessionId":        id,
		"sessionTitle":     sess.GetTitle(),
		"notificationType": notifType,
		"timestamp":        time.Now().Unix(),
		"url":              buildSessionURL(id),
	}
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
