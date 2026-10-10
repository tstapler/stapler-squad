package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func isolateConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	return filepath.Join(home, ".stapler-squad", ConfigFileName)
}

func TestGetFeatureFlagScoped_ShouldApplyScopeThenGlobalThenDefault(t *testing.T) {
	var nilCfg *Config
	if nilCfg.GetFeatureFlagScoped("f", "kind:review", true) != true {
		t.Fatal("nil config must return the default")
	}
	cfg := &Config{}
	if cfg.GetFeatureFlagScoped("f", "kind:review", false) {
		t.Fatal("no values: default")
	}
	cfg.FeatureFlags = map[string]bool{"f": true}
	cfg.FeatureFlagScopes = map[string]map[string]bool{"f": {"kind:review": false}}
	if cfg.GetFeatureFlagScoped("f", "kind:review", false) {
		t.Error("explicit scope false must win over global true")
	}
	if !cfg.GetFeatureFlagScoped("f", "kind:diagnose", false) {
		t.Error("scope without override follows explicit global true")
	}
	if v, ok := cfg.GetFeatureFlagScopedOverride("f", "kind:review"); !ok || v {
		t.Errorf("override read-back = (%v,%v), want (false,true)", v, ok)
	}
	if _, ok := cfg.GetFeatureFlagScopedOverride("f", "kind:other"); ok {
		t.Error("absent scope must report ok=false")
	}
}

// T-FL-06: a config written by the previous version loads unchanged, and
// writing then deleting scopes returns the file to a byte-identical shape.
func TestConfig_ShouldLoadUnchangedAndRoundTripWithoutScopesKey_WhenFileWrittenByPreviousVersion(t *testing.T) {
	path := isolateConfigHome(t)
	cfg := &Config{}
	if err := cfg.SetFeatureFlag("hidden_session_gate", true); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	before := mustMarshal(t, loaded)
	if loaded.FeatureFlagScopes != nil {
		t.Fatal("a file without feature_flag_scopes must load with a nil map")
	}
	if err := loaded.SetFeatureFlagScope("hidden_session_gate", "kind:review", false); err != nil {
		t.Fatal(err)
	}
	if err := loaded.SetFeatureFlagScope("hidden_session_gate", "kind:diagnose", true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.FeatureFlagScopeOverrides("hidden_session_gate"); len(got) != 2 || got["kind:review"] || !got["kind:diagnose"] {
		t.Fatalf("persisted scopes = %v", got)
	}
	for _, s := range []string{"kind:review", "kind:diagnose"} {
		if err := reloaded.DeleteFeatureFlagScope("hidden_session_gate", s); err != nil {
			t.Fatal(err)
		}
	}
	if after := mustMarshal(t, reloaded); after != before {
		t.Fatalf("config not byte-identical after deleting every scope:\nbefore %s\nafter  %s", before, after)
	}
}

func mustMarshal(t *testing.T, c *Config) string {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
