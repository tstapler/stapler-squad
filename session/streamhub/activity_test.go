package streamhub_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/qos"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

type activityRig struct {
	clock *streamhub.FakeClock
	qos   *qos.Tracker
	got   chan qos.Activity
	fails atomic.Int32 // the next N sink calls report the hint as undelivered
}

func (r *activityRig) sinkErr() error {
	for {
		n := r.fails.Load()
		if n <= 0 {
			return nil
		}
		if r.fails.CompareAndSwap(n, n-1) {
			return errors.New("daemon unreachable")
		}
	}
}

func newActivityRig() *activityRig {
	clock := streamhub.NewFakeClock(time.Unix(5000, 0))
	return &activityRig{clock: clock, qos: qos.NewTracker(clock.Now, qos.InteractiveWindow), got: make(chan qos.Activity, 16)}
}

func (r *activityRig) hub(name string, extra ...streamhub.HubOption) *streamhub.StreamHub {
	opts := make([]streamhub.HubOption, 0, 4+len(extra))
	opts = append(opts,
		streamhub.WithClock(r.clock), streamhub.WithQoSTracker(r.qos),
		streamhub.WithActivitySink(func(a qos.Activity) error { r.got <- a; return r.sinkErr() }),
		streamhub.WithTeardownGrace(time.Hour),
	)
	return streamhub.NewStreamHub(name, newFakeSessionController(), append(opts, extra...)...)
}

// hub starts a hub and consumes its initial hint, which every fresh hub sends unconditionally.
func (r *activityRig) startedHub(t *testing.T, name string, extra ...streamhub.HubOption) *streamhub.StreamHub {
	t.Helper()
	h := r.hub(name, extra...)
	require.Equal(t, qos.ActivityNormal, r.next(t), "a fresh hub asserts its class instead of assuming the daemon's")
	return h
}

func (r *activityRig) next(t *testing.T) qos.Activity {
	t.Helper()
	select {
	case a := <-r.got:
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("no activity hint was sent")
		return qos.ActivityNormal
	}
}

// pendingTimers waits until exactly n fake timers are armed. The retry timer is armed by the
// pump goroutine after the sink returns, so advancing the clock earlier would miss it.
func (r *activityRig) pendingTimers(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return r.clock.Pending() == n }, 5*time.Second, time.Millisecond)
}

func (r *activityRig) none(t *testing.T) {
	t.Helper()
	select {
	case a := <-r.got:
		t.Fatalf("unexpected activity hint %v", a)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_should_SendInteractiveOnce_ThenNormalWhenTypingStops(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	defer hub.ForceTeardown()

	hub.NoteInteractive("viewer")
	assert.Equal(t, qos.ActivityInteractive, rig.next(t))
	rig.clock.Advance(2 * time.Second)
	hub.NoteInteractive("viewer")
	hub.NoteInteractive("viewer")
	rig.none(t) // more keystrokes in the window are not new hints

	rig.clock.Advance(qos.InteractiveWindow - time.Second)
	rig.none(t) // the window slid with the last keystroke: still interactive
	rig.clock.Advance(time.Second)
	assert.Equal(t, qos.ActivityNormal, rig.next(t))
}

func TestHub_should_ResendTheHint_Periodically_WhileTypingContinues(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	defer hub.ForceTeardown()

	base := rig.clock.Pending()
	hub.NoteInteractive("viewer")
	require.Equal(t, qos.ActivityInteractive, rig.next(t))
	rig.pendingTimers(t, base+2) // the typing expiry and the refresh, armed after the sink returns
	for elapsed := time.Duration(0); elapsed < 50*time.Second; elapsed += 4 * time.Second {
		rig.clock.Advance(4 * time.Second)
		hub.NoteInteractive("viewer")
	}
	rig.none(t)
	for elapsed := time.Duration(0); elapsed < 12*time.Second; elapsed += 4 * time.Second {
		rig.clock.Advance(4 * time.Second)
		hub.NoteInteractive("viewer")
	}
	assert.Equal(t, qos.ActivityInteractive, rig.next(t), "a restarted daemon forgot the hint; the refresh restores it")
}

func TestHub_should_BeBackground_WhileAnotherSessionIsInteractive(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	typing := rig.startedHub(t, "typing")
	idle := rig.startedHub(t, "idle")
	defer typing.ForceTeardown()
	defer idle.ForceTeardown()

	typing.NoteInteractive("viewer")
	seen := map[qos.Activity]int{rig.next(t): 1}
	seen[rig.next(t)]++
	assert.Equal(t, 1, seen[qos.ActivityInteractive], "typing hub")
	assert.Equal(t, 1, seen[qos.ActivityBackground], "idle hub")

	rig.clock.Advance(qos.InteractiveWindow)
	seen = map[qos.Activity]int{rig.next(t): 1}
	seen[rig.next(t)]++
	assert.Equal(t, 2, seen[qos.ActivityNormal], "both settle once typing stops")
}

func TestHub_should_ClearItsHint_When_TornDownWhileInteractive(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	hub.NoteInteractive("viewer")
	require.Equal(t, qos.ActivityInteractive, rig.next(t))

	hub.ForceTeardown()
	assert.Equal(t, qos.ActivityNormal, rig.next(t), "a dead hub must not leave its pane boosted")
}

func TestHub_should_HoldBackgroundOutputToTheCeiling_WhileAnotherSessionTypes(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	typing := rig.startedHub(t, "typing")
	idle := rig.startedHub(t, "idle")
	defer typing.ForceTeardown()
	defer idle.ForceTeardown()
	sink := streamhub.NewMemoryTransport()
	idle.AttachSubscriber(sink, streamhub.SubscriberCapability{})
	baseline := len(sink.ReceivedFrames())

	typing.NoteInteractive("viewer")
	rig.next(t)
	rig.next(t)

	idle.OnRawOutput([]byte("background bytes"))
	idle.TryFlush()
	assert.Equal(t, baseline, len(sink.ReceivedFrames()), "opportunistic flush is skipped for a background hub")

	rig.clock.Advance(streamhub.MaxBatchWindow)
	assert.Equal(t, baseline, len(sink.ReceivedFrames()), "background ceiling is stretched past the normal one")
	rig.clock.Advance(streamhub.BackgroundBatchWindow - streamhub.MaxBatchWindow)
	assert.Eventually(t, func() bool { return len(sink.ReceivedFrames()) > baseline }, 5*time.Second, time.Millisecond,
		"background output still flushes at the bounded ceiling")
}

func TestHub_should_FlushInteractiveOutputImmediately(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	typing := rig.startedHub(t, "typing")
	defer typing.ForceTeardown()
	sink := streamhub.NewMemoryTransport()
	typing.AttachSubscriber(sink, streamhub.SubscriberCapability{})
	baseline := len(sink.ReceivedFrames())
	typing.NoteInteractive("viewer")

	typing.OnRawOutput([]byte("echo"))
	typing.TryFlush()
	assert.Eventually(t, func() bool { return len(sink.ReceivedFrames()) > baseline }, 5*time.Second, time.Millisecond)
}

func TestHub_should_SendAnInitialHint_When_Created(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.hub("a")
	defer hub.ForceTeardown()

	assert.Equal(t, qos.ActivityNormal, rig.next(t), "a previous hub may have left the pane demoted")
	rig.none(t)
}

func TestHub_should_RetryAHint_When_TheSinkReportsItUndelivered(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	defer hub.ForceTeardown()
	base := rig.clock.Pending()
	rig.fails.Store(1)

	hub.NoteInteractive("viewer")
	require.Equal(t, qos.ActivityInteractive, rig.next(t)) // fails
	rig.none(t)                                            // no hot loop on failure
	rig.pendingTimers(t, base+2)                           // the typing expiry and the retry
	rig.clock.Advance(2 * time.Second)
	assert.Equal(t, qos.ActivityInteractive, rig.next(t), "the unconfirmed hint is retried, not assumed sent")
	rig.clock.Advance(2 * time.Second)
	rig.none(t) // confirmed now: the next send is the 1-minute refresh
}

func TestHub_should_RetryTheFinalNormal_When_ItFailsAtTeardown(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	hub.NoteInteractive("viewer")
	require.Equal(t, qos.ActivityInteractive, rig.next(t))
	rig.fails.Store(1)

	hub.ForceTeardown()
	require.Equal(t, qos.ActivityNormal, rig.next(t)) // fails: the daemon still holds Interactive
	rig.pendingTimers(t, 1)
	rig.clock.Advance(2 * time.Second)
	assert.Equal(t, qos.ActivityNormal, rig.next(t), "a dead hub keeps trying to clear its pane")
	rig.clock.Advance(time.Minute)
	rig.none(t)
}

func TestHub_should_StopRetryingAFinalNormal_AfterABoundedNumberOfAttempts(t *testing.T) {
	t.Parallel()
	rig := newActivityRig()
	hub := rig.startedHub(t, "a")
	hub.NoteInteractive("viewer")
	require.Equal(t, qos.ActivityInteractive, rig.next(t))
	rig.fails.Store(1000)

	hub.ForceTeardown()
	require.Equal(t, qos.ActivityNormal, rig.next(t))
	for i := 0; i < 4; i++ {
		rig.pendingTimers(t, 1)
		rig.clock.Advance(2 * time.Second)
		require.Equal(t, qos.ActivityNormal, rig.next(t))
	}
	rig.pendingTimers(t, 0) // the fifth failure arms nothing
	rig.clock.Advance(time.Minute)
	rig.none(t)
}
