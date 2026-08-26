package streamhub_test

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

func mustSize(t *testing.T, cols, rows int) streamhub.TerminalSize {
	t.Helper()
	size, err := streamhub.NewTerminalSize(cols, rows)
	if err != nil {
		t.Fatalf("NewTerminalSize(%d, %d) returned unexpected error: %v", cols, rows, err)
	}
	return size
}

func TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_NegotiatedSizeChanges(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	controller.captureContent = "snapshot"
	hub := streamhub.NewStreamHub("claude-session-42", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(30*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	// Registered after the goleak defer above, so — per Go's LIFO defer
	// order — this runs BEFORE goleak's check even if a t.Fatalf below
	// Goexits out of the rest of the test. ForceTeardown is idempotent (a
	// no-op once already torn down), so this is safe alongside the trailing
	// explicit ForceTeardown call some of this file's tests still make to
	// assert on its return value.
	defer hub.ForceTeardown()

	id := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})

	// Establish an initial baseline — a real production hub always starts
	// from some negotiated size before ever reaching {100,30}.
	hub.RequestResize(id, mustSize(t, 80, 24))

	// Multiple subscribers voting for the same target size must still only
	// produce one SetWindowSize(100, 30) call.
	secondID := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	hub.RequestResize(id, mustSize(t, 100, 30))
	hub.RequestResize(secondID, mustSize(t, 100, 30))

	if got := controller.resizeCallCount(100, 30); got != 1 {
		t.Fatalf("expected exactly one SetWindowSize(100, 30) call, got %d", got)
	}

	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("ForceTeardown() returned unexpected error: %v", err)
	}
}

func TestStreamHub_should_SuppressBroadcast_When_ResizeIsInProgress(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	hub := streamhub.NewStreamHub("test-session", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(200*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(150*time.Millisecond),
	)
	// See TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_NegotiatedSizeChanges's
	// comment on this defer's placement/LIFO-ordering rationale.
	defer hub.ForceTeardown()

	transport := newMemoryTransport()
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true})

	// Set only after attach, so AttachSubscriber's own attach-time
	// CatchUpSnapshot (which ran above against the fake's zero-value ""
	// content and was therefore a no-op) doesn't pre-empt this test's
	// suppression assertion below.
	controller.captureContent = "post-resize-snapshot"

	resizeDone := make(chan struct{})
	go func() {
		hub.RequestResize(id, mustSize(t, 100, 30))
		close(resizeDone)
	}()

	// Wait for the resize to actually start (SetWindowSize called) before
	// asserting suppression.
	if !waitFor(t, time.Second, func() bool { return controller.setWindowSizeCalls.Load() > 0 }) {
		t.Fatal("expected SetWindowSize to be called while resize is in progress")
	}

	// Raw output arriving mid-resize must not be broadcast.
	hub.OnRawOutput([]byte("mid-resize-noise"))
	time.Sleep(20 * time.Millisecond)
	if got := transport.receivedCount(); got != 0 {
		t.Fatalf("expected 0 frames delivered while resize is in progress, got %d", got)
	}

	<-resizeDone

	// After quiescence (timeout, since nothing signals it explicitly here)
	// the hub captures once and broadcasts — the suppressed frame is never
	// replayed, only the fresh snapshot is delivered.
	if !waitFor(t, time.Second, func() bool { return transport.receivedCount() == 1 }) {
		t.Fatalf("expected exactly 1 frame (the post-resize snapshot) delivered, got %d", transport.receivedCount())
	}

	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("ForceTeardown() returned unexpected error: %v", err)
	}
}

func TestStreamHub_should_BroadcastSingleCapturedSnapshotToAllSubscribers_When_QuiescenceReached(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	hub := streamhub.NewStreamHub("test-session", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(30*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	// See TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_NegotiatedSizeChanges's
	// comment on this defer's placement/LIFO-ordering rationale.
	defer hub.ForceTeardown()

	transportA := newMemoryTransport()
	transportC := newMemoryTransport()
	idA := hub.AttachSubscriber(transportA, streamhub.SubscriberCapability{CanResize: true})
	idB := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	_ = idA
	idC := hub.AttachSubscriber(transportC, streamhub.SubscriberCapability{CanResize: false})
	_ = idC

	// Each attach above already made its own attach-time CatchUpSnapshot
	// attempt against the fake's zero-value "" content — a real
	// CapturePaneContent call each, even though the empty result meant no
	// frame was delivered and nothing was cached. Baseline against that so
	// the assertion below stays scoped to this test's actual target: the
	// resize/quiescence pipeline itself captures exactly once and shares
	// that one capture across every subscriber (Task 1.3.2e), regardless of
	// how many subscriber attaches preceded it.
	captureCallsBeforeResize := controller.captureCalls.Load()
	controller.captureContent = "shared-snapshot"

	// Resize is triggered by B's vote; A and C never voted at all (C can't).
	hub.RequestResize(idB, mustSize(t, 90, 28))

	if !waitFor(t, time.Second, func() bool {
		return transportA.receivedCount() == 1 && transportC.receivedCount() == 1
	}) {
		t.Fatalf("expected both A and C to receive the shared snapshot, got A=%d C=%d",
			transportA.receivedCount(), transportC.receivedCount())
	}
	if got := controller.captureCalls.Load() - captureCallsBeforeResize; got != 1 {
		t.Fatalf("expected exactly one CapturePaneContent call for the resize, got %d", got)
	}

	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("ForceTeardown() returned unexpected error: %v", err)
	}
}

func TestStreamHub_should_BroadcastStreamEndedSentinelAndTearDown_When_SetWindowSizeErrors(t *testing.T) {
	defer goleak.VerifyNone(t)

	wantErr := errors.New("resize failed")
	controller := newFakeSessionController()
	controller.setWindowSizeErr = wantErr
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))
	// Defensive, not load-bearing for this test's own assertions: the hub is
	// expected to tear itself down as the behavior under test, but if a
	// waitFor below times out and Fatalf's before that happens, this closes
	// the same leak risk as the other tests in this file — see
	// TestStreamHub_should_CallSetWindowSizeExactlyOnce_When_NegotiatedSizeChanges's
	// comment.
	defer hub.ForceTeardown()

	transport1 := newMemoryTransport()
	transport2 := newMemoryTransport()
	id1 := hub.AttachSubscriber(transport1, streamhub.SubscriberCapability{CanResize: true})
	hub.AttachSubscriber(transport2, streamhub.SubscriberCapability{CanResize: true})

	// Each attach above may have already made its own attach-time
	// CatchUpSnapshot attempt (a no-op against the fake's zero-value ""
	// content) — baseline against that so the assertion below is scoped to
	// "the failed resize path itself never calls CapturePaneContent", not
	// "CapturePaneContent is never called at all".
	captureCallsBeforeResize := controller.captureCalls.Load()

	hub.RequestResize(id1, mustSize(t, 100, 30))

	if !waitFor(t, time.Second, func() bool {
		return transport1.receivedCount() == 1 && transport2.receivedCount() == 1
	}) {
		t.Fatalf("expected both subscribers to receive the stream-ended sentinel, got t1=%d t2=%d",
			transport1.receivedCount(), transport2.receivedCount())
	}
	if !waitFor(t, time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected hub to tear down after a SetWindowSize error, got %v", hub.State())
	}
	if got := controller.captureCalls.Load(); got != captureCallsBeforeResize {
		t.Fatalf("expected CapturePaneContent never called as part of the failed resize path, got %d additional calls", got-captureCallsBeforeResize)
	}
}

func TestStreamHub_should_BroadcastStreamEndedSentinelAndTearDown_When_CapturePaneContentErrors(t *testing.T) {
	defer goleak.VerifyNone(t)

	wantErr := errors.New("capture failed")
	controller := newFakeSessionController()
	controller.captureErr = wantErr
	hub := streamhub.NewStreamHub("test-session", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(30*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	// Defensive — see the previous test's identical comment.
	defer hub.ForceTeardown()

	transport1 := newMemoryTransport()
	transport2 := newMemoryTransport()
	id1 := hub.AttachSubscriber(transport1, streamhub.SubscriberCapability{CanResize: true})
	hub.AttachSubscriber(transport2, streamhub.SubscriberCapability{CanResize: true})

	hub.RequestResize(id1, mustSize(t, 100, 30))

	if !waitFor(t, time.Second, func() bool {
		return transport1.receivedCount() == 1 && transport2.receivedCount() == 1
	}) {
		t.Fatalf("expected both subscribers to receive the stream-ended sentinel, got t1=%d t2=%d",
			transport1.receivedCount(), transport2.receivedCount())
	}
	if !waitFor(t, time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected hub to tear down after a CapturePaneContent error, got %v", hub.State())
	}
}

// TestStreamHub_should_StayAliveAndRetryLater_When_CapturePaneContentErrorsWithSessionNotStarted
// is the regression test for the 2026-08-25 incident (see ErrSessionNotStarted's doc
// comment): a subscriber attaching microseconds before its session finishes cold-starting
// used to tear the entire hub down — killing every other attached subscriber too — over a
// condition that resolves itself well under a second later. Unlike the previous test's
// generic capture error (which must still tear down), ErrSessionNotStarted must leave the
// hub alive so a later resize (once the session is actually running) succeeds normally.
func TestStreamHub_should_StayAliveAndRetryLater_When_CapturePaneContentErrorsWithSessionNotStarted(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	controller.captureErr = streamhub.ErrSessionNotStarted
	hub := streamhub.NewStreamHub("test-session", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(30*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	defer hub.ForceTeardown()

	transport1 := newMemoryTransport()
	transport2 := newMemoryTransport()
	id1 := hub.AttachSubscriber(transport1, streamhub.SubscriberCapability{CanResize: true})
	hub.AttachSubscriber(transport2, streamhub.SubscriberCapability{CanResize: true})

	hub.RequestResize(id1, mustSize(t, 100, 30))

	// Give applyNegotiatedSize's goroutine time to run and (incorrectly, pre-fix)
	// tear the hub down; then assert it's still alive and neither subscriber was
	// sent the stream-ended sentinel.
	time.Sleep(100 * time.Millisecond)
	if got := hub.State(); got == streamhub.HubTornDown {
		t.Fatalf("hub torn down after a session-not-started capture error — this exact transient condition must not kill the hub")
	}
	if got := transport1.receivedCount(); got != 0 {
		t.Fatalf("subscriber 1 should not have received a stream-ended sentinel, got %d frames", got)
	}
	if got := transport2.receivedCount(); got != 0 {
		t.Fatalf("subscriber 2 should not have received a stream-ended sentinel, got %d frames", got)
	}

	// Once the session is "running" (controller recovers), the next resize must
	// succeed normally — proving the hub wasn't left in some half-dead state.
	controller.captureErr = nil
	controller.captureContent = "now running"
	hub.RequestResize(id1, mustSize(t, 120, 40))

	if !waitFor(t, time.Second, func() bool { return controller.resizeCallCount(120, 40) == 1 }) {
		t.Fatalf("expected the hub to still service a later resize once the controller recovers")
	}
}
