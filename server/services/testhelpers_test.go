package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeSettings writes content to <dir>/.claude/settings.local.json, creating the directory.
// Used by hook injector tests.
func writeSettings(t *testing.T, dir, content string) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("writeSettings: mkdir %s: %v", claudeDir, err)
	}
	path := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeSettings: write %s: %v", path, err)
	}
}

// readSettings reads and top-level-parses <dir>/.claude/settings.local.json.
// Used by hook injector tests.
func readSettings(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	path := filepath.Join(dir, ".claude", "settings.local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readSettings: read %s: %v", path, err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("readSettings: parse JSON: %v", err)
	}
	return result
}
