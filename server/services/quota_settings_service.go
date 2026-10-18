package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

var _ sessionv1connect.QuotaServiceHandler = (*QuotaSettingsService)(nil)

const noBudgetWarning = "Gate is on but no budget is set, so the proactive signal never fires"

// QuotaSettingsService reads and edits the quota gate settings in config.json.
// The gate re-reads config.json on every reconcile, so saves apply without restart.
type QuotaSettingsService struct{}

// NewQuotaSettingsService creates the service; it holds no state.
func NewQuotaSettingsService() *QuotaSettingsService { return &QuotaSettingsService{} }

// +api: GetQuotaConfig
func (s *QuotaSettingsService) GetQuotaConfig(
	_ context.Context, _ *connect.Request[sessionv1.GetQuotaConfigRequest],
) (*connect.Response[sessionv1.GetQuotaConfigResponse], error) {
	v := quotaView(config.LoadConfig())
	return connect.NewResponse(&sessionv1.GetQuotaConfigResponse{
		Settings: v.settings, Mode: v.mode, Warnings: v.warnings, QuotaSource: v.source,
	}), nil
}

// +api: UpdateQuotaConfig
func (s *QuotaSettingsService) UpdateQuotaConfig(
	_ context.Context, req *connect.Request[sessionv1.UpdateQuotaConfigRequest],
) (*connect.Response[sessionv1.UpdateQuotaConfigResponse], error) {
	in := req.Msg.GetSettings()
	err := config.UpdateConfig(func(cfg *config.Config) error {
		merged := mergeQuotaSettings(cfg.Quota, in)
		if err := validateQuotaConfig(merged); err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
		now := time.Now()
		cfg.Quota = merged
		cfg.QuotaConfiguredAt = &now
		return nil
	})
	if err != nil {
		var ce *connect.Error
		if errors.As(err, &ce) {
			return nil, err
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save quota settings: %w", err))
	}

	// Read back what was stored rather than echoing the request.
	stored := config.LoadConfig()
	v := quotaView(stored)
	LogQuotaEffectiveConfig("quota settings saved", stored.Quota)
	return connect.NewResponse(&sessionv1.UpdateQuotaConfigResponse{
		Settings: v.settings, Mode: v.mode, Warnings: v.warnings, QuotaSource: v.source,
	}), nil
}

// mergeQuotaSettings overlays the supplied fields on the stored (defaults-applied)
// config. The USD default is authoritative: absent in the request means off.
func mergeQuotaSettings(stored config.QuotaConfig, in *sessionv1.QuotaSettings) config.QuotaConfig {
	out := stored.QuotaConfigOrDefault()
	overlay(&out.Enabled, in.Enabled)
	overlay(&out.PauseBelowHeadroomPct, in.PauseBelowHeadroomPct)
	overlay(&out.ResumeMarginPct, in.ResumeMarginPct)
	overlay(&out.AssumedWindowTokenBudget, in.AssumedWindowTokenBudget)
	overlayInt(&out.ConsecutiveTicksToPause, in.ConsecutiveTicksToPause)
	overlayInt(&out.ConsecutiveTicksToResume, in.ConsecutiveTicksToResume)
	overlayInt(&out.RateLimitWindowMinutes, in.RateLimitWindowMinutes)
	overlayInt(&out.ManualOverrideGraceMinutes, in.ManualOverrideGraceMinutes)
	overlayInt(&out.ForegroundThrottleDelaySeconds, in.ForegroundThrottleDelaySeconds)
	out.DefaultCostBudgetThresholdUSD = nil
	if in.DefaultCostBudgetThresholdUsd != nil {
		v := in.GetDefaultCostBudgetThresholdUsd()
		out.DefaultCostBudgetThresholdUSD = &v
	}
	return out
}

func overlay[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func overlayInt(dst *int, src *int32) {
	if src != nil {
		*dst = int(*src)
	}
}

// validateQuotaConfig rejects values that QuotaConfigOrDefault would silently
// replace (zero or negative) and pairs that make resume unreachable.
func validateQuotaConfig(q config.QuotaConfig) error {
	if err := validateQuotaPercentages(q); err != nil {
		return err
	}
	if err := validateQuotaCounts(q); err != nil {
		return err
	}
	if q.PauseBelowHeadroomPct+q.ResumeMarginPct > 100 {
		return errors.New("pause_below_headroom_pct + resume_margin_pct must be at most 100")
	}
	if q.AssumedWindowTokenBudget < 0 {
		return fmt.Errorf("assumed_window_token_budget must not be negative (got %d); use 0 for reactive-only", q.AssumedWindowTokenBudget)
	}
	if u := q.DefaultCostBudgetThresholdUSD; u != nil && (math.IsNaN(*u) || math.IsInf(*u, 0) || *u < 0) {
		return fmt.Errorf("default_cost_budget_threshold_usd must be a finite number of at least 0 (got %v)", *u)
	}
	return nil
}

func validateQuotaPercentages(q config.QuotaConfig) error {
	pct := []struct {
		name         string
		value        float64
		defaultValue int
	}{
		{"pause_below_headroom_pct", q.PauseBelowHeadroomPct, 20},
		{"resume_margin_pct", q.ResumeMarginPct, 15},
	}
	for _, p := range pct {
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) || p.value <= 0 || p.value > 100 {
			return fmt.Errorf("%s must be between 1 and 100 (got %v); zero would be silently replaced by the default %d",
				p.name, p.value, p.defaultValue)
		}
	}
	return nil
}

func validateQuotaCounts(q config.QuotaConfig) error {
	ints := []struct {
		name         string
		value        int
		defaultValue int
	}{
		{"consecutive_ticks_to_pause", q.ConsecutiveTicksToPause, 2},
		{"consecutive_ticks_to_resume", q.ConsecutiveTicksToResume, 3},
		{"rate_limit_window_minutes", q.RateLimitWindowMinutes, 30},
		{"manual_override_grace_minutes", q.ManualOverrideGraceMinutes, 10},
		{"foreground_throttle_delay_seconds", q.ForegroundThrottleDelaySeconds, 300},
	}
	for _, i := range ints {
		if i.value <= 0 {
			return fmt.Errorf("%s must be greater than zero (got %d); zero would be silently replaced by the default %d",
				i.name, i.value, i.defaultValue)
		}
	}
	return nil
}

type quotaViewData struct {
	settings *sessionv1.QuotaSettings
	mode     sessionv1.QuotaMode
	warnings []string
	source   sessionv1.QuotaSource
}

func quotaView(cfg *config.Config) quotaViewData {
	q := cfg.Quota.QuotaConfigOrDefault()
	v := quotaViewData{
		settings: &sessionv1.QuotaSettings{
			Enabled:                        proto.Bool(q.Enabled),
			PauseBelowHeadroomPct:          proto.Float64(q.PauseBelowHeadroomPct),
			ResumeMarginPct:                proto.Float64(q.ResumeMarginPct),
			ConsecutiveTicksToPause:        proto.Int32(int32(q.ConsecutiveTicksToPause)),
			ConsecutiveTicksToResume:       proto.Int32(int32(q.ConsecutiveTicksToResume)),
			AssumedWindowTokenBudget:       proto.Int64(q.AssumedWindowTokenBudget),
			RateLimitWindowMinutes:         proto.Int32(int32(q.RateLimitWindowMinutes)),
			ManualOverrideGraceMinutes:     proto.Int32(int32(q.ManualOverrideGraceMinutes)),
			ForegroundThrottleDelaySeconds: proto.Int32(int32(q.ForegroundThrottleDelaySeconds)),
		},
		mode:   quotaModeProto(q.Mode()),
		source: sessionv1.QuotaSource_QUOTA_SOURCE_NOT_CONFIGURED,
	}
	if q.DefaultCostBudgetThresholdUSD != nil {
		v.settings.DefaultCostBudgetThresholdUsd = proto.Float64(*q.DefaultCostBudgetThresholdUSD)
	}
	if q.Mode() == config.QuotaModeReactiveOnly {
		v.warnings = append(v.warnings, noBudgetWarning)
	}
	if cfg.QuotaIsConfigured() {
		v.source = sessionv1.QuotaSource_QUOTA_SOURCE_CONFIGURED
	}
	return v
}

func quotaModeProto(m config.QuotaMode) sessionv1.QuotaMode {
	switch m {
	case config.QuotaModeProactive:
		return sessionv1.QuotaMode_QUOTA_MODE_PROACTIVE
	case config.QuotaModeReactiveOnly:
		return sessionv1.QuotaMode_QUOTA_MODE_REACTIVE_ONLY
	case config.QuotaModeDisabled:
		return sessionv1.QuotaMode_QUOTA_MODE_DISABLED
	default:
		return sessionv1.QuotaMode_QUOTA_MODE_UNSPECIFIED
	}
}
