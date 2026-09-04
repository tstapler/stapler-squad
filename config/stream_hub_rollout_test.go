package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestResolveGlobalStreamHubDefault_should_ReturnFalseWithNoError_When_RequestedIsFalse
// verifies the gate never blocks the safe direction: requesting the global
// default be false is always permitted, regardless of whether the rollback
// rehearsal has been recorded.
func TestResolveGlobalStreamHubDefault_should_ReturnFalseWithNoError_When_RequestedIsFalse(t *testing.T) {
	cfg := &Config{} // RollbackRehearsalCompletedAt unset

	got, err := ResolveGlobalStreamHubDefault(cfg, false)
	if err != nil {
		t.Fatalf("expected no error requesting false, got %v", err)
	}
	if got != false {
		t.Fatalf("expected false, got %v", got)
	}
}

// TestResolveGlobalStreamHubDefault_should_FailFast_When_RehearsalNotCompleted
// is plan.md Story 3.3.1's AC2/Task 3.3.1e scenario (pre-mortem P1 #4): with
// RollbackRehearsalCompletedAt unset, requesting the global default resolve
// to true must fail with an explicit error, not silently fall back to false
// without signaling why.
func TestResolveGlobalStreamHubDefault_should_FailFast_When_RehearsalNotCompleted(t *testing.T) {
	cfg := &Config{} // RollbackRehearsalCompletedAt unset

	got, err := ResolveGlobalStreamHubDefault(cfg, true)
	if !errors.Is(err, ErrRollbackRehearsalNotCompleted) {
		t.Fatalf("expected ErrRollbackRehearsalNotCompleted, got %v", err)
	}
	if got != false {
		t.Fatalf("expected false to be returned alongside the error, got %v", got)
	}
}

// TestResolveGlobalStreamHubDefault_should_FailFast_When_ConfigIsNil verifies
// the gate is nil-safe: a nil *Config must behave like an unset rehearsal
// timestamp rather than panicking.
func TestResolveGlobalStreamHubDefault_should_FailFast_When_ConfigIsNil(t *testing.T) {
	got, err := ResolveGlobalStreamHubDefault(nil, true)
	if !errors.Is(err, ErrRollbackRehearsalNotCompleted) {
		t.Fatalf("expected ErrRollbackRehearsalNotCompleted for nil config, got %v", err)
	}
	if got != false {
		t.Fatalf("expected false, got %v", got)
	}
}

// TestResolveGlobalStreamHubDefault_should_Succeed_When_RehearsalCompleted is
// Task 3.3.1e's happy path: once RollbackRehearsalCompletedAt is set to a
// valid, non-zero timestamp, the same resolution that previously failed now
// succeeds and permits the global default to be true.
func TestResolveGlobalStreamHubDefault_should_Succeed_When_RehearsalCompleted(t *testing.T) {
	completedAt := time.Now()
	cfg := &Config{RollbackRehearsalCompletedAt: &completedAt}

	got, err := ResolveGlobalStreamHubDefault(cfg, true)
	if err != nil {
		t.Fatalf("expected no error once rehearsal is recorded, got %v", err)
	}
	if got != true {
		t.Fatalf("expected true, got %v", got)
	}
}

// TestRecordRollbackRehearsalCompleted_should_PersistTimestamp_And_UnblockResolution
// exercises Task 3.3.2c end to end: recording a completed rehearsal persists
// RollbackRehearsalCompletedAt to disk, and a freshly reloaded config
// subsequently permits ResolveGlobalStreamHubDefault to return true where it
// previously refused.
func TestRecordRollbackRehearsalCompleted_should_PersistTimestamp_And_UnblockResolution(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

	cfg := &Config{}

	// Before recording, the gate refuses.
	if _, err := ResolveGlobalStreamHubDefault(cfg, true); !errors.Is(err, ErrRollbackRehearsalNotCompleted) {
		t.Fatalf("expected gate to refuse before rehearsal is recorded, got %v", err)
	}

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

	if _, err := ResolveGlobalStreamHubDefault(cfg, true); err != nil {
		t.Fatalf("expected gate to succeed after rehearsal is recorded, got %v", err)
	}

	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after RecordRollbackRehearsalCompleted: %v", err)
	}
	if reloaded.RollbackRehearsalCompletedAt == nil {
		t.Fatal("expected persisted config to carry RollbackRehearsalCompletedAt")
	}
	if _, err := ResolveGlobalStreamHubDefault(reloaded, true); err != nil {
		t.Fatalf("expected reloaded config's gate to succeed, got %v", err)
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
// Task 3.3.1b's per-session override storage: setting an override persists
// it, and passing nil clears it again, in both cases surviving a reload.
func TestSetStreamHubSessionOverride_should_SetAndClear_And_Persist(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

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
