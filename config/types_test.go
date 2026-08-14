package config

import "testing"

func TestQuotaConfigOrDefault_should_FillAllDefaults_When_ZeroValueStruct(t *testing.T) {
	cfg := QuotaConfig{}

	out := cfg.QuotaConfigOrDefault()

	if out.Enabled != false {
		t.Errorf("Enabled = %v, want false", out.Enabled)
	}
	if out.PauseBelowHeadroomPct != 20.0 {
		t.Errorf("PauseBelowHeadroomPct = %v, want 20.0", out.PauseBelowHeadroomPct)
	}
	if out.ResumeMarginPct != 15.0 {
		t.Errorf("ResumeMarginPct = %v, want 15.0", out.ResumeMarginPct)
	}
	if out.ConsecutiveTicksToPause != 2 {
		t.Errorf("ConsecutiveTicksToPause = %v, want 2", out.ConsecutiveTicksToPause)
	}
	if out.ConsecutiveTicksToResume != 3 {
		t.Errorf("ConsecutiveTicksToResume = %v, want 3", out.ConsecutiveTicksToResume)
	}
	if out.AssumedWindowTokenBudget != 0 {
		t.Errorf("AssumedWindowTokenBudget = %v, want 0", out.AssumedWindowTokenBudget)
	}
	if out.RateLimitWindowMinutes != 30 {
		t.Errorf("RateLimitWindowMinutes = %v, want 30", out.RateLimitWindowMinutes)
	}
	if out.ManualOverrideGraceMinutes != 10 {
		t.Errorf("ManualOverrideGraceMinutes = %v, want 10", out.ManualOverrideGraceMinutes)
	}
	if out.ForegroundThrottleDelaySeconds != 300 {
		t.Errorf("ForegroundThrottleDelaySeconds = %v, want 300", out.ForegroundThrottleDelaySeconds)
	}
}

func TestResyncFastLaneSlotsOrDefault_should_ReturnFour_When_Unset(t *testing.T) {
	cfg := TmuxExecGateConfig{}

	out := cfg.ResyncFastLaneSlotsOrDefault()

	if out != 4 {
		t.Errorf("ResyncFastLaneSlotsOrDefault() = %v, want 4", out)
	}
}

func TestResyncFastLaneSlotsOrDefault_should_PreserveExplicitValue_When_FieldAlreadySet(t *testing.T) {
	cfg := TmuxExecGateConfig{ResyncFastLaneSlots: 12}

	out := cfg.ResyncFastLaneSlotsOrDefault()

	if out != 12 {
		t.Errorf("ResyncFastLaneSlotsOrDefault() = %v, want 12 (explicit value must survive defaulting)", out)
	}
}

func TestQuotaConfigOrDefault_should_PreserveExplicitValue_When_FieldAlreadySet(t *testing.T) {
	cfg := QuotaConfig{PauseBelowHeadroomPct: 35.0}

	out := cfg.QuotaConfigOrDefault()

	if out.PauseBelowHeadroomPct != 35.0 {
		t.Errorf("PauseBelowHeadroomPct = %v, want 35.0 (explicit value must survive defaulting)", out.PauseBelowHeadroomPct)
	}
	if out.ResumeMarginPct != 15.0 {
		t.Errorf("ResumeMarginPct = %v, want 15.0 (default)", out.ResumeMarginPct)
	}
	if out.ConsecutiveTicksToPause != 2 {
		t.Errorf("ConsecutiveTicksToPause = %v, want 2 (default)", out.ConsecutiveTicksToPause)
	}
}
