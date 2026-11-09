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

// stuckCaptureController models a daemon that never answers capture-pane within the budget.
type stuckCaptureController struct {
	*fakeSessionController
	stuck *atomic.Bool
}

func (c stuckCaptureController) CapturePaneContentRawContext(ctx context.Context) (streamhub.RawPaneContent, error) {
	if !c.stuck.Load() {
		return c.fakeSessionController.CapturePaneContentRawContext(ctx)
	}
	c.captureCalls.Add(1)
	<-ctx.Done()
	return "", ctx.Err()
}

type recordingTransport struct{ frames chan []byte }

func (r recordingTransport) Send(b []byte) error { r.frames <- b; return nil }
func (r recordingTransport) Close() error        { return nil }

// A resize whose capture never returns must give up within its own budget and let live output
// flow again: output is suppressed while a resize applies, and that suppression is what the
// user feels as "my typing does not register".
func TestRequestResize_should_GiveUpAndResumeOutput_When_CaptureOutlastsTheResizeBudget(t *testing.T) {
	t.Parallel()
	controller := stuckCaptureController{newFakeSessionController(), &atomic.Bool{}}
	hub := streamhub.NewStreamHub("budget", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithResizeApplyTimeout(20*time.Millisecond),
		streamhub.WithQuiescenceTimeout(time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	defer hub.ForceTeardown()
	id := hub.AttachSubscriber(recordingTransport{frames: make(chan []byte, 8)}, streamhub.SubscriberCapability{CanResize: true})
	controller.stuck.Store(true) // the daemon stalls after the attach-time capture
	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		hub.RequestResize(context.Background(), id, size)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RequestResize outlived its budget; output stays suppressed for the whole wait")
	}

	_, resizing := hub.ResizeActivity()
	assert.False(t, resizing, "output suppression ends when the resize gives up")
	assert.GreaterOrEqual(t, int(controller.captureCalls.Load()), 1)
}

// stuckSetSizeController models a daemon that accepts capture-pane but hangs on resize-window.
type stuckSetSizeController struct{ *fakeSessionController }

func (c stuckSetSizeController) SetWindowSizeContext(ctx context.Context, _, _ int) error {
	<-ctx.Done()
	return ctx.Err()
}

// A resize-window that burns its own budget must not starve the capture of one: subscribers
// get a fresh snapshot instead of staying on the pre-resize screen.
func TestRequestResize_should_StillCapture_When_SetWindowSizeExhaustsItsOwnBudget(t *testing.T) {
	t.Parallel()
	controller := stuckSetSizeController{newFakeSessionController()}
	hub := streamhub.NewStreamHub("setsize-budget", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithResizeApplyTimeout(20*time.Millisecond))
	defer hub.ForceTeardown()
	id := hub.AttachSubscriber(recordingTransport{frames: make(chan []byte, 8)}, streamhub.SubscriberCapability{CanResize: true})
	before := controller.captureCalls.Load()
	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)

	hub.RequestResize(context.Background(), id, size)

	assert.Greater(t, controller.captureCalls.Load(), before, "the capture ran after resize-window timed out")
	_, resizing := hub.ResizeActivity()
	assert.False(t, resizing)
}

// When the capture also gives up, subscribers are left stale unless something re-arms it.
func TestRequestResize_should_RearmACapture_When_TheResizeCaptureGivesUp(t *testing.T) {
	t.Parallel()
	controller := stuckCaptureController{newFakeSessionController(), &atomic.Bool{}}
	hub := streamhub.NewStreamHub("rearm", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithResizeApplyTimeout(20*time.Millisecond),
		streamhub.WithQuiescenceTimeout(time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	defer hub.ForceTeardown()
	frames := make(chan []byte, 16)
	id := hub.AttachSubscriber(recordingTransport{frames: frames}, streamhub.SubscriberCapability{CanResize: true})
	controller.stuck.Store(true)
	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)

	hub.RequestResize(context.Background(), id, size)
	controller.stuck.Store(false) // the daemon recovers
	after := controller.captureCalls.Load()

	// The fake counts only stuck calls while stuck, so a recovered capture shows up as a frame.
	for len(frames) > 0 {
		<-frames
	}
	require.Eventually(t, func() bool { return len(frames) > 0 }, 5*time.Second, 5*time.Millisecond,
		"a re-armed capture must publish a snapshot after the daemon recovers (captures so far: %d)", after)
}
