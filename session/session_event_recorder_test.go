package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionEventKind_Should_NameEveryKindExhaustively_When_Enumerated(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for k := SessionEventKind(0); k < sessionEventKindCount; k++ {
		switch k {
		case SessionEventStatusChanged, SessionEventTurnEnded, SessionEventAttentionRaised,
			SessionEventAttentionCleared, SessionEventAcknowledged, SessionEventNotificationSuppressed,
			SessionEventLifecycleStarted, SessionEventLifecycleExited, SessionEventLifecycleStopped,
			SessionEventApprovalResolved, SessionEventOutput:
		default:
			t.Fatalf("kind %d not handled: extend this switch and the name table", k)
		}
		name := k.String()
		assert.NotEmpty(t, name)
		assert.NotEqual(t, "unknown", name)
		assert.False(t, seen[name], "duplicate name %q", name)
		seen[name] = true
	}
	assert.Equal(t, "unknown", sessionEventKindCount.String())
}

func TestNoopSessionEventRecorder_Should_NotPanic_When_NilOrNoop(t *testing.T) {
	t.Parallel()
	r := orNoopRecorder(nil)
	r.RecordTurnEnded("demo", TurnEnded{})
	r.RecordAttentionRaised("demo", ReasonTaskComplete)
	r.RecordAttentionCleared("demo", ReasonTaskComplete)
	r.RecordStatusChanged("demo", Running, Stopped)
	r.RecordAcknowledged("demo", false)
	r.RecordLifecycle("demo", EventStarted)
	r.RecordNotificationSuppressed("demo", "x")
	r.RecordApprovalResolved("demo", true)
}
