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
	n.Bus.Publish(events.NewNotificationEvent(
		"", "", uuid.New().String(),
		notificationType, priority,
		title, message,
		map[string]string{"item_id": itemID},
	))
}
