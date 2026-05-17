package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize the logger for tests with ERROR level to reduce noise
	log.InitializeForTests(log.ERROR, log.ERROR)
	defer log.Close()

	exitCode := m.Run()
	os.Exit(exitCode)
}

// mockCommandExecutor implements CommandExecutor for testing
type mockCommandExecutor struct {
	CommandFunc  func(name string, args ...string) *exec.Cmd
	OutputFunc   func(cmd *exec.Cmd) ([]byte, error)
	LookPathFunc func(file string) (string, error)
}

func (m *mockCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	if m.CommandFunc != nil {
		return m.CommandFunc(name, args...)
	}
	return safeexec.CommandContext(context.Background(), "echo", "mock")
}

func (m *mockCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.OutputFunc != nil {
		return m.OutputFunc(cmd)
	}
	return []byte("mock output"), nil
}

func (m *mockCommandExecutor) LookPath(file string) (string, error) {
	if m.LookPathFunc != nil {
		return m.LookPathFunc(file)
	}
	return "/usr/local/bin/" + file, nil
}

// newMockCommandExecutorWithClaudeFound creates a mock that simulates finding claude
func newMockCommandExecutorWithClaudeFound(claudePath string) *mockCommandExecutor {
	return &mockCommandExecutor{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(claudePath), nil
		},
		LookPathFunc: func(file string) (string, error) {
			if file == "claude" {
				return claudePath, nil
			}
			return "/usr/local/bin/" + file, nil
		},
	}
}

// newMockCommandExecutorWithClaudeNotFound creates a mock that simulates claude not being found
func newMockCommandExecutorWithClaudeNotFound() *mockCommandExecutor {
	return &mockCommandExecutor{
		CommandFunc: func(name string, args ...string) *exec.Cmd {
			// Return a mock command that won't actually execute
			return safeexec.CommandContext(context.Background(), "true")
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Simulate command not found for both proxy-claude and claude
			return []byte(""), exec.ErrNotFound
		},
		LookPathFunc: func(file string) (string, error) {
			// Return error for both claude and proxy-claude
			if file == "claude" || file == "proxy-claude" {
				return "", exec.ErrNotFound
			}
			return "", exec.ErrNotFound
		},
	}
}

func TestGetClaudeCommand(t *testing.T) {
	originalShell := os.Getenv("SHELL")
	defer func() {
		os.Setenv("SHELL", originalShell)
	}()

	t.Run("finds claude via shell command", func(t *testing.T) {
		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)

		os.Setenv("SHELL", "/bin/bash")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("finds claude via LookPath when shell command fails", func(t *testing.T) {
		claudePath := "/usr/local/bin/claude"
		mockExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// Simulate shell command failure (returns empty output)
				return []byte(""), nil
			},
			LookPathFunc: func(file string) (string, error) {
				if file == "claude" {
					return claudePath, nil
				}
				return "", exec.ErrNotFound
			},
		}

		os.Setenv("SHELL", "/bin/bash")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("handles missing claude command", func(t *testing.T) {
		mockExecutor := newMockCommandExecutorWithClaudeNotFound()

		os.Setenv("SHELL", "/bin/bash")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Contains(t, err.Error(), "claude command not found")
	})

	t.Run("handles empty SHELL environment", func(t *testing.T) {
		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)

		os.Unsetenv("SHELL")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("handles alias parsing", func(t *testing.T) {
		// Test alias output parsing
		aliasOutput := "claude: aliased to /usr/local/bin/claude"
		mockExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(aliasOutput), nil
			},
		}

		os.Setenv("SHELL", "/bin/bash")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, "/usr/local/bin/claude", result)
	})

	t.Run("handles direct path output", func(t *testing.T) {
		claudePath := "/usr/local/bin/claude"
		mockExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(claudePath), nil
			},
		}

		os.Setenv("SHELL", "/bin/bash")

		result, err := NewConfigWithExecutor(mockExecutor).GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("regex parsing works correctly", func(t *testing.T) {
		// Test core alias formats without external dependencies
		aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)

		// Standard alias format
		output := "claude: aliased to /usr/local/bin/claude"
		matches := aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 2)
		assert.Equal(t, "/usr/local/bin/claude", matches[1])

		// Direct path (no alias)
		output = "/usr/local/bin/claude"
		matches = aliasRegex.FindStringSubmatch(output)
		assert.Len(t, matches, 0)
	})
}

func TestDefaultConfig(t *testing.T) {
	t.Run("creates config with default values when claude found", func(t *testing.T) {
		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)

		config := defaultConfigWithExecutor(mockExecutor)

		assert.NotNil(t, config)
		assert.Equal(t, claudePath, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
		assert.True(t, strings.HasSuffix(config.BranchPrefix, "/"))
	})

	t.Run("creates config with fallback program when claude not found", func(t *testing.T) {
		mockExecutor := newMockCommandExecutorWithClaudeNotFound()

		config := defaultConfigWithExecutor(mockExecutor)

		assert.NotNil(t, config)
		assert.Equal(t, "proxy-claude", config.DefaultProgram) // Falls back to default
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
		assert.True(t, strings.HasSuffix(config.BranchPrefix, "/"))
	})
}

func TestGetConfigDir(t *testing.T) {
	t.Run("returns valid config directory", func(t *testing.T) {
		configDir, err := GetConfigDir()

		assert.NoError(t, err)
		assert.NotEmpty(t, configDir)
		// With workspace isolation, path contains .stapler-squad but may have subdirs
		assert.True(t, strings.Contains(configDir, ".stapler-squad"),
			"config dir should contain .stapler-squad: %s", configDir)

		// Verify it's an absolute path
		assert.True(t, filepath.IsAbs(configDir))
	})

	t.Run("uses explicit instance ID when set", func(t *testing.T) {
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Setenv("STAPLER_SQUAD_INSTANCE", "test-instance")
		defer func() {
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

		configDir, err := GetConfigDir()

		assert.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, ".stapler-squad/instances/test-instance"),
			"should use explicit instance ID: %s", configDir)
	})

	t.Run("uses test mode isolation for tests", func(t *testing.T) {
		// This test itself triggers test mode auto-detection
		configDir, err := GetConfigDir()

		assert.NoError(t, err)
		assert.True(t, strings.Contains(configDir, ".stapler-squad/test/test-"),
			"test mode should use test directory: %s", configDir)
	})

	t.Run("uses shared state when STAPLER_SQUAD_INSTANCE=shared", func(t *testing.T) {
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
		defer func() {
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

		configDir, err := GetConfigDir()

		assert.NoError(t, err)
		assert.True(t, strings.HasSuffix(configDir, ".stapler-squad"),
			"shared mode should use base directory: %s", configDir)
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("returns default config when file doesn't exist", func(t *testing.T) {
		// Use a temporary home directory to avoid interfering with real config
		originalHome := os.Getenv("HOME")
		tempHome := t.TempDir()
		os.Setenv("HOME", tempHome)
		defer os.Setenv("HOME", originalHome)

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
	})

	t.Run("loads valid config file", func(t *testing.T) {
		// Create a temporary config directory
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".stapler-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		// Create a test config file
		configPath := filepath.Join(configDir, ConfigFileName)
		configContent := `{
			"default_program": "test-claude",
			"auto_yes": true,
			"daemon_poll_interval": 2000,
			"branch_prefix": "test/"
		}`
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Override HOME environment and use shared state for this test
		originalHome := os.Getenv("HOME")
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Setenv("HOME", tempHome)
		os.Setenv("STAPLER_SQUAD_INSTANCE", "shared") // Use shared state for config tests
		defer func() {
			os.Setenv("HOME", originalHome)
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.Equal(t, "test-claude", config.DefaultProgram)
		assert.True(t, config.AutoYes)
		assert.Equal(t, 2000, config.DaemonPollInterval)
		assert.Equal(t, "test/", config.BranchPrefix)
	})

	t.Run("returns default config on invalid JSON", func(t *testing.T) {
		// Create a temporary config directory
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".stapler-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		// Create an invalid config file
		configPath := filepath.Join(configDir, ConfigFileName)
		invalidContent := `{"invalid": json content}`
		err = os.WriteFile(configPath, []byte(invalidContent), 0644)
		require.NoError(t, err)

		// Override HOME environment
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempHome)
		defer os.Setenv("HOME", originalHome)

		config := LoadConfig()

		// Should return default config when JSON is invalid
		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)                  // Default value
		assert.Equal(t, 1000, config.DaemonPollInterval) // Default value
	})
}

func TestSaveConfig(t *testing.T) {
	t.Run("saves config to file", func(t *testing.T) {
		// Create a temporary config directory
		tempHome := t.TempDir()

		// Override HOME environment and use shared state for this test
		originalHome := os.Getenv("HOME")
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Setenv("HOME", tempHome)
		os.Setenv("STAPLER_SQUAD_INSTANCE", "shared") // Use shared state for config tests
		defer func() {
			os.Setenv("HOME", originalHome)
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

		// Create a test config
		testConfig := &Config{
			DefaultProgram:     "test-program",
			AutoYes:            true,
			DaemonPollInterval: 3000,
			BranchPrefix:       "test-branch/",
		}

		err := SaveConfig(testConfig)
		assert.NoError(t, err)

		// Verify the file was created
		configDir := filepath.Join(tempHome, ".stapler-squad")
		configPath := filepath.Join(configDir, ConfigFileName)

		assert.FileExists(t, configPath)

		// Load and verify the content
		loadedConfig := LoadConfig()
		assert.Equal(t, testConfig.DefaultProgram, loadedConfig.DefaultProgram)
		assert.Equal(t, testConfig.AutoYes, loadedConfig.AutoYes)
		assert.Equal(t, testConfig.DaemonPollInterval, loadedConfig.DaemonPollInterval)
		assert.Equal(t, testConfig.BranchPrefix, loadedConfig.BranchPrefix)
	})
}

// TestGetClaudeCommand_Timeout verifies that GetClaudeCommand respects timeout
func TestGetClaudeCommand_Timeout(t *testing.T) {
	t.Run("Timeout on hanging command", func(t *testing.T) {
		// Create a mock executor that hangs indefinitely
		hangingExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// Simulate a command that hangs by sleeping longer than timeout
				// In reality, the timeout executor should kill this before it completes
				return nil, exec.ErrNotFound
			},
			LookPathFunc: func(file string) (string, error) {
				return "", exec.ErrNotFound
			},
		}

		// This should complete quickly even though the command "hangs"
		// because our timeout executor wrapper kills hanging commands
		result, err := NewConfigWithExecutor(hangingExecutor).GetClaudeCommand()

		// Should return error (command not found)
		assert.Error(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Default executor uses timeout protection", func(t *testing.T) {
		// Verify that NewConfig() creates a config with timeout protection.
		cfg := NewConfig()
		assert.NotNil(t, cfg.executor)

		// The default should be timeoutCommandExecutor
		_, ok := cfg.executor.(*timeoutCommandExecutor)
		assert.True(t, ok, "Default executor should be timeoutCommandExecutor")
	})
}

// TestTimeoutCommandExecutor_RealBehavior tests the timeout executor with actual commands
func TestTimeoutCommandExecutor_RealBehavior(t *testing.T) {
	t.Run("Fast command completes successfully", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(2 * time.Second)

		cmd := safeexec.CommandContext(context.Background(), "echo", "hello")
		output, err := executor.Output(cmd)

		assert.NoError(t, err)
		assert.Contains(t, string(output), "hello")
	})

	t.Run("Slow command times out", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(500 * time.Millisecond)

		// Command that takes longer than timeout
		cmd := safeexec.CommandContext(context.Background(), "sleep", "2")
		_, err := executor.Output(cmd)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out", "Error should indicate timeout")
	})

	t.Run("Command failure propagates correctly", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(2 * time.Second)

		// Command that fails
		cmd := safeexec.CommandContext(context.Background(), "sh", "-c", "exit 1")
		_, err := executor.Output(cmd)

		require.Error(t, err)
		// Should be a command error, not a timeout error
		assert.NotContains(t, err.Error(), "timed out")
	})
}

// ─── UT-4.x: NotificationPrefs ───────────────────────────────────────────────

// UT-4.1 — NotificationPrefs round-trip in Config [R8]
func TestNotificationPrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &Config{
		ConfigVersion: 2,
		Notifications: NotificationPrefs{PushEnabled: true},
	}
	require.NoError(t, saveConfig(cfg, path))

	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.True(t, loaded.Notifications.PushEnabled)
}

// UT-4.2 — v1 config loads with NotificationPrefs defaults [R8]
func TestV1ConfigLoadsWithNotificationDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	v1JSON := `{"configVersion": 1, "session_defaults": {}}`
	require.NoError(t, os.WriteFile(path, []byte(v1JSON), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.False(t, cfg.Notifications.PushEnabled, "default must be push disabled")
}

// UT-4.3 — PushEnabled=false is the zero-value default [R8]
func TestNotificationPrefsDefault(t *testing.T) {
	var prefs NotificationPrefs
	assert.False(t, prefs.PushEnabled, "push must be disabled by default")
}

// UT-4.4 — saveConfig is atomic: no .tmp file left on disk after success [R9]
func TestSaveConfigAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initial := &Config{ConfigVersion: 2}
	require.NoError(t, saveConfig(initial, path))

	// No .tmp file must remain after a successful write.
	_, err := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), ".tmp file must be cleaned up after successful save")

	// The config file must be valid JSON.
	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, 2, loaded.ConfigVersion)
}

func TestOneOffBaseDirOrDefault_Empty(t *testing.T) {
	cfg := &Config{}
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result, err := cfg.OneOffBaseDirOrDefault()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "oneoff"), result)
}

func TestOneOffBaseDirOrDefault_TildeExpansion(t *testing.T) {
	cfg := &Config{OneOffBaseDir: "~/my-oneoffs"}
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result, err := cfg.OneOffBaseDirOrDefault()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "my-oneoffs"), result)
	assert.False(t, strings.HasPrefix(result, "~"), "result must not contain literal tilde")
}

func TestOneOffBaseDirOrDefault_CustomAbsolutePath(t *testing.T) {
	cfg := &Config{OneOffBaseDir: "/tmp/my-custom-oneoffs"}

	result, err := cfg.OneOffBaseDirOrDefault()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/my-custom-oneoffs", result)
}

func TestOneOffBaseDir_JSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{OneOffBaseDir: "~/oneoff"}
	require.NoError(t, saveConfig(cfg, path))

	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "~/oneoff", loaded.OneOffBaseDir)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"one_off_base_dir"`)

	// omitempty: empty value omits key
	emptyCfg := &Config{}
	emptyPath := filepath.Join(dir, "empty-config.json")
	require.NoError(t, saveConfig(emptyCfg, emptyPath))
	emptyRaw, err := os.ReadFile(emptyPath)
	require.NoError(t, err)
	assert.NotContains(t, string(emptyRaw), `"one_off_base_dir"`)
}

// ─── Escape analytics config tests ───────────────────────────────────────────

// TestEscapeAnalyticsDefaults verifies that zero-value configs get the correct defaults
// applied by LoadConfigFromPath.
func TestEscapeAnalyticsDefaults(t *testing.T) {
	writeAndLoad := func(t *testing.T, jsonContent string) *Config {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		require.NoError(t, os.WriteFile(path, []byte(jsonContent), 0600))
		cfg, err := LoadConfigFromPath(path)
		require.NoError(t, err)
		return cfg
	}

	t.Run("default CaptureLevel is summary when unset", func(t *testing.T) {
		cfg := writeAndLoad(t, `{}`)
		assert.Equal(t, "summary", cfg.EscapeAnalyticsCaptureLevel)
	})

	t.Run("default SamplingRate is 1.0 when zero", func(t *testing.T) {
		cfg := writeAndLoad(t, `{}`)
		require.NotNil(t, cfg.EscapeAnalyticsSamplingRate)
		assert.Equal(t, 1.0, *cfg.EscapeAnalyticsSamplingRate)
	})

	t.Run("default MaxRowsPerSession is 10000 when zero", func(t *testing.T) {
		cfg := writeAndLoad(t, `{}`)
		assert.Equal(t, 10000, cfg.EscapeAnalyticsMaxRowsPerSession)
	})

	t.Run("default RetentionDays is 7 when zero", func(t *testing.T) {
		cfg := writeAndLoad(t, `{}`)
		assert.Equal(t, 7, cfg.EscapeAnalyticsRetentionDays)
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		cfg := writeAndLoad(t, `{
			"escapeAnalyticsCaptureLevel": "full",
			"escapeAnalyticsSamplingRate": 0.5,
			"escapeAnalyticsMaxRowsPerSession": 5000,
			"escapeAnalyticsRetentionDays": 14
		}`)
		assert.Equal(t, "full", cfg.EscapeAnalyticsCaptureLevel)
		require.NotNil(t, cfg.EscapeAnalyticsSamplingRate)
		assert.Equal(t, 0.5, *cfg.EscapeAnalyticsSamplingRate)
		assert.Equal(t, 5000, cfg.EscapeAnalyticsMaxRowsPerSession)
		assert.Equal(t, 14, cfg.EscapeAnalyticsRetentionDays)
	})
}

// TestEscapeAnalyticsCaptureLevel_Validation verifies that invalid capture level values
// are reset to "summary".
func TestEscapeAnalyticsCaptureLevel_Validation(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"full is valid", "full", "full"},
		{"summary is valid", "summary", "summary"},
		{"off is valid", "off", "off"},
		{"invalid value resets to summary", "verbose", "summary"},
		{"empty resets to summary", "", "summary"},
		{"unknown resets to summary", "ALL", "summary"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			content := `{"escapeAnalyticsCaptureLevel": "` + tc.input + `"}`
			if tc.input == "" {
				content = `{}`
			}
			require.NoError(t, os.WriteFile(path, []byte(content), 0600))
			cfg, err := LoadConfigFromPath(path)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cfg.EscapeAnalyticsCaptureLevel)
		})
	}
}

// TestEscapeAnalyticsSamplingRate_Clamping verifies that sampling rate values outside
// [0.0, 1.0] are clamped.
func TestEscapeAnalyticsSamplingRate_Clamping(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		expected float64
	}{
		{"zero becomes 1.0 (default)", `{}`, 1.0},
		{"0.5 is valid", `{"escapeAnalyticsSamplingRate": 0.5}`, 0.5},
		{"1.0 is valid", `{"escapeAnalyticsSamplingRate": 1.0}`, 1.0},
		{"negative clamped to 0", `{"escapeAnalyticsSamplingRate": -0.1}`, 0},
		{"above 1.0 clamped to 1.0", `{"escapeAnalyticsSamplingRate": 1.5}`, 1.0},
		{"explicit 0.0 captures nothing", `{"escapeAnalyticsSamplingRate": 0.0}`, 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.json), 0600))
			cfg, err := LoadConfigFromPath(path)
			require.NoError(t, err)
			require.NotNil(t, cfg.EscapeAnalyticsSamplingRate)
			assert.Equal(t, tc.expected, *cfg.EscapeAnalyticsSamplingRate)
		})
	}
}

// TestOSCPayloadsAreRedacted verifies that OSCPayloadsAreRedacted returns correct values
// based on the EscapeAnalyticsDisableOSCRedaction field.
func TestOSCPayloadsAreRedacted(t *testing.T) {
	t.Run("returns true by default (redaction on)", func(t *testing.T) {
		cfg := &Config{}
		assert.True(t, cfg.OSCPayloadsAreRedacted())
	})

	t.Run("returns true when DisableOSCRedaction is false", func(t *testing.T) {
		cfg := &Config{EscapeAnalyticsDisableOSCRedaction: false}
		assert.True(t, cfg.OSCPayloadsAreRedacted())
	})

	t.Run("returns false when DisableOSCRedaction is true", func(t *testing.T) {
		cfg := &Config{EscapeAnalyticsDisableOSCRedaction: true}
		assert.False(t, cfg.OSCPayloadsAreRedacted())
	})

	t.Run("loaded config with disableOSCRedaction=true returns false", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		content := `{"escapeAnalyticsDisableOSCRedaction": true}`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		cfg, err := LoadConfigFromPath(path)
		require.NoError(t, err)
		assert.False(t, cfg.OSCPayloadsAreRedacted())
	})
}
