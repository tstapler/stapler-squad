package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// fakeResultsWithUsage builds a single fake ParseResult with one in-window
// (1h ago) turn totaling exactly usedTokens across Input+Output+CacheCreation+CacheRead.
func fakeResultsWithUsage(usedTokens int64) []*tokens.ParseResult {
	return []*tokens.ParseResult{
		{
			TurnTimeline: []tokens.TurnStats{
				{Timestamp: time.Now().Add(-time.Hour), Input: usedTokens},
			},
		},
	}
}

// countingFeatureController is a call-count-tracking test double for
// FeatureController, distinct from fakeFeatureController (feature_flags_test.go)
// which only tracks bool flags, not counts — several QuotaGate regression
// tests need to assert exact call counts across multiple Reconcile ticks.
type countingFeatureController struct {
	enabled      bool
	enableCalls  int
	disableCalls int
}

func (c *countingFeatureController) Enable(_ context.Context) error {
	c.enableCalls++
	c.enabled = true
	return nil
}

func (c *countingFeatureController) Disable() error {
	c.disableCalls++
	c.enabled = false
	return nil
}

func (c *countingFeatureController) IsEnabled() bool { return c.enabled }

func newTestQuotaGate(cfg config.QuotaConfig, store *fakeTokenStore, poller *mockInstancePoller, ctrl *countingFeatureController, bus *events.EventBus) *QuotaGate {
	return NewQuotaGate(func() config.QuotaConfig { return cfg }, store, poller, ctrl, bus)
}

// ---------------------------------------------------------------------------
// RateLimitAggregate
// ---------------------------------------------------------------------------

func TestRateLimitAggregate_should_ReportRecent_When_EventWithinWindow(t *testing.T) {
	now := time.Now()
	agg := RateLimitAggregate{}
	agg.recordRateLimitEvent(now.Add(-10 * time.Minute))

	if !agg.hasRecentRateLimitEvent(now, 30*time.Minute) {
		t.Error("hasRecentRateLimitEvent(30m window) = false, want true")
	}
	if agg.hasRecentRateLimitEvent(now, 5*time.Minute) {
		t.Error("hasRecentRateLimitEvent(5m window) = true, want false (event is 10m old)")
	}
}

func TestRateLimitAggregate_should_ReportNotRecent_When_NoEventRecorded(t *testing.T) {
	agg := RateLimitAggregate{}
	if agg.hasRecentRateLimitEvent(time.Now(), 30*time.Minute) {
		t.Error("hasRecentRateLimitEvent = true, want false (no event ever recorded)")
	}
}

// ---------------------------------------------------------------------------
// QuotaGate constructor
// ---------------------------------------------------------------------------

func TestNewQuotaGate_should_ReturnZeroStateGate_When_Constructed(t *testing.T) {
	store := &fakeTokenStore{}
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{}
	bus := events.NewEventBus(10)

	qg := newTestQuotaGate(config.QuotaConfig{}, store, poller, ctrl, bus)

	if qg.IsPausedByQuota() {
		t.Error("IsPausedByQuota() = true, want false on a fresh gate")
	}
}

// ---------------------------------------------------------------------------
// Reconcile — hard override, soft threshold, resume, hard-blocks-resume
// ---------------------------------------------------------------------------

func TestReconcile_should_DisableBacklogImmediately_When_HardSignalActive(t *testing.T) {
	store := &fakeTokenStore{}
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: true}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true

	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)
	qg.recordRateLimitEvent(time.Now().Add(-1 * time.Minute))

	qg.Reconcile(context.Background())

	if ctrl.disableCalls != 1 {
		t.Errorf("disableCalls = %d, want 1", ctrl.disableCalls)
	}
	if !qg.IsPausedByQuota() {
		t.Error("IsPausedByQuota() = false, want true")
	}
}

func TestReconcile_should_RequireConsecutiveTicksBeforePausingOrResuming_When_SoftThresholdCrossed(t *testing.T) {
	store := &fakeTokenStore{results: nil} // no usage data; budget below drives PctRemaining via computeHeadroom
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: true}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.ConsecutiveTicksToPause = 2
	cfg.PauseBelowHeadroomPct = 20.0
	cfg.AssumedWindowTokenBudget = 1000

	// Drive PctRemaining to 10.0 (below the 20.0 threshold): budget=1000, used=900.
	store.results = fakeResultsWithUsage(900)
	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)

	qg.Reconcile(context.Background())
	if ctrl.disableCalls != 0 {
		t.Fatalf("disableCalls after 1st below-threshold tick = %d, want 0", ctrl.disableCalls)
	}

	qg.Reconcile(context.Background())
	if ctrl.disableCalls != 1 {
		t.Fatalf("disableCalls after 2nd consecutive below-threshold tick = %d, want 1", ctrl.disableCalls)
	}
}

func TestReconcile_should_ResumeAfterConsecutiveTicksAboveMargin_When_PausedByQuota(t *testing.T) {
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: false}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.PauseBelowHeadroomPct = 20.0
	cfg.ResumeMarginPct = 15.0
	cfg.ConsecutiveTicksToResume = 3
	cfg.AssumedWindowTokenBudget = 1000

	store := &fakeTokenStore{results: fakeResultsWithUsage(0)} // 100% headroom, well above 35 threshold

	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)
	qg.state.pausedByQuota = true
	v := false
	qg.state.lastSetEnabled = &v

	qg.Reconcile(context.Background())
	qg.Reconcile(context.Background())
	if ctrl.enableCalls != 0 {
		t.Fatalf("enableCalls after 2 healthy ticks = %d, want 0 (need 3 consecutive)", ctrl.enableCalls)
	}

	qg.Reconcile(context.Background())
	if ctrl.enableCalls != 1 {
		t.Fatalf("enableCalls after 3rd consecutive healthy tick = %d, want 1", ctrl.enableCalls)
	}
}

func TestReconcile_should_NeverCallEnable_When_HardSignalStillActiveAcrossMultipleResumeEligibleTicks(t *testing.T) {
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: false}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.PauseBelowHeadroomPct = 20.0
	cfg.ResumeMarginPct = 15.0
	cfg.ConsecutiveTicksToResume = 3
	cfg.AssumedWindowTokenBudget = 1000
	cfg.RateLimitWindowMinutes = 30

	store := &fakeTokenStore{results: fakeResultsWithUsage(0)} // healthy headroom every tick

	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)
	qg.state.pausedByQuota = true
	v := false
	qg.state.lastSetEnabled = &v
	qg.recordRateLimitEvent(time.Now()) // hard signal active

	for i := 0; i < 5; i++ {
		qg.Reconcile(context.Background())
	}
	if ctrl.enableCalls != 0 {
		t.Fatalf("enableCalls while hard signal active = %d, want 0 across all 5 ticks", ctrl.enableCalls)
	}

	// Clear the hard signal (older than the window) and run a fresh consecutive run.
	qg.mu.Lock()
	qg.rateLimits.LastEventAt = time.Now().Add(-time.Hour)
	qg.mu.Unlock()

	for i := 0; i < 3; i++ {
		qg.Reconcile(context.Background())
	}
	if ctrl.enableCalls != 1 {
		t.Errorf("enableCalls after hard signal clears + fresh consecutive run = %d, want 1", ctrl.enableCalls)
	}
}

func TestReconcile_should_NeverAutoResume_When_BacklogWasManuallyDisabled(t *testing.T) {
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: false} // manually disabled, pausedByQuota stays false
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.AssumedWindowTokenBudget = 1000

	store := &fakeTokenStore{results: fakeResultsWithUsage(0)}
	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)

	for i := 0; i < 10; i++ {
		qg.Reconcile(context.Background())
	}

	if ctrl.enableCalls != 0 {
		t.Errorf("enableCalls = %d, want 0 (pausedByQuota is false — a manual disable must never be auto-resumed)", ctrl.enableCalls)
	}
}

func TestReconcile_should_RepauseAndBypassCooldown_When_ManuallyReenabledWhileQuotaStillLow(t *testing.T) {
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: false}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.PauseBelowHeadroomPct = 20.0
	cfg.ConsecutiveTicksToPause = 2
	cfg.ManualOverrideGraceMinutes = 10
	cfg.AssumedWindowTokenBudget = 1000

	store := &fakeTokenStore{results: fakeResultsWithUsage(900)} // 10% remaining, below threshold
	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)
	qg.state.pausedByQuota = true
	vFalse := false
	qg.state.lastSetEnabled = &vFalse
	qg.state.lastPauseNotifyAt = time.Now().Add(-2 * time.Minute) // inside normal 5-min cooldown

	// Simulate a manual re-enable via the toggle, bypassing QuotaGate.
	ctrl.enabled = true

	qg.Reconcile(context.Background()) // provenance detects the external change
	qg.Reconcile(context.Background()) // 2nd consecutive below-threshold tick re-pauses

	if ctrl.disableCalls == 0 {
		t.Fatal("disableCalls = 0, want at least 1 (re-pause after manual re-enable)")
	}
	if qg.state.lastPauseNotifyAt.Before(time.Now().Add(-time.Second)) {
		// lastPauseNotifyAt should have been refreshed by the bypassed-cooldown notify.
		t.Error("lastPauseNotifyAt was not refreshed — expected notifyPaused to bypass cooldown via manualOverrideAt grace window")
	}
}

// ---------------------------------------------------------------------------
// Foreground throttle
// ---------------------------------------------------------------------------

func TestForegroundSessionActive_should_ReturnTrue_When_NonBacklogSessionIsActive(t *testing.T) {
	instances := []*session.Instance{
		{Category: "", Status: session.Active},
	}
	if !foregroundSessionActive(instances) {
		t.Error("foregroundSessionActive = false, want true")
	}
}

func TestForegroundSessionActive_should_ReturnFalse_When_OnlyBacklogOwnedSessionsAreActive(t *testing.T) {
	instances := []*session.Instance{
		{Category: session.CategoryBacklog, Status: session.Active},
		{Category: session.CategoryBacklog, Status: session.Active},
		{Category: session.CategoryBacklog, Status: session.Active},
	}
	if foregroundSessionActive(instances) {
		t.Error("foregroundSessionActive = true, want false (all sessions are backlog-owned)")
	}
}

func TestShouldThrottleForeground_should_ExpireAfterDelay_When_NoForegroundActivityObservedForFullWindow(t *testing.T) {
	poller := &mockInstancePoller{instances: []*session.Instance{
		{Category: "", Status: session.Active},
	}}
	ctrl := &countingFeatureController{enabled: true}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.ForegroundThrottleDelaySeconds = 300

	store := &fakeTokenStore{}
	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)

	qg.Reconcile(context.Background())
	if !qg.ShouldThrottleForeground() {
		t.Fatal("ShouldThrottleForeground() = false immediately after observing foreground activity, want true")
	}

	// Simulate 5 more idle minutes elapsing (no foreground session) by manipulating
	// the internal deadline directly, since we can't sleep 5 real minutes in a test.
	qg.mu.Lock()
	qg.foregroundThrottleUntil = time.Now().Add(-time.Second)
	qg.mu.Unlock()

	poller.instances = nil
	qg.Reconcile(context.Background())
	if qg.ShouldThrottleForeground() {
		t.Error("ShouldThrottleForeground() = true after the delay window with no foreground activity, want false")
	}
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

func TestNotifyPaused_should_IncludeObservedAndThresholdValues_When_SoftThresholdPauseFires(t *testing.T) {
	bus := events.NewEventBus(10)
	ch, id := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(id)

	qg := newTestQuotaGate(config.QuotaConfig{}.QuotaConfigOrDefault(), &fakeTokenStore{}, &mockInstancePoller{}, &countingFeatureController{}, bus)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.PauseBelowHeadroomPct = 20.0

	qg.notifyPaused(cfg, "soft_threshold", HeadroomEstimate{PctRemaining: 15.0})

	select {
	case ev := <-ch:
		if !strings.Contains(ev.NotificationMessage, "15") || !strings.Contains(ev.NotificationMessage, "20") {
			t.Errorf("message = %q, want it to contain both observed (15) and threshold (20)", ev.NotificationMessage)
		}
		if ev.NotificationMetadata["type"] != quotaGatePausedType {
			t.Errorf("metadata[type] = %q, want %q", ev.NotificationMetadata["type"], quotaGatePausedType)
		}
		if ev.SessionID != quotaGateNotifierKey {
			t.Errorf("SessionID (itemID) = %q, want %q", ev.SessionID, quotaGateNotifierKey)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification published")
	}
}

func TestNotifyPaused_should_SuppressRepeatedNotification_When_AlreadyPausedWithinCooldownAndNoManualOverride(t *testing.T) {
	bus := events.NewEventBus(10)
	ch, id := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(id)

	qg := newTestQuotaGate(config.QuotaConfig{}.QuotaConfigOrDefault(), &fakeTokenStore{}, &mockInstancePoller{}, &countingFeatureController{}, bus)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	qg.state.lastPauseNotifyAt = time.Now().Add(-2 * time.Minute) // inside 5-min cooldown
	// manualOverrideAt left zero — no grace-window bypass.

	qg.notifyPaused(cfg, "hard_override", HeadroomEstimate{})

	select {
	case ev := <-ch:
		t.Fatalf("unexpected notification published within cooldown: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no notification.
	}
}

func TestNotifyResumed_should_NeverIncludeETAOrCountdown_When_ResumeTransitionFires(t *testing.T) {
	bus := events.NewEventBus(10)
	ch, id := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(id)

	qg := newTestQuotaGate(config.QuotaConfig{}.QuotaConfigOrDefault(), &fakeTokenStore{}, &mockInstancePoller{}, &countingFeatureController{}, bus)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()

	qg.notifyResumed(cfg, HeadroomEstimate{PctRemaining: 42.0})

	select {
	case ev := <-ch:
		if !strings.Contains(ev.NotificationMessage, "42") {
			t.Errorf("message = %q, want it to contain the recovered value (42)", ev.NotificationMessage)
		}
		if !strings.Contains(ev.NotificationMessage, "automatically") {
			t.Errorf("message = %q, want it to say resumption is automatic", ev.NotificationMessage)
		}
		for _, forbidden := range []string{"AM", "PM", "in 5 minutes", "in 10 minutes", "ETA"} {
			if strings.Contains(ev.NotificationMessage, forbidden) {
				t.Errorf("message = %q, must not contain a time-of-day/countdown phrase %q", ev.NotificationMessage, forbidden)
			}
		}
		if ev.NotificationMetadata["type"] != quotaGateResumedType {
			t.Errorf("metadata[type] = %q, want %q", ev.NotificationMetadata["type"], quotaGateResumedType)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification published")
	}
}

func TestQuotaGate_should_PublishNotificationOnRealEventBus_When_TransitionOccurs(t *testing.T) {
	bus := events.NewEventBus(10)
	ch, id := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(id)

	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: true}
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true

	qg := newTestQuotaGate(cfg, &fakeTokenStore{}, poller, ctrl, bus)
	qg.recordRateLimitEvent(time.Now())

	qg.Reconcile(context.Background())

	select {
	case ev := <-ch:
		if ev.NotificationTitle != "Backlog Automation Paused" {
			t.Errorf("title = %q, want %q", ev.NotificationTitle, "Backlog Automation Paused")
		}
	case <-time.After(time.Second):
		t.Fatal("no notification observed on the real event bus")
	}
}

// ---------------------------------------------------------------------------
// Non-goal safeguard: pause never touches session-instance-scoped state.
// ---------------------------------------------------------------------------

// TestReconcile_should_OnlyCallFeatureControllerDisable_When_PausingForQuota
// verifies the pause path calls FeatureController.Disable() exactly once.
// The broader Non-Goal ("never kill an in-flight session, only stop new
// dispatch") is a structural guarantee, not a runtime one: FeatureController
// (session_service.go) has exactly three methods — Enable, Disable,
// IsEnabled — none of them session-instance-scoped, and QuotaGate's only
// other dependencies (InstancePoller.GetInstances, tokens.TokenStoreReader)
// are read-only. A spy asserting "no session-stop method was called" would
// be vacuously true regardless of correctness, since QuotaGate has no
// reference to any such method in the first place — the type system already
// forecloses it, so this test only needs to pin the one real, checkable
// behavior: Disable() fires exactly once per pause.
func TestReconcile_should_OnlyCallFeatureControllerDisable_When_PausingForQuota(t *testing.T) {
	poller := &mockInstancePoller{instances: []*session.Instance{
		{Category: "", Status: session.Active},
	}}
	ctrl := &countingFeatureController{enabled: true}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true

	qg := newTestQuotaGate(cfg, &fakeTokenStore{}, poller, ctrl, bus)
	qg.recordRateLimitEvent(time.Now())

	qg.Reconcile(context.Background())

	if ctrl.disableCalls != 1 {
		t.Fatalf("disableCalls = %d, want exactly 1", ctrl.disableCalls)
	}
}

// TestReconcile_should_ResetHysteresisCounters_When_HardOverrideFires is the
// architecture-review BLOCKER regression guard: plan.md Task 2.1.2a requires
// the hard-override disable branch to reset both consecutiveBelow/Above
// counters, since the hard override bypasses them entirely. Without the
// reset, a soft-threshold near-miss (consecutiveBelow left nonzero by an
// interrupted run) can survive a hard-override pause and cause a premature
// re-pause on the very first tick after a later manual re-enable, instead of
// requiring a fresh ConsecutiveTicksToPause run.
func TestReconcile_should_ResetHysteresisCounters_When_HardOverrideFires(t *testing.T) {
	poller := &mockInstancePoller{}
	ctrl := &countingFeatureController{enabled: true}
	bus := events.NewEventBus(10)
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.ConsecutiveTicksToPause = 2
	cfg.PauseBelowHeadroomPct = 20.0
	cfg.AssumedWindowTokenBudget = 1000

	// Drive one below-threshold tick first, leaving a stale consecutiveBelow==1
	// (not yet enough to trigger the soft-threshold pause on its own).
	store := &fakeTokenStore{results: fakeResultsWithUsage(900)} // 10% remaining
	qg := newTestQuotaGate(cfg, store, poller, ctrl, bus)
	qg.Reconcile(context.Background())
	qg.mu.Lock()
	stale := qg.state.consecutiveBelow
	qg.mu.Unlock()
	if stale != 1 {
		t.Fatalf("test precondition failed: consecutiveBelow = %d after one below-threshold tick, want 1", stale)
	}

	// Now the hard signal fires, disabling immediately and — per the fix —
	// resetting both counters rather than leaving the stale 1 in place.
	qg.recordRateLimitEvent(time.Now())
	qg.Reconcile(context.Background())

	qg.mu.Lock()
	below, above := qg.state.consecutiveBelow, qg.state.consecutiveAbove
	qg.mu.Unlock()
	if below != 0 || above != 0 {
		t.Errorf("consecutiveBelow=%d consecutiveAbove=%d after hard-override disable, want both 0", below, above)
	}
	if ctrl.disableCalls != 1 {
		t.Fatalf("disableCalls = %d, want 1", ctrl.disableCalls)
	}
}
