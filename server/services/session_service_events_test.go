package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

const failureType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE)

// crashEnv is a SessionService on a recording bus. A sentinel event published
// after the calls under test marks the end of what the bus will deliver, so no
// test waits on a timer to prove an event did not happen.
type crashEnv struct {
	svc *SessionService
	bus *events.EventBus
	ch  <-chan *events.Event
}

func newCrashEnv(t *testing.T, gated bool) *crashEnv {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	storage := createTestStorage(t)
	var svc *SessionService
	if gated {
		svc = newGatedSessionService(storage)
		require.NoError(t, config.LoadConfig().SetFeatureFlag(config.HiddenSessionGateFeatureFlag, true))
		svc.DeliveryGate().Flags().Reload()
	} else {
		svc = NewSessionService(storage, events.NewEventBus(100))
	}
	t.Cleanup(svc.Shutdown)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := svc.eventBus.Subscribe(ctx)
	return &crashEnv{svc: svc, bus: svc.eventBus, ch: ch}
}

const crashSentinelID = "crash-sentinel"

var testNow = time.Unix(1_700_000_000, 0)

// collect returns every event delivered before the sentinel.
func (e *crashEnv) collect(t *testing.T) []*events.Event {
	t.Helper()
	e.bus.Publish(events.NewNotificationEvent("", "", crashSentinelID,
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), 1, "s", "s", nil))
	var out []*events.Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev := <-e.ch:
			if ev.Type == events.EventNotification && ev.NotificationID == crashSentinelID {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("sentinel never arrived")
		}
	}
}

func notificationsOf(evs []*events.Event, typ int32) []*events.Event {
	var out []*events.Event
	for _, ev := range evs {
		if ev.Type == events.EventNotification && ev.NotificationType == typ {
			out = append(out, ev)
		}
	}
	return out
}

func crashedSnap(uuid, title, reason string, at time.Time) *session.InstanceSnapshot {
	return &session.InstanceSnapshot{UUID: uuid, Title: title, ExitReason: reason, Status: session.Crashed, UpdatedAt: at}
}

func TestSessionExitedPublisher_ShouldPublishOneFailureAndOneSessionUpdated_WhenCrashedAndNoneWhenStopped(t *testing.T) {
	e := newCrashEnv(t, false)
	storage := e.svc.storage.(*session.Storage)
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "crashy", Path: "/tmp/test", Status: session.Paused, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	instances, err := e.svc.loadInstancesWithWiring()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	inst := instances[0]

	inst.ExitReason = "pane exited 137"
	inst.ForceStatus(session.Crashed)
	inst.FireLifecycleEventForTest(session.EventExited, "dead-pane-crashed")
	// The publisher goroutine publishes session.updated then the FAILURE.
	var seen []*events.Event
	for len(notificationsOf(seen, failureType)) == 0 {
		select {
		case ev := <-e.ch:
			seen = append(seen, ev)
		case <-time.After(5 * time.Second):
			t.Fatalf("no crash notification; saw %d events", len(seen))
		}
	}
	got := notificationsOf(append(seen, e.collect(t)...), failureType)
	require.Len(t, got, 1)
	f := got[0]
	assert.Equal(t, inst.UUID, f.SessionID)
	assert.True(t, strings.HasPrefix(f.NotificationID, "session-crashed-"+inst.UUID+"-"))
	assert.Equal(t, "Session crashed", f.NotificationTitle)
	assert.Contains(t, f.NotificationMessage, "crashy")
	assert.Contains(t, f.NotificationMessage, "pane exited 137")
	assert.Equal(t, derivePriority(true, true), f.NotificationPriority)
	assert.Equal(t, "true", f.NotificationMetadata["session_scoped"])
	updated := 0
	for _, ev := range seen {
		if ev.Type == events.EventSessionUpdated {
			updated++
		}
	}
	assert.Equal(t, 1, updated, "the existing session.updated publish still happens once")

	// A Stopped exit publishes no crash notification.
	e.svc.publishCrash(&session.InstanceSnapshot{UUID: "u2", Title: "stopped", Status: session.Stopped})
	assert.Empty(t, notificationsOf(e.collect(t), failureType))
}

func TestSessionExitedPublisher_ShouldPublishOnlyWhenSnapshotStatusCrashed_WhenExitedEventFromAnotherSource(t *testing.T) {
	e := newCrashEnv(t, false)
	storage := e.svc.storage.(*session.Storage)
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "other-exit", Path: "/tmp/test", Status: session.Paused, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	instances, err := e.svc.loadInstancesWithWiring()
	require.NoError(t, err)
	inst := instances[0]

	inst.ForceStatus(session.Active) // not Crashed when the publisher reads it
	inst.FireLifecycleEventForTest(session.EventExited, "pty-eof")
	for { // wait for the publisher's session.updated, which precedes any crash publish
		ev := <-e.ch
		if ev.Type == events.EventSessionUpdated {
			break
		}
	}
	assert.Empty(t, notificationsOf(e.collect(t), failureType))

	inst.ForceStatus(session.Crashed)
	inst.FireLifecycleEventForTest(session.EventExited, "dead-pane-crashed")
	for { // the crash's own FAILURE arrives after its session.updated
		ev := <-e.ch
		if ev.Type == events.EventNotification && ev.NotificationType == failureType {
			break
		}
	}
}

func TestCrash_ShouldRefireWithNewIDAndHigherCount_WhenResumedAndCrashedAgain(t *testing.T) {
	e := newCrashEnv(t, false)
	t0 := time.Unix(1_700_000_000, 0)
	e.svc.publishCrash(crashedSnap("u1", "crashy", "exit 1", t0))
	e.svc.crashes.windowStart = time.Time{} // a later window, so the limiter is not what differs
	e.svc.publishCrash(crashedSnap("u1", "crashy", "exit 1", t0.Add(5*time.Minute)))

	got := notificationsOf(e.collect(t), failureType)
	require.Len(t, got, 2)
	assert.NotEqual(t, got[0].NotificationID, got[1].NotificationID)
	assert.Equal(t, got[0].SessionID, got[1].SessionID, "the history store coalesces by (session, type)")
}

func TestSessionExitedPublisher_ShouldSanitizeAndTruncateExitReason_WhenReasonHasControlOrIsLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 500)
	ev := NewSessionCrashEvent("u1", "my\nsession", "bad\x1b[31m\nreason "+long, time.Unix(1, 0))
	assert.NotContains(t, ev.NotificationMessage, "\n")
	assert.NotContains(t, ev.NotificationMessage, "\x1b")
	assert.LessOrEqual(t, len([]rune(ev.NotificationMessage)), 160+len("Session \"my session\" crashed: ")+3)
	assert.True(t, strings.HasSuffix(ev.NotificationMessage, "..."))
}

func TestCrashCoalescing_ShouldPublish3ThenOneSummaryForRemaining7_WhenTenCrashesInSixtySeconds(t *testing.T) {
	e := newCrashEnv(t, true)
	clk := newGateTestClock()
	e.svc.crashes.now = clk.Now
	at := clk.Now()
	for i := 0; i < 10; i++ {
		e.svc.publishCrash(crashedSnap(fmt.Sprintf("u%d", i), fmt.Sprintf("sess-%d", i), "exit", at))
		clk.Advance(time.Second)
	}
	got := notificationsOf(e.collect(t), failureType)
	individual, summaries := 0, map[string]*events.Event{}
	for _, ev := range got {
		if strings.HasPrefix(ev.NotificationID, crashSummaryIDPrefix) {
			summaries[ev.NotificationID] = ev
			continue
		}
		individual++
	}
	assert.Equal(t, 3, individual)
	require.Len(t, summaries, 1, "one summary, updated in place by its stable id")
	var last *events.Event
	for _, ev := range got {
		if strings.HasPrefix(ev.NotificationID, crashSummaryIDPrefix) {
			last = ev
		}
	}
	assert.Contains(t, last.NotificationMessage, "7 more sessions crashed")
	assert.Contains(t, last.NotificationMessage, "and 2 more", "five titles are listed, the rest counted")
	assert.Equal(t, uint64(7), e.svc.DeliveryGate().Metrics().Value(deliverygate.CounterCrashCoalesced))

	// A new window starts individual publishing again.
	clk.Advance(2 * time.Minute)
	e.svc.publishCrash(crashedSnap("u-late", "late", "exit", clk.Now()))
	late := notificationsOf(e.collect(t), failureType)
	require.Len(t, late, 1)
	assert.Equal(t, "u-late", late[0].SessionID)
}

func TestCrashCoalescing_ShouldCountHiddenAndVisibleInOneLimiter_WhenMixedCrashes(t *testing.T) {
	e := newCrashEnv(t, false)
	clk := newGateTestClock()
	e.svc.crashes.now = clk.Now
	snaps := []*session.InstanceSnapshot{
		crashedSnap("h1", "hidden-1", "x", clk.Now()), crashedSnap("v1", "visible-1", "x", clk.Now()),
		crashedSnap("h2", "hidden-2", "x", clk.Now()), crashedSnap("v2", "visible-2", "x", clk.Now()),
	}
	snaps[0].Hidden, snaps[2].Hidden = true, true
	for _, s := range snaps {
		e.svc.publishCrash(s)
	}
	got := notificationsOf(e.collect(t), failureType)
	require.Len(t, got, 4)
	summary := 0
	for _, ev := range got {
		if strings.HasPrefix(ev.NotificationID, crashSummaryIDPrefix) {
			summary++
		}
	}
	assert.Equal(t, 1, summary, "the fourth crash folds regardless of hidden or visible")
}
