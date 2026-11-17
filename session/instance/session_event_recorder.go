package instance

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

	SessionEventKindCount // must stay last
)

var sessionEventKindNames = [SessionEventKindCount]string{
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
	if k < 0 || k >= SessionEventKindCount {
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
type NoopSessionEventRecorder struct{}

func (NoopSessionEventRecorder) RecordTurnEnded(string, domain.TurnEnded)                 {}
func (NoopSessionEventRecorder) RecordAttentionRaised(string, queue.AttentionReason)      {}
func (NoopSessionEventRecorder) RecordAttentionCleared(string, queue.AttentionReason)     {}
func (NoopSessionEventRecorder) RecordStatusChanged(string, domain.Status, domain.Status) {}
func (NoopSessionEventRecorder) RecordAcknowledged(string, bool)                          {}
func (NoopSessionEventRecorder) RecordLifecycle(string, domain.LifecycleEvent)            {}
func (NoopSessionEventRecorder) RecordNotificationSuppressed(string, string)              {}
func (NoopSessionEventRecorder) RecordApprovalResolved(string, bool)                      {}

// NewNoopSessionEventRecorder returns the recorder that discards every event.
func NewNoopSessionEventRecorder() SessionEventRecorder { return NoopSessionEventRecorder{} }

// orNoopRecorder maps a nil recorder to the no-op one so callers never nil-check.
func OrNoopRecorder(r SessionEventRecorder) SessionEventRecorder {
	if r == nil {
		return NoopSessionEventRecorder{}
	}
	return r
}
