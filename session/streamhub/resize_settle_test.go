package streamhub_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// hookedController lets a test inject behaviour into the capture that follows a resize.
type hookedController struct {
	*fakeSessionController
	afterCapture atomic.Pointer[func()]
	stuck        atomic.Bool
}

func (c *hookedController) CapturePaneContentRawContext(ctx context.Context) (streamhub.RawPaneContent, error) {
	if c.stuck.Load() {
		c.captureCalls.Add(1)
		<-ctx.Done()
		return "", ctx.Err()
	}
	out, err := c.fakeSessionController.CapturePaneContentRawContext(ctx)
	if f := c.afterCapture.Load(); f != nil {
		(*f)()
	}
	return out, err
}

type settleRig struct {
	hub  *streamhub.StreamHub
	ctrl *hookedController
	sink *streamhub.MemoryTransport
	id   streamhub.SubscriberID
}

func newSettleRig(t *testing.T) *settleRig {
	t.Helper()
	ctrl := &hookedController{fakeSessionController: newFakeSessionController()}
	hub := streamhub.NewStreamHub("settle", ctrl,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithResizeApplyTimeout(50*time.Millisecond),
		streamhub.WithResizeSettleDelay(time.Millisecond),
		streamhub.WithQuiescenceTimeout(time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	t.Cleanup(func() { _ = hub.ForceTeardown() })
	sink := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(sink, streamhub.SubscriberCapability{CanResize: true})
	return &settleRig{hub: hub, ctrl: ctrl, sink: sink, id: id}
}

func (r *settleRig) resize(t *testing.T, cols, rows int) {
	t.Helper()
	size, err := streamhub.NewTerminalSize(cols, rows)
	require.NoError(t, err)
	r.hub.RequestResize(context.Background(), r.id, size)
}

// Output that arrives while a resize is applying is dropped, and the post-resize snapshot was
// taken before it: nothing else would repaint the pane, so the hub must capture once more.
func TestRequestResize_should_RecaptureOnce_When_OutputWasDroppedAfterTheSnapshot(t *testing.T) {
	t.Parallel()
	r := newSettleRig(t)
	drop := func() { r.hub.OnRawOutput([]byte("arrived after the snapshot")) }
	r.ctrl.afterCapture.Store(&drop)
	before := int(r.ctrl.captureCalls.Load())
	settledBefore := r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle)

	r.resize(t, 100, 30)
	clear := func() {}
	r.ctrl.afterCapture.Store(&clear) // the settle capture itself must not re-arm another

	assert.Eventually(t, func() bool {
		return r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle) == settledBefore+1
	}, 5*time.Second, time.Millisecond, "a settle capture is published after the resize ends")
	assert.Equal(t, before+2, int(r.ctrl.captureCalls.Load()), "one capture for the resize, one for the settle")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, before+2, int(r.ctrl.captureCalls.Load()), "and exactly one: the settle does not re-arm itself")
}

// A capture that blows the resize budget leaves subscribers on the pre-resize screen; once the
// daemon answers again the settle capture repairs it.
func TestRequestResize_should_RepaintAfterTheDaemonRecovers_When_TheResizeCaptureTimedOut(t *testing.T) {
	t.Parallel()
	r := newSettleRig(t)
	r.ctrl.stuck.Store(true)
	published := r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle)
	frames := len(r.sink.ReceivedFrames())

	r.resize(t, 90, 25)
	r.ctrl.stuck.Store(false)

	assert.Eventually(t, func() bool {
		return r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle) > published
	}, 5*time.Second, time.Millisecond)
	assert.Eventually(t, func() bool { return len(r.sink.ReceivedFrames()) > frames }, 5*time.Second, time.Millisecond,
		"subscribers receive the repaint")
}

func TestRequestResize_should_NotRecapture_When_NothingWasDropped(t *testing.T) {
	t.Parallel()
	r := newSettleRig(t)
	before := int(r.ctrl.captureCalls.Load())
	settled := r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle)

	r.resize(t, 100, 30)
	time.Sleep(30 * time.Millisecond)

	assert.Equal(t, before+1, int(r.ctrl.captureCalls.Load()))
	assert.Equal(t, settled, r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle))
}
