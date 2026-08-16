package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// newStaleTestInstance builds a bare *session.Instance for StaleSessionNotifier tests.
// lastMeaningfulOutput is written through both the plain field and SyncAtomicTimestamps()
// so GetTimeSinceLastMeaningfulOutput()'s fast (atomic-shadow) path reflects it immediately,
// mirroring the pattern session/review_queue_poller_test.go's makeAcknowledgedInstance uses.
func newStaleTestInstance(title, uuid string, status session.Status, lastMeaningfulOutput time.Time) *session.Instance {
	inst := &session.Instance{
		Title:     title,
		UUID:      uuid,
		Status:    status,
		CreatedAt: time.Now(),
	}
	inst.LastMeaningfulOutput = lastMeaningfulOutput
	inst.SyncAtomicTimestamps()
	return inst
}

// setLastMeaningfulOutput updates inst's idle clock and keeps the atomic shadow in sync,
// so a later checkAll() call sees the new value via GetTimeSinceLastMeaningfulOutput's
// fast path (see newStaleTestInstance's doc comment).
func setLastMeaningfulOutput(inst *session.Instance, t time.Time) {
	inst.LastMeaningfulOutput = t
	inst.SyncAtomicTimestamps()
}

// newTestNotifierPoller builds a ReviewQueuePoller pre-loaded with instances, for
// StaleSessionNotifier tests that only need GetInstances() -- never Start()ed, so no
// polling goroutine or tmux/status-manager dependency is required.
func newTestNotifierPoller(instances ...*session.Instance) *session.ReviewQueuePoller {
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), nil, nil)
	poller.SetInstances(instances)
	return poller
}

// writeStaleSessionConfig writes a minimal config.json under dir with the given
// stale-session settings, for tests exercising checkAll()'s live config.LoadConfig() reload.
func writeStaleSessionConfig(t *testing.T, dir string, thresholdMinutes int, notifyEnabled bool) {
	t.Helper()
	cfg := map[string]any{
		"stale_session": map[string]any{
			"threshold_minutes": thresholdMinutes,
			"notify_enabled":    notifyEnabled,
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), data, 0644))
}

// drainOneNotification waits briefly for exactly one notification event on ch, failing the
// test if none arrives.
func drainOneNotification(t *testing.T, ch <-chan *events.Event) *events.Event {
	t.Helper()
	select {
	case ev := <-ch:
		require.Equal(t, events.EventNotification, ev.Type)
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event, got none")
		return nil
	}
}

// assertNoNotification fails the test if a notification event arrives within a short
// window.
func assertNoNotification(t *testing.T, ch <-chan *events.Event) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("expected no notification event, got %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStaleSessionNotifier_checkAll_should_FireExactlyOnce_When_SessionStaysStaleAcrossMultipleTicks(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeStaleSessionConfig(t, dir, 30, true)

	inst := newStaleTestInstance("sess-1", "uuid-1", session.Active, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)

	notifier.checkAll()
	drainOneNotification(t, ch)

	// No new output between ticks -- still stale, but must not re-notify.
	notifier.checkAll()
	notifier.checkAll()
	assertNoNotification(t, ch)
}

func TestStaleSessionNotifier_checkAll_should_ReArm_When_SessionRecoversAndGoesStaleAgain(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeStaleSessionConfig(t, dir, 30, true)

	inst := newStaleTestInstance("sess-2", "uuid-2", session.Active, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)

	// First stale episode.
	notifier.checkAll()
	drainOneNotification(t, ch)

	// Session produces fresh output -- recovers below threshold.
	setLastMeaningfulOutput(inst, time.Now())
	notifier.checkAll()
	assertNoNotification(t, ch)

	// Goes stale a second time.
	setLastMeaningfulOutput(inst, time.Now().Add(-40*time.Minute))
	notifier.checkAll()
	drainOneNotification(t, ch)
}

func TestStaleSessionNotifier_checkAll_should_NotNotify_When_SessionIsNotActive(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeStaleSessionConfig(t, dir, 30, true)

	inst := newStaleTestInstance("sess-3", "uuid-3", session.Paused, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)
	notifier.checkAll()
	assertNoNotification(t, ch)
}

func TestStaleSessionNotifier_checkAll_should_NotNotify_When_NotifyEnabledIsFalse(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeStaleSessionConfig(t, dir, 30, false)

	inst := newStaleTestInstance("sess-4", "uuid-4", session.Active, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)
	notifier.checkAll()
	assertNoNotification(t, ch)
}

func TestStaleSessionNotifier_checkAll_should_ObserveConfigChange_When_ConfigFileChangesBetweenTicks(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	// Threshold starts high enough that a 40-minute idle session is NOT yet stale.
	writeStaleSessionConfig(t, dir, 60, true)

	inst := newStaleTestInstance("sess-5", "uuid-5", session.Active, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)

	notifier.checkAll()
	assertNoNotification(t, ch)

	// Lower the threshold below the session's already-elapsed idle time -- no restart,
	// no re-construction of the notifier.
	writeStaleSessionConfig(t, dir, 30, true)

	notifier.checkAll()
	drainOneNotification(t, ch)
}

func TestStaleSessionNotifier_checkAll_should_ReNotify_When_SessionPausesThenResumesStillStale(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	writeStaleSessionConfig(t, dir, 30, true)

	inst := newStaleTestInstance("sess-6", "uuid-6", session.Active, time.Now().Add(-40*time.Minute))
	poller := newTestNotifierPoller(inst)
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	notifier := NewStaleSessionNotifier(poller, bus)

	// First stale episode.
	notifier.checkAll()
	drainOneNotification(t, ch)

	notifier.mu.Lock()
	_, stillTracked := notifier.notifiedSessions["uuid-6"]
	notifier.mu.Unlock()
	require.True(t, stillTracked, "dedup entry should exist right after notifying")

	// Pause while still stale -- the dedup entry must clear on this transition alone,
	// not just on idle-time recovery.
	inst.ForceStatus(session.Paused)
	notifier.checkAll()
	assertNoNotification(t, ch)

	notifier.mu.Lock()
	_, clearedOnPause := notifier.notifiedSessions["uuid-6"]
	notifier.mu.Unlock()
	require.False(t, clearedOnPause, "dedup entry should be cleared on the active->paused transition")

	// Resume while still past threshold -- a naive "only clear on recovery" implementation
	// would still suppress this, since idle time never dropped below threshold.
	inst.ForceStatus(session.Active)
	notifier.checkAll()
	drainOneNotification(t, ch)
}
