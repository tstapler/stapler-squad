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

// A resize that is still capturing has already resized the pane; the notify callback fires at
// that point so keystrokes need not wait for the capture.
func TestRequestResizeNotify_should_SignalAfterThePaneResize_NotAfterTheCapture(t *testing.T) {
	t.Parallel()
	ctrl := &hookedController{fakeSessionController: newFakeSessionController()}
	hub := streamhub.NewStreamHub("notify", ctrl,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithResizeApplyTimeout(200*time.Millisecond),
		streamhub.WithResizeSettleDelay(time.Hour),
		streamhub.WithQuiescenceTimeout(time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	defer func() { _ = hub.ForceTeardown() }()
	id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	ctrl.stuck.Store(true) // the capture will block until the budget ends
	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)

	var signalled atomic.Bool
	var sawResizeStillRunning atomic.Bool
	done := make(chan struct{})
	go func() {
		hub.RequestResizeNotify(context.Background(), id, size, func() {
			if _, resizing := hub.ResizeActivity(); resizing {
				sawResizeStillRunning.Store(true)
			}
			signalled.Store(true)
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RequestResizeNotify did not return")
	}
	assert.True(t, signalled.Load())
	assert.True(t, sawResizeStillRunning.Load(), "signalled while the capture was still in flight")
}

func TestRequestResizeNotify_should_SignalOnce_EvenWhenNothingChanged(t *testing.T) {
	t.Parallel()
	hub := streamhub.NewStreamHub("notify-noop", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	size, err := streamhub.NewTerminalSize(100, 30)
	require.NoError(t, err)
	hub.RequestResize(context.Background(), id, size) // negotiates 100x30

	var calls atomic.Int32
	hub.RequestResizeNotify(context.Background(), id, size, func() { calls.Add(1) })
	assert.Equal(t, int32(1), calls.Load(), "an unchanged size still reports, so a gate never waits on a no-op")
}

func TestRequestResizeNotify_should_SignalOnce_ForAnUnknownSubscriber(t *testing.T) {
	t.Parallel()
	hub := streamhub.NewStreamHub("notify-unknown", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()
	size, err := streamhub.NewTerminalSize(80, 24)
	require.NoError(t, err)
	var calls atomic.Int32
	hub.RequestResizeNotify(context.Background(), "ghost", size, func() { calls.Add(1) })
	assert.Equal(t, int32(1), calls.Load())
}
