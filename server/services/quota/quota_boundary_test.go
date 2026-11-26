package quota

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestQuotaGateReconcile_should_NotPause_When_HeadroomEqualsThreshold(t *testing.T) {
	t.Parallel()
	cfg := goldenCfg(1000)
	cfg.PauseBelowHeadroomPct, cfg.ConsecutiveTicksToPause = 25, 1 // 750/1000 used is exactly 25% remaining
	ctrl := &svctest.CountingFeatureController{Enabled: true}
	g := newTestQuotaGate(cfg, &svctest.FakeTokenStore{Results: svctest.FakeResultsWithUsage(750)},
		&svctest.MockInstancePoller{}, ctrl, events.NewEventBus(1))

	g.Reconcile(context.Background())

	assert.Zero(t, ctrl.DisableCalls)
}

func TestStatusDetail_should_BeEmpty_When_GateDisabledWithZeroBudget(t *testing.T) {
	t.Parallel()
	cfg := config.QuotaConfig{}.QuotaConfigOrDefault()
	g := newTestQuotaGate(cfg, &svctest.FakeTokenStore{}, &svctest.MockInstancePoller{},
		&svctest.CountingFeatureController{}, events.NewEventBus(1))

	assert.Empty(t, g.StatusDetail())
}

// Not parallel: captureQuotaSlog swaps the process-global logger.
func TestQuotaGateReconcile_should_LogThrottleCleared_When_DelayNoLongerExtendsTheDeadline(t *testing.T) {
	buf := captureQuotaSlog(t)
	cfg := goldenCfg(0)
	cfg.ForegroundThrottleDelaySeconds = 0
	g := newTestQuotaGate(cfg, &svctest.FakeTokenStore{},
		&svctest.MockInstancePoller{Instances: []*instance.Instance{{Status: domain.Active}}},
		&svctest.CountingFeatureController{Enabled: true}, events.NewEventBus(1))
	g.foregroundThrottleUntil = time.Now().Add(time.Hour)

	g.Reconcile(context.Background())

	assert.Contains(t, buf.String(), "QuotaGate: foreground throttle cleared")
	assert.False(t, g.ShouldThrottleForeground())
}
