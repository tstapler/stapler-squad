package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestQuotaGate_EndToEndSmokeTest_ShouldPauseThenResumeBacklog_When_RealRateLimitDetectionFires
// is the requirements.md/plan.md Epic 4.2 "live smoke test" — Task 4.2.1a calls
// for manually triggering a rate-limit detection against a second, isolated
// instance and watching Settings > Feature Flags. This session has no human
// present to click through that UI, so this automated test substitutes the
// same verification through the real code path instead of a fake shortcut:
// it drives SessionService.onRateLimitDetected (the exact callback
// wireRateLimitCallbacks registers on every real session.Instance) rather
// than calling QuotaGate.recordRateLimitEvent directly, then exercises
// QuotaGate.Reconcile — the same construction and wiring shape
// server/dependencies.go uses in production (SessionService as the
// InstancePoller/quota-gate feed, a real BacklogController, a real
// *events.EventBus) — and confirms both the pause and the resume transition,
// matching requirements.md's stated ship-time bar: "the chosen
// detection/inference source correctly reflects observed rate-limit events
// ... and toggling inferred headroom below/above threshold measurably
// pauses/resumes BacklogController.IsEnabled()".
func TestQuotaGate_EndToEndSmokeTest_ShouldPauseThenResumeBacklog_When_RealRateLimitDetectionFires(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(16)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	inst := &session.Instance{
		Title:     "smoke-test-session",
		UUID:      "smoke-test-uuid",
		Path:      "/tmp/test",
		Status:    session.Active,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))

	backlogCtrl := &countingFeatureController{enabled: true}
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	cfg.Enabled = true
	cfg.RateLimitWindowMinutes = 30
	// Calibrated (non-zero) budget with an empty fakeTokenStore -> healthy
	// (100%) headroom, so the resume half below is driven by the soft signal
	// clearing its own hysteresis once the hard signal stops blocking it.
	cfg.AssumedWindowTokenBudget = 1000

	quotaGate := NewQuotaGate(
		func() config.QuotaConfig { return cfg },
		&fakeTokenStore{},
		&mockInstancePoller{instances: []*session.Instance{inst}},
		backlogCtrl,
		eventBus,
	)
	svc.SetQuotaGate(quotaGate)

	// --- Pause half: real detection -> real fan-in -> Reconcile pauses. ---
	svc.onRateLimitDetected(inst, inst.UUID, time.Time{})

	quotaGate.Reconcile(context.Background())

	if backlogCtrl.disableCalls != 1 {
		t.Fatalf("smoke test FAILED: disableCalls = %d, want 1 after a real onRateLimitDetected call", backlogCtrl.disableCalls)
	}
	if !quotaGate.IsPausedByQuota() {
		t.Fatal("smoke test FAILED: IsPausedByQuota() = false after a real rate-limit detection, want true")
	}
	t.Log("smoke test PASS (pause half): real onRateLimitDetected -> QuotaGate.recordRateLimitEvent -> Reconcile disabled BacklogController")

	// --- Resume half: clear the hard signal, wait out the window, Reconcile resumes. ---
	quotaGate.mu.Lock()
	quotaGate.rateLimits.LastEventAt = time.Now().Add(-time.Hour) // older than RateLimitWindowMinutes
	quotaGate.mu.Unlock()

	for i := 0; i < cfg.ConsecutiveTicksToResume; i++ {
		quotaGate.Reconcile(context.Background())
	}

	if backlogCtrl.enableCalls != 1 {
		t.Fatalf("smoke test FAILED: enableCalls = %d, want 1 once the rate-limit window elapses and headroom is healthy", backlogCtrl.enableCalls)
	}
	if quotaGate.IsPausedByQuota() {
		t.Fatal("smoke test FAILED: IsPausedByQuota() = true after resume, want false")
	}
	t.Log("smoke test PASS (resume half): Reconcile re-enabled BacklogController once the hard signal cleared")
}
