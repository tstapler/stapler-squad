package events

import (
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"time"
)

// EventType represents the type of session event that occurred.
type EventType string

const (
	// EventSessionCreated is emitted when a new session is created
	EventSessionCreated EventType = "session.created"
	// EventSessionUpdated is emitted when session properties are modified
	EventSessionUpdated EventType = "session.updated"
	// EventSessionDeleted is emitted when a session is deleted
	EventSessionDeleted EventType = "session.deleted"
	// EventUserInteraction is emitted when user interacts with a session
	EventUserInteraction EventType = "session.user_interaction"
	// EventSessionAcknowledged is emitted when user acknowledges a session
	EventSessionAcknowledged EventType = "session.acknowledged"
	// EventApprovalResponse is emitted when user responds to an approval prompt
	EventApprovalResponse EventType = "session.approval_response"
	// EventNotification is emitted when a session sends a notification
	EventNotification EventType = "session.notification"
	// EventBacklogItemChanged is emitted when a backlog item is mutated
	// (status transition, verdict recorded, session attached, item updated,
	// archived, removed, or triage progress updated).
	EventBacklogItemChanged EventType = "backlog_item_changed"
)

// BacklogChangeKind identifies which kind of backlog item mutation a
// BacklogItemEventPayload describes.
type BacklogChangeKind string

const (
	// BacklogChangeStatusTransition is emitted when an item's status changes.
	BacklogChangeStatusTransition BacklogChangeKind = "status_transition"
	// BacklogChangeVerdictRecorded is emitted when a review verdict is saved.
	BacklogChangeVerdictRecorded BacklogChangeKind = "verdict_recorded"
	// BacklogChangeSessionAttached is emitted when a session is attached to an item.
	BacklogChangeSessionAttached BacklogChangeKind = "session_attached"
	// BacklogChangeItemUpdated is emitted when item fields (title, description, etc.) change.
	BacklogChangeItemUpdated BacklogChangeKind = "item_updated"
	// BacklogChangeItemArchived is emitted when an item is archived.
	BacklogChangeItemArchived BacklogChangeKind = "item_archived"
	// BacklogChangeItemRemoved is emitted when an item is deleted.
	BacklogChangeItemRemoved BacklogChangeKind = "item_removed"
	// BacklogChangeTriageProgressUpdated is emitted when in-flight triage progress
	// is written (UpdateItemSessionTriageResult). Converts to the existing
	// BacklogItemUpdatedEvent oneof variant on the wire, not a new proto message.
	BacklogChangeTriageProgressUpdated BacklogChangeKind = "triage_progress_updated"
)

// BacklogItemEventPayload carries the backlog-specific data for an
// EventBacklogItemChanged event. Only the fields relevant to Kind are
// expected to be populated.
type BacklogItemEventPayload struct {
	// Kind identifies which backlog mutation this payload describes.
	Kind BacklogChangeKind
	// Item is the current snapshot of the backlog item after the mutation.
	Item *session.BacklogItemData
	// OldStatus is the prior status for BacklogChangeStatusTransition.
	OldStatus string
	// NewStatus is the new status for BacklogChangeStatusTransition.
	NewStatus string
	// UpdatedFields lists which fields changed for BacklogChangeItemUpdated
	// (and BacklogChangeTriageProgressUpdated).
	UpdatedFields []string
	// SessionID identifies the session for BacklogChangeSessionAttached.
	SessionID string
	// ClaimantHostID is the attaching process's own stable host identifier for
	// BacklogChangeSessionAttached, mirrored from session.BacklogItemChange —
	// never derived from the session being attached.
	ClaimantHostID string
	// ArchivedAt is the archival timestamp for BacklogChangeItemArchived.
	ArchivedAt *time.Time
	// RemovedReason describes why an item was removed for BacklogChangeItemRemoved.
	RemovedReason string
	// Verdict mirrors BacklogItemChange.Verdict one-to-one; populated only
	// when Kind == BacklogChangeVerdictRecorded, copied straight through by
	// the adapter so the verdict reaches subscribers as first-class payload
	// data rather than something derived by joining item_sessions.
	Verdict *session.ReviewVerdictData
	// IsSnapshot is true when this event was generated as part of an
	// initial-snapshot send (e.g. WatchBacklogItems's first batch) rather
	// than a live mutation.
	IsSnapshot bool
}

// Event represents a session state change event.
// This is the internal Go representation that will be converted to protobuf events.
type Event struct {
	// Seq is assigned by the EventBus when Publish is called. Zero means unpublished.
	Seq uint64
	// Type of the event
	Type EventType
	// Timestamp when the event occurred
	Timestamp time.Time
	// Session affected by the event (may be nil for delete events)
	Session *session.Instance
	// SessionID for delete events when Session is nil
	SessionID string
	// UpdatedFields tracks which fields were modified (for update events)
	UpdatedFields []string
	// OldStatus for status change events
	OldStatus session.Status
	// NewStatus for status change events
	NewStatus session.Status
	// DetectedStatus is kept for legacy compatibility; no longer serialized to wire.
	DetectedStatus string
	// DetectedContext is the human-readable context from the terminal detector
	// (e.g. "Waiting for tool approval"). Empty when DetectedStatus is empty.
	DetectedContext string
	// DetectedStatusTyped is the typed DetectedStatus for SessionUpdatedEvent.
	// Zero value (detection.StatusUnknown) means no detection data is available.
	DetectedStatusTyped detection.DetectedStatus
	// InteractionType for user interaction events
	InteractionType string
	// Approved for approval response events (true = approved, false = denied)
	Approved bool
	// Context provides additional context about the event
	Context string
	// Notification fields for notification events
	NotificationID       string
	NotificationType     int32 // Maps to sessionv1.NotificationType
	NotificationPriority int32 // Maps to sessionv1.NotificationPriority
	NotificationTitle    string
	NotificationMessage  string
	NotificationMetadata map[string]string
	// BacklogItemPayload carries backlog item change data for
	// EventBacklogItemChanged events. Nil for all other event types.
	BacklogItemPayload *BacklogItemEventPayload
}

// NewSessionCreatedEvent creates an event for session creation.
func NewSessionCreatedEvent(sess *session.Instance) *Event {
	return &Event{
		Type:      EventSessionCreated,
		Timestamp: time.Now(),
		Session:   sess,
	}
}

// NewSessionUpdatedEvent creates an event for session updates.
func NewSessionUpdatedEvent(sess *session.Instance, updatedFields []string) *Event {
	return &Event{
		Type:          EventSessionUpdated,
		Timestamp:     time.Now(),
		Session:       sess,
		UpdatedFields: updatedFields,
	}
}

// NewSessionUpdatedEventWithDetection creates a session update event that includes
// typed detected-status information from the terminal detection layer.
// Use this instead of NewSessionUpdatedEvent when the detection state is known
// and should be propagated to frontend clients (e.g. the UpdateSession RPC path).
func NewSessionUpdatedEventWithDetection(
	sess *session.Instance,
	updatedFields []string,
	detectedStatus detection.DetectedStatus,
	detectedContext string,
) *Event {
	return &Event{
		Type:                EventSessionUpdated,
		Timestamp:           time.Now(),
		Session:             sess,
		UpdatedFields:       updatedFields,
		DetectedStatusTyped: detectedStatus,
		DetectedContext:     detectedContext,
	}
}

// NewSessionDeletedEvent creates an event for session deletion.
func NewSessionDeletedEvent(sessionID string) *Event {
	return &Event{
		Type:      EventSessionDeleted,
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
}

// NewUserInteractionEvent creates an event for user interactions.
func NewUserInteractionEvent(sessionID, interactionType, context string) *Event {
	return &Event{
		Type:            EventUserInteraction,
		Timestamp:       time.Now(),
		SessionID:       sessionID,
		InteractionType: interactionType,
		Context:         context,
	}
}

// NewSessionAcknowledgedEvent creates an event for session acknowledgments.
func NewSessionAcknowledgedEvent(sessionID, reason string) *Event {
	return &Event{
		Type:      EventSessionAcknowledged,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Context:   reason,
	}
}

// NewApprovalResponseEvent creates an event for approval responses.
func NewApprovalResponseEvent(sessionID string, approved bool, context string) *Event {
	return &Event{
		Type:      EventApprovalResponse,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Approved:  approved,
		Context:   context,
	}
}

// NewBacklogItemChangedEvent creates an event for a backlog item mutation.
func NewBacklogItemChangedEvent(payload *BacklogItemEventPayload) *Event {
	return &Event{
		Type:               EventBacklogItemChanged,
		Timestamp:          time.Now(),
		BacklogItemPayload: payload,
	}
}

// NewNotificationEvent creates an event for session notifications.
func NewNotificationEvent(
	sessionID string,
	sessionName string,
	notificationID string,
	notificationType int32,
	priority int32,
	title string,
	message string,
	metadata map[string]string,
) *Event {
	return &Event{
		Type:                 EventNotification,
		Timestamp:            time.Now(),
		SessionID:            sessionID,
		Context:              sessionName,
		NotificationID:       notificationID,
		NotificationType:     notificationType,
		NotificationPriority: priority,
		NotificationTitle:    title,
		NotificationMessage:  message,
		NotificationMetadata: metadata,
	}
}
