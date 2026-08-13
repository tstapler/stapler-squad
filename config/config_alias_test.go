package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/config"
)

func TestAliasConfig_LoadsFromJSON_WhenAliasesFieldPresent(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"listen_address": "localhost:9999",
		"session_defaults": map[string]interface{}{
			"aliases": []interface{}{
				map[string]interface{}{
					"name":        "myproj",
					"path":        "~/code/myproj",
					"program":     "claude",
					"description": "My project",
					"group":       "work",
				},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, data, 0644)

	loaded, err := config.LoadConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loaded.SessionDefaults.Aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(loaded.SessionDefaults.Aliases))
	}
	a := loaded.SessionDefaults.Aliases[0]
	if a.Name != "myproj" || a.Path != "~/code/myproj" || a.Program != "claude" {
		t.Errorf("alias fields mismatch: %+v", a)
	}
}

func TestAliasConfig_InitializesEmpty_WhenAliasesFieldAbsent(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"listen_address": "localhost:9999",
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, data, 0644)

	loaded, err := config.LoadConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.SessionDefaults.Aliases == nil {
		t.Error("Aliases should be initialized to empty slice, not nil")
	}
	if len(loaded.SessionDefaults.Aliases) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(loaded.SessionDefaults.Aliases))
	}
}

func TestAliasConfig_RoundTrip_WhenWrittenAndReloaded(t *testing.T) {
	dir := t.TempDir()
	original := map[string]interface{}{
		"listen_address": "localhost:9999",
		"session_defaults": map[string]interface{}{
			"aliases": []interface{}{
				map[string]interface{}{
					"name":      "quick",
					"cli_flags": "--model haiku",
					"group":     "tools",
				},
			},
		},
	}
	data, _ := json.Marshal(original)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, data, 0644)

	loaded, err := config.LoadConfigFromPath(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loaded.SessionDefaults.Aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(loaded.SessionDefaults.Aliases))
	}
	a := loaded.SessionDefaults.Aliases[0]
	if a.Name != "quick" || a.CLIFlags != "--model haiku" || a.Group != "tools" {
		t.Errorf("alias round-trip failed: %+v", a)
	}
}
