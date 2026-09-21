package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
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
	ev := drainOneNotification(t, ch)
	// Push-gate classification table: urgent and important — approaching the
	// memory limit risks an OOM kill right now.
	assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), ev.NotificationPriority, "Memory usage near limit must derive to URGENT (urgent=true, important=true)")

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
//
// The clear step DOES fire an explicit clear notification (backlog item
// cfda07b7-73fb-42e1-a21b-7fdf8a052a14, AC4) -- intentionally updated from the
// prior "clearing itself doesn't notify" behavior, which left the persisted
// notification record (and the status banner reading it) stuck at "near
// limit" forever after the condition actually ended. Same stable ID and
// NotificationType as the warn notification, so it updates that record in
// place instead of creating a second one.
func TestMemoryPressureNotifier_checkOnce_should_ReArm_When_RatioDropsBelowClearThreshold(t *testing.T) {
	poller := newTestNotifierPoller()
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewMemoryPressureNotifier(poller, bus)

	notifier.ratioFunc = fakeRatio(0.95)
	notifier.checkOnce()
	warnEvent := drainOneNotification(t, ch)

	// Dropped, but still inside the hysteresis gap -- not re-armed yet.
	notifier.ratioFunc = fakeRatio(0.80)
	notifier.checkOnce()
	assertNoNotification(t, ch)

	// Below the clear threshold -- re-armed, and fires an explicit clear signal
	// carrying the same stable ID/type as the warn notification.
	notifier.ratioFunc = fakeRatio(0.70)
	notifier.checkOnce()
	clearEvent := drainOneNotification(t, ch)
	assert.Equal(t, warnEvent.NotificationID, clearEvent.NotificationID, "clear must reuse the warn notification's stable ID")
	assert.Equal(t, warnEvent.NotificationType, clearEvent.NotificationType, "clear must keep NotificationType constant so the dedup key still matches")
	assert.Equal(t, "ok", clearEvent.NotificationMetadata["memory_pressure_level"])

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
