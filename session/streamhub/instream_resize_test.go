package streamhub_test

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// inStreamController stands in for a sequenced backend: ResyncInStream feeds the hub's raw-output
// path the way the pump does with a spliced redraw, then output that raced the capture.
type inStreamController struct {
	*fakeSessionController
	hub      atomic.Pointer[streamhub.StreamHub]
	supports atomic.Bool
	err      error
	calls    atomic.Int32

	capturesBeforeResize int32
}

func (c *inStreamController) SupportsInStreamResync() bool { return c.supports.Load() }

func (c *inStreamController) ResyncInStream(context.Context) error {
	c.calls.Add(1)
	if c.err != nil {
		return c.err
	}
	hub := c.hub.Load()
	hub.OnRawOutput([]byte("REDRAW"))
	hub.OnRawOutput([]byte("late")) // produced after the capture, before the resize call returns
	return nil
}

// CapturePaneContentRawContext is the legacy path; a frame arriving while it runs is the one the
// old drop-during-resize design lost.
func (c *inStreamController) CapturePaneContentRawContext(ctx context.Context) (streamhub.RawPaneContent, error) {
	if hub := c.hub.Load(); hub != nil {
		hub.OnRawOutput([]byte("late"))
	}
	return c.fakeSessionController.CapturePaneContentRawContext(ctx)
}

func resizeThroughHub(t *testing.T, c *inStreamController) string {
	t.Helper()
	c.captureContent = "SNAP"
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(50*time.Millisecond), streamhub.WithQuiescenceQuietPeriod(2*time.Millisecond),
		streamhub.WithBatchMaxWindow(5*time.Millisecond))
	defer hub.ForceTeardown()
	tr := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{CanResize: true})
	attached := len(tr.ReceivedFrames())
	c.hub.Store(hub) // armed only now, so the attach-time catch-up capture does not feed it
	c.capturesBeforeResize = c.captureCalls.Load()

	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)
	hub.RequestResize(context.Background(), id, size)
	hub.TryFlush()
	time.Sleep(50 * time.Millisecond)

	var got bytes.Buffer
	for _, f := range tr.ReceivedFrames()[attached:] {
		got.Write(f)
	}
	return got.String()
}

func TestHub_should_DeliverOutputThatRacesTheCapture_When_ControllerResyncsInStream(t *testing.T) {
	c := &inStreamController{fakeSessionController: newFakeSessionController()}
	c.supports.Store(true)

	got := resizeThroughHub(t, c)

	assert.Equal(t, int32(1), c.calls.Load())
	assert.Equal(t, "REDRAWlate", got, "the spliced redraw, then the output after the capture, each once")
	assert.Equal(t, c.capturesBeforeResize, c.captureCalls.Load(), "no separate capture to race the stream")
}

func TestHub_should_FallBackToCapture_When_InStreamResyncFails(t *testing.T) {
	c := &inStreamController{fakeSessionController: newFakeSessionController(), err: errors.New("tail evicted")}
	c.supports.Store(true)

	got := resizeThroughHub(t, c)

	assert.Equal(t, int32(1), c.calls.Load())
	assert.Contains(t, got, "SNAP", "the legacy snapshot still repaints subscribers")
}

func TestHub_should_UseCapture_When_StreamIsNotSequenced(t *testing.T) {
	c := &inStreamController{fakeSessionController: newFakeSessionController()}

	got := resizeThroughHub(t, c)

	assert.Equal(t, int32(0), c.calls.Load())
	assert.Contains(t, got, "SNAP")
	t.Logf("legacy path delivered %q (the frame fed during its capture is dropped)", got)
}
