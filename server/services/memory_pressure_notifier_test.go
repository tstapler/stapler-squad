package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeRatio returns a ratioFunc that always reports the given (ratio, true).
func fakeRatio(ratio float64) func() (float64, bool) {
	return func() (float64, bool) { return ratio, true }
}

func TestMemoryPressureNotifier_checkOnce_should_FireOnce_When_RatioCrossesWarnThreshold(t *testing.T) {
	poller := newTestNotifierPoller()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewMemoryPressureNotifier(poller, bus)
	notifier.ratioFunc = fakeRatio(0.95)

	notifier.checkOnce()
	drainOneNotification(t, ch)

	// Still over the warn threshold on later ticks -- must not re-notify.
	notifier.checkOnce()
	notifier.checkOnce()
	assertNoNotification(t, ch)
}

func TestMemoryPressureNotifier_checkOnce_should_NotNotify_When_RatioStaysBelowWarnThreshold(t *testing.T) {
	poller := newTestNotifierPoller()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewMemoryPressureNotifier(poller, bus)
	notifier.ratioFunc = fakeRatio(0.5)

	notifier.checkOnce()
	assertNoNotification(t, ch)
}

func TestMemoryPressureNotifier_checkOnce_should_NotNotify_When_RatioFuncReportsUnavailable(t *testing.T) {
	poller := newTestNotifierPoller()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewMemoryPressureNotifier(poller, bus)
	notifier.ratioFunc = func() (float64, bool) { return 0, false }

	notifier.checkOnce()
	assertNoNotification(t, ch)
}

// TestMemoryPressureNotifier_checkOnce_should_ReArm_When_RatioDropsBelowClearThreshold is the
// regression test for the hysteresis gap (memoryPressureWarnRatio=0.90,
// memoryPressureClearRatio=0.75): usage sitting between the two thresholds must neither
// re-notify nor re-arm, only a drop below the clear threshold re-arms.
func TestMemoryPressureNotifier_checkOnce_should_ReArm_When_RatioDropsBelowClearThreshold(t *testing.T) {
	poller := newTestNotifierPoller()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewMemoryPressureNotifier(poller, bus)

	notifier.ratioFunc = fakeRatio(0.95)
	notifier.checkOnce()
	drainOneNotification(t, ch)

	// Dropped, but still inside the hysteresis gap -- not re-armed yet.
	notifier.ratioFunc = fakeRatio(0.80)
	notifier.checkOnce()
	assertNoNotification(t, ch)

	// Below the clear threshold -- re-armed.
	notifier.ratioFunc = fakeRatio(0.70)
	notifier.checkOnce()
	assertNoNotification(t, ch) // clearing itself doesn't notify

	// Back over the warn threshold -- fires again now that it's re-armed.
	notifier.ratioFunc = fakeRatio(0.95)
	notifier.checkOnce()
	drainOneNotification(t, ch)
}

func TestMemoryPressureNotifier_reclaimOptionsText_should_ListOnlyIdleActiveSessions_SortedLongestFirst(t *testing.T) {
	shortIdle := newStaleTestInstance("short-idle-sess", "uuid-1", session.Active, time.Now().Add(-11*time.Minute))
	longIdle := newStaleTestInstance("long-idle-sess", "uuid-2", session.Active, time.Now().Add(-30*time.Minute))
	busySession := newStaleTestInstance("busy-sess", "uuid-3", session.Active, time.Now())
	pausedSession := newStaleTestInstance("paused-sess", "uuid-4", session.Paused, time.Now().Add(-60*time.Minute))
	poller := newTestNotifierPoller(shortIdle, longIdle, busySession, pausedSession)

	notifier := NewMemoryPressureNotifier(poller, nil)
	text := notifier.reclaimOptionsText()

	longIdx := strings.Index(text, "long-idle-sess")
	shortIdx := strings.Index(text, "short-idle-sess")
	if longIdx == -1 || shortIdx == -1 || longIdx > shortIdx {
		t.Errorf("reclaimOptionsText() = %q, want long-idle-sess listed before short-idle-sess (sorted longest-idle first)", text)
	}
	for _, unwanted := range []string{"busy-sess", "paused-sess"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("reclaimOptionsText() = %q, want it to omit %q (not idle past the floor, or not active)", text, unwanted)
		}
	}
}
