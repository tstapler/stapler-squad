package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

const mcpServerName = "stapler-squad"

// InjectMCPConfig writes (or merges) the stapler-squad MCP server entry into
// <rootDir>/.claude/settings.local.json.
//
// Behavior:
//   - If the file already contains our entry pointing to the same binary, it is a no-op.
//   - If the file exists without our entry, the entry is merged in.
//   - If the file does not exist, it is created.
//   - The write is atomic (temp file + rename).
//
// binaryPath should be the absolute path to the stapler-squad binary (use os.Executable()).
func InjectMCPConfig(rootDir, binaryPath string) error {
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Read existing settings.
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Warn("[InjectMCPConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", err)
			repaired, repairErr := repairSettingsJSON(data)
			if repairErr == nil {
				_ = json.Unmarshal(repaired, &raw)
			} else {
				log.Warn("[InjectMCPConfig] could not repair settings file, resetting", "path", settingsPath, "err", repairErr)
				raw = map[string]json.RawMessage{}
			}
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
					log.Debug("[InjectMCPConfig] entry already present", "path", settingsPath)
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

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}

// RemoveMCPConfig removes the stapler-squad entry from
// <rootDir>/.claude/settings.local.json. If the file is missing, it is a no-op.
func RemoveMCPConfig(rootDir string) error {
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", settingsPath, err)
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

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}

func writeSettingsAtomic(settingsPath, claudeDir string, raw map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %s: %w", tmpPath, err)
	}
	log.Info("[InjectMCPConfig] wrote settings", "path", settingsPath)
	return nil
}
