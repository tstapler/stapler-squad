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

// StartDeliverySubscriber subscribes to the EventBus and fans push notifications
// out to all provided Notifiers. It exits when ctx is cancelled.
// A single failing Notifier does not prevent delivery to the others.
func StartDeliverySubscriber(ctx context.Context, bus *events.EventBus, notifiers []Notifier) {
	if bus == nil {
		log.Warn("DeliverySubscriber EventBus is nil, not starting")
		return
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		log.Info("DeliverySubscriber started", "notifiers", len(notifiers))
		defer log.Info("DeliverySubscriber stopped")

		var mu sync.Mutex
		lastSent := make(map[string]time.Time)
		const dedupWindow = 2 * time.Second

		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				if event == nil {
					continue
				}

				dn, ok := buildDeliveryNotification(event)
				if !ok {
					continue
				}

				// Dedup: skip if the same tag was sent within the dedup window.
				mu.Lock()
				if last, seen := lastSent[dn.Tag]; seen && time.Since(last) < dedupWindow {
					mu.Unlock()
					continue
				}
				lastSent[dn.Tag] = time.Now()
				mu.Unlock()

				fanout(ctx, notifiers, dn)

			case <-ctx.Done():
				return
			}
		}
	}()
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
	case events.EventSessionStatusChanged:
		return newStatus == session.Stopped || newStatus == session.NeedsApproval
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
func buildDeliveryNotification(event *events.Event) (DeliveryNotification, bool) {
	switch event.Type {
	case events.EventSessionStatusChanged:
		return buildStatusChangeNotification(event)
	case events.EventNotification:
		return buildInlineNotification(event)
	default:
		return DeliveryNotification{}, false
	}
}

func buildStatusChangeNotification(event *events.Event) (DeliveryNotification, bool) {
	if !shouldNotify(event.Type, 0, 0, event.NewStatus) {
		return DeliveryNotification{}, false
	}

	sess := event.Session
	var title, body, tag string
	var data map[string]interface{}
	requireInteraction := false
	renotify := false

	switch event.NewStatus {
	case session.Stopped:
		title = "Session Completed"
		if sess != nil {
			body = fmt.Sprintf("Session '%s' has completed", sess.Title)
			tag = "session-completed-" + stableID(sess)
			data = buildDataMap(sess, "SESSION_COMPLETE", false)
		}
	case session.NeedsApproval:
		title = "Approval Required"
		requireInteraction = true
		renotify = true
		if sess != nil {
			body = fmt.Sprintf("Session '%s' requires approval", sess.Title)
			tag = "approval-required-" + stableID(sess)
			data = buildDataMap(sess, "APPROVAL_NEEDED", true)
		}
	}

	if title == "" || body == "" {
		return DeliveryNotification{}, false
	}

	return DeliveryNotification{
		Title:              title,
		Body:               body,
		Icon:               "/icons/icon-192.png",
		Tag:                tag,
		Data:               data,
		RequireInteraction: requireInteraction,
		Renotify:           renotify,
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
	case events.EventSessionStatusChanged:
		return buildApprovalNotification(sess)
	default:
		return buildCompletedNotification(sess)
	}
}

// buildApprovalNotification constructs an approval-required notification for sess.
func buildApprovalNotification(sess *session.Instance) DeliveryNotification {
	return DeliveryNotification{
		Title:              "Approval Required",
		Body:               fmt.Sprintf("Session '%s' requires approval", sess.Title),
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
		Body:               fmt.Sprintf("Session '%s' has completed", sess.Title),
		Icon:               "/icons/icon-192.png",
		Tag:                "session-completed-" + stableID(sess),
		Data:               buildDataMap(sess, "SESSION_COMPLETE", false),
		RequireInteraction: false,
		Renotify:           false,
	}
}

// stableID returns the stable identifier for a session: ID when non-empty, Title otherwise.
func stableID(sess *session.Instance) string {
	if sess.ID != "" {
		return sess.ID
	}
	return sess.Title
}

// buildDataMap builds the FCM-compatible data map for a notification.
func buildDataMap(sess *session.Instance, notifType string, isApproval bool) map[string]interface{} {
	id := stableID(sess)
	data := map[string]interface{}{
		"sessionId":        id,
		"sessionTitle":     sess.Title,
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
