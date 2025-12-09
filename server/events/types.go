package events

import (
	"claude-squad/session"
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
	// EventSessionStatusChanged is emitted when session status transitions
	EventSessionStatusChanged EventType = "session.status_changed"
	// EventUserInteraction is emitted when user interacts with a session
	EventUserInteraction EventType = "session.user_interaction"
	// EventSessionAcknowledged is emitted when user acknowledges a session
	EventSessionAcknowledged EventType = "session.acknowledged"
	// EventApprovalResponse is emitted when user responds to an approval prompt
	EventApprovalResponse EventType = "session.approval_response"
	// EventNotification is emitted when a session sends a notification
	EventNotification EventType = "session.notification"
)

// Event represents a session state change event.
// This is the internal Go representation that will be converted to protobuf events.
type Event struct {
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

// NewSessionDeletedEvent creates an event for session deletion.
func NewSessionDeletedEvent(sessionID string) *Event {
	return &Event{
		Type:      EventSessionDeleted,
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
}

// NewSessionStatusChangedEvent creates an event for status transitions.
func NewSessionStatusChangedEvent(sess *session.Instance, oldStatus, newStatus session.Status) *Event {
	return &Event{
		Type:      EventSessionStatusChanged,
		Timestamp: time.Now(),
		Session:   sess,
		SessionID: sess.Title,
		OldStatus: oldStatus,
		NewStatus: newStatus,
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
