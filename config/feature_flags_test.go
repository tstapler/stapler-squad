package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetFeatureFlag_NilConfig verifies that GetFeatureFlag on a nil *Config returns false.
func TestGetFeatureFlag_NilConfig(t *testing.T) {
	var cfg *Config
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag on nil config: got %v, want false", got)
	}
}

// TestGetFeatureFlag_NilMap verifies that a non-nil Config with a nil FeatureFlags map returns false.
func TestGetFeatureFlag_NilMap(t *testing.T) {
	cfg := &Config{} // FeatureFlags is nil by zero value
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag with nil map: got %v, want false", got)
	}
}

// TestGetFeatureFlag_MissingKey verifies that an absent key returns false.
func TestGetFeatureFlag_MissingKey(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"other": true,
		},
	}
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag for missing key: got %v, want false", got)
	}
}

// TestGetFeatureFlag_Present verifies that a key in the map returns the stored value.
func TestGetFeatureFlag_Present(t *testing.T) {
	tests := []struct {
		name  string
		value bool
	}{
		{"returns true when flag is true", true},
		{"returns false when flag is explicitly false", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				FeatureFlags: map[string]bool{
					"backlog": tc.value,
				},
			}
			if got := cfg.GetFeatureFlag("backlog"); got != tc.value {
				t.Errorf("GetFeatureFlag: got %v, want %v", got, tc.value)
			}
		})
	}
}

// TestGetFeatureFlagWithDefault_NilConfig verifies that GetFeatureFlagWithDefault on a
// nil *Config returns defaultValue, not false.
func TestGetFeatureFlagWithDefault_NilConfig(t *testing.T) {
	var cfg *Config
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault on nil config: got %v, want true", got)
	}
}

// TestGetFeatureFlagWithDefault_NilMap verifies that a non-nil Config with a nil
// FeatureFlags map returns defaultValue.
func TestGetFeatureFlagWithDefault_NilMap(t *testing.T) {
	cfg := &Config{} // FeatureFlags is nil by zero value
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault with nil map: got %v, want true", got)
	}
}

// TestGetFeatureFlagWithDefault_MissingKey verifies that a genuinely absent key falls
// through to defaultValue, not false.
func TestGetFeatureFlagWithDefault_MissingKey(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"other": true,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault for missing key: got %v, want true (the default)", got)
	}
}

// TestGetFeatureFlagWithDefault_ExplicitFalseOverridesDefault is the regression case
// this helper exists for: a flag graduated to "default true" must still respect an
// explicitly persisted false, distinguishing "never set" from "set to false" — which a
// plain map[string]bool lookup alone (GetFeatureFlag) cannot do.
func TestGetFeatureFlagWithDefault_ExplicitFalseOverridesDefault(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"terminal:resync-exec-gate-fast-lane": false,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != false {
		t.Errorf("GetFeatureFlagWithDefault: got %v, want false (explicit override of the true default)", got)
	}
}

// TestGetFeatureFlagWithDefault_ExplicitTrueMatchesDefault verifies the unsurprising
// case: an explicit true reads back as true regardless of defaultValue.
func TestGetFeatureFlagWithDefault_ExplicitTrueMatchesDefault(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"terminal:resync-exec-gate-fast-lane": true,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", false); got != true {
		t.Errorf("GetFeatureFlagWithDefault: got %v, want true (explicit value, defaultValue irrelevant)", got)
	}
}

// TestSetFeatureFlag_InitializesMap verifies that SetFeatureFlag works when FeatureFlags is nil,
// initializing the map and persisting the flag to disk.
func TestSetFeatureFlag_InitializesMap(t *testing.T) {
	// Direct the config save to a temp directory so we don't touch real state.
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

	cfg := &Config{} // FeatureFlags is nil

	if err := cfg.SetFeatureFlag("backlog", true); err != nil {
		t.Fatalf("SetFeatureFlag returned error: %v", err)
	}

	// Map must be initialized and contain the flag.
	if cfg.FeatureFlags == nil {
		t.Fatal("SetFeatureFlag did not initialize FeatureFlags map")
	}
	if got := cfg.FeatureFlags["backlog"]; got != true {
		t.Errorf("FeatureFlags[backlog]: got %v, want true", got)
	}

	// Verify that the config was written to disk and is readable back.
	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after SetFeatureFlag: %v", err)
	}
	if !reloaded.GetFeatureFlag("backlog") {
		t.Error("persisted config does not have backlog flag set to true")
	}
}

// TestSetFeatureFlag_UpdatesExistingMap verifies that SetFeatureFlag updates a pre-existing map
// and persists correctly.
func TestSetFeatureFlag_UpdatesExistingMap(t *testing.T) {
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

	cfg := &Config{
		FeatureFlags: map[string]bool{
			"backlog": true,
		},
	}

	// Disable the flag.
	if err := cfg.SetFeatureFlag("backlog", false); err != nil {
		t.Fatalf("SetFeatureFlag returned error: %v", err)
	}

	if cfg.GetFeatureFlag("backlog") {
		t.Error("expected GetFeatureFlag to return false after SetFeatureFlag(false)")
	}

	// Verify persistence.
	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if reloaded.GetFeatureFlag("backlog") {
		t.Error("persisted config still shows backlog as true after disabling")
	}
}
