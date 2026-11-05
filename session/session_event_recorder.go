package session

import "time"

// SessionEventKind enumerates the kinds of session events (plan Glossary: SessionEventKind).
type SessionEventKind int

const (
	SessionEventStatusChanged SessionEventKind = iota
	SessionEventTurnEnded
	SessionEventAttentionRaised
	SessionEventAttentionCleared
	SessionEventAcknowledged
	SessionEventNotificationSuppressed
	SessionEventLifecycleStarted
	SessionEventLifecycleExited
	SessionEventLifecycleStopped
	SessionEventApprovalResolved
	// SessionEventOutput is activity-only (Touch); the recorder never logs it.
	SessionEventOutput

	sessionEventKindCount // must stay last
)

var sessionEventKindNames = [sessionEventKindCount]string{
	SessionEventStatusChanged:          "status_changed",
	SessionEventTurnEnded:              "turn_ended",
	SessionEventAttentionRaised:        "attention_raised",
	SessionEventAttentionCleared:       "attention_cleared",
	SessionEventAcknowledged:           "acknowledged",
	SessionEventNotificationSuppressed: "notification_suppressed",
	SessionEventLifecycleStarted:       "lifecycle_started",
	SessionEventLifecycleExited:        "lifecycle_exited",
	SessionEventLifecycleStopped:       "lifecycle_stopped",
	SessionEventApprovalResolved:       "approval_resolved",
	SessionEventOutput:                 "output",
}

func (k SessionEventKind) String() string {
	if k < 0 || k >= sessionEventKindCount {
		return "unknown"
	}
	return sessionEventKindNames[k]
}

// TurnEndSource says which path observed the end of an agent turn.
type TurnEndSource string

const (
	TurnEndSourceHook   TurnEndSource = "hook"
	TurnEndSourceScreen TurnEndSource = "screen"
)

// TurnEnded is the minimal turn-end fact; Episode/Kind arrive with Story 3.1 constructors.
type TurnEnded struct {
	Excerpt string
	Source  TurnEndSource
	At      time.Time
}

// SessionEventRecorder is the flag-gated, synchronous event-log writer.
// Each method has exactly one caller (plan "Event Ownership"); none may be called under Instance.mu.
type SessionEventRecorder interface {
	RecordTurnEnded(title string, turn TurnEnded)
	RecordAttentionRaised(title string, reason AttentionReason)
	RecordAttentionCleared(title string, reason AttentionReason)
	RecordStatusChanged(title string, from, to Status)
	// RecordAcknowledged: undone is true for UnacknowledgeSession.
	RecordAcknowledged(title string, undone bool)
	RecordLifecycle(title string, event LifecycleEvent)
	RecordNotificationSuppressed(title string, reason string)
	RecordApprovalResolved(title string, approved bool)
}

// noopSessionEventRecorder is the recorder used when session_event_log is off.
type noopSessionEventRecorder struct{}

func (noopSessionEventRecorder) RecordTurnEnded(string, TurnEnded)              {}
func (noopSessionEventRecorder) RecordAttentionRaised(string, AttentionReason)  {}
func (noopSessionEventRecorder) RecordAttentionCleared(string, AttentionReason) {}
func (noopSessionEventRecorder) RecordStatusChanged(string, Status, Status)     {}
func (noopSessionEventRecorder) RecordAcknowledged(string, bool)                {}
func (noopSessionEventRecorder) RecordLifecycle(string, LifecycleEvent)         {}
func (noopSessionEventRecorder) RecordNotificationSuppressed(string, string)    {}
func (noopSessionEventRecorder) RecordApprovalResolved(string, bool)            {}

// NewNoopSessionEventRecorder returns the recorder that discards every event.
func NewNoopSessionEventRecorder() SessionEventRecorder { return noopSessionEventRecorder{} }

// orNoopRecorder maps a nil recorder to the no-op one so callers never nil-check.
func orNoopRecorder(r SessionEventRecorder) SessionEventRecorder {
	if r == nil {
		return noopSessionEventRecorder{}
	}
	return r
}
