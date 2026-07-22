package services

import (
	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/server/events"
)

// EventBusNotifier adapts an *events.EventBus to session.Notifier. The session
// package cannot import pkg/events directly (pkg/events imports session, so the
// reverse import would be a cycle), so this adapter lives here instead and is
// wired in via BacklogLifecycleListener.SetNotifier / BacklogService.SetEventBus.
type EventBusNotifier struct {
	Bus *events.EventBus
}

// Notify implements session.Notifier.
func (n *EventBusNotifier) Notify(itemID, title, message string, notificationType, priority int32) {
	if n == nil || n.Bus == nil {
		return
	}
	// itemID is threaded through as the event's sessionID (not just stuffed into
	// metadata) so the notification subscriber's coalescing key
	// (sessionID:notificationType, see server/notifications/subscriber.go) differentiates
	// between different backlog items. Leaving sessionID empty made every same-type
	// notification for every item share one coalescing bucket, so two different items'
	// same-type notifications landing in the same 500ms window silently clobbered each
	// other in the persisted history (the live toast still fired — only the durable
	// record was lost).
	n.Bus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		notificationType, priority,
		title, message,
		map[string]string{"item_id": itemID},
	))
}
