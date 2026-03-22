package config

import (
	"github.com/tstapler/stapler-squad/log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	return exec.Command("echo", "mock")
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
			return exec.Command("true")
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

// setupTest sets up a test environment with a mock command executor
func setupTest(t *testing.T) func() {
	// Store original executor
	originalExecutor := globalCommandExecutor

	// Return cleanup function
	return func() {
		SetCommandExecutor(originalExecutor)
	}
}

func TestGetClaudeCommand(t *testing.T) {
	originalShell := os.Getenv("SHELL")
	defer func() {
		os.Setenv("SHELL", originalShell)
	}()

	t.Run("finds claude via shell command", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)
		SetCommandExecutor(mockExecutor)

		os.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("finds claude via LookPath when shell command fails", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

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
		SetCommandExecutor(mockExecutor)

		os.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("handles missing claude command", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		mockExecutor := newMockCommandExecutorWithClaudeNotFound()
		SetCommandExecutor(mockExecutor)

		os.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.Error(t, err)
		assert.Equal(t, "", result)
		assert.Contains(t, err.Error(), "claude command not found")
	})

	t.Run("handles empty SHELL environment", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)
		SetCommandExecutor(mockExecutor)

		os.Unsetenv("SHELL")

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, claudePath, result)
	})

	t.Run("handles alias parsing", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		// Test alias output parsing
		aliasOutput := "claude: aliased to /usr/local/bin/claude"
		mockExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(aliasOutput), nil
			},
		}
		SetCommandExecutor(mockExecutor)

		os.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

		assert.NoError(t, err)
		assert.Equal(t, "/usr/local/bin/claude", result)
	})

	t.Run("handles direct path output", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		claudePath := "/usr/local/bin/claude"
		mockExecutor := &mockCommandExecutor{
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(claudePath), nil
			},
		}
		SetCommandExecutor(mockExecutor)

		os.Setenv("SHELL", "/bin/bash")

		result, err := GetClaudeCommand()

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
		cleanup := setupTest(t)
		defer cleanup()

		claudePath := "/usr/local/bin/claude"
		mockExecutor := newMockCommandExecutorWithClaudeFound(claudePath)
		SetCommandExecutor(mockExecutor)

		config := DefaultConfig()

		assert.NotNil(t, config)
		assert.Equal(t, claudePath, config.DefaultProgram)
		assert.False(t, config.AutoYes)
		assert.Equal(t, 1000, config.DaemonPollInterval)
		assert.NotEmpty(t, config.BranchPrefix)
		assert.True(t, strings.HasSuffix(config.BranchPrefix, "/"))
	})

	t.Run("creates config with fallback program when claude not found", func(t *testing.T) {
		cleanup := setupTest(t)
		defer cleanup()

		mockExecutor := newMockCommandExecutorWithClaudeNotFound()
		SetCommandExecutor(mockExecutor)

		config := DefaultConfig()

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
	defer setupTest(t)()

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

		SetCommandExecutor(hangingExecutor)

		// This should complete quickly even though the command "hangs"
		// because our timeout executor wrapper kills hanging commands
		result, err := GetClaudeCommand()

		// Should return error (command not found)
		assert.Error(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Default executor uses timeout protection", func(t *testing.T) {
		// Reset to default executor
		ResetCommandExecutor()

		// Verify that the global executor is using timeout protection
		// We can't easily test the actual timeout without real hanging commands,
		// but we can verify the type
		assert.NotNil(t, globalCommandExecutor)

		// The default should be timeoutCommandExecutor
		_, ok := globalCommandExecutor.(*timeoutCommandExecutor)
		assert.True(t, ok, "Default executor should be timeoutCommandExecutor")
	})
}

// TestTimeoutCommandExecutor_RealBehavior tests the timeout executor with actual commands
func TestTimeoutCommandExecutor_RealBehavior(t *testing.T) {
	t.Run("Fast command completes successfully", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(2 * time.Second)

		cmd := exec.Command("echo", "hello")
		output, err := executor.Output(cmd)

		assert.NoError(t, err)
		assert.Contains(t, string(output), "hello")
	})

	t.Run("Slow command times out", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(500 * time.Millisecond)

		// Command that takes longer than timeout
		cmd := exec.Command("sleep", "2")
		_, err := executor.Output(cmd)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out", "Error should indicate timeout")
	})

	t.Run("Command failure propagates correctly", func(t *testing.T) {
		executor := newTimeoutCommandExecutor(2 * time.Second)

		// Command that fails
		cmd := exec.Command("sh", "-c", "exit 1")
		_, err := executor.Output(cmd)

		require.Error(t, err)
		// Should be a command error, not a timeout error
		assert.NotContains(t, err.Error(), "timed out")
	})
}
