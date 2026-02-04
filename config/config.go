package config

import (
	"claude-squad/executor"
	"claude-squad/log"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CommandExecutor defines the interface for executing external commands
type CommandExecutor interface {
	Command(name string, args ...string) *exec.Cmd
	Output(cmd *exec.Cmd) ([]byte, error)
	LookPath(file string) (string, error)
}

// realCommandExecutor implements CommandExecutor using actual system commands
type realCommandExecutor struct{}

func (r *realCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (r *realCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func (r *realCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// timeoutCommandExecutor wraps command execution with timeout protection
// This prevents commands from hanging indefinitely, which is critical for
// preventing hangs on external commands like 'which claude'
type timeoutCommandExecutor struct {
	executor executor.Executor
	timeout  time.Duration
}

func newTimeoutCommandExecutor(timeout time.Duration) *timeoutCommandExecutor {
	return &timeoutCommandExecutor{
		executor: executor.NewTimeoutExecutor(timeout),
		timeout:  timeout,
	}
}

func (t *timeoutCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (t *timeoutCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	// Use the timeout executor's OutputWithPipes for reliable capture
	return t.executor.(*executor.TimeoutExecutor).OutputWithPipes(cmd)
}

func (t *timeoutCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Global command executor instance - uses timeout protection by default
// 5-second timeout prevents indefinite hangs on external commands
var globalCommandExecutor CommandExecutor = newTimeoutCommandExecutor(5 * time.Second)

// SetCommandExecutor sets the global command executor (primarily for testing)
func SetCommandExecutor(executor CommandExecutor) {
	globalCommandExecutor = executor
}

// ResetCommandExecutor resets the global command executor to the default implementation
// Uses timeout protection by default (5 seconds)
func ResetCommandExecutor() {
	globalCommandExecutor = newTimeoutCommandExecutor(5 * time.Second)
}

const (
	ConfigFileName = "config.json"
	defaultProgram = "proxy-claude"
)

// isTestMode detects if the application is running in test/benchmark mode
func isTestMode() bool {
	// Check command line arguments for test/benchmark indicators
	for _, arg := range os.Args {
		// Match test binary names and flags
		if strings.Contains(arg, ".test") ||
			strings.Contains(arg, "-test.") ||
			strings.HasSuffix(arg, ".test.exe") ||
			strings.Contains(arg, "-bench") {
			return true
		}
	}
	return false
}

// GetConfigDir returns the path to the application's configuration directory
// with hierarchical isolation for safe multi-instance and test execution.
//
// Priority hierarchy:
//  1. Test directory override via CLAUDE_SQUAD_TEST_DIR (for --test-mode flag)
//  2. Explicit instance ID via CLAUDE_SQUAD_INSTANCE environment variable
//  3. Test mode auto-detection (automatic isolation for tests/benchmarks)
//  4. Workspace-based isolation (default for production, per-directory state)
//  5. Global shared state (fallback, backward compatibility)
func GetConfigDir() (string, error) {
	// Priority 1: Test directory override (from --test-mode flag)
	if testDir := os.Getenv("CLAUDE_SQUAD_TEST_DIR"); testDir != "" {
		// Create the test directory if it doesn't exist
		if err := os.MkdirAll(testDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create test directory: %w", err)
		}
		return testDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".claude-squad")

	// Priority 2: Explicit instance ID (tests, named instances, backward compat)
	if instanceID := os.Getenv("CLAUDE_SQUAD_INSTANCE"); instanceID != "" {
		// Special value "shared" maintains backward compatibility
		if instanceID == "shared" {
			return baseDir, nil
		}
		return filepath.Join(baseDir, "instances", instanceID), nil
	}

	// Priority 3: Test mode auto-detection (automatic isolation)
	if isTestMode() {
		// Each test/benchmark process gets its own isolated state
		pid := os.Getpid()
		return filepath.Join(baseDir, "test", fmt.Sprintf("test-%d", pid)), nil
	}

	// Priority 4: Workspace-based isolation (production default)
	// Can be disabled with CLAUDE_SQUAD_WORKSPACE_MODE=false
	if os.Getenv("CLAUDE_SQUAD_WORKSPACE_MODE") != "false" {
		workDir, err := os.Getwd()
		if err == nil {
			// Hash the workspace path for a stable, filesystem-safe identifier
			hash := sha256.Sum256([]byte(workDir))
			workspaceID := fmt.Sprintf("%x", hash[:8])
			return filepath.Join(baseDir, "workspaces", workspaceID), nil
		}
		// If we can't get working directory, fall through to shared state
		log.WarningLog.Printf("Failed to get working directory for workspace isolation: %v", err)
	}

	// Priority 5: Global shared state (fallback, backward compatibility)
	return baseDir, nil
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// DetectNewSessions is a flag to enable detection of new sessions from other windows
	DetectNewSessions bool `json:"detect_new_sessions"`
	// SessionDetectionInterval is the interval (ms) at which the daemon checks for new sessions
	SessionDetectionInterval int `json:"session_detection_interval"`
	// StateRefreshInterval is the interval (ms) at which the state is refreshed from disk
	StateRefreshInterval int `json:"state_refresh_interval"`
	// LogsEnabled is a flag to enable logging to files
	LogsEnabled bool `json:"logs_enabled"`
	// LogsDir is the directory where logs are stored (defaults to ~/.claude-squad/logs)
	LogsDir string `json:"logs_dir"`
	// LogMaxSize is the maximum size of a log file in megabytes before it gets rotated
	LogMaxSize int `json:"log_max_size"`
	// LogMaxFiles is the maximum number of rotated log files to keep (not including the current log file)
	LogMaxFiles int `json:"log_max_files"`
	// LogMaxAge is the maximum number of days to keep rotated log files
	LogMaxAge int `json:"log_max_age"`
	// LogCompress is a flag to enable compression of rotated log files
	LogCompress bool `json:"log_compress"`
	// UseSessionLogs is a flag to enable per-session log files
	UseSessionLogs bool `json:"use_session_logs"`
	// TmuxSessionPrefix allows customizing the tmux session prefix for process isolation
	TmuxSessionPrefix string `json:"tmux_session_prefix"`
	// PerformBackgroundHealthChecks enables non-blocking health checks for session maintenance
	PerformBackgroundHealthChecks bool `json:"perform_background_health_checks"`
	// KeyCategories defines custom category mappings for key bindings in help system
	KeyCategories map[string]string `json:"key_categories"`
	// TerminalStreamingMode controls how terminal output is streamed to the client
	// Options: "raw" (direct PTY streaming), "state" (MOSH-style state sync), "hybrid" (both)
	TerminalStreamingMode string `json:"terminal_streaming_mode"`
	// VCSPreference controls which version control system to prefer when both are available
	// Options: "auto" (prefer JJ if available), "jj" (always use JJ), "git" (always use Git)
	VCSPreference string `json:"vcs_preference"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix: func() string {
			user, err := user.Current()
			if err != nil || user == nil || user.Username == "" {
				log.ErrorLog.Printf("failed to get current user: %v", err)
				return "session/"
			}
			return fmt.Sprintf("%s/", strings.ToLower(user.Username))
		}(),
		DetectNewSessions:        true,
		SessionDetectionInterval: 5000,
		StateRefreshInterval:     3000,
		LogsEnabled:              true,
		LogsDir:                  "", // Empty string means use default location
		LogMaxSize:               10, // 10MB
		LogMaxFiles:              5,  // Keep 5 rotated files
		LogMaxAge:                30, // 30 days
		LogCompress:              true,
		UseSessionLogs:                true,
		TmuxSessionPrefix:             "claudesquad_", // Default prefix for backward compatibility
		PerformBackgroundHealthChecks: true,            // Enabled by default for automated session maintenance
		KeyCategories:                 getDefaultKeyCategories(),
		TerminalStreamingMode:         "raw",  // Default to raw streaming (simpler, more reliable)
		VCSPreference:                 "auto", // Default to auto-detection (prefer JJ if available)
	}
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution (proxy-claude, then claude)
// 2. PATH lookup
//
// If both fail, it returns an error.
func GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Try to resolve aliases for both proxy-claude and claude
	candidates := []string{"proxy-claude", "claude"}

	for _, candidate := range candidates {
		// Attempt to get the alias definition from the shell
		var shellCmd string
		if strings.Contains(shell, "zsh") {
			// For zsh, use 'alias <name>' to get the full definition
			shellCmd = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; alias %s 2>/dev/null || which %s 2>/dev/null", candidate, candidate)
		} else if strings.Contains(shell, "bash") {
			// For bash, use 'alias <name>' to get the full definition
			shellCmd = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; alias %s 2>/dev/null || which %s 2>/dev/null", candidate, candidate)
		} else {
			shellCmd = fmt.Sprintf("which %s", candidate)
		}

		cmd := globalCommandExecutor.Command(shell, "-c", shellCmd)
		output, err := globalCommandExecutor.Output(cmd)
		if err == nil && len(output) > 0 {
			result := strings.TrimSpace(string(output))
			if result != "" {
				// Check if it's an alias definition
				// Formats:
				// 1. "claude: aliased to /path/to/command" (zsh alias output)
				// 2. "alias proxy-claude='command'" (bash/zsh alias definition)
				// 3. "proxy-claude='command'" (simplified alias format)
				// 4. "/path/to/command" (direct path from which)

				if strings.Contains(result, "aliased to ") {
					// Format: "name: aliased to /path/to/command"
					// Extract everything after "aliased to "
					parts := strings.SplitN(result, "aliased to ", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1]), nil
					}
				} else if strings.Contains(result, "alias ") {
					// Extract the command from alias definition
					// Pattern: alias name='command' or alias name="command"
					aliasRegex := regexp.MustCompile(`alias\s+\S+\s*=\s*['"](.+?)['"]`)
					matches := aliasRegex.FindStringSubmatch(result)
					if len(matches) > 1 {
						return matches[1], nil
					}
				} else if strings.Contains(result, "=") && (strings.Contains(result, "'") || strings.Contains(result, "\"")) {
					// Format: proxy-claude='command'
					aliasRegex := regexp.MustCompile(`\S+\s*=\s*['"](.+?)['"]`)
					matches := aliasRegex.FindStringSubmatch(result)
					if len(matches) > 1 {
						return matches[1], nil
					}
				} else {
					// It's just a path from 'which'
					return result, nil
				}
			}
		}
	}

	// Fallback: try to find in PATH directly
	for _, candidate := range candidates {
		path, err := globalCommandExecutor.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	// Apply defaults for fields that might not be in saved config (e.g., newly added fields)
	if config.KeyCategories == nil {
		config.KeyCategories = getDefaultKeyCategories()
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}

// getDefaultKeyCategories returns the default key category mappings
func getDefaultKeyCategories() map[string]string {
	return map[string]string{
		// Session Management
		"n":     "Session Management",
		"D":     "Session Management",
		"enter": "Session Management",
		"c":     "Session Management",
		"r":     "Session Management",

		// Git Integration
		"g": "Git Integration",
		"P": "Git Integration",

		// Navigation
		"up":    "Navigation",
		"down":  "Navigation",
		"left":  "Navigation",
		"right": "Navigation",
		"j":     "Navigation",
		"k":     "Navigation",
		"h":     "Navigation",
		"l":     "Navigation",
		"/":     "Navigation",
		"s":     "Navigation",

		// Organization
		"f":     "Organization",
		"C":     "Organization",
		"space": "Organization",

		// System
		"tab": "System",
		"?":   "System",
		"q":   "System",
		"esc": "System",
	}
}

// GetKeyCategoryForKey returns the category for a specific key, or empty string if not found
func (c *Config) GetKeyCategoryForKey(key string) string {
	if c.KeyCategories == nil {
		return ""
	}
	return c.KeyCategories[key]
}

// SetKeyCategory updates the category for a specific key
func (c *Config) SetKeyCategory(key, category string) {
	if c.KeyCategories == nil {
		c.KeyCategories = make(map[string]string)
	}
	c.KeyCategories[key] = category
}

// RemoveKeyCategory removes the category mapping for a specific key
func (c *Config) RemoveKeyCategory(key string) {
	if c.KeyCategories != nil {
		delete(c.KeyCategories, key)
	}
}
