package streamhub

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

type labeledTransport struct {
	sent chan struct{}
}

func (l *labeledTransport) Send([]byte) error { l.sent <- struct{}{}; return nil }
func (l *labeledTransport) Close() error      { return nil }
func (l *labeledTransport) LatencyLabels() termlat.Labels {
	return termlat.Labels{Backend: "native", Listener: "remote"}
}

func outputTotal(stage string) uint64 {
	for _, s := range termlat.Default.Report() {
		if s.Stage == stage && s.Backend == "native" && s.Listener == "remote" {
			return s.Total
		}
	}
	return 0
}

func TestSubscriber_should_RecordOutputSendAndQueueDepthUnderTransportLabels(t *testing.T) {
	t.Parallel()
	tr := &labeledTransport{sent: make(chan struct{}, 4)}
	s := newSubscriber("lat", tr, SubscriberCapability{}, 4)
	sendBefore, depthBefore := outputTotal(termlat.StageOutputSend), outputTotal(termlat.StageOutputQueue)

	s.startWriter(func(SubscriberID, error) {})
	delivered, _ := s.trySend([]byte("frame"))
	<-tr.sent
	s.close()
	<-s.done

	assert.True(t, delivered)
	assert.Greater(t, outputTotal(termlat.StageOutputSend), sendBefore)
	assert.Greater(t, outputTotal(termlat.StageOutputQueue), depthBefore)
}

func dropTotal() uint64 {
	var n uint64
	for _, d := range termlat.Default.DropCounts() {
		if d.Cause == termlat.DropHubQueueFull && d.Backend == "native" {
			n += d.Total
		}
	}
	return n
}

func TestSubscriber_should_RecordTimeAFrameWaitedInTheOutboundQueue(t *testing.T) {
	t.Parallel()
	tr := &labeledTransport{sent: make(chan struct{}, 4)}
	s := newSubscriber("wait", tr, SubscriberCapability{}, 4)
	before := outputTotal(termlat.StageOutputQueueWait)

	delivered, _ := s.trySend([]byte("frame")) // queued before any writer runs
	s.startWriter(func(SubscriberID, error) {})
	<-tr.sent
	s.close()
	<-s.done

	assert.True(t, delivered)
	assert.Greater(t, outputTotal(termlat.StageOutputQueueWait), before)
}

func TestSubscriber_should_CountAFrameDroppedOnAFullQueue(t *testing.T) {
	t.Parallel()
	tr := &labeledTransport{sent: make(chan struct{}, 4)}
	s := newSubscriber("full", tr, SubscriberCapability{}, 1)
	before := dropTotal()

	first, _ := s.trySend([]byte("a")) // no writer: the one slot stays full
	second, _ := s.trySend([]byte("b"))

	assert.True(t, first)
	assert.False(t, second)
	assert.GreaterOrEqual(t, dropTotal(), before+1)
}
