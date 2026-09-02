package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/xeipuuv/gojsonschema"
)

// Common errors for Claude config operations
var (
	ErrConfigNotFound = fmt.Errorf("config file not found")
	ErrInvalidConfig  = fmt.Errorf("invalid config file")
	ErrInvalidJSON    = fmt.Errorf("invalid JSON")
)

// settingsJSONSchema defines the expected structure for settings.json
// This is a basic schema that can be extended as needed
const settingsJSONSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"properties": {
		"model": {"type": "string"},
		"api_key": {"type": "string"},
		"organization": {"type": "string"},
		"custom_instructions": {"type": "string"},
		"tools": {
			"type": "array",
			"items": {"type": "string"}
		}
	}
}`

// ConfigFile represents a single Claude configuration file
type ConfigFile struct {
	// Name is the filename (e.g., "CLAUDE.md", "settings.json", "agents.md")
	Name string
	// Path is the absolute path to the file
	Path string
	// Content is the file contents
	Content string
	// ModTime is the last modification timestamp
	ModTime time.Time
}

// ClaudeConfigManager manages access to Claude configuration files
// located in the ~/.claude directory
type ClaudeConfigManager struct {
	// claudeDir is the path to the ~/.claude directory
	claudeDir string
	// mu provides thread-safe access to config operations
	mu sync.RWMutex
}

// NewClaudeConfigManager creates a new ClaudeConfigManager instance
// with the ~/.claude directory resolved
func NewClaudeConfigManager() (*ClaudeConfigManager, error) {
	claudeDir, err := GetClaudeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get Claude directory: %w", err)
	}

	return &ClaudeConfigManager{
		claudeDir: claudeDir,
	}, nil
}

// GetClaudeDir returns the path to the ~/.claude directory
func GetClaudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, ".claude"), nil
}

// ResolveClaudeHistoryDir returns the directory holding Claude Code's JSONL
// session-history files, which two independent consumers walk and watch:
// server's TokenStore/ArtifactExtractor (BuildRuntimeDeps) and session's
// HistoryLinker fsnotify watcher.
//
// In production (isIsolated=false) that is the real ~/.claude/projects. An
// isolated test/demo instance must not touch it: on a dev machine with a long
// Claude Code history that tree is thousands of real JSONL files, some large,
// which overflows the consumers' bounded parse/scan queues and made the
// walk+parse take 100s-1500s+ in CI, well past any reasonable test timeout.
// Under isolation this returns (and creates) a per-instance directory inside
// the isolated config dir instead — it starts empty, so both consumers stay
// fully functional against test fixtures without ever reading real session
// data. Same hazard class as IsIsolatedInstance's other documented case (the
// shared tmux socket); here the isolated resource is the config dir.
//
// isIsolated is a parameter rather than an internal IsIsolatedInstance() call
// so both branches are directly unit-testable — every test exercising this is
// itself a go test binary, where IsIsolatedInstance() is unconditionally true.
func ResolveClaudeHistoryDir(homeDir string, isIsolated bool) (string, error) {
	if !isIsolated {
		return filepath.Join(homeDir, ".claude", "projects"), nil
	}
	configDir, err := GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve isolated config dir for Claude history: %w", err)
	}
	historyDir := filepath.Join(configDir, "claude-projects")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create isolated Claude history dir %s: %w", historyDir, err)
	}
	return historyDir, nil
}

// toValidUTF8 ensures the content is valid UTF-8 for protobuf marshaling.
// Invalid UTF-8 sequences are replaced with the Unicode replacement character.
func toValidUTF8(content []byte) string {
	if utf8.Valid(content) {
		return string(content)
	}
	// Replace invalid UTF-8 sequences with replacement character (U+FFFD)
	return strings.ToValidUTF8(string(content), "�")
}

// resolveConfigPath joins filename onto claudeDir and rejects any result that
// escapes claudeDir (e.g. filename containing "../"). filename ultimately
// comes from RPC requests (see server/services/config_service.go), so it must
// not be trusted to stay within the Claude config directory.
func resolveConfigPath(claudeDir, filename string) (string, error) {
	base := filepath.Clean(claudeDir)
	joined := filepath.Clean(filepath.Join(base, filename))
	if !strings.HasPrefix(joined+string(filepath.Separator), base+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid filename: %q escapes the Claude config directory", filename)
	}
	return joined, nil
}

// GetConfig reads a specific Claude configuration file by name
// Common file names include "CLAUDE.md", "settings.json", "agents.md"
func (m *ClaudeConfigManager) GetConfig(filename string) (*ConfigFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filePath, err := resolveConfigPath(m.claudeDir, filename)
	if err != nil {
		return nil, err
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, filename)
		}
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	// Read file contents
	// #nosec G304 -- filePath was already validated above via resolveConfigPath,
	// which rejects any result escaping m.claudeDir.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return &ConfigFile{
		Name:    filename,
		Path:    filePath,
		Content: toValidUTF8(content),
		ModTime: info.ModTime(),
	}, nil
}

// ListConfigs returns all configuration files in the ~/.claude directory
func (m *ClaudeConfigManager) ListConfigs() ([]ConfigFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if directory exists
	if _, err := os.Stat(m.claudeDir); err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist yet - return empty list
			return []ConfigFile{}, nil
		}
		return nil, fmt.Errorf("failed to access Claude directory: %w", err)
	}

	// Read directory entries
	entries, err := os.ReadDir(m.claudeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read Claude directory: %w", err)
	}

	configs := make([]ConfigFile, 0, len(entries))
	for _, entry := range entries {
		// Skip directories and hidden files
		if entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		filePath := filepath.Join(m.claudeDir, entry.Name())
		// #nosec G304 -- filePath is built from entry.Name(), enumerated by os.ReadDir
		// over m.claudeDir itself, not from caller-supplied input.
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		configs = append(configs, ConfigFile{
			Name:    entry.Name(),
			Path:    filePath,
			Content: toValidUTF8(content),
			ModTime: info.ModTime(),
		})
	}

	return configs, nil
}

// UpdateConfig updates a Claude configuration file atomically with backup
// It creates a .bak file before writing, and uses a temporary file for atomicity.
// JSON files are validated before writing to prevent corrupt settings files.
func (m *ClaudeConfigManager) UpdateConfig(filename string, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate JSON content before writing to prevent corrupt settings files.
	if strings.HasSuffix(filename, ".json") {
		var v interface{}
		if err := json.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
	}

	filePath, err := resolveConfigPath(m.claudeDir, filename)
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(m.claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create Claude directory: %w", err)
	}

	// If file exists, create a backup
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filePath + ".bak"
		if err := copyFile(filePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Write to temporary file first for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomically rename temp file to actual file
	if err := os.Rename(tmpPath, filePath); err != nil {
		// Clean up temp file on failure
		os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically update file: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// #nosec G304 -- copyFile's only caller (UpdateConfig) passes filePath, which was
	// already validated by resolveConfigPath to stay within m.claudeDir.
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}

// ValidateJSON validates a JSON configuration file against a schema
// Returns nil if valid, error with details if invalid
func (m *ClaudeConfigManager) ValidateJSON(filename string, content string) error {
	// Only validate JSON files
	if !strings.HasSuffix(filename, ".json") {
		return nil // Not a JSON file, skip validation
	}

	// Check if content is valid JSON first
	var jsonData interface{}
	if err := json.Unmarshal([]byte(content), &jsonData); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	// For settings.json, validate against schema
	if filename == "settings.json" {
		schemaLoader := gojsonschema.NewStringLoader(settingsJSONSchema)
		documentLoader := gojsonschema.NewStringLoader(content)

		result, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			return fmt.Errorf("schema validation error: %w", err)
		}

		if !result.Valid() {
			// Collect validation errors
			var errMsgs []string
			for _, desc := range result.Errors() {
				errMsgs = append(errMsgs, desc.String())
			}
			return fmt.Errorf("%w: %s", ErrInvalidJSON, strings.Join(errMsgs, "; "))
		}
	}

	return nil
}

// UpdateConfigWithValidation updates a config file with JSON validation
// This is a convenience method that combines validation and update
func (m *ClaudeConfigManager) UpdateConfigWithValidation(filename string, content string) error {
	// Validate JSON content before writing
	if err := m.ValidateJSON(filename, content); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Proceed with atomic update
	return m.UpdateConfig(filename, content)
}
