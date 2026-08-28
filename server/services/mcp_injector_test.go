package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeMCPJSON writes content to <dir>/.mcp.json.
func writeMCPJSON(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeMCPJSON: write %s: %v", path, err)
	}
}

// readMCPJSON reads and top-level-parses <dir>/.mcp.json.
func readMCPJSON(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	path := filepath.Join(dir, ".mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readMCPJSON: read %s: %v", path, err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("readMCPJSON: parse JSON: %v", err)
	}
	return result
}

// TestInjectMCPConfigCreatesFile (U-3.1): InjectMCPConfig creates .mcp.json
// with the correct MCP server entry when it does not exist.
func TestInjectMCPConfigCreatesFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	err := InjectMCPConfig(tmpDir, "/usr/local/bin/stapler-squad")
	if err != nil {
		t.Fatalf("InjectMCPConfig returned unexpected error: %v", err)
	}

	top := readMCPJSON(t, tmpDir)

	mcpRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("expected mcpServers key in .mcp.json, not found")
	}

	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("parse mcpServers: %v", err)
	}

	entryRaw, ok := servers["stapler-squad"]
	if !ok {
		t.Fatal("expected mcpServers.stapler-squad, not found")
	}

	var entry struct {
		Type    string   `json:"type"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		t.Fatalf("parse stapler-squad entry: %v", err)
	}

	if entry.Command != "/usr/local/bin/stapler-squad" {
		t.Errorf("command: got %q, want %q", entry.Command, "/usr/local/bin/stapler-squad")
	}
	if entry.Type != "stdio" {
		t.Errorf("type: got %q, want %q", entry.Type, "stdio")
	}
	if len(entry.Args) == 0 || entry.Args[0] != "--mcp" {
		t.Errorf("args: got %v, want [\"--mcp\"]", entry.Args)
	}
}

// TestInjectMCPConfigMerges (U-3.2): InjectMCPConfig preserves existing mcpServers
// entries while adding stapler-squad.
func TestInjectMCPConfigMerges(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	existing := `{"mcpServers":{"other-tool":{"type":"stdio","command":"/usr/bin/other","args":[]}}}`
	writeMCPJSON(t, tmpDir, existing)

	if err := InjectMCPConfig(tmpDir, "/usr/bin/ss"); err != nil {
		t.Fatalf("InjectMCPConfig: %v", err)
	}

	top := readMCPJSON(t, tmpDir)

	mcpRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("mcpServers not present after InjectMCPConfig")
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("parse mcpServers: %v", err)
	}
	// existing entry must still be present
	if _, ok := servers["other-tool"]; !ok {
		t.Error("existing other-tool entry was removed after InjectMCPConfig")
	}
	// stapler-squad must be added
	if _, ok := servers["stapler-squad"]; !ok {
		t.Error("mcpServers.stapler-squad not found")
	}
}

// TestInjectMCPConfigIdempotent (U-3.3): calling InjectMCPConfig twice yields the same
// result and exactly one stapler-squad entry.
func TestInjectMCPConfigIdempotent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if err := InjectMCPConfig(tmpDir, "/usr/bin/ss"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := InjectMCPConfig(tmpDir, "/usr/bin/ss"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	top := readMCPJSON(t, tmpDir)
	mcpRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("mcpServers not found")
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("parse mcpServers: %v", err)
	}

	count := 0
	for k := range servers {
		if k == "stapler-squad" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 stapler-squad entry, got %d", count)
	}
}

// TestInjectMCPConfigUpdatesPath (U-3.4): when the binary path changes InjectMCPConfig
// updates the command field to the new path.
func TestInjectMCPConfigUpdatesPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Pre-seed with old path.
	old := `{"mcpServers":{"stapler-squad":{"type":"stdio","command":"/old/path","args":["--mcp"]}}}`
	writeMCPJSON(t, tmpDir, old)

	if err := InjectMCPConfig(tmpDir, "/new/path"); err != nil {
		t.Fatalf("InjectMCPConfig: %v", err)
	}

	top := readMCPJSON(t, tmpDir)
	mcpRaw := top["mcpServers"]
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("parse mcpServers: %v", err)
	}
	var entry struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(servers["stapler-squad"], &entry); err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	if entry.Command != "/new/path" {
		t.Errorf("command: got %q, want %q", entry.Command, "/new/path")
	}
}

// TestInjectMCPConfigMalformedJSON (U-3.5): InjectMCPConfig must not panic on truncated JSON.
func TestInjectMCPConfigMalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	writeMCPJSON(t, tmpDir, `{"mcpServers": {`)

	var callErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("InjectMCPConfig panicked: %v", r)
			}
		}()
		callErr = InjectMCPConfig(tmpDir, "/usr/bin/ss")
	}()

	if callErr != nil {
		// Returning an error on unrecoverable malformed JSON is acceptable.
		return
	}

	// If no error, the written file must be valid JSON containing our entry.
	top := readMCPJSON(t, tmpDir)
	mcpRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("no error returned but mcpServers also not present")
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("parse mcpServers: %v", err)
	}
	if _, ok := servers["stapler-squad"]; !ok {
		t.Error("stapler-squad not found after repair")
	}
}

// TestRemoveMCPConfig (bonus): RemoveMCPConfig removes the stapler-squad entry.
func TestRemoveMCPConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if err := InjectMCPConfig(tmpDir, "/usr/bin/ss"); err != nil {
		t.Fatalf("InjectMCPConfig: %v", err)
	}

	if err := RemoveMCPConfig(tmpDir); err != nil {
		t.Fatalf("RemoveMCPConfig: %v", err)
	}

	// File should exist but have no mcpServers.stapler-squad key.
	path := filepath.Join(tmpDir, ".mcp.json")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read .mcp.json after remove: %v", err)
	}
	if len(data) == 0 {
		// File removed entirely — acceptable.
		return
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("parse .mcp.json after remove: %v", err)
	}
	if mcpRaw, ok := top["mcpServers"]; ok {
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(mcpRaw, &servers); err == nil {
			if _, found := servers["stapler-squad"]; found {
				t.Error("stapler-squad still present after RemoveMCPConfig")
			}
		}
	}
}
