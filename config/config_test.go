package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

// TestPruneStaleTestDirs is the regression test for the leak that filled
// ~/.stapler-squad/test with 13,530 orphaned test-<pid> dirs (6.1G) going
// back to March 2026: every go-test binary created one via GetConfigDirForDir
// but nothing ever removed it. Verifies a dead pid's dir gets swept while a
// live pid's dir and unrelated entries survive.
func TestPruneStaleTestDirs(t *testing.T) {
	testBaseDir := t.TempDir()

	deadCmd := safeexec.CommandContext(context.Background(), "true")
	require.NoError(t, deadCmd.Run())
	deadPID := deadCmd.Process.Pid

	alivePID := os.Getpid()

	deadDir := filepath.Join(testBaseDir, fmt.Sprintf("test-%d", deadPID))
	aliveDir := filepath.Join(testBaseDir, fmt.Sprintf("test-%d", alivePID))
	junkDir := filepath.Join(testBaseDir, "not-a-test-dir")
	require.NoError(t, os.MkdirAll(deadDir, 0755))
	require.NoError(t, os.MkdirAll(aliveDir, 0755))
	require.NoError(t, os.MkdirAll(junkDir, 0755))

	pruneStaleTestDirs(testBaseDir)

	assert.NoDirExists(t, deadDir, "dead process's test dir should be pruned")
	assert.DirExists(t, aliveDir, "live process's test dir must survive")
	assert.DirExists(t, junkDir, "non test-<pid> entries must be left alone")
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
		// GetConfigDir checks STAPLER_SQUAD_TEST_DIR (priority 1) and
		// STAPLER_SQUAD_INSTANCE (priority 2) before falling through to test
		// mode auto-detection (priority 3). Both can be set in the ambient
		// environment this test process inherits (e.g. a stapler-squad
		// session sets STAPLER_SQUAD_INSTANCE for its own tooling), which
		// would otherwise short-circuit test mode detection and make this
		// test order- and environment-dependent. Clear both explicitly so
		// this test always exercises pure test mode auto-detection.
		originalTestDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
		os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		defer func() {
			if originalTestDir == "" {
				os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
			} else {
				os.Setenv("STAPLER_SQUAD_TEST_DIR", originalTestDir)
			}
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

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

	// This pins the LoadConfig TOCTOU fix's default-fallback save path for the
	// shared-state case (STAPLER_SQUAD_INSTANCE=shared), which resolves straight
	// to baseDir (HOME/.stapler-squad) per GetConfigDirForDir's Priority 2 —
	// bypassing test-mode auto-detection (Priority 3), which is unconditionally
	// true inside a `go test` binary and would otherwise make the real
	// HOME-derived path unreachable here (see TestGetConfigDir's "uses shared
	// state when STAPLER_SQUAD_INSTANCE=shared" subtest for the same pattern).
	t.Run("returns default config and persists it when STAPLER_SQUAD_INSTANCE=shared", func(t *testing.T) {
		originalHome := os.Getenv("HOME")
		originalTestDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		tempHome := t.TempDir()
		os.Setenv("HOME", tempHome)
		os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
		os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
		// The default-fallback save path doesn't MkdirAll the target directory
		// (a pre-existing, separate gap from the TOCTOU fix under test) — create
		// it up front, matching how the sibling "loads valid config file" and
		// "backfills quota defaults" subtests below already pre-create their dirs.
		require.NoError(t, os.MkdirAll(filepath.Join(tempHome, ".stapler-squad"), 0755))
		defer func() {
			os.Setenv("HOME", originalHome)
			if originalTestDir == "" {
				os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
			} else {
				os.Setenv("STAPLER_SQUAD_TEST_DIR", originalTestDir)
			}
			if originalInstance == "" {
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			} else {
				os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
			}
		}()

		config := LoadConfig()

		assert.NotNil(t, config)
		assert.NotEmpty(t, config.DefaultProgram)
		assert.False(t, config.AutoYes)

		expectedPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
		data, err := os.ReadFile(expectedPath)
		require.NoError(t, err, "default config should have been persisted to the HOME-derived path")
		var persisted Config
		require.NoError(t, json.Unmarshal(data, &persisted))
		assert.Equal(t, config.DefaultProgram, persisted.DefaultProgram)
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

	t.Run("backfills quota defaults when existing config file missing quota field", func(t *testing.T) {
		// A config.json written before this feature shipped has no "quota" key.
		tempHome := t.TempDir()
		configDir := filepath.Join(tempHome, ".stapler-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		configPath := filepath.Join(configDir, ConfigFileName)
		configContent := `{"default_program": "test-claude"}`
		err = os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		originalHome := os.Getenv("HOME")
		originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
		os.Setenv("HOME", tempHome)
		os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
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
		assert.False(t, config.Quota.Enabled)
		assert.Equal(t, 20.0, config.Quota.PauseBelowHeadroomPct)
		assert.Equal(t, 15.0, config.Quota.ResumeMarginPct)
		assert.Equal(t, 2, config.Quota.ConsecutiveTicksToPause)
		assert.Equal(t, 3, config.Quota.ConsecutiveTicksToResume)
		assert.Equal(t, 30, config.Quota.RateLimitWindowMinutes)
		assert.Equal(t, 10, config.Quota.ManualOverrideGraceMinutes)
		assert.Equal(t, 300, config.Quota.ForegroundThrottleDelaySeconds)
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

// TestLoadConfigWithDefaultFallback_UsesGivenConfigPathNotLiveEnvReRead pins the
// LoadConfig TOCTOU fix: the fallback path (config file doesn't exist yet)
// must save the default config to the SAME path it was given (the one LoadConfig
// already resolved and found missing), not re-derive the path via a second,
// live GetConfigDir() call inside the fallback itself.
//
// GetConfigDirForDir does a live, uncached os.Getenv("STAPLER_SQUAD_TEST_DIR")
// read on every call (config.go:127). Two overlapping callers sharing this
// process each t.Setenv the same env var to their own isolated dir; if
// loadConfigWithDefaultFallback re-resolved the path instead of using the
// configPath parameter it was called with, a repoint of that shared env var
// after path resolution but before the fallback save would make the save land
// at whatever dir the env var *now* points to — silently overwriting another
// caller's already-written, profile-bearing config (the observed
// `--verbose` -> "" symptom).
//
// Unlike a prior version of this test (which only ever called saveConfig()
// directly with pre-resolved paths — safe even without the fix, and so unable
// to catch a regression), this calls the actual production function under
// test, loadConfigWithDefaultFallback, so reintroducing the live-re-read bug
// makes this test fail.
func TestLoadConfigWithDefaultFallback_UsesGivenConfigPathNotLiveEnvReRead(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// Resolve iteration A's config path (mirrors the top of LoadConfig) and
	// confirm the file doesn't exist yet, just like the fallback branch expects.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dirA)
	configDirA, err := GetConfigDir()
	require.NoError(t, err)
	configPathA := filepath.Join(configDirA, ConfigFileName)
	_, err = os.ReadFile(configPathA)
	require.True(t, os.IsNotExist(err), "precondition: iteration A's config must not exist yet")

	// Before calling the fallback for configPathA, the shared env var is
	// repointed at dirB (simulating a concurrent caller) and dirB gets its own
	// profile-bearing config written.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dirB)
	configDirB, err := GetConfigDir()
	require.NoError(t, err)
	configPathB := filepath.Join(configDirB, ConfigFileName)
	require.NoError(t, saveConfig(&Config{DefaultProgram: "claude --verbose"}, configPathB))

	// The env var now points at dirB, but we call the fallback with iteration
	// A's already-resolved configPathA. A correct implementation saves to
	// configPathA regardless of what GetConfigDir() would resolve to right
	// now; the bug re-derived the path via a second internal GetConfigDir()
	// call and would silently write into dirB/configPathB instead.
	got := loadConfigWithDefaultFallback(configPathA)
	require.NotNil(t, got)

	if _, statErr := os.Stat(configPathA); statErr != nil {
		t.Fatalf("fallback save must land at the given configPath (%s): %v", configPathA, statErr)
	}

	loadedB, err := LoadConfigFromPath(configPathB)
	require.NoError(t, err)
	assert.Equal(t, "claude --verbose", loadedB.DefaultProgram,
		"iteration B's config must not be clobbered by a stale live-env re-read inside the fallback")
}

// TestLoadConfig_ConcurrentFirstLoad_NoTOCTOURace is a -race stress test that
// exercises LoadConfig() itself (not a helper called around it) from many
// goroutines the first time a config path is created, to catch a reintroduced
// TOCTOU race between the "does it exist" check and the fallback save (e.g. a
// missing or wrongly-scoped per-path lock in loadConfigWithDefaultFallback).
// Run with `go test -race`: a data race on the shared config file, or a
// corrupt/partial config.json from an unserialized concurrent write, fails
// this test.
func TestLoadConfig_ConcurrentFirstLoad_NoTOCTOURace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)

	const numGoroutines = 20
	var wg sync.WaitGroup
	results := make([]*Config, numGoroutines)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = LoadConfig()
		}(i)
	}
	wg.Wait()

	for i, cfg := range results {
		require.NotNilf(t, cfg, "goroutine %d: LoadConfig returned nil", i)
	}

	configDir, err := GetConfigDir()
	require.NoError(t, err)
	configPath := filepath.Join(configDir, ConfigFileName)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var onDisk Config
	require.NoErrorf(t, json.Unmarshal(data, &onDisk), "config file must be valid JSON, not corrupted by a concurrent TOCTOU write race: %s", string(data))
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
		// Verify that NewConfig() creates a config with a non-nil executor.
		cfg := NewConfig()
		assert.NotNil(t, cfg.executor)

		// In test mode the default executor is lookPathOnlyExecutor (avoids slow
		// shell config sourcing); in production it is timeoutCommandExecutor.
		_, isTimeout := cfg.executor.(*timeoutCommandExecutor)
		_, isLookPath := cfg.executor.(*lookPathOnlyExecutor)
		assert.True(t, isTimeout || isLookPath, "Default executor should be timeoutCommandExecutor or lookPathOnlyExecutor, got %T", cfg.executor)
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

// TestLoadConfigFromPath_should_ResolveAC7Defaults_When_RetryPolicyKeyMissing
// round-trips a config.json predating the session-retry-backoff feature (no
// "retry_policy" key at all) and asserts it resolves to today's exact
// single-retry, immediate-restart behavior — AC7.
func TestLoadConfigFromPath_should_ResolveAC7Defaults_When_RetryPolicyKeyMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	preFeatureJSON := `{"configVersion": 2, "session_defaults": {}}`
	require.NoError(t, os.WriteFile(path, []byte(preFeatureJSON), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)

	assert.True(t, cfg.RetryPolicy.EnabledOrDefault(), "a pre-feature config.json must resolve to retry enabled")
	assert.Equal(t, 1, cfg.RetryPolicy.MaxAttemptsOrDefault(), "must resolve to today's exact single-retry behavior")
	assert.Equal(t, 0, cfg.RetryPolicy.InitialDelaySeconds, "must resolve to today's immediate-restart behavior")
	assert.Equal(t, "exponential", cfg.RetryPolicy.Backoff, "load-time normalization must fill in the default strategy")
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

// TestSaveConfig_ConcurrentWritesToSamePath_NeverProduceCorruptJSON pins the
// saveConfigMu regression: without it, two goroutines racing saveConfig
// against the same configPath can interleave WriteFile/Rename on the shared
// ".tmp" file, producing a rename failure or a torn write that LoadConfig
// silently swallows into DefaultConfig() (see saveConfigMu's doc comment).
// Mirrors TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON
// in server/services/hook_injector_test.go.
func TestSaveConfig_ConcurrentWritesToSamePath_NeverProduceCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := &Config{ConfigVersion: i}
			errCh <- saveConfig(cfg, path)
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err, "saveConfig must not error under concurrent writers to the same path")
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "config file must exist after concurrent saveConfig calls")

	var parsed Config
	require.NoError(t, json.Unmarshal(data, &parsed), "config.json must be valid JSON after concurrent writes, not torn/corrupt: %s", data)

	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	// Every writer used a distinct ConfigVersion in [0, n). A torn/corrupt
	// write that LoadConfig silently swallowed into DefaultConfig() would
	// surface here as a value outside that range instead.
	assert.True(t, loaded.ConfigVersion >= 0 && loaded.ConfigVersion < n,
		"loaded ConfigVersion %d must be one written by a goroutine, not a fallback default", loaded.ConfigVersion)
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

// ─── RemoteConfig tests (ssh-remote-workspaces Phase 3, Epic 3.1) ────────────

// TestRemoteConfigRoundTrip covers plan.md Story 3.1.1's acceptance criterion:
// a RemoteConfig saved via config.Save round-trips through config.json with
// its exact field values intact.
func TestRemoteConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Remotes: []RemoteConfig{
			{
				Name:        "prod-box",
				Host:        "prod.example.com",
				User:        "tyler",
				BasePath:    "/srv/workspaces",
				IdentityRef: "ssh-key:prod-box",
			},
		},
	}
	require.NoError(t, saveConfig(cfg, path))

	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	require.Len(t, loaded.Remotes, 1)
	assert.Equal(t, "prod-box", loaded.Remotes[0].Name)
	assert.Equal(t, "prod.example.com", loaded.Remotes[0].Host)
	assert.Equal(t, "tyler", loaded.Remotes[0].User)
	assert.Equal(t, "/srv/workspaces", loaded.Remotes[0].BasePath)
	assert.Equal(t, "ssh-key:prod-box", loaded.Remotes[0].IdentityRef)

	// omitempty: no configured remotes omits the "remotes" key entirely.
	emptyCfg := &Config{}
	emptyPath := filepath.Join(dir, "empty-config.json")
	require.NoError(t, saveConfig(emptyCfg, emptyPath))
	emptyRaw, err := os.ReadFile(emptyPath)
	require.NoError(t, err)
	assert.NotContains(t, string(emptyRaw), `"remotes"`)
}

// TestRemoteConfig_SavedJSONContainsNoPlaintextSecret pins plan.md Story
// 3.1.1's "no secret material in config.json" acceptance criterion: a saved
// RemoteConfig's raw JSON never contains a PEM-shaped value or a
// private-key/passphrase field, because RemoteConfig has no such field —
// IdentityRef is only an opaque pointer resolved against the OS keychain
// (sshremote.KeyStore, Epic 3.2), never the key material itself.
func TestRemoteConfig_SavedJSONContainsNoPlaintextSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Remotes: []RemoteConfig{
			{
				Name:        "prod-box",
				Host:        "prod.example.com",
				User:        "tyler",
				BasePath:    "/srv/workspaces",
				IdentityRef: "ssh-key:prod-box",
			},
		},
	}
	require.NoError(t, saveConfig(cfg, path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	rawJSON := string(raw)

	assert.False(t, strings.Contains(rawJSON, "BEGIN"), "config.json must not contain PEM-shaped content")
	assert.NotContains(t, rawJSON, "private_key")
	assert.NotContains(t, rawJSON, "passphrase")

	// Sanity: the remote's non-secret fields are actually present, so this
	// test isn't vacuously passing against an empty/failed save.
	assert.Contains(t, rawJSON, `"remotes"`)
	assert.Contains(t, rawJSON, "prod-box")
}

// TestConfig_RemoteByName covers the lookup helper consumed by session
// creation (Phase 4) and Settings UI validation (Phase 6).
func TestConfig_RemoteByName(t *testing.T) {
	cfg := &Config{
		Remotes: []RemoteConfig{
			{Name: "prod-box", Host: "prod.example.com", User: "tyler", BasePath: "/srv/workspaces", IdentityRef: "ssh-key:prod-box"},
			{Name: "staging-box", Host: "staging.example.com", User: "tyler", BasePath: "/srv/workspaces", IdentityRef: "ssh-key:staging-box"},
		},
	}

	found, ok := cfg.RemoteByName("staging-box")
	require.True(t, ok)
	require.NotNil(t, found)
	assert.Equal(t, "staging.example.com", found.Host)

	_, ok = cfg.RemoteByName("does-not-exist")
	assert.False(t, ok)

	var nilCfg *Config
	_, ok = nilCfg.RemoteByName("prod-box")
	assert.False(t, ok, "RemoteByName must be nil-safe")
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

// TestDefaultConfigMirrorsEscapeAnalyticsDefaults is a regression test for BUG-025:
// DefaultConfig() must produce the same escape analytics defaults as
// LoadConfigFromPath's post-decode defaulting (see the comment above the
// SessionDefaults init in DefaultConfig — the two code paths must be
// equivalent). Before this fix, DefaultConfig() left these fields at their Go
// zero values, so a fresh install's very first LoadConfig() call (which
// returns DefaultConfig() directly, before any config.json exists) would pass
// EscapeAnalyticsMaxRowsPerSession=0 into the batch writer at server startup —
// disabling the per-session row cap instead of applying the intended default.
func TestDefaultConfigMirrorsEscapeAnalyticsDefaults(t *testing.T) {
	fresh := DefaultConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	loaded, err := LoadConfigFromPath(path)
	require.NoError(t, err)

	assert.Equal(t, loaded.EscapeAnalyticsCaptureLevel, fresh.EscapeAnalyticsCaptureLevel)
	require.NotNil(t, fresh.EscapeAnalyticsSamplingRate)
	require.NotNil(t, loaded.EscapeAnalyticsSamplingRate)
	assert.Equal(t, *loaded.EscapeAnalyticsSamplingRate, *fresh.EscapeAnalyticsSamplingRate)
	assert.Equal(t, loaded.EscapeAnalyticsMaxRowsPerSession, fresh.EscapeAnalyticsMaxRowsPerSession)
	assert.Equal(t, loaded.EscapeAnalyticsRetentionDays, fresh.EscapeAnalyticsRetentionDays)
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

// ─── U-GO-16, U-GO-17, U-GO-18: GetFeatureFlag default-off ─────────────────

// TestGetFeatureFlag_defaultsFalse verifies backlog flag defaults to false on empty Config.
func TestGetFeatureFlag_defaultsFalse(t *testing.T) {
	cfg := &Config{}
	assert.False(t, cfg.GetFeatureFlag("backlog"), "backlog feature flag should be false by default")
	assert.False(t, cfg.GetFeatureFlag("nonexistent"), "unknown feature flag should be false")
}

// TestGetFeatureFlag_unknownKeyDefaultsFalse verifies that any unrecognized key returns false.
func TestGetFeatureFlag_unknownKeyDefaultsFalse(t *testing.T) {
	cfg := &Config{FeatureFlags: map[string]bool{"other": true}}
	assert.False(t, cfg.GetFeatureFlag("nonexistent"))
}

// TestGetFeatureFlag_knownKeyReturnsValue verifies that an explicitly set flag returns its value.
func TestGetFeatureFlag_knownKeyReturnsValue(t *testing.T) {
	cfg := &Config{FeatureFlags: map[string]bool{"backlog": true}}
	assert.True(t, cfg.GetFeatureFlag("backlog"))
}

// ─── IsNamedInstance ────────────────────────────────────────────────────────

// TestIsNamedInstance_should_ReturnFalse_When_InstanceEnvVarUnset covers the
// default production case: no STAPLER_SQUAD_INSTANCE set at all.
func TestIsNamedInstance_should_ReturnFalse_When_InstanceEnvVarUnset(t *testing.T) {
	original := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Unsetenv("STAPLER_SQUAD_INSTANCE")
	defer func() {
		if original == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", original)
		}
	}()

	assert.False(t, IsNamedInstance())
}

// TestIsNamedInstance_should_ReturnFalse_When_InstanceIsShared verifies "shared"
// is treated the same as unset — GetConfigDirForDir's own backward-compat special case.
func TestIsNamedInstance_should_ReturnFalse_When_InstanceIsShared(t *testing.T) {
	original := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		if original == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", original)
		}
	}()

	assert.False(t, IsNamedInstance())
}

// TestIsNamedInstance_should_ReturnTrue_When_InstanceIsExplicitlyNamed is the
// regression test for the incident this function exists to prevent: the E2E test
// harness runs with STAPLER_SQUAD_INSTANCE=e2e-local (real production binary, not a
// `go test` binary — IsTestMode() alone doesn't catch it) and must be recognized as
// unsafe for a shared-tmux-socket orphan sweep.
func TestIsNamedInstance_should_ReturnTrue_When_InstanceIsExplicitlyNamed(t *testing.T) {
	original := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("STAPLER_SQUAD_INSTANCE", "e2e-local")
	defer func() {
		if original == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", original)
		}
	}()

	assert.True(t, IsNamedInstance())
}

// ─── IsIsolatedInstance ─────────────────────────────────────────────────────

// TestIsIsolatedInstance_should_ReturnTrue_When_TestDirOverrideSet is the
// regression test for the incident this function exists to prevent: the demo
// server harness (tests/demo/helpers.go StartDemoServer) launches the real
// production binary with --test-mode --test-dir, setting STAPLER_SQUAD_TEST_DIR
// but neither STAPLER_SQUAD_INSTANCE nor a `go test` binary suffix — so neither
// IsTestMode() nor IsNamedInstance() alone caught it, and its startup orphan
// sweep killed real production tmux sessions on the shared default socket.
func TestIsIsolatedInstance_should_ReturnTrue_When_TestDirOverrideSet(t *testing.T) {
	original := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	os.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	defer func() {
		if original == "" {
			os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
		} else {
			os.Setenv("STAPLER_SQUAD_TEST_DIR", original)
		}
	}()

	assert.True(t, IsIsolatedInstance())
}

// TestIsIsolatedInstance_should_ReturnTrue_When_NamedInstanceSet covers the
// pre-existing IsNamedInstance mechanism composing correctly into the umbrella check.
func TestIsIsolatedInstance_should_ReturnTrue_When_NamedInstanceSet(t *testing.T) {
	original := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("STAPLER_SQUAD_INSTANCE", "e2e-local")
	defer func() {
		if original == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", original)
		}
	}()

	assert.True(t, IsIsolatedInstance())
}

// ─── SlackConfig ────────────────────────────────────────────────────────────

// TestLoadConfig_SlackConfig_DefaultsToZeroValue_When_NoSlackKeyPresent verifies
// REQ-1's happy path: a config.json predating the Slack feature (no "slack" key)
// loads to a zero-value SlackConfig, not an error.
func TestLoadConfig_SlackConfig_DefaultsToZeroValue_When_NoSlackKeyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, SlackConfig{}, cfg.Slack)
}

// TestLoadConfig_SlackConfig_PopulatesFields_When_SlackKeyPresent verifies REQ-1:
// a stored "slack" block populates the corresponding SlackConfig fields.
func TestLoadConfig_SlackConfig_PopulatesFields_When_SlackKeyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"slack": {"notify_on_queue_item": true, "queue_depth_threshold": 5}}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.True(t, cfg.Slack.NotifyOnQueueItem)
	assert.Equal(t, 5, cfg.Slack.QueueDepthThreshold)
}

// TestLoadConfig_SlackEnvOverride_TakesPrecedenceOverStoredValue verifies REQ-2:
// SLACK_WEBHOOK_URL, when set, wins over a stored (ciphertext) value —
// mirroring ANTHROPIC_API_KEY's env-override precedence, per ADR-001.
func TestLoadConfig_SlackEnvOverride_TakesPrecedenceOverStoredValue(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/T0/B0/TEST")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"slack": {"webhook_url_encrypted": "dummy-ciphertext"}}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T0/B0/TEST", cfg.SlackWebhookURLOverride())
	// The stored ciphertext is left untouched by the env override.
	assert.Equal(t, "dummy-ciphertext", cfg.Slack.WebhookURLEncrypted)
}

// TestSlackWebhookURLOverride_ReturnsEmptyString_When_EnvVarUnset verifies REQ-2's
// edge path: with no SLACK_WEBHOOK_URL in the environment, the override getter
// returns "" rather than some stale/default value.
func TestSlackWebhookURLOverride_ReturnsEmptyString_When_EnvVarUnset(t *testing.T) {
	original, wasSet := os.LookupEnv("SLACK_WEBHOOK_URL")
	os.Unsetenv("SLACK_WEBHOOK_URL")
	defer func() {
		if wasSet {
			os.Setenv("SLACK_WEBHOOK_URL", original)
		}
	}()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "", cfg.SlackWebhookURLOverride())
}

// ─── HandoffSummaryConfig ───────────────────────────────────────────────────

// TestHandoffSummaryConfigOrDefault_AppliesDefaultToZeroBudget verifies a
// zero-value HandoffSummaryConfig (nil Enabled, unset budget) resolves to the
// enabled-by-default, 12000-token-budget state.
func TestHandoffSummaryConfigOrDefault_AppliesDefaultToZeroBudget(t *testing.T) {
	cfg := HandoffSummaryConfig{}

	out := cfg.HandoffSummaryConfigOrDefault()

	assert.True(t, out.EnabledOrDefault())
	assert.Equal(t, 12000, out.MaxMiddleExcerptTokens)
}

// TestLoadConfig_HandoffSummaryExplicitlyDisabled_StaysDisabled verifies the
// gotcha this feature is prone to: an explicit "enabled": false in config.json
// must survive LoadConfigFromPath unchanged, not get silently re-defaulted to
// enabled.
func TestLoadConfig_HandoffSummaryExplicitlyDisabled_StaysDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"handoff_summary": {"enabled": false}}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.False(t, cfg.HandoffSummary.EnabledOrDefault())
	assert.Equal(t, 12000, cfg.HandoffSummary.MaxMiddleExcerptTokens)
}

// TestLoadConfig_HandoffSummaryAbsentFromExistingConfig_DefaultsToEnabled
// covers the actual real-world upgrade path: a config.json that predates this
// feature (no "handoff_summary" key at all, unlike the empty-file/fresh-config
// path DefaultConfig() takes) must still resolve to enabled — this is exactly
// the case a plain `bool` field (indistinguishable zero-value from "absent")
// would get wrong, which is why Enabled is *bool.
func TestLoadConfig_HandoffSummaryAbsentFromExistingConfig_DefaultsToEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"some_other_field_from_before_this_feature_existed": true}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	cfg, err := LoadConfigFromPath(path)
	require.NoError(t, err)
	assert.True(t, cfg.HandoffSummary.EnabledOrDefault())
	assert.Equal(t, 12000, cfg.HandoffSummary.MaxMiddleExcerptTokens)
}

// TestResolveGlobalTymuxDefault_AlwaysAllowsFalse verifies the gate never
// blocks the safe direction: requesting the global tymux default be false is
// always permitted, regardless of whether the tymux rollback rehearsal has
// been recorded. Mirrors
// TestResolveGlobalStreamHubDefault_should_ReturnFalseWithNoError_When_RequestedIsFalse
// (config/stream_hub_rollout_test.go).
func TestResolveGlobalTymuxDefault_AlwaysAllowsFalse(t *testing.T) {
	cfg := &Config{} // TymuxRollbackRehearsalCompletedAt unset

	got, err := ResolveGlobalTymuxDefault(cfg, false)
	if err != nil {
		t.Fatalf("expected no error requesting false, got %v", err)
	}
	if got != false {
		t.Fatalf("expected false, got %v", got)
	}
}

// TestResolveGlobalTymuxDefault_RefusesTrueWithoutRehearsal is ADR-002's
// mechanical gate: with TymuxRollbackRehearsalCompletedAt unset, requesting
// the global tymux default resolve to true must fail with an explicit error,
// not silently fall back to false without signaling why. Mirrors
// TestResolveGlobalStreamHubDefault_should_FailFast_When_RehearsalNotCompleted.
func TestResolveGlobalTymuxDefault_RefusesTrueWithoutRehearsal(t *testing.T) {
	cfg := &Config{} // TymuxRollbackRehearsalCompletedAt unset

	got, err := ResolveGlobalTymuxDefault(cfg, true)
	if !errors.Is(err, ErrTymuxRollbackRehearsalNotCompleted) {
		t.Fatalf("expected ErrTymuxRollbackRehearsalNotCompleted, got %v", err)
	}
	if got != false {
		t.Fatalf("expected false to be returned alongside the error, got %v", got)
	}
}

// TestResolveGlobalTymuxDefault_AllowsTrueAfterRehearsal is the happy path:
// once TymuxRollbackRehearsalCompletedAt is set to a valid, non-zero
// timestamp, the same resolution that previously failed now succeeds and
// permits the global tymux default to be true.
func TestResolveGlobalTymuxDefault_AllowsTrueAfterRehearsal(t *testing.T) {
	completedAt := time.Now()
	cfg := &Config{TymuxRollbackRehearsalCompletedAt: &completedAt}

	got, err := ResolveGlobalTymuxDefault(cfg, true)
	if err != nil {
		t.Fatalf("expected no error once rehearsal is recorded, got %v", err)
	}
	if got != true {
		t.Fatalf("expected true, got %v", got)
	}
}

// TestRecordTymuxRollbackRehearsalCompleted_PersistsTimestamp exercises
// recording a completed tymux rehearsal end to end: it persists
// TymuxRollbackRehearsalCompletedAt to disk, and a freshly reloaded config
// subsequently permits ResolveGlobalTymuxDefault to return true where it
// previously refused. Mirrors
// TestRecordRollbackRehearsalCompleted_should_PersistTimestamp_And_UnblockResolution.
func TestRecordTymuxRollbackRehearsalCompleted_PersistsTimestamp(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

	cfg := &Config{}

	// Before recording, the gate refuses.
	if _, err := ResolveGlobalTymuxDefault(cfg, true); !errors.Is(err, ErrTymuxRollbackRehearsalNotCompleted) {
		t.Fatalf("expected gate to refuse before rehearsal is recorded, got %v", err)
	}

	before := time.Now()
	if err := cfg.RecordTymuxRollbackRehearsalCompleted(); err != nil {
		t.Fatalf("RecordTymuxRollbackRehearsalCompleted returned error: %v", err)
	}
	after := time.Now()

	if cfg.TymuxRollbackRehearsalCompletedAt == nil {
		t.Fatal("expected TymuxRollbackRehearsalCompletedAt to be set in memory")
	}
	if cfg.TymuxRollbackRehearsalCompletedAt.Before(before) || cfg.TymuxRollbackRehearsalCompletedAt.After(after) {
		t.Fatalf("expected TymuxRollbackRehearsalCompletedAt to be within [%v, %v], got %v", before, after, *cfg.TymuxRollbackRehearsalCompletedAt)
	}

	if _, err := ResolveGlobalTymuxDefault(cfg, true); err != nil {
		t.Fatalf("expected gate to succeed after rehearsal is recorded, got %v", err)
	}

	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after RecordTymuxRollbackRehearsalCompleted: %v", err)
	}
	if reloaded.TymuxRollbackRehearsalCompletedAt == nil {
		t.Fatal("expected persisted config to carry TymuxRollbackRehearsalCompletedAt")
	}
	if _, err := ResolveGlobalTymuxDefault(reloaded, true); err != nil {
		t.Fatalf("expected reloaded config's gate to succeed, got %v", err)
	}
}

// TestGetTymuxSessionOverride_NilConfigIsNilSafe verifies the nil-safe
// zero-value behavior mirroring GetStreamHubSessionOverride's shape: a nil
// *Config and a config with a nil TymuxSessionOverrides map both report no
// override rather than panicking.
func TestGetTymuxSessionOverride_NilConfigIsNilSafe(t *testing.T) {
	var nilCfg *Config
	if _, ok := nilCfg.GetTymuxSessionOverride("canary-1"); ok {
		t.Fatal("expected nil config to report no override")
	}

	cfg := &Config{} // TymuxSessionOverrides is nil
	if _, ok := cfg.GetTymuxSessionOverride("canary-1"); ok {
		t.Fatal("expected config with nil map to report no override")
	}
}

// TestSetTymuxSessionOverride_ForceTrueThenForceFalse exercises Task
// 4.1.1b's both-directions regression guard (plan.md Epic 4.1): setting the
// same session name's override to true, then to false, must actually flip
// the stored value both ways and persist each direction to disk. This is
// the accessor-level proof that config storage carries no directional bias
// toward true, unlike streamhub/ownership.go's resolveLocked (see
// research/features.md (b).5), which only ever pushes toward true.
func TestSetTymuxSessionOverride_ForceTrueThenForceFalse(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	cfg := &Config{}

	// Direction 1: force tymux (true).
	forceTymux := true
	if err := cfg.SetTymuxSessionOverride("canary-1", &forceTymux); err != nil {
		t.Fatalf("SetTymuxSessionOverride(true) returned error: %v", err)
	}
	if got, ok := cfg.GetTymuxSessionOverride("canary-1"); !ok || !got {
		t.Fatalf("expected override to force tymux, got (%v, %v)", got, ok)
	}
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after forcing true: %v", err)
	}
	if got, ok := reloaded.GetTymuxSessionOverride("canary-1"); !ok || !got {
		t.Fatalf("expected persisted override to force tymux, got (%v, %v)", got, ok)
	}

	// Direction 2: same session name, now force tmux (false). If the
	// accessor had streamhub's resolveLocked bias, this would fail to move
	// the effective value back to false.
	forceTmux := false
	if err := cfg.SetTymuxSessionOverride("canary-1", &forceTmux); err != nil {
		t.Fatalf("SetTymuxSessionOverride(false) returned error: %v", err)
	}
	if got, ok := cfg.GetTymuxSessionOverride("canary-1"); !ok || got {
		t.Fatalf("expected override to force tmux (false), got (%v, %v)", got, ok)
	}
	reloadedAfterFalse, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after forcing false: %v", err)
	}
	if got, ok := reloadedAfterFalse.GetTymuxSessionOverride("canary-1"); !ok || got {
		t.Fatalf("expected persisted override to force tmux (false), got (%v, %v)", got, ok)
	}

	// nil clears the override entirely, falling back to the global default.
	if err := cfg.SetTymuxSessionOverride("canary-1", nil); err != nil {
		t.Fatalf("SetTymuxSessionOverride(nil) returned error: %v", err)
	}
	if _, ok := cfg.GetTymuxSessionOverride("canary-1"); ok {
		t.Fatal("expected override to be cleared")
	}
	reloadedAfterClear, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after clear: %v", err)
	}
	if _, ok := reloadedAfterClear.GetTymuxSessionOverride("canary-1"); ok {
		t.Fatal("expected persisted config to no longer have the override")
	}
}
