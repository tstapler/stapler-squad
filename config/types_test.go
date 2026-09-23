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

func TestThresholdMinutesOrDefault_should_ReturnThirty_When_ZeroValueStruct(t *testing.T) {
	cfg := StaleSessionConfig{}

	out := cfg.ThresholdMinutesOrDefault()

	if out != 30 {
		t.Errorf("ThresholdMinutesOrDefault() = %v, want 30", out)
	}
}

func TestNotifyEnabledOrDefault_should_ReturnTrue_When_ZeroValueStruct(t *testing.T) {
	cfg := StaleSessionConfig{}

	out := cfg.NotifyEnabledOrDefault()

	if out != true {
		t.Errorf("NotifyEnabledOrDefault() = %v, want true", out)
	}
}

func TestThresholdMinutesOrDefault_should_ReturnThirty_When_NegativeValue(t *testing.T) {
	cfg := StaleSessionConfig{ThresholdMinutes: -5}

	out := cfg.ThresholdMinutesOrDefault()

	if out != 30 {
		t.Errorf("ThresholdMinutesOrDefault() = %v, want 30 (negative value must never resolve to itself or to 0, which would make every session immediately stale)", out)
	}
}

func TestThresholdMinutesOrDefault_should_PreserveExplicitValue_When_Positive(t *testing.T) {
	cfg := StaleSessionConfig{ThresholdMinutes: 45}

	out := cfg.ThresholdMinutesOrDefault()

	if out != 45 {
		t.Errorf("ThresholdMinutesOrDefault() = %v, want 45 (explicit value must survive defaulting)", out)
	}
}

func TestNotifyEnabledOrDefault_should_ReturnFalse_When_ExplicitlyDisabled(t *testing.T) {
	disabled := false
	cfg := StaleSessionConfig{NotifyEnabled: &disabled}

	out := cfg.NotifyEnabledOrDefault()

	if out != false {
		t.Errorf("NotifyEnabledOrDefault() = %v, want false", out)
	}
}

func TestRetryPolicyConfig_EnabledOrDefault_should_ReturnTrue_When_FieldIsNil(t *testing.T) {
	cfg := RetryPolicyConfig{}
	if !cfg.EnabledOrDefault() {
		t.Error("EnabledOrDefault() = false, want true when Enabled is nil (AC7)")
	}
}

func TestRetryPolicyConfig_EnabledOrDefault_should_ReturnFalse_When_ExplicitlyDisabled(t *testing.T) {
	disabled := false
	cfg := RetryPolicyConfig{Enabled: &disabled}
	if cfg.EnabledOrDefault() {
		t.Error("EnabledOrDefault() = true, want false when explicitly disabled")
	}
}

func TestRetryPolicyConfig_MaxAttemptsOrDefault_should_ReturnOne_When_MaxAttemptsIsZeroOrNegative(t *testing.T) {
	cases := []int{0, -1, -100}
	for _, v := range cases {
		cfg := RetryPolicyConfig{MaxAttempts: v}
		if got := cfg.MaxAttemptsOrDefault(); got != 1 {
			t.Errorf("MaxAttemptsOrDefault() with MaxAttempts=%d = %d, want 1 (a fat-fingered zero/negative must not silently disable retry)", v, got)
		}
	}
}

func TestRetryPolicyConfig_MaxAttemptsOrDefault_should_PreserveExplicitValue_When_Positive(t *testing.T) {
	cfg := RetryPolicyConfig{MaxAttempts: 5}
	if got := cfg.MaxAttemptsOrDefault(); got != 5 {
		t.Errorf("MaxAttemptsOrDefault() = %d, want 5", got)
	}
}

func TestRetryPolicyConfig_RetryOnOrDefault_should_ReturnAllThree_When_Empty(t *testing.T) {
	cfg := RetryPolicyConfig{}
	got := cfg.RetryOnOrDefault()
	if len(got) != 3 {
		t.Errorf("RetryOnOrDefault() = %v, want all 3 known reasons", got)
	}
}

func TestRetryPolicyConfig_RetryOnOrDefault_should_DropUnknownEntries_When_ConfigHasATypo(t *testing.T) {
	cfg := RetryPolicyConfig{RetryOn: []string{"crashed", "crashd"}}
	got := cfg.RetryOnOrDefault()
	if len(got) != 1 || got[0] != "crashed" {
		t.Errorf("RetryOnOrDefault() = %v, want [crashed] (typo'd entry dropped, not silently kept or falling back to all three)", got)
	}
}

func TestRetryPolicyConfig_StaleTriggersRetry_should_DefaultToNilFalse_When_Unset(t *testing.T) {
	cfg := RetryPolicyConfig{}
	if cfg.StaleTriggersRetry != nil {
		t.Error("StaleTriggersRetry should be nil (opt-in flag stays off until the sibling project's consumer wiring lands)")
	}
}

func TestRetryPolicyConfig_BackoffOrWarn_should_FallBackToExponential_When_ValueIsUnknown(t *testing.T) {
	cfg := RetryPolicyConfig{Backoff: "linear"}
	if got := cfg.BackoffOrWarn(); got != "exponential" {
		t.Errorf("BackoffOrWarn() = %q, want exponential", got)
	}
}

func TestRetryPolicyConfig_BackoffOrWarn_should_ReturnExponential_When_Unset(t *testing.T) {
	cfg := RetryPolicyConfig{}
	if got := cfg.BackoffOrWarn(); got != "exponential" {
		t.Errorf("BackoffOrWarn() = %q, want exponential", got)
	}
}

func TestCreationStaleConfig_ThresholdMinutesOrDefault_should_Return10_When_Unset(t *testing.T) {
	cfg := CreationStaleConfig{}

	out := cfg.ThresholdMinutesOrDefault()

	if out != 10 {
		t.Errorf("ThresholdMinutesOrDefault() = %v, want 10", out)
	}
}

func TestCreationStaleConfig_ThresholdMinutesOrDefault_should_ReturnConfiguredValue_When_Set(t *testing.T) {
	cfg := CreationStaleConfig{ThresholdMinutes: 20}

	out := cfg.ThresholdMinutesOrDefault()

	if out != 20 {
		t.Errorf("ThresholdMinutesOrDefault() = %v, want 20 (explicit value must survive defaulting)", out)
	}
}
