package services

import (
	"context"
	"math"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func updateQuota(t testing.TB, s *sessionv1.QuotaSettings) (*sessionv1.UpdateQuotaConfigResponse, error) {
	t.Helper()
	resp, err := NewQuotaSettingsService().UpdateQuotaConfig(context.Background(),
		connect.NewRequest(&sessionv1.UpdateQuotaConfigRequest{Settings: s}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func getQuota(t testing.TB) *sessionv1.GetQuotaConfigResponse {
	t.Helper()
	resp, err := NewQuotaSettingsService().GetQuotaConfig(context.Background(),
		connect.NewRequest(&sessionv1.GetQuotaConfigRequest{}))
	require.NoError(t, err)
	return resp.Msg
}

func TestUpdateQuotaConfig_PersistsAndReportsProactive_WhenEnabledWithBudget(t *testing.T) {
	withIsolatedStateDir(t)

	resp, err := updateQuota(t, &sessionv1.QuotaSettings{
		Enabled:                  proto.Bool(true),
		AssumedWindowTokenBudget: proto.Int64(5_000_000),
	})
	require.NoError(t, err)
	assert.Equal(t, sessionv1.QuotaMode_QUOTA_MODE_PROACTIVE, resp.GetMode())
	assert.Empty(t, resp.GetWarnings())

	got := getQuota(t)
	assert.Equal(t, sessionv1.QuotaMode_QUOTA_MODE_PROACTIVE, got.GetMode())
	assert.True(t, got.GetSettings().GetEnabled())
	assert.Equal(t, int64(5_000_000), got.GetSettings().GetAssumedWindowTokenBudget())
	assert.InDelta(t, 20.0, got.GetSettings().GetPauseBelowHeadroomPct(), 0, "untouched fields keep their effective default")
	assert.InDelta(t, 15.0, got.GetSettings().GetResumeMarginPct(), 0)

	// The gate's config closure (dependencies.go) re-reads config.json on every tick.
	gateCfg := config.LoadConfig().Quota.QuotaConfigOrDefault()
	assert.True(t, gateCfg.Enabled)
	assert.Equal(t, int64(5_000_000), gateCfg.AssumedWindowTokenBudget)
}

func TestUpdateQuotaConfig_RejectsInvalidInput_WhenPauseResumeSumOver100OrPauseZeroOrBadUsd(t *testing.T) {
	withIsolatedStateDir(t)
	before := config.LoadConfig()

	tests := []struct {
		name string
		in   *sessionv1.QuotaSettings
		want string
	}{
		{"sum over 100", &sessionv1.QuotaSettings{PauseBelowHeadroomPct: proto.Float64(60), ResumeMarginPct: proto.Float64(50)},
			"pause_below_headroom_pct + resume_margin_pct must be at most 100"},
		{"pause zero", &sessionv1.QuotaSettings{PauseBelowHeadroomPct: proto.Float64(0)},
			"silently replaced by the default 20"},
		{"resume zero", &sessionv1.QuotaSettings{ResumeMarginPct: proto.Float64(0)},
			"silently replaced by the default 15"},
		{"pause NaN", &sessionv1.QuotaSettings{PauseBelowHeadroomPct: proto.Float64(math.NaN())}, "pause_below_headroom_pct"},
		{"ticks zero", &sessionv1.QuotaSettings{ConsecutiveTicksToPause: proto.Int32(0)}, "consecutive_ticks_to_pause"},
		{"negative budget", &sessionv1.QuotaSettings{AssumedWindowTokenBudget: proto.Int64(-1)}, "assumed_window_token_budget"},
		{"usd negative", &sessionv1.QuotaSettings{DefaultCostBudgetThresholdUsd: proto.Float64(-1)}, "default_cost_budget_threshold_usd"},
		{"usd NaN", &sessionv1.QuotaSettings{DefaultCostBudgetThresholdUsd: proto.Float64(math.NaN())}, "default_cost_budget_threshold_usd"},
		{"usd +Inf", &sessionv1.QuotaSettings{DefaultCostBudgetThresholdUsd: proto.Float64(math.Inf(1))}, "default_cost_budget_threshold_usd"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := updateQuota(t, tc.in)
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Contains(t, err.Error(), tc.want)
			after := config.LoadConfig()
			assert.Equal(t, before.Quota, after.Quota, "nothing may be saved on rejection")
			assert.Nil(t, after.QuotaConfiguredAt)
		})
	}

	t.Run("zero usd accepted and absent stores nil", func(t *testing.T) {
		_, err := updateQuota(t, &sessionv1.QuotaSettings{DefaultCostBudgetThresholdUsd: proto.Float64(0)})
		require.NoError(t, err)
		stored := config.LoadConfig().Quota.DefaultCostBudgetThresholdUSD
		require.NotNil(t, stored)
		assert.Zero(t, *stored)
		assert.NotNil(t, getQuota(t).GetSettings().DefaultCostBudgetThresholdUsd, "0.0 is a valid threshold, not off")

		_, err = updateQuota(t, &sessionv1.QuotaSettings{Enabled: proto.Bool(false)})
		require.NoError(t, err)
		assert.Nil(t, config.LoadConfig().Quota.DefaultCostBudgetThresholdUSD)
		assert.Nil(t, getQuota(t).GetSettings().DefaultCostBudgetThresholdUsd)
	})
}

func TestGetQuotaConfig_ReturnsReactiveOnlyWithWarning_WhenEnabledAndBudgetZero(t *testing.T) {
	withIsolatedStateDir(t)

	_, err := updateQuota(t, &sessionv1.QuotaSettings{Enabled: proto.Bool(true)})
	require.NoError(t, err)

	got := getQuota(t)
	assert.Equal(t, sessionv1.QuotaMode_QUOTA_MODE_REACTIVE_ONLY, got.GetMode())
	require.Len(t, got.GetWarnings(), 1)
	assert.Contains(t, got.GetWarnings()[0], "no budget is set, so the proactive signal never fires")

	_, err = updateQuota(t, &sessionv1.QuotaSettings{Enabled: proto.Bool(false)})
	require.NoError(t, err)
	got = getQuota(t)
	assert.Equal(t, sessionv1.QuotaMode_QUOTA_MODE_DISABLED, got.GetMode())
	assert.Empty(t, got.GetWarnings())
}

func TestGetQuotaConfig_ReportsQuotaSource_NotConfiguredForZeroStoredValueAndConfiguredForDefaultEqualPageSave(t *testing.T) {
	withIsolatedStateDir(t)

	// A fresh config.json holds the zero quota object, which LoadConfig turns into defaults.
	got := getQuota(t)
	assert.Equal(t, sessionv1.QuotaSource_QUOTA_SOURCE_NOT_CONFIGURED, got.GetQuotaSource())
	assert.InDelta(t, 20.0, got.GetSettings().GetPauseBelowHeadroomPct(), 0, "effective values are the defaults")

	// A page save of default-equal values is a deliberate configuration.
	resp, err := updateQuota(t, &sessionv1.QuotaSettings{
		Enabled:                  proto.Bool(false),
		AssumedWindowTokenBudget: proto.Int64(0),
		PauseBelowHeadroomPct:    proto.Float64(20),
		ResumeMarginPct:          proto.Float64(15),
	})
	require.NoError(t, err)
	assert.Equal(t, sessionv1.QuotaSource_QUOTA_SOURCE_CONFIGURED, resp.GetQuotaSource())
	assert.Equal(t, sessionv1.QuotaSource_QUOTA_SOURCE_CONFIGURED, getQuota(t).GetQuotaSource())
}

func TestGetQuotaConfig_ReportsConfigured_WhenStoredValuesDifferFromDefaultsWithoutAPageSave(t *testing.T) {
	withIsolatedStateDir(t)
	require.NoError(t, config.UpdateConfig(func(c *config.Config) error {
		c.Quota.Enabled = true // a hand edit of config.json
		return nil
	}))
	assert.Equal(t, sessionv1.QuotaSource_QUOTA_SOURCE_CONFIGURED, getQuota(t).GetQuotaSource())
}

// Proof entries (ADR-006): each flips the setting through UpdateQuotaConfig and
// asserts the observable the gate derives from it differs.
func quotaModeAfterUpdate(t *testing.T, settings *sessionv1.QuotaSettings) any {
	t.Helper()
	_, err := updateQuota(t, settings)
	require.NoError(t, err)
	return getQuota(t).GetMode()
}

func TestConfigSurfaceProof_QuotaEnabled_ChangesQuotaMode(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		return quotaModeAfterUpdate(t, &sessionv1.QuotaSettings{
			Enabled: proto.Bool(setting.(bool)), AssumedWindowTokenBudget: proto.Int64(5_000_000),
		})
	}, false, true)
}

func TestConfigSurfaceProof_QuotaAssumedWindowTokenBudget_ChangesQuotaMode(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		return quotaModeAfterUpdate(t, &sessionv1.QuotaSettings{
			Enabled: proto.Bool(true), AssumedWindowTokenBudget: proto.Int64(setting.(int64)),
		})
	}, int64(0), int64(5_000_000))
}
