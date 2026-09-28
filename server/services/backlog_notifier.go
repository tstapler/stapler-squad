package services

import (
	"fmt"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/diagnose"
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

// DiagnoseEventNotification derives the live-toast title/message/
// notificationType/urgent/important fields for a completed diagnose dispatch
// outcome (Story 5.2.2, Task 5.2.2a), keyed off diagnose.DiagnoseOutcomeKind.
// Callers pass the result straight into (*EventBusNotifier).Notify, mirroring
// notifyReworkCapHit's inline title/message construction
// (backlog_service_triage.go) but as a reusable sibling function rather than
// duplicated per call site, since notifyDiagnoseEvent (diagnose_dispatcher.go)
// isn't the only place a diagnose outcome will eventually be reported from
// (see that function's doc comment).
func DiagnoseEventNotification(itemTitle string, outcome diagnose.DiagnoseOutcome) (title, message string, notificationType int32, urgent, important bool) {
	switch outcome.Kind {
	case diagnose.DiagnoseOutcomeKindNudged:
		return nudgedNotification(itemTitle)
	case diagnose.DiagnoseOutcomeKindBugFiled:
		return bugFiledNotification(itemTitle, outcome)
	case diagnose.DiagnoseOutcomeKindInconclusiveNoteFiled:
		return inconclusiveNotification(itemTitle, outcome)
	case diagnose.DiagnoseOutcomeKindSkippedSafetyGate:
		return skippedSafetyGateNotification(itemTitle, outcome)
	case diagnose.DiagnoseOutcomeKindDispatchFailed:
		return dispatchFailedNotification(itemTitle, outcome)
	default:
		return "Diagnose: outcome recorded",
			fmt.Sprintf("%s — diagnose dispatch completed with outcome %q.", itemTitle, outcome.Kind),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), false, false
	}
}

func nudgedNotification(itemTitle string) (title, message string, notificationType int32, urgent, important bool) {
	return "Diagnose: nudged stuck session",
		fmt.Sprintf("%s — the diagnostic agent nudged the stuck session.", itemTitle),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), false, true
}

func bugFiledNotification(itemTitle string, outcome diagnose.DiagnoseOutcome) (title, message string, notificationType int32, urgent, important bool) {
	bugItemID := ""
	if outcome.BugItemID != nil {
		bugItemID = *outcome.BugItemID
	}
	return "Diagnose: bug filed",
		fmt.Sprintf("%s — the diagnostic agent filed bug %s.", itemTitle, bugItemID),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), false, true
}

func inconclusiveNotification(itemTitle string, outcome diagnose.DiagnoseOutcome) (title, message string, notificationType int32, urgent, important bool) {
	note := ""
	if outcome.NoteText != nil {
		note = *outcome.NoteText
	}
	return "Diagnose: inconclusive",
		fmt.Sprintf("%s — the diagnostic agent couldn't reach a confident conclusion: %s", itemTitle, note),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), false, false
}

func skippedSafetyGateNotification(itemTitle string, outcome diagnose.DiagnoseOutcome) (title, message string, notificationType int32, urgent, important bool) {
	reason := ""
	if outcome.GateReason != nil {
		reason = outcome.GateReason.String()
	}
	return "Diagnose: nudge skipped (safety gate)",
		fmt.Sprintf("%s — a nudge was withheld: %s.", itemTitle, reason),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), true, true
}

func dispatchFailedNotification(itemTitle string, outcome diagnose.DiagnoseOutcome) (title, message string, notificationType int32, urgent, important bool) {
	reason := ""
	if outcome.FailureReason != nil {
		reason = *outcome.FailureReason
	}
	return "Diagnose: dispatch failed",
		fmt.Sprintf("%s — could not dispatch a diagnostic session: %s. Left for manual review.", itemTitle, reason),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), true, true
}
