package session

import (
	"sync"
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

// attentionCall is one RecordAttentionRaised/Cleared invocation.
type attentionCall struct {
	kind   SessionEventKind
	title  string
	reason AttentionReason
}

type attentionRecorder struct {
	noopSessionEventRecorder
	mu    sync.Mutex
	calls []attentionCall
}

func (r *attentionRecorder) add(c attentionCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *attentionRecorder) RecordAttentionRaised(title string, reason AttentionReason) {
	r.add(attentionCall{SessionEventAttentionRaised, title, reason})
}

func (r *attentionRecorder) RecordAttentionCleared(title string, reason AttentionReason) {
	r.add(attentionCall{SessionEventAttentionCleared, title, reason})
}

func TestPoller_Should_InvokeRecordAttentionRaisedAndCleared_When_ItemAddedThenRemoved(t *testing.T) {
	t.Parallel()
	add := chainAdd(PriorityMedium, ReasonTaskComplete)
	poller, inst := pollerFor(add, func(*Instance) {})
	rec := &attentionRecorder{}
	poller.SetEventRecorder(rec)

	poller.checkSession(inst, nil)
	poller.statusDeterminer = stubDeterminer{result: DetectionResult{Action: DetectionActionRemove}}
	poller.checkSession(inst, nil)

	want := []attentionCall{
		{SessionEventAttentionRaised, "demo", ReasonTaskComplete},
		{SessionEventAttentionCleared, "demo", ReasonTaskComplete},
	}
	assert.Equal(t, want, rec.calls)
	_, queued := poller.queue.Get("demo")
	assert.False(t, queued, "queue behavior must be unchanged by the recorder")
}

func TestPoller_Should_RecordRaisedOnce_When_SameReasonIsReAdded(t *testing.T) {
	t.Parallel()
	poller, inst := pollerFor(chainAdd(PriorityMedium, ReasonTaskComplete), func(*Instance) {})
	rec := &attentionRecorder{}
	poller.SetEventRecorder(rec)

	poller.checkSession(inst, nil)
	poller.checkSession(inst, nil)

	assert.Len(t, rec.calls, 1)
}
