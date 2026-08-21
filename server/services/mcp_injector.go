package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

const mcpServerName = "stapler-squad"

// InjectMCPConfig writes (or updates) the stapler-squad MCP server entry into
// <rootDir>/.mcp.json (the Claude Code project-scope MCP file).
//
// Behavior:
//   - If the file already contains our entry pointing to the same binary, it is a no-op.
//   - If the file exists without our entry, the entry is merged in.
//   - If the file does not exist, it is created.
//   - The write is atomic (temp file + rename).
//
// binaryPath should be the absolute path to the stapler-squad binary (use os.Executable()).
//
// Note: sessions spawned by stapler-squad also receive a per-session --mcp-config flag
// via buildClaudeCommand/claudeMCPConfigArgs, which is the primary MCP injection path.
// InjectMCPConfig serves as a fallback for tools that read .mcp.json directly (e.g. the
// MCP tools_lifecycle inject_mcp_config tool).
func InjectMCPConfig(rootDir, binaryPath string) error {
	mcpPath := filepath.Join(rootDir, ".mcp.json")

	// Read existing .mcp.json.
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(mcpPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", mcpPath, err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Warn("[InjectMCPConfig] .mcp.json has invalid JSON, resetting", "path", mcpPath, "err", err)
			raw = map[string]json.RawMessage{}
		}
	}

	// Check if our entry already points to this binary.
	if mcpRaw, ok := raw["mcpServers"]; ok {
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(mcpRaw, &servers); err == nil {
			if entryRaw, ok := servers[mcpServerName]; ok {
				var entry struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(entryRaw, &entry); err == nil && entry.Command == binaryPath {
					log.Debug("[InjectMCPConfig] entry already present", "path", mcpPath)
					return nil
				}
			}
		}
	}

	// Build / merge mcpServers map.
	mcpServers := map[string]json.RawMessage{}
	if mcpRaw, ok := raw["mcpServers"]; ok {
		_ = json.Unmarshal(mcpRaw, &mcpServers)
	}

	entry := map[string]interface{}{
		"type":    "stdio",
		"command": binaryPath,
		"args":    []string{"--mcp"},
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal mcp entry: %w", err)
	}
	mcpServers[mcpServerName] = json.RawMessage(entryJSON)

	mcpJSON, err := json.Marshal(mcpServers)
	if err != nil {
		return fmt.Errorf("marshal mcpServers: %w", err)
	}
	raw["mcpServers"] = json.RawMessage(mcpJSON)

	return writeSettingsAtomic(mcpPath, filepath.Dir(mcpPath), raw)
}

// RemoveMCPConfig removes the stapler-squad entry from <rootDir>/.mcp.json.
// If the file is missing or has no entry for stapler-squad, it is a no-op.
func RemoveMCPConfig(rootDir string) error {
	mcpPath := filepath.Join(rootDir, ".mcp.json")

	data, err := os.ReadFile(mcpPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", mcpPath, err)
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", mcpPath, err)
	}

	mcpRaw, ok := raw["mcpServers"]
	if !ok {
		return nil // no mcpServers key — nothing to remove
	}

	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		return fmt.Errorf("parse mcpServers: %w", err)
	}
	delete(servers, mcpServerName)

	if len(servers) == 0 {
		delete(raw, "mcpServers")
	} else {
		updated, err := json.Marshal(servers)
		if err != nil {
			return fmt.Errorf("marshal mcpServers: %w", err)
		}
		raw["mcpServers"] = json.RawMessage(updated)
	}

	return writeSettingsAtomic(mcpPath, filepath.Dir(mcpPath), raw)
}

func writeSettingsAtomic(settingsPath, claudeDir string, raw map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	// Unique temp file (not settingsPath+".tmp") so two concurrent writers targeting
	// the same settingsPath — e.g. InjectHooksConfig and RemoveHooksConfig racing on
	// the same rootDir from two goroutines — can't clobber each other's temp file
	// mid-write and produce a truncated/corrupt settingsPath after rename. Mirrors
	// internal/claudehooks/claudehooks.go's mutate(), which documents the same fix
	// for the identical fixed-tmp-filename hazard.
	tmp, err := os.CreateTemp(claudeDir, filepath.Base(settingsPath)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", claudeDir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup if rename fails
	if _, err := tmp.Write(out); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		return fmt.Errorf("rename %s: %w", tmpPath, err)
	}
	log.Info("[InjectMCPConfig] wrote settings", "path", settingsPath)
	return nil
}
