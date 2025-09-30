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
