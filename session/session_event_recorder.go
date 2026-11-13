package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/queue"
)

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

// SessionEventRecorder is the flag-gated, synchronous event-log writer.
// Each method has exactly one caller (plan "Event Ownership"); none may be called under Instance.mu.
type SessionEventRecorder interface {
	RecordTurnEnded(title string, turn domain.TurnEnded)
	RecordAttentionRaised(title string, reason queue.AttentionReason)
	RecordAttentionCleared(title string, reason queue.AttentionReason)
	RecordStatusChanged(title string, from, to domain.Status)
	// RecordAcknowledged: undone is true for UnacknowledgeSession.
	RecordAcknowledged(title string, undone bool)
	RecordLifecycle(title string, event domain.LifecycleEvent)
	RecordNotificationSuppressed(title string, reason string)
	RecordApprovalResolved(title string, approved bool)
}

// noopSessionEventRecorder is the recorder used when session_event_log is off.
type noopSessionEventRecorder struct{}

func (noopSessionEventRecorder) RecordTurnEnded(string, domain.TurnEnded)                 {}
func (noopSessionEventRecorder) RecordAttentionRaised(string, queue.AttentionReason)      {}
func (noopSessionEventRecorder) RecordAttentionCleared(string, queue.AttentionReason)     {}
func (noopSessionEventRecorder) RecordStatusChanged(string, domain.Status, domain.Status) {}
func (noopSessionEventRecorder) RecordAcknowledged(string, bool)                          {}
func (noopSessionEventRecorder) RecordLifecycle(string, domain.LifecycleEvent)            {}
func (noopSessionEventRecorder) RecordNotificationSuppressed(string, string)              {}
func (noopSessionEventRecorder) RecordApprovalResolved(string, bool)                      {}

// NewNoopSessionEventRecorder returns the recorder that discards every event.
func NewNoopSessionEventRecorder() SessionEventRecorder { return noopSessionEventRecorder{} }

// orNoopRecorder maps a nil recorder to the no-op one so callers never nil-check.
func orNoopRecorder(r SessionEventRecorder) SessionEventRecorder {
	if r == nil {
		return noopSessionEventRecorder{}
	}
	return r
}
