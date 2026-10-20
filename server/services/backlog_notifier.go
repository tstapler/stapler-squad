package services

import (
	"strconv"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
)

// EventBusNotifier adapts an *events.EventBus to session.Notifier. The session
// package cannot import pkg/events directly (pkg/events imports session, so the
// reverse import would be a cycle), so this adapter lives here instead and is
// wired in via BacklogLifecycleListener.SetNotifier / BacklogService.SetEventBus.
type EventBusNotifier struct {
	Bus *events.EventBus
}

// Notify implements session.Notifier. urgent/important are converted to the stored
// NotificationPriority via derivePriority — see that function's doc comment.
func (n *EventBusNotifier) Notify(itemID, title, message string, notificationType int32, urgent, important bool) {
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
		notificationType, derivePriority(urgent, important),
		title, message,
		map[string]string{"item_id": itemID},
	))
}

// NotifySession implements session.Notifier for a session with no linked BacklogItem —
// see that interface method's doc comment (session/backlog_lifecycle.go) for why this
// must not reuse Notify. Metadata is deliberately left empty (no "item_id" key):
// NotificationItem.tsx's "View Session" link renders exactly when metadata["item_id"] is
// absent and sessionId is set.
func (n *EventBusNotifier) NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool) {
	if n == nil || n.Bus == nil {
		return
	}
	n.Bus.Publish(events.NewNotificationEvent(
		sessionID, "", uuid.New().String(),
		notificationType, derivePriority(urgent, important),
		title, message,
		map[string]string{},
	))
}

// NotifyDecision is Notify for an item awaiting an operator decision: it uses the
// needs-human CONFIRMATION_NEEDED type (ADR-002 decision 6, so the delivery gate
// keeps it for hidden sessions) and stamps decision_schema so the history page
// can offer the choices inline (Story 2.2).
//
// A schema that offers merge also carries pr_number (when prNumber > 0), so the
// Merge confirm names the pull request it will merge.
func (n *EventBusNotifier) NotifyDecision(itemID, title, message string, schema DecisionSchema, prNumber int) {
	if n == nil || n.Bus == nil {
		return
	}
	meta := map[string]string{metaItemID: itemID, metaDecisionSchema: EncodeDecisionSchema(schema)}
	if prNumber > 0 && schemaOffers(schema, choiceMerge) {
		meta[metaPRNumber] = strconv.Itoa(prNumber)
	}
	n.Bus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_CONFIRMATION_NEEDED), derivePriority(true, true),
		title, message,
		meta,
	))
}

// metaPRNumber is the pull request a merge decision acts on.
const metaPRNumber = "pr_number"

func schemaOffers(s DecisionSchema, choice string) bool {
	for _, c := range s.Choices {
		if c.Value == choice {
			return true
		}
	}
	return false
}

// NotifyAutoRemediating implements session.AutoRemediatingNotifier: Notify plus the
// auto_remediating stamp, so the server's IsPendingDecision treats the WARNING as
// informational while automation is still acting.
func (n *EventBusNotifier) NotifyAutoRemediating(itemID, title, message string, notificationType int32, urgent, important bool) {
	if n == nil || n.Bus == nil {
		return
	}
	n.Bus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		notificationType, derivePriority(urgent, important),
		title, message,
		map[string]string{"item_id": itemID, pkgevents.MetadataKeyAutoRemediating: "true"},
	))
}
