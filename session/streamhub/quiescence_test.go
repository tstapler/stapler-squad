package streamhub_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// armSignalClock is a FakeClock that reports the duration of every timer armed on it, so a test
// can wait for the hub to arm a timer before advancing time instead of polling real time.
type armSignalClock struct {
	*streamhub.FakeClock
	armed chan time.Duration
}

func (c *armSignalClock) AfterFunc(d time.Duration, f func()) streamhub.Timer {
	t := c.FakeClock.AfterFunc(d, f)
	c.armed <- d
	return t
}

// awaitArmed blocks until the clock reports a timer armed for d, discarding other durations.
func (c *armSignalClock) awaitArmed(t *testing.T, d time.Duration) {
	t.Helper()
	for {
		select {
		case got := <-c.armed:
			if got == d {
				return
			}
		case <-time.After(time.Minute): // hang guard only; never elapses in a passing run
			t.Fatalf("no timer armed for %s", d)
		}
	}
}

// runResizeUnderFakeClock drives one RequestResize through the hub's quiescence wait on a fake
// clock and returns the captured logs. feed runs while the wait is blocked: it receives the
// controller and the clock, with the 500ms deadline and the first 200ms quiet timer already armed.
func runResizeUnderFakeClock(t *testing.T, sessionName string, feed func(*fakeSessionController, *armSignalClock)) string {
	t.Helper()
	buf := captureLogs(t)
	controller := newFakeSessionController()
	controller.captureContent = "snapshot"
	clock := &armSignalClock{FakeClock: streamhub.NewFakeClock(time.Unix(0, 0)), armed: make(chan time.Duration, 256)}
	hub := streamhub.NewStreamHub(sessionName, controller, streamhub.WithClock(clock), streamhub.WithTeardownGrace(time.Hour))
	defer func() { _ = hub.ForceTeardown() }()

	id := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	resized := make(chan struct{})
	go func() {
		defer close(resized)
		hub.RequestResize(context.Background(), id, mustSize(t, 100, 30))
	}()
	clock.awaitArmed(t, 500*time.Millisecond) // deadline
	clock.awaitArmed(t, 200*time.Millisecond) // first quiet timer
	feed(controller, clock)
	<-resized
	return buf.String()
}

// TestWaitForQuiescence_should_LogHubScopedWarn_When_QuiescenceTimesOutAfter500ms
// closes a coverage gap found in a spec-compliance sweep: quiescence.go's
// hub-scoped WARN (fired when waitForQuiescence's hard deadline elapses
// before the quiet period ever settles, Task 1.3.2f) had no dedicated test
// asserting it actually fires.
//
// The WARN only fires on the deadline branch of quiescence.go's select, which only wins if
// updates keep arriving faster than the quiet period. On the hub's fake clock (500ms timeout /
// 200ms quiet period, the NewStreamHub defaults) an update lands every 50ms of fake time, so no
// real scheduling gap can let the quiet timer win; the final 50ms step reaches exactly the 500ms
// deadline, which also satisfies quiescence.go's `elapsed >= timeout-5ms` guard.
func TestWaitForQuiescence_should_LogHubScopedWarn_When_QuiescenceTimesOutAfter500ms(t *testing.T) {
	defer goleak.VerifyNone(t)
	const sessionName = "quiescence-timeout-session"

	got := runResizeUnderFakeClock(t, sessionName, func(c *fakeSessionController, clock *armSignalClock) {
		for elapsed := 50 * time.Millisecond; elapsed < 500*time.Millisecond; elapsed += 50 * time.Millisecond {
			clock.Advance(50 * time.Millisecond)
			c.mu.Lock()
			c.updates <- []byte("x")
			c.mu.Unlock()
			clock.awaitArmed(t, 200*time.Millisecond) // the update re-armed the quiet timer
		}
		clock.Advance(50 * time.Millisecond) // t=500ms: the deadline wins, the last quiet timer is due at 650ms
	})

	if !strings.Contains(got, "streamhub quiescence timed out") {
		t.Fatalf("expected a hub-scoped quiescence-timeout WARN log line, got: %s", got)
	}
	if !strings.Contains(got, sessionName) {
		t.Fatalf("expected the WARN log line to identify the session %q, got: %s", sessionName, got)
	}
}

// The quiet timer winning is the normal path and must stay silent.
func TestWaitForQuiescence_should_NotLogWarn_When_QuietPeriodSettlesBeforeDeadline(t *testing.T) {
	defer goleak.VerifyNone(t)

	got := runResizeUnderFakeClock(t, "quiescence-quiet-session", func(_ *fakeSessionController, clock *armSignalClock) {
		clock.Advance(200 * time.Millisecond)
	})

	if strings.Contains(got, "streamhub quiescence timed out") {
		t.Fatalf("a settled quiet period must not log the timeout WARN, got: %s", got)
	}
}
