package config

import (
	"path/filepath"
	"testing"
	"time"
)

// TestEffectiveStreamHubEnabled covers the single source of truth that
// replaced two independently-derived (and quietly diverged) copies of this
// logic in server/services.useStreamHub and session.effectiveStreamHubFlag:
// the "stream_hub" feature flag defaults to on, and an explicit value
// (set via SetStreamHubGlobalOverride, i.e. the settings panel) wins either
// direction.
func TestEffectiveStreamHubEnabled_should_DefaultOn_When_FlagUnset(t *testing.T) {
	if got := EffectiveStreamHubEnabled(&Config{}); got != true {
		t.Fatalf("expected default-on, got %v", got)
	}
}

func TestEffectiveStreamHubEnabled_should_HonorExplicitFlagValue(t *testing.T) {
	cfg := &Config{}
	if err := cfg.SetStreamHubGlobalOverride(boolPtr(false)); err != nil {
		t.Fatalf("SetStreamHubGlobalOverride(false): %v", err)
	}
	if got := EffectiveStreamHubEnabled(cfg); got != false {
		t.Fatalf("expected explicit false to win, got %v", got)
	}

	if err := cfg.SetStreamHubGlobalOverride(boolPtr(true)); err != nil {
		t.Fatalf("SetStreamHubGlobalOverride(true): %v", err)
	}
	if got := EffectiveStreamHubEnabled(cfg); got != true {
		t.Fatalf("expected explicit true, got %v", got)
	}

	if err := cfg.SetStreamHubGlobalOverride(nil); err != nil {
		t.Fatalf("SetStreamHubGlobalOverride(nil): %v", err)
	}
	if got := EffectiveStreamHubEnabled(cfg); got != true {
		t.Fatalf("expected clearing the override to revert to the on-by-default value, got %v", got)
	}
}

// TestGetStreamHubGlobalOverride_should_ReportUnset_Then_Set mirrors
// GetStreamHubSessionOverride's (value, ok) shape for the global flag.
func TestGetStreamHubGlobalOverride_should_ReportUnset_Then_Set(t *testing.T) {
	cfg := &Config{}
	if _, ok := cfg.GetStreamHubGlobalOverride(); ok {
		t.Fatal("expected no override on a fresh config")
	}

	if err := cfg.SetStreamHubGlobalOverride(boolPtr(false)); err != nil {
		t.Fatalf("SetStreamHubGlobalOverride: %v", err)
	}
	if got, ok := cfg.GetStreamHubGlobalOverride(); !ok || got != false {
		t.Fatalf("expected (false, true), got (%v, %v)", got, ok)
	}
}

// TestRecordRollbackRehearsalCompleted_should_PersistTimestamp is now a
// purely historical record — EffectiveStreamHubEnabled no longer gates on
// it — but RecordRollbackRehearsalCompleted still exists and still persists,
// so cover that it does.
func TestRecordRollbackRehearsalCompleted_should_PersistTimestamp(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "shared")

	cfg := &Config{}
	before := time.Now()
	if err := cfg.RecordRollbackRehearsalCompleted(); err != nil {
		t.Fatalf("RecordRollbackRehearsalCompleted returned error: %v", err)
	}
	after := time.Now()

	if cfg.RollbackRehearsalCompletedAt == nil {
		t.Fatal("expected RollbackRehearsalCompletedAt to be set in memory")
	}
	if cfg.RollbackRehearsalCompletedAt.Before(before) || cfg.RollbackRehearsalCompletedAt.After(after) {
		t.Fatalf("expected RollbackRehearsalCompletedAt to be within [%v, %v], got %v", before, after, *cfg.RollbackRehearsalCompletedAt)
	}

	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if reloaded.RollbackRehearsalCompletedAt == nil {
		t.Fatal("expected persisted config to carry RollbackRehearsalCompletedAt")
	}
}

// TestGetStreamHubSessionOverride_should_ReportNoOverride_When_Unset verifies
// the nil-safe zero-value behavior mirroring GetFeatureFlag's shape.
func TestGetStreamHubSessionOverride_should_ReportNoOverride_When_Unset(t *testing.T) {
	var nilCfg *Config
	if _, ok := nilCfg.GetStreamHubSessionOverride("canary-1"); ok {
		t.Fatal("expected nil config to report no override")
	}

	cfg := &Config{} // StreamHubSessionOverrides is nil
	if _, ok := cfg.GetStreamHubSessionOverride("canary-1"); ok {
		t.Fatal("expected config with nil map to report no override")
	}
}

// TestSetStreamHubSessionOverride_should_SetAndClear_And_Persist exercises
// the per-session override storage: setting an override persists it, and
// passing nil clears it again, in both cases surviving a reload. Unaffected
// by the global-default change above — this path never went through
// STAPLER_SQUAD_USE_STREAM_HUB or the rehearsal gate.
func TestSetStreamHubSessionOverride_should_SetAndClear_And_Persist(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "shared")

	cfg := &Config{}
	forceHub := true
	if err := cfg.SetStreamHubSessionOverride("canary-1", &forceHub); err != nil {
		t.Fatalf("SetStreamHubSessionOverride returned error: %v", err)
	}
	if got, ok := cfg.GetStreamHubSessionOverride("canary-1"); !ok || !got {
		t.Fatalf("expected override to force hub-owned, got (%v, %v)", got, ok)
	}

	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if got, ok := reloaded.GetStreamHubSessionOverride("canary-1"); !ok || !got {
		t.Fatalf("expected persisted override to force hub-owned, got (%v, %v)", got, ok)
	}

	// Clear it.
	if err := cfg.SetStreamHubSessionOverride("canary-1", nil); err != nil {
		t.Fatalf("SetStreamHubSessionOverride(nil) returned error: %v", err)
	}
	if _, ok := cfg.GetStreamHubSessionOverride("canary-1"); ok {
		t.Fatal("expected override to be cleared")
	}

	reloadedAfterClear, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after clear: %v", err)
	}
	if _, ok := reloadedAfterClear.GetStreamHubSessionOverride("canary-1"); ok {
		t.Fatal("expected persisted config to no longer have the override")
	}
}

func boolPtr(b bool) *bool { return &b }
