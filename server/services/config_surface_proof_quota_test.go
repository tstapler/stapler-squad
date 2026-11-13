package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Proof entries (ADR-006) for the quota tuning fields. Each saves the value via
// UpdateQuotaConfig, then drives a QuotaGate wired the way server/dependencies.go
// wires it (cfgFn re-reads config.json), and observes a gate decision.

const prfQuotaBudget = 1000

type prfQuotaRun struct {
	gate  *QuotaGate
	ctrl  *svctest.CountingFeatureController
	store *svctest.FakeTokenStore
}

// prfSaveQuota applies s on top of a base that enables the gate with a budget.
func prfSaveQuota(t *testing.T, s *sessionv1.QuotaSettings) {
	t.Helper()
	s.Enabled = proto.Bool(true)
	s.AssumedWindowTokenBudget = proto.Int64(prfQuotaBudget)
	_, err := updateQuota(t, s)
	require.NoError(t, err)
}

// prfNewQuotaRun builds a gate over a backlog that starts enabled, with usedTokens
// of the budget already consumed.
func prfNewQuotaRun(usedTokens int64, poller *svctest.MockInstancePoller) *prfQuotaRun {
	store := &svctest.FakeTokenStore{Results: svctest.FakeResultsWithUsage(usedTokens)}
	ctrl := &svctest.CountingFeatureController{Enabled: true}
	gate := NewQuotaGate(
		func() config.QuotaConfig { return config.LoadConfig().Quota.QuotaConfigOrDefault() },
		store, poller, ctrl, events.NewEventBus(10))
	return &prfQuotaRun{gate: gate, ctrl: ctrl, store: store}
}

func (r *prfQuotaRun) reconcile(ticks int) {
	for i := 0; i < ticks; i++ {
		r.gate.Reconcile(context.Background())
	}
}

func TestConfigSurfaceProof_QuotaPauseBelowHeadroomPct_ChangesPauseDecision(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{PauseBelowHeadroomPct: proto.Float64(setting.(float64))})
		r := prfNewQuotaRun(600, &svctest.MockInstancePoller{}) // 40% headroom remaining
		r.reconcile(2)                                          // default ticks-to-pause is 2
		return r.ctrl.DisableCalls
	}, 30.0, 70.0)
}

func TestConfigSurfaceProof_QuotaConsecutiveTicksToPause_ChangesPauseTiming(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{ConsecutiveTicksToPause: proto.Int32(int32(setting.(int)))})
		r := prfNewQuotaRun(900, &svctest.MockInstancePoller{}) // 10% remaining, below the 20% default
		r.reconcile(1)
		return r.ctrl.DisableCalls
	}, 1, 3)
}

func TestConfigSurfaceProof_QuotaConsecutiveTicksToResume_ChangesResumeTiming(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{
			ConsecutiveTicksToPause:  proto.Int32(1),
			ConsecutiveTicksToResume: proto.Int32(int32(setting.(int))),
		})
		r := prfNewQuotaRun(900, &svctest.MockInstancePoller{})
		r.reconcile(1) // pauses
		require.Equal(t, 1, r.ctrl.DisableCalls)
		r.store.Results = svctest.FakeResultsWithUsage(0) // full headroom
		r.reconcile(1)
		return r.ctrl.EnableCalls
	}, 1, 3)
}

func TestConfigSurfaceProof_QuotaResumeMarginPct_ChangesResumeDecision(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{
			PauseBelowHeadroomPct:    proto.Float64(20),
			ResumeMarginPct:          proto.Float64(setting.(float64)),
			ConsecutiveTicksToPause:  proto.Int32(1),
			ConsecutiveTicksToResume: proto.Int32(1),
		})
		r := prfNewQuotaRun(900, &svctest.MockInstancePoller{})
		r.reconcile(1)                                      // pauses
		r.store.Results = svctest.FakeResultsWithUsage(600) // 40% remaining: above 20+15, below 20+30
		r.reconcile(1)
		return r.ctrl.EnableCalls
	}, 15.0, 30.0)
}

func TestConfigSurfaceProof_QuotaRateLimitWindowMinutes_ChangesHardOverride(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{RateLimitWindowMinutes: proto.Int32(int32(setting.(int)))})
		r := prfNewQuotaRun(0, &svctest.MockInstancePoller{})
		r.gate.recordRateLimitEvent(time.Now().Add(-10 * time.Minute))
		r.reconcile(1)
		return r.ctrl.DisableCalls
	}, 5, 30)
}

func TestConfigSurfaceProof_QuotaForegroundThrottleDelaySeconds_ChangesThrottleDeadline(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{ForegroundThrottleDelaySeconds: proto.Int32(int32(setting.(int)))})
		poller := &svctest.MockInstancePoller{Instances: []*session.Instance{{Category: "", Status: domain.Active}}}
		r := prfNewQuotaRun(0, poller)
		r.reconcile(1)
		r.gate.mu.Lock()
		defer r.gate.mu.Unlock()
		return time.Until(r.gate.foregroundThrottleUntil) > 10*time.Minute // 60s vs 3600s: far from the cutoff
	}, 60, 3600)
}

func TestConfigSurfaceProof_QuotaManualOverrideGraceMinutes_ChangesNotifyCooldownBypass(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		prfSaveQuota(t, &sessionv1.QuotaSettings{
			ManualOverrideGraceMinutes: proto.Int32(int32(setting.(int))),
			ConsecutiveTicksToPause:    proto.Int32(1),
		})
		r := prfNewQuotaRun(900, &svctest.MockInstancePoller{})
		now := time.Now()
		// A manual override 5 minutes ago and a pause notice 2 minutes ago (inside the 5-minute cooldown).
		v := true
		r.gate.state.lastSetEnabled = &v
		r.gate.state.manualOverrideAt = now.Add(-5 * time.Minute)
		r.gate.state.lastPauseNotifyAt = now.Add(-2 * time.Minute)
		r.reconcile(1)
		require.Equal(t, 1, r.ctrl.DisableCalls)
		return r.gate.state.lastPauseNotifyAt.After(now.Add(-time.Minute)) // true only if the cooldown was bypassed
	}, 3, 10)
}
