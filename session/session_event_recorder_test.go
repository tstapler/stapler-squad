package session

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/queue"
)

// attentionCall is one RecordAttentionRaised/Cleared invocation.
type attentionCall struct {
	kind   SessionEventKind
	title  string
	reason queue.AttentionReason
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

func (r *attentionRecorder) RecordAttentionRaised(title string, reason queue.AttentionReason) {
	r.add(attentionCall{SessionEventAttentionRaised, title, reason})
}

func (r *attentionRecorder) RecordAttentionCleared(title string, reason queue.AttentionReason) {
	r.add(attentionCall{SessionEventAttentionCleared, title, reason})
}

func TestPoller_Should_InvokeRecordAttentionRaisedAndCleared_When_ItemAddedThenRemoved(t *testing.T) {
	t.Parallel()
	add := chainAdd(queue.PriorityMedium, queue.ReasonTaskComplete)
	poller, inst := pollerFor(add, func(*Instance) {})
	rec := &attentionRecorder{}
	poller.SetEventRecorder(rec)

	poller.checkSession(inst, nil)
	poller.statusDeterminer = stubDeterminer{result: DetectionResult{Action: DetectionActionRemove}}
	poller.checkSession(inst, nil)

	want := []attentionCall{
		{SessionEventAttentionRaised, "demo", queue.ReasonTaskComplete},
		{SessionEventAttentionCleared, "demo", queue.ReasonTaskComplete},
	}
	assert.Equal(t, want, rec.calls)
	_, queued := poller.queue.Get("demo")
	assert.False(t, queued, "queue behavior must be unchanged by the recorder")
}

func TestPoller_Should_RecordRaisedOnce_When_SameReasonIsReAdded(t *testing.T) {
	t.Parallel()
	poller, inst := pollerFor(chainAdd(queue.PriorityMedium, queue.ReasonTaskComplete), func(*Instance) {})
	rec := &attentionRecorder{}
	poller.SetEventRecorder(rec)

	poller.checkSession(inst, nil)
	poller.checkSession(inst, nil)

	assert.Len(t, rec.calls, 1)
}
