package streamhub_test

import (
	"bytes"
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
	time.Sleep(20 * time.Millisecond) //nolint:notimesleeptest absence check: a re-armed settle would capture again within this window
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
	time.Sleep(30 * time.Millisecond) //nolint:notimesleeptest absence check: no settle capture may run for a resize that suppressed no output

	assert.Equal(t, before+1, int(r.ctrl.captureCalls.Load()))
	assert.Equal(t, settled, r.hub.ResyncCaptures(streamhub.ResyncReasonResizeSettle))
}

// Output that arrives while the settle capture is in flight is neither painted over by the older
// snapshot nor discarded with the batch: it must reach the subscriber after the repaint.
func TestSettle_should_DeliverLiveOutputAfterTheRepaint_When_OutputArrivesDuringTheSettleCapture(t *testing.T) {
	t.Parallel()
	r := newSettleRig(t)
	var captures atomic.Int32
	hook := func() {
		switch captures.Add(1) {
		case 1:
			r.hub.OnRawOutput([]byte("dropped by the resize")) // arms the settle
		case 2:
			r.hub.OnRawOutput([]byte("LIVE-DURING-SETTLE"))
		}
	}
	r.ctrl.afterCapture.Store(&hook)

	r.resize(t, 100, 30)

	liveIndex := func() int {
		for i, f := range r.sink.ReceivedFrames() {
			if bytes.Contains(f, []byte("LIVE-DURING-SETTLE")) {
				return i
			}
		}
		return -1
	}
	require.Eventually(t, func() bool { return liveIndex() >= 0 }, 5*time.Second, time.Millisecond,
		"the frame that arrived during the settle capture is delivered")
	require.Equal(t, int32(2), captures.Load())
	assert.Greater(t, liveIndex(), 0, "and it follows the repaint rather than preceding it")
}
