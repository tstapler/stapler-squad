package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

func TestSessionEventKind_Should_NameEveryKindExhaustively_When_Enumerated(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for k := instance.SessionEventKind(0); k < instance.SessionEventKindCount; k++ {
		switch k {
		case instance.SessionEventStatusChanged, instance.SessionEventTurnEnded, instance.SessionEventAttentionRaised,
			instance.SessionEventAttentionCleared, instance.SessionEventAcknowledged, instance.SessionEventNotificationSuppressed,
			instance.SessionEventLifecycleStarted, instance.SessionEventLifecycleExited, instance.SessionEventLifecycleStopped,
			instance.SessionEventApprovalResolved, instance.SessionEventOutput:
		default:
			t.Fatalf("kind %d not handled: extend this switch and the name table", k)
		}
		name := k.String()
		assert.NotEmpty(t, name)
		assert.NotEqual(t, "unknown", name)
		assert.False(t, seen[name], "duplicate name %q", name)
		seen[name] = true
	}
	assert.Equal(t, "unknown", instance.SessionEventKindCount.String())
}

func TestNoopSessionEventRecorder_Should_NotPanic_When_NilOrNoop(t *testing.T) {
	t.Parallel()
	r := instance.OrNoopRecorder(nil)
	r.RecordTurnEnded("demo", domain.TurnEnded{})
	r.RecordAttentionRaised("demo", queue.ReasonTaskComplete)
	r.RecordAttentionCleared("demo", queue.ReasonTaskComplete)
	r.RecordStatusChanged("demo", domain.Active, domain.Stopped)
	r.RecordAcknowledged("demo", false)
	r.RecordLifecycle("demo", domain.EventStarted)
	r.RecordNotificationSuppressed("demo", "x")
	r.RecordApprovalResolved("demo", true)
}
