package tymux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

func stageTotal(rec *termlat.Recorder, stage string) uint64 {
	for _, s := range rec.Report() {
		if s.Stage == stage && s.Backend == "tymux" {
			return s.Total
		}
	}
	return 0
}

func TestClientFanout_should_RecordQueueDepthOnEveryBroadcast(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	f := newClientFanoutRecording(rec)
	_, _ = f.Subscribe()

	f.Broadcast([]byte("a"))
	f.Broadcast([]byte("b"))

	assert.Equal(t, uint64(2), stageTotal(rec, termlat.StageFanoutDepth))
}

func TestClientFanout_should_CountAFrameDroppedOnAFullSubscriber(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	f := newClientFanoutRecording(rec)
	_, _ = f.Subscribe()

	for range subscriberBufferSize + 3 {
		f.Broadcast([]byte("x"))
	}

	drops := rec.DropCounts()
	require.Len(t, drops, 1)
	assert.Equal(t, termlat.DropCount{Cause: termlat.DropFanoutFull, Backend: "tymux", Total: 3}, drops[0])
}

func TestClientFanout_should_RecordHowLongATimedSubscribersFrameWaited(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	f := newClientFanoutRecording(rec)
	id, ch := f.SubscribeTimed()

	f.Broadcast([]byte("a"))
	f.Broadcast([]byte("b"))
	<-ch
	f.Dequeued(id)
	<-ch
	f.Dequeued(id)
	f.Dequeued(id) // nothing left: must not record a bogus sample

	assert.Equal(t, uint64(2), stageTotal(rec, termlat.StageFanoutWait))
}

func TestClientFanout_should_NotStampFramesDroppedOnAFullTimedSubscriber(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	f := newClientFanoutRecording(rec)
	id, ch := f.SubscribeTimed()

	for range subscriberBufferSize + 5 {
		f.Broadcast([]byte("x"))
	}
	for range subscriberBufferSize {
		<-ch
		f.Dequeued(id)
	}
	f.Dequeued(id)

	assert.Equal(t, uint64(subscriberBufferSize), stageTotal(rec, termlat.StageFanoutWait))
}
