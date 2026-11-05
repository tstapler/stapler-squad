package streamhub

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

type armedTimer struct {
	d time.Duration
	f func()
}

// recordingClock is a FakeClock that also hands every armed callback to the test, so a test can
// invoke a superseded quiet callback by hand, as a timer that fired just before it was re-armed.
type recordingClock struct {
	*FakeClock
	armed chan armedTimer
}

func newRecordingClock() *recordingClock {
	return &recordingClock{FakeClock: NewFakeClock(time.Unix(0, 0)), armed: make(chan armedTimer, 64)}
}

func (c *recordingClock) AfterFunc(d time.Duration, f func()) Timer {
	t := c.FakeClock.AfterFunc(d, f)
	c.armed <- armedTimer{d, f}
	return t
}

type quiescenceRun struct {
	clock   *recordingClock
	updates chan []byte
	done    chan struct{}
}

func startQuiescence(timeout, quiet time.Duration) *quiescenceRun {
	r := &quiescenceRun{clock: newRecordingClock(), updates: make(chan []byte), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		waitForQuiescence(r.clock, r.updates, timeout, quiet, "s")
	}()
	return r
}

// update delivers one update and returns the quiet timer it re-armed. It fails if the wait had
// already returned.
func (r *quiescenceRun) update(t *testing.T) armedTimer {
	t.Helper()
	select {
	case r.updates <- []byte("u"):
	case <-r.done:
		t.Fatal("wait returned before the update arrived")
	}
	return <-r.clock.armed
}

func TestWaitForQuiescence_should_IgnoreSupersededQuietCallback(t *testing.T) {
	defer goleak.VerifyNone(t)
	// The race is between the loop reaching its select and the stale callback's signal, so the
	// unguarded code only sometimes ends early; repeating makes that near-certain without sleeping.
	for i := 0; i < 200; i++ {
		r := startQuiescence(500*time.Millisecond, 200*time.Millisecond)
		<-r.clock.armed // deadline
		stale := <-r.clock.armed
		r.update(t)
		stale.f() // the callback of the quiet timer the update just superseded

		select {
		case r.updates <- []byte("u"):
		case <-r.done:
			t.Fatalf("iteration %d: a superseded quiet callback ended the wait early", i)
		}
		<-r.clock.armed

		r.clock.Advance(200 * time.Millisecond)
		<-r.done
	}
}

func TestWaitForQuiescence_should_Return_When_QuietPeriodPassesWithoutUpdates(t *testing.T) {
	defer goleak.VerifyNone(t)
	r := startQuiescence(500*time.Millisecond, 200*time.Millisecond)
	<-r.clock.armed
	<-r.clock.armed
	r.clock.Advance(199 * time.Millisecond)
	if got := r.clock.Pending(); got != 2 {
		t.Fatalf("before the quiet period ends the deadline and quiet timers must both be armed, got %d", got)
	}
	r.clock.Advance(time.Millisecond)
	<-r.done
}

func TestWaitForQuiescence_should_BoundWaitAtDeadline_When_UpdatesNeverStop(t *testing.T) {
	defer goleak.VerifyNone(t)
	r := startQuiescence(500*time.Millisecond, 200*time.Millisecond)
	<-r.clock.armed
	<-r.clock.armed
	for i := 0; i < 3; i++ { // updates every 150 ms keep the 200 ms quiet period from ever elapsing
		r.update(t)
		r.clock.Advance(150 * time.Millisecond)
	}
	r.update(t) // fourth update, at 450 ms
	r.clock.Advance(50 * time.Millisecond)
	<-r.done
}
