package mux

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HooksConfig represents the Claude Code hooks configuration file format.
// When CLAUDE_CODE_HOOKS_PATH is set, Claude Code reads hooks from this file.
type HooksConfig struct {
	Hooks []HookDefinition `json:"hooks"`
}

// HookDefinition defines a single hook in the hooks configuration.
type HookDefinition struct {
	// Matcher specifies when this hook should trigger
	Matcher HookMatcher `json:"matcher"`
	// Hooks contains the actual hook commands
	Hooks []HookCommand `json:"hooks"`
}

// HookMatcher specifies the conditions for triggering a hook.
type HookMatcher struct {
	// Event is the hook event type: "Notification", "Stop", "PermissionRequest", "PostToolUse"
	Event string `json:"event"`
}

// HookCommand defines a command to execute when a hook triggers.
// Supports both "command" hooks (shell execution) and "http" hooks (HTTP POST).
type HookCommand struct {
	// Type is "command" or "http"
	Type string `json:"type"`
	// Command is the shell command to execute (command hooks only)
	Command string `json:"command,omitempty"`
	// URL is the HTTP endpoint to POST to (http hooks only)
	URL string `json:"url,omitempty"`
	// Headers are optional HTTP headers (http hooks only)
	Headers map[string]string `json:"headers,omitempty"`
	// Timeout in seconds for http hooks, milliseconds for command hooks (optional)
	Timeout int `json:"timeout,omitempty"`
}

// HooksMetadata contains context to be injected into hook environment variables.
type HooksMetadata struct {
	// SocketPath is the path to the mux Unix domain socket
	SocketPath string
	// TmuxSession is the tmux session name for this mux instance
	TmuxSession string
	// PID is the process ID of the ssq-mux wrapper
	PID int
	// Cwd is the current working directory
	Cwd string
	// Command is the command being run (typically "claude")
	Command string
}

// GenerateHooksFile creates a temporary hooks configuration file for ssq-mux.
// The file enables Claude Code hooks to send notifications to stapler-squad with
// proper session context for correlation and deep linking.
//
// Returns the path to the generated hooks file, which should be set as
// CLAUDE_CODE_HOOKS_PATH environment variable before starting Claude.
func GenerateHooksFile(meta *HooksMetadata) (string, error) {
	if meta == nil {
		return "", fmt.Errorf("hooks metadata is required")
	}

	// Find the hooks handler script
	hookHandler, err := findHooksHandler()
	if err != nil {
		return "", fmt.Errorf("hooks handler not found: %w", err)
	}

	// Build environment variable prefix for the hook commands
	// These variables will be available to cs-hook-handler for session correlation
	envPrefix := buildEnvPrefix(meta)

	// Create hooks configuration for all relevant events
	config := HooksConfig{
		Hooks: []HookDefinition{
			{
				Matcher: HookMatcher{Event: "Notification"},
				Hooks: []HookCommand{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s %s notification", envPrefix, hookHandler),
						Timeout: 5000, // 5 second timeout
					},
				},
			},
			{
				Matcher: HookMatcher{Event: "Stop"},
				Hooks: []HookCommand{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s %s stop", envPrefix, hookHandler),
						Timeout: 5000,
					},
				},
			},
			{
				// HTTP hook blocks Claude Code until the user approves/denies in the web UI.
				// Timeout 300s matches Claude Code's max hook timeout.
				Matcher: HookMatcher{Event: "PermissionRequest"},
				Hooks: []HookCommand{
					{
						Type:    "http",
						URL:     "http://localhost:8543/api/hooks/permission-request",
						Timeout: 300,
						Headers: map[string]string{
							"X-CS-Session-ID": meta.TmuxSession,
						},
					},
				},
			},
			{
				Matcher: HookMatcher{Event: "PostToolUse"},
				Hooks: []HookCommand{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s %s post-tool", envPrefix, hookHandler),
						Timeout: 5000,
					},
				},
			},
		},
	}

	// Serialize to JSON
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal hooks config: %w", err)
	}

	// Create temporary file for hooks configuration
	// Use OS temp directory for proper cleanup and permissions
	hooksPath := filepath.Join(os.TempDir(), fmt.Sprintf("ssq-mux-hooks-%d.json", meta.PID))

	// Write configuration file
	if err := os.WriteFile(hooksPath, configData, 0600); err != nil {
		return "", fmt.Errorf("failed to write hooks file: %w", err)
	}

	return hooksPath, nil
}

// CleanupHooksFile removes the generated hooks configuration file.
// This should be called when ssq-mux exits.
func CleanupHooksFile(path string) error {
	if path == "" {
		return nil
	}

	// Only remove files in temp directory that match our naming pattern
	if filepath.Dir(path) != os.TempDir() {
		return fmt.Errorf("refusing to delete hooks file outside temp directory: %s", path)
	}

	return os.Remove(path)
}

// CleanupStaleHooksFiles removes any hooks files left behind by crashed ssq-mux instances.
// This should be called on startup to clean up orphaned files.
func CleanupStaleHooksFiles() error {
	pattern := filepath.Join(os.TempDir(), "ssq-mux-hooks-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob hooks files: %w", err)
	}

	for _, match := range matches {
		// Extract PID from filename and check if process is still running
		var pid int
		_, err := fmt.Sscanf(filepath.Base(match), "ssq-mux-hooks-%d.json", &pid)
		if err != nil {
			continue // Skip files that don't match pattern
		}

		// Check if process with this PID exists
		if !processExists(pid) {
			os.Remove(match)
		}
	}

	return nil
}

// findHooksHandler locates the ssq-hook-handler script.
// Search order:
// 1. Same directory as the current executable (ssq-hook-handler, then cs-hook-handler for compat)
// 2. scripts/ subdirectory relative to the executable's parent (project root, for dev)
// 3. scripts/ subdirectory relative to the current working directory (running from project root)
// 4. ~/.local/bin/ssq-hook-handler
// 5. ~/.stapler-squad/scripts/ssq-hook-handler
func findHooksHandler() (string, error) {
	names := []string{"ssq-hook-handler", "cs-hook-handler"}

	// Try same directory as executable
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		for _, name := range names {
			if p := filepath.Join(execDir, name); fileExists(p) {
				return p, nil
			}
		}
		// scripts/ sibling of executable's parent (e.g. project root when built to ./ssq-mux)
		for _, name := range names {
			if p := filepath.Join(filepath.Dir(execDir), "scripts", name); fileExists(p) {
				return p, nil
			}
		}
	}

	// Try scripts/ relative to cwd (useful when running from the project root)
	if cwd, err := os.Getwd(); err == nil {
		for _, name := range names {
			if p := filepath.Join(cwd, "scripts", name); fileExists(p) {
				return p, nil
			}
		}
	}

	homeDir, _ := os.UserHomeDir()

	// Try ~/.local/bin
	if homeDir != "" {
		for _, name := range names {
			if p := filepath.Join(homeDir, ".local", "bin", name); fileExists(p) {
				return p, nil
			}
		}
	}

	// Try ~/.stapler-squad/scripts
	if homeDir != "" {
		for _, name := range names {
			if p := filepath.Join(homeDir, ".stapler-squad", "scripts", name); fileExists(p) {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("ssq-hook-handler not found in standard locations")
}

// buildEnvPrefix creates environment variable exports for the hook command.
// These variables provide session context to cs-hook-handler.
func buildEnvPrefix(meta *HooksMetadata) string {
	vars := []string{}

	if meta.SocketPath != "" {
		vars = append(vars, fmt.Sprintf("CS_MUX_SOCKET_PATH=%q", meta.SocketPath))
	}
	if meta.TmuxSession != "" {
		vars = append(vars, fmt.Sprintf("CS_MUX_TMUX_SESSION=%q", meta.TmuxSession))
	}
	if meta.PID > 0 {
		vars = append(vars, fmt.Sprintf("CS_MUX_PID=%d", meta.PID))
	}
	if meta.Cwd != "" {
		vars = append(vars, fmt.Sprintf("CS_MUX_CWD=%q", meta.Cwd))
	}
	if meta.Command != "" {
		vars = append(vars, fmt.Sprintf("CS_MUX_COMMAND=%q", meta.Command))
	}

	// Build export command
	if len(vars) == 0 {
		return ""
	}

	result := "env"
	for _, v := range vars {
		result += " " + v
	}
	return result
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// processExists checks if a process with the given PID exists.
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. We need to send signal 0 to check.
	err = process.Signal(os.Signal(nil))
	return err == nil
}
