package streamhub_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// TestWaitForQuiescence_should_LogHubScopedWarn_When_QuiescenceTimesOutAfter500ms
// closes a coverage gap found in a spec-compliance sweep: quiescence.go's
// hub-scoped WARN (fired when waitForQuiescence's hard deadline elapses
// before the quiet period ever settles, Task 1.3.2f) had no dedicated test
// asserting it actually fires. Reuses captureLogs/syncLogBuffer from
// observability_test.go (a mutex-guarded slog buffer) per this area's
// existing convention rather than inventing a new logging seam.
//
// The WARN only fires on the deadline branch of quiescence.go's select —
// which only wins if updates keep arriving faster than the quiet period, so
// the quiet timer never gets a chance to elapse on its own first. This test
// exercises that with NewStreamHub's real defaults (500ms timeout / 200ms
// quiet period, matching the constant this WARN's message and the test name
// reference), feeding the fake SessionController's update channel every 50ms
// — comfortably faster than the 200ms quiet window — until the 500ms
// deadline wins.
func TestWaitForQuiescence_should_LogHubScopedWarn_When_QuiescenceTimesOutAfter500ms(t *testing.T) {
	defer goleak.VerifyNone(t)
	buf := captureLogs(t)

	const sessionName = "quiescence-timeout-session"
	controller := newFakeSessionController()
	controller.captureContent = "snapshot"
	hub := streamhub.NewStreamHub(sessionName, controller, streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()

	stopFeed := make(chan struct{})
	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopFeed:
				return
			case <-ticker.C:
				// The send must happen while still holding controller.mu, not
				// after re-reading ch/subscribed and releasing the lock: this
				// is the exact same lock UnsubscribeControlModeUpdates holds
				// while it closes and reassigns f.updates
				// (lifecycle_test.go), so releasing it before sending leaves a
				// window where this goroutine's non-blocking send on the
				// captured channel value races a concurrent close of that
				// same channel — a genuine data race under -race regardless
				// of the select-default, since nothing then orders the send
				// against the close.
				controller.mu.Lock()
				if controller.subscribed {
					select {
					case controller.updates <- []byte("x"):
					default:
					}
				}
				controller.mu.Unlock()
			}
		}
	}()

	id := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	hub.RequestResize(context.Background(), id, mustSize(t, 100, 30))

	close(stopFeed)
	<-feedDone

	got := buf.String()
	if !strings.Contains(got, "streamhub quiescence timed out") {
		t.Fatalf("expected a hub-scoped quiescence-timeout WARN log line, got: %s", got)
	}
	if !strings.Contains(got, sessionName) {
		t.Fatalf("expected the WARN log line to identify the session %q, got: %s", sessionName, got)
	}
}
