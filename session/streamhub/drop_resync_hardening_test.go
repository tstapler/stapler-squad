package streamhub_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// stepTransport delivers its first free Sends at once and every later one only when the test
// hands it a permit.
type stepTransport struct {
	stalledTransport
	permits chan struct{}
	parked  chan struct{} // one value each time a Send starts waiting for a permit
}

func newStepTransport(free int32) *stepTransport {
	t := &stepTransport{stalledTransport: stalledTransport{gate: make(chan struct{})}, permits: make(chan struct{}, 64), parked: make(chan struct{}, 64)}
	t.free.Store(free)
	return t
}

func (s *stepTransport) Send(data []byte) error {
	if s.free.Add(-1) < 0 {
		s.parked <- struct{}{}
		<-s.permits
	}
	s.mu.Lock()
	s.got = append(s.got, string(data))
	s.mu.Unlock()
	return nil
}

// captureHookController runs onCapture while a repaint capture is in flight.
type captureHookController struct {
	*redrawController
	onCapture atomic.Pointer[func()]
}

func (c *captureHookController) CapturePaneRedraw(ctx context.Context) (streamhub.FullStateSnapshot, bool, error) {
	if f := c.onCapture.Load(); f != nil {
		(*f)()
	}
	return c.redrawController.CapturePaneRedraw(ctx)
}

// Output that arrives while the repaint capture runs is not in the capture, so it must be
// delivered after the repaint, even when the writer keeps draining during the capture (a busy
// pane); a repaint must not be refused or lose it.
func TestHub_should_RepaintAndKeepFrames_When_WriterDrainsDuringCapture(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	c := &captureHookController{redrawController: &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: "REDRAW"}, ok: true}}
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour), streamhub.WithClock(clock),
		streamhub.WithSubscriberBufferSize(2), streamhub.WithSlowSubscriberGrace(time.Hour))
	tr := newStepTransport(1)
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{})
	defer hub.DetachSubscriber(id)

	// f1 is held in the blocked Send, f2 and f3 fill the queue, f4 and f5 are dropped.
	hub.Broadcast([]byte("f1"))
	<-tr.parked
	for _, f := range []string{"f2", "f3", "f4", "f5"} {
		hub.Broadcast([]byte(f))
	}
	tr.permits <- struct{}{} // f1 goes out; the writer then holds f2 in a blocked Send
	<-tr.parked

	// While the capture runs, new output arrives and the writer drains f2 and f3.
	hook := func() {
		hub.Broadcast([]byte("late"))
		tr.permits <- struct{}{}
		tr.permits <- struct{}{}
		require.Eventually(t, func() bool { return count(tr.frames(), "f3") == 1 }, 5*time.Second, time.Millisecond)
	}
	c.onCapture.Store(&hook)
	clock.Advance(resyncStep)

	tr.permits <- struct{}{}
	tr.permits <- struct{}{}
	require.Eventually(t, func() bool { return len(tr.frames()) == 6 }, 5*time.Second, time.Millisecond)
	require.Equal(t, []string{"REDRAW", "f1", "f2", "f3", "REDRAW", "late"}, tr.frames())
}

// A writer blocked on a dead transport must be evicted at the default grace, not kept alive by
// repeated repaints that only queue behind the stalled write.
func TestHub_should_EvictStalledSubscriber_When_DefaultGraceAndWriterNeverDrains(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	c := &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: "REDRAW"}, ok: true}
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour), streamhub.WithClock(clock),
		streamhub.WithSubscriberBufferSize(2)) // default slow-subscriber grace
	tr := newStalledTransport(2) // the catch-up and f1 go out; everything after blocks
	defer close(tr.gate)
	hub.AttachSubscriber(tr, streamhub.SubscriberCapability{})
	hub.Broadcast([]byte("f1"))
	require.Eventually(t, func() bool { return count(tr.frames(), "f1") == 1 }, 5*time.Second, time.Millisecond)

	// Park the writer inside the blocked Send of f2 first: only then is "writer sent nothing since
	// the drop" settled, so the repaint is deterministically withheld and the grace timer evicts.
	hub.Broadcast([]byte("f2"))
	require.Eventually(t, func() bool { return tr.free.Load() < 0 }, 5*time.Second, time.Millisecond)
	for _, f := range []string{"f3", "f4", "f5", "f6"} {
		hub.Broadcast([]byte(f))
	}
	for i := 0; i < 20 && hub.SlowSubscriberDropsTotal() == 0; i++ {
		clock.Advance(50 * time.Millisecond)
	}
	require.Equal(t, int64(1), hub.SlowSubscriberDropsTotal())
	require.Equal(t, 1, count(tr.frames(), "REDRAW"), "no repaint was sent to the stalled writer")
}

// failingCaptureController cannot capture at all, like a dead pane.
type failingCaptureController struct {
	*fakeSessionController
	calls atomic.Int32
}

func (c *failingCaptureController) CapturePaneContentRawContext(context.Context) (streamhub.RawPaneContent, error) {
	c.calls.Add(1)
	return "", errors.New("pane gone")
}

// A failed capture must keep the subscriber flagged, retry no faster than the minimum interval,
// and repaint once the capture works again.
func TestHub_should_BackOffRepaintRetries_When_CaptureFails(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	c := &failingCaptureController{fakeSessionController: newFakeSessionController()}
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour), streamhub.WithClock(clock),
		streamhub.WithSubscriberBufferSize(2), streamhub.WithSlowSubscriberGrace(time.Hour))
	tr := newStalledTransport(1)
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{})
	defer hub.DetachSubscriber(id)
	attachCalls := c.calls.Load()

	for _, f := range []string{"f1", "f2", "f3", "f4", "f5", "f6"} {
		hub.Broadcast([]byte(f))
	}
	close(tr.gate)
	require.Eventually(t, func() bool { return len(tr.frames()) >= 2 }, 5*time.Second, time.Millisecond)

	clock.Advance(50 * time.Millisecond) // debounce: first attempt fails
	require.Equal(t, attachCalls+1, c.calls.Load())
	clock.Advance(490 * time.Millisecond)
	require.Equal(t, attachCalls+1, c.calls.Load(), "no second capture before the backed-off interval")
	clock.Advance(10 * time.Millisecond)
	require.Equal(t, attachCalls+2, c.calls.Load(), "the subscriber stayed flagged and is retried")
	clock.Advance(time.Second - time.Millisecond)
	require.Equal(t, attachCalls+2, c.calls.Load(), "the backoff doubled again")
	clock.Advance(time.Millisecond)
	require.Equal(t, attachCalls+3, c.calls.Load())
}
