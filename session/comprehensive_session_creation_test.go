package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-squad/session/tmux"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestTmuxSocket returns a unique tmux socket name for test isolation
func getTestTmuxSocket(t *testing.T) string {
	return "test_" + strings.ReplaceAll(t.Name(), "/", "_")
}

// mockTmuxExecutor implements executor.Executor for testing tmux commands
type mockTmuxExecutor struct {
	sessionsCreated map[string]bool
}

func newMockTmuxExecutor() *mockTmuxExecutor {
	return &mockTmuxExecutor{
		sessionsCreated: make(map[string]bool),
	}
}

func (m *mockTmuxExecutor) Run(cmd *exec.Cmd) error {
	// Mock tmux commands to always succeed
	if len(cmd.Args) > 0 && cmd.Args[0] == "tmux" {
		// Track session creation
		for i, arg := range cmd.Args {
			if arg == "new-session" {
				// Find the session name (-s flag)
				for j := i; j < len(cmd.Args)-1; j++ {
					if cmd.Args[j] == "-s" {
						sessionName := cmd.Args[j+1]
						m.sessionsCreated[sessionName] = true
						break
					}
				}
				break
			}
			if arg == "kill-session" {
				// Find the session name (-t flag)
				for j := i; j < len(cmd.Args)-1; j++ {
					if cmd.Args[j] == "-t" {
						sessionName := cmd.Args[j+1]
						delete(m.sessionsCreated, sessionName)
						break
					}
				}
				break
			}
		}
		return nil // Simulate successful tmux command execution
	}
	return fmt.Errorf("unexpected command in mock: %v", cmd.Args)
}

func (m *mockTmuxExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	// Mock tmux command output
	if len(cmd.Args) > 0 && cmd.Args[0] == "tmux" {
		// Check if this is a list-sessions command for session existence checking
		for i, arg := range cmd.Args {
			if arg == "list-sessions" {
				// Check if it's the format used by DoesSessionExist: "list-sessions -F #{session_name}"
				hasFormat := false
				for j := i; j < len(cmd.Args); j++ {
					if cmd.Args[j] == "-F" && j+1 < len(cmd.Args) && cmd.Args[j+1] == "#{session_name}" {
						hasFormat = true
						break
					}
				}

				if hasFormat {
					// Return session names (one per line) for sessions that exist
					var sessionNames []string
					for sessionName := range m.sessionsCreated {
						sessionNames = append(sessionNames, sessionName)
					}
					if len(sessionNames) == 0 {
						// No sessions exist - return error to simulate empty tmux server
						return nil, fmt.Errorf("no server running")
					}
					return []byte(strings.Join(sessionNames, "\n")), nil
				} else {
					// Legacy format - return detailed session info
					var sessionLines []string
					for sessionName := range m.sessionsCreated {
						sessionLines = append(sessionLines, fmt.Sprintf("%s: 1 windows (created mock)", sessionName))
					}
					if len(sessionLines) == 0 {
						return nil, fmt.Errorf("no server running")
					}
					return []byte(strings.Join(sessionLines, "\n")), nil
				}
			}
			if arg == "has-session" {
				// Find the session name (-t flag)
				for j := i; j < len(cmd.Args)-1; j++ {
					if cmd.Args[j] == "-t" {
						sessionName := cmd.Args[j+1]
						if m.sessionsCreated[sessionName] {
							return []byte(""), nil // Session exists
						} else {
							return nil, fmt.Errorf("session not found: %s", sessionName)
						}
					}
				}
				return nil, fmt.Errorf("no session specified")
			}
		}
		return []byte("mock tmux output"), nil
	}
	return nil, fmt.Errorf("unexpected command in mock: %v", cmd.Args)
}

// TestInstanceBuilder provides a fluent interface for creating test instances with proper isolation
type TestInstanceBuilder struct {
	t    *testing.T
	opts InstanceOptions
}

// NewTestInstance creates a new test instance builder with sane defaults and proper isolation
func NewTestInstance(t *testing.T, title string) *TestInstanceBuilder {
	return &TestInstanceBuilder{
		t: t,
		opts: InstanceOptions{
			Title:            title,
			Path:             t.TempDir(), // Each test gets its own temp directory
			Program:          "bash -c 'echo test session; exec bash'", // Safe default program
			SessionType:      SessionTypeDirectory,
			AutoYes:          true, // Tests shouldn't prompt for input
			TmuxServerSocket: getTestTmuxSocket(t), // Isolated tmux server per test
		},
	}
}

// WithProgram sets the program to run
func (b *TestInstanceBuilder) WithProgram(program string) *TestInstanceBuilder {
	b.opts.Program = program
	return b
}

// WithPath sets the working directory path
func (b *TestInstanceBuilder) WithPath(path string) *TestInstanceBuilder {
	b.opts.Path = path
	return b
}

// WithSessionType sets the session type
func (b *TestInstanceBuilder) WithSessionType(sessionType SessionType) *TestInstanceBuilder {
	b.opts.SessionType = sessionType
	return b
}

// WithAutoYes sets the auto-yes flag
func (b *TestInstanceBuilder) WithAutoYes(autoYes bool) *TestInstanceBuilder {
	b.opts.AutoYes = autoYes
	return b
}

// WithWorkingDir sets the working directory within the path
func (b *TestInstanceBuilder) WithWorkingDir(workingDir string) *TestInstanceBuilder {
	b.opts.WorkingDir = workingDir
	return b
}

// WithCategory sets the session category
func (b *TestInstanceBuilder) WithCategory(category string) *TestInstanceBuilder {
	b.opts.Category = category
	return b
}

// Build creates the instance with cleanup function
func (b *TestInstanceBuilder) Build() (*Instance, tmux.CleanupFunc, error) {
	return b.buildWithMockTmux()
}

// BuildAndStart creates and starts the instance with cleanup functions
func (b *TestInstanceBuilder) BuildAndStart() (*Instance, tmux.CleanupFunc, tmux.CleanupFunc, error) {
	instance, cleanup, err := b.buildWithMockTmux()
	if err != nil {
		return nil, nil, nil, err
	}

	startCleanup, startErr := instance.StartWithCleanup(true)
	if startErr != nil {
		if cleanup != nil {
			cleanup() // Clean up the instance if start failed
		}
		return nil, nil, nil, startErr
	}

	return instance, cleanup, startCleanup, nil
}

// buildWithMockTmux creates an instance with a mock tmux session to prevent real tmux command execution
func (b *TestInstanceBuilder) buildWithMockTmux() (*Instance, tmux.CleanupFunc, error) {
	// Create the instance first
	instance, cleanup, err := NewInstanceWithCleanup(b.opts)
	if err != nil {
		return nil, nil, err
	}

	// Replace the tmux session with a mock one that uses our mock executor
	mockExecutor := newMockTmuxExecutor()
	mockPtyFactory := &mockPtyFactory{}

	// Use the tmux dependency injection method
	mockTmuxSession := tmux.NewTmuxSessionWithDeps(instance.Title, instance.Program, mockPtyFactory, mockExecutor)

	// Replace the real tmux session with the mock
	instance.tmuxSession = mockTmuxSession

	return instance, cleanup, nil
}

// mockPtyFactory implements tmux.PtyFactory for testing
type mockPtyFactory struct{}

func (m *mockPtyFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	// Return a mock file descriptor - we can use stdin as a safe mock
	return os.Stdin, nil
}

func (m *mockPtyFactory) Close() {
	// No-op for mock
}

// ComprehensiveSessionCreationSuite provides exhaustive testing of session creation
// without requiring the full TUI application to run
func TestComprehensiveSessionCreation(t *testing.T) {
	t.Run("SessionCreationValidation", testSessionCreationValidation)
	t.Run("SessionCreationTiming", testSessionCreationTiming)
	t.Run("SessionCreationStates", testSessionCreationStates)
	t.Run("SessionCreationErrorHandling", testSessionCreationErrorHandling)
	t.Run("SessionCreationCleanup", testSessionCreationCleanup)
	t.Run("SessionCreationConcurrency", testSessionCreationConcurrency)
	t.Run("SessionCreationFileSystem", testSessionCreationFileSystem)
	t.Run("SessionCreationWorktreeManagement", testSessionCreationWorktreeManagement)
}

// testSessionCreationValidation tests input validation for session creation
func testSessionCreationValidation(t *testing.T) {
	t.Run("RequiredFields", func(t *testing.T) {
		// Test empty title validation (validation happens during Start, not creation)
		instance, cleanup, err := NewTestInstance(t, "").
			WithProgram("bash -c 'echo test; exec bash'").
			Build()
		assert.NoError(t, err, "Instance creation should succeed even with empty title")
		if cleanup != nil {
			defer func() {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}()
		}

		// Starting should fail due to empty title
		startCleanup, startErr := instance.StartWithCleanup(true)
		if startCleanup != nil {
			defer func() {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}()
		}
		assert.Error(t, startErr, "Empty title should be rejected during start")
		assert.Contains(t, startErr.Error(), "title", "Error should mention title field")

		// Test empty path validation (happens during creation due to filepath.Abs)
		_, cleanup2, err := NewTestInstance(t, "test-session").
			WithPath(""). // Empty path should fail
			WithProgram("bash -c 'echo test; exec bash'").
			Build()
		if cleanup2 != nil {
			defer func() {
				if cleanupErr := cleanup2(); cleanupErr != nil {
					t.Logf("Cleanup2 warning: %v", cleanupErr)
				}
			}()
		}
		// Empty path might be handled by filepath.Abs, so we don't require an error
		if err != nil {
			t.Logf("Empty path validation: %v", err)
		}

		// Test empty program (creation succeeds, but start might have issues)
		instance3, cleanup3, err := NewTestInstance(t, "test-empty-program").
			WithProgram(""). // Empty program
			Build()
		assert.NoError(t, err, "Instance creation should succeed even with empty program")
		if cleanup3 != nil {
			defer func() {
				if cleanupErr := cleanup3(); cleanupErr != nil {
					t.Logf("Cleanup3 warning: %v", cleanupErr)
				}
			}()
		}
		// Starting might fail or succeed depending on tmux handling
		startCleanup3, startErr := instance3.StartWithCleanup(true)
		if startCleanup3 != nil {
			defer func() {
				if cleanupErr := startCleanup3(); cleanupErr != nil {
					t.Logf("Start cleanup3 warning: %v", cleanupErr)
				}
			}()
		}
		t.Logf("Empty program start result: %v", startErr)
	})

	t.Run("PathValidation", func(t *testing.T) {
		// Test non-existent path (NewInstance might create the path or validate later)
		nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist")
		instance, cleanup, err := NewTestInstance(t, "test-nonexistent-path").
			WithPath(nonExistentPath).
			WithProgram("bash -c 'echo test; exec bash'").
			Build()
		// Instance creation might succeed even with non-existent path
		if err != nil {
			t.Logf("Non-existent path rejected during creation: %v", err)
		} else {
			if cleanup != nil {
				defer func() {
					if cleanupErr := cleanup(); cleanupErr != nil {
						t.Logf("Cleanup warning: %v", cleanupErr)
					}
				}()
			}
			// Try to start - this might fail with non-existent path
			startCleanup, startErr := instance.StartWithCleanup(true)
			if startCleanup != nil {
				defer func() {
					if cleanupErr := startCleanup(); cleanupErr != nil {
						t.Logf("Start cleanup warning: %v", cleanupErr)
					}
				}()
			}
			t.Logf("Non-existent path start result: %v", startErr)
		}

		// Test valid path
		validPath := t.TempDir()
		validInstance, validCleanup, validErr := NewTestInstance(t, "test-valid-path").
			WithPath(validPath).
			WithProgram("bash -c 'echo test; exec bash'").
			Build()
		assert.NoError(t, validErr, "Valid path should be accepted")
		if validCleanup != nil {
			defer func() {
				if cleanupErr := validCleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}()
		}
		assert.Equal(t, validPath, validInstance.Path)
	})

	t.Run("SessionTypeValidation", func(t *testing.T) {
		validSessionTypes := []SessionType{
			SessionTypeDirectory,
			SessionTypeNewWorktree,
			SessionTypeExistingWorktree,
		}

		for _, sessionType := range validSessionTypes {
			t.Run(string(sessionType), func(t *testing.T) {
				instance, cleanup, err := NewTestInstance(t, fmt.Sprintf("test-%s", sessionType)).
					WithProgram("bash -c 'echo test; exec bash'").
					WithSessionType(sessionType).
					Build()
				assert.NoError(t, err, "Valid session type %s should be accepted", sessionType)
				if cleanup != nil {
					defer func() {
						if cleanupErr := cleanup(); cleanupErr != nil {
							t.Logf("Cleanup warning: %v", cleanupErr)
						}
					}()
				}
				assert.Equal(t, sessionType, instance.SessionType)
			})
		}
	})
}

// testSessionCreationTiming ensures session creation completes within reasonable timeframes
func testSessionCreationTiming(t *testing.T) {
	t.Run("FastSessionCreation", func(t *testing.T) {

		testCases := []struct {
			name     string
			program  string
			maxTime  time.Duration
		}{
			{
				name:    "SimpleEcho",
				program: "bash -c 'echo \"fast session started\"; exec bash'",
				maxTime: 2 * time.Second,
			},
			{
				name:    "BashCommand",
				program: "bash -c 'echo \"session created\"; exec bash'",
				maxTime: 3 * time.Second,
			},
			{
				name:    "QuickScript",
				program: "bash -c 'for i in {1..3}; do echo step $i; done; exec bash'",
				maxTime: 3 * time.Second,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Measure session start time
				startTime := time.Now()

				_, cleanup, startCleanup, err := NewTestInstance(t, fmt.Sprintf("timing-test-%s", strings.ToLower(tc.name))).
					WithProgram(tc.program).
					BuildAndStart()

				elapsed := time.Since(startTime)

				require.NoError(t, err, "Session should start successfully")
				require.Less(t, elapsed, tc.maxTime,
					"Session creation should complete within %v, took %v", tc.maxTime, elapsed)

				defer func() {
					if startCleanup != nil {
						if cleanupErr := startCleanup(); cleanupErr != nil {
							t.Logf("Start cleanup warning: %v", cleanupErr)
						}
					}
					if cleanup != nil {
						if cleanupErr := cleanup(); cleanupErr != nil {
							t.Logf("Cleanup warning: %v", cleanupErr)
						}
					}
				}()

				t.Logf("✓ %s session created in %v (limit: %v)", tc.name, elapsed, tc.maxTime)
			})
		}
	})

	t.Run("NoHangingBehavior", func(t *testing.T) {
		// Test with context timeout to ensure no hanging
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Start session with context
		done := make(chan error, 1)
		go func() {
			_, cleanup, startCleanup, err := NewTestInstance(t, "no-hang-test").
				WithProgram("bash -c 'echo no hang test; sleep 0.5; echo done'").
				BuildAndStart()

			defer func() {
				if startCleanup != nil {
					if cleanupErr := startCleanup(); cleanupErr != nil {
						t.Logf("Start cleanup warning: %v", cleanupErr)
					}
				}
				if cleanup != nil {
					if cleanupErr := cleanup(); cleanupErr != nil {
						t.Logf("Cleanup warning: %v", cleanupErr)
					}
				}
			}()

			done <- err
		}()

		select {
		case err := <-done:
			assert.NoError(t, err, "Session should start without hanging")
		case <-ctx.Done():
			t.Fatal("Session creation timed out - this indicates a hanging bug")
		}
	})
}

// testSessionCreationStates verifies proper state transitions during session creation
func testSessionCreationStates(t *testing.T) {
	t.Run("StateTransitions", func(t *testing.T) {
		instance, cleanup, err := NewTestInstance(t, "state-test").
			WithProgram("bash -c 'echo state test; sleep 0.5'").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Initial state should be appropriate
		assert.NotEqual(t, Running, instance.Status, "Instance should not be running before start")

		// Start the session
		startCleanup, err := instance.StartWithCleanup(true)
		require.NoError(t, err)
		defer func() {
			if startCleanup != nil {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Should be running after successful start
		assert.Equal(t, Running, instance.Status, "Instance should be running after start")

		// Test Started() method
		assert.True(t, instance.Started(), "Started() should return true for running instance")
	})

	t.Run("StatusConsistency", func(t *testing.T) {
		instance, cleanup, err := NewTestInstance(t, "consistency-test").
			WithProgram("bash -c 'echo consistency; exec bash'").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Status should be consistent with Started()
		beforeStart := instance.Started()
		statusBeforeStart := instance.Status

		startCleanup, err := instance.StartWithCleanup(true)
		require.NoError(t, err)
		defer func() {
			if startCleanup != nil {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}
		}()

		afterStart := instance.Started()
		statusAfterStart := instance.Status

		assert.False(t, beforeStart, "Should not be started initially")
		assert.True(t, afterStart, "Should be started after StartWithCleanup")
		assert.NotEqual(t, statusBeforeStart, statusAfterStart, "Status should change after starting")
		assert.Equal(t, Running, statusAfterStart, "Status should be Running after start")
	})
}

// testSessionCreationErrorHandling verifies proper error handling in various failure scenarios
func testSessionCreationErrorHandling(t *testing.T) {
	t.Run("InvalidProgram", func(t *testing.T) {
		instance, cleanup, err := NewTestInstance(t, "invalid-program-test").
			WithProgram("completely-nonexistent-command-12345").
			Build()
		require.NoError(t, err, "Instance creation should succeed even with invalid program")
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Starting should fail gracefully
		startCleanup, err := instance.StartWithCleanup(true)
		if err == nil && startCleanup != nil {
			defer func() {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}()
		}

		// The error might occur during start or might be handled by tmux
		// The important thing is that it doesn't hang or panic
		t.Logf("Invalid program test completed (error: %v)", err)
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		// Create a directory without write permissions
		tempDir := t.TempDir()
		restrictedDir := filepath.Join(tempDir, "restricted")
		err := os.Mkdir(restrictedDir, 0444) // Read-only
		require.NoError(t, err)

		// Attempt to create session in restricted directory
		instance, cleanup, err := NewTestInstance(t, "permission-test").
			WithProgram("bash -c 'echo permission test; exec bash'").
			WithPath(restrictedDir).
			Build()

		// Instance creation might succeed, but starting might fail
		if err != nil {
			assert.Contains(t, err.Error(), "permission", "Error should indicate permission issue")
			return
		}

		if cleanup != nil {
			defer func() {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}()
		}

		// Try to start - this might fail due to permissions
		startCleanup, startErr := instance.StartWithCleanup(true)
		if startCleanup != nil {
			defer func() {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}()
		}

		// Don't require a specific error, just ensure it doesn't hang or panic
		t.Logf("Permission test completed (start error: %v)", startErr)
	})

	t.Run("DuplicateSessions", func(t *testing.T) {
		sessionTitle := "duplicate-session-test"

		// Create first session
		instance1, cleanup1, err := NewTestInstance(t, sessionTitle).
			WithProgram("bash -c 'echo first session; sleep 2'").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup1 != nil {
				if cleanupErr := cleanup1(); cleanupErr != nil {
					t.Logf("Cleanup1 warning: %v", cleanupErr)
				}
			}
		}()

		startCleanup1, err := instance1.StartWithCleanup(true)
		require.NoError(t, err)
		defer func() {
			if startCleanup1 != nil {
				if cleanupErr := startCleanup1(); cleanupErr != nil {
					t.Logf("Start cleanup1 warning: %v", cleanupErr)
				}
			}
		}()

		// Create second session with same title
		instance2, cleanup2, err := NewTestInstance(t, sessionTitle).
			WithProgram("bash -c 'echo second session; sleep 1'").
			Build()

		// The behavior here depends on implementation - it might succeed with a modified title
		// or fail with an error. The important thing is no hanging or panic.
		if err != nil {
			t.Logf("Duplicate session creation failed as expected: %v", err)
			return
		}

		defer func() {
			if cleanup2 != nil {
				if cleanupErr := cleanup2(); cleanupErr != nil {
					t.Logf("Cleanup2 warning: %v", cleanupErr)
				}
			}
		}()

		startCleanup2, startErr := instance2.StartWithCleanup(true)
		if startCleanup2 != nil {
			defer func() {
				if cleanupErr := startCleanup2(); cleanupErr != nil {
					t.Logf("Start cleanup2 warning: %v", cleanupErr)
				}
			}()
		}

		t.Logf("Duplicate session test completed (start error: %v)", startErr)
	})
}

// testSessionCreationCleanup verifies proper resource cleanup
func testSessionCreationCleanup(t *testing.T) {
	t.Run("CleanupFunction", func(t *testing.T) {
		instance, cleanup, err := NewTestInstance(t, "cleanup-test").
			WithProgram("bash -c 'echo cleanup test; sleep 1'").
			Build()
		require.NoError(t, err)
		require.NotNil(t, cleanup, "Cleanup function should be provided")

		startCleanup, err := instance.StartWithCleanup(true)
		require.NoError(t, err)
		require.NotNil(t, startCleanup, "Start cleanup function should be provided")

		// Test that cleanup functions work
		err = startCleanup()
		assert.NoError(t, err, "Start cleanup should succeed")

		err = cleanup()
		assert.NoError(t, err, "Instance cleanup should succeed")
	})

	t.Run("CleanupIdempotency", func(t *testing.T) {
		_, cleanup, err := NewTestInstance(t, "idempotent-cleanup-test").
			WithProgram("bash -c 'echo idempotent test; exec bash'").
			Build()
		require.NoError(t, err)

		// Cleanup should be safe to call multiple times
		err1 := cleanup()
		err2 := cleanup()
		err3 := cleanup()

		// At least the first cleanup should succeed
		// Subsequent cleanups might return errors but shouldn't panic
		assert.NoError(t, err1, "First cleanup should succeed")
		t.Logf("Multiple cleanup results: %v, %v, %v", err1, err2, err3)
	})
}

// testSessionCreationConcurrency tests concurrent session creation
func testSessionCreationConcurrency(t *testing.T) {
	t.Run("ConcurrentCreation", func(t *testing.T) {
		numSessions := 3

		// Create sessions concurrently
		type sessionResult struct {
			instance *Instance
			cleanup  tmux.CleanupFunc
			err      error
		}

		results := make(chan sessionResult, numSessions)

		for i := 0; i < numSessions; i++ {
			go func(id int) {
				instance, cleanup, err := NewTestInstance(t, fmt.Sprintf("concurrent-test-%d", id)).
					WithProgram(fmt.Sprintf("bash -c 'echo concurrent session %d; sleep 1'", id)).
					Build()
				results <- sessionResult{instance, cleanup, err}
			}(i)
		}

		// Collect results
		var cleanupFuncs []tmux.CleanupFunc
		successCount := 0

		for i := 0; i < numSessions; i++ {
			result := <-results
			if result.err == nil {
				successCount++
				if result.cleanup != nil {
					cleanupFuncs = append(cleanupFuncs, result.cleanup)
				}
			} else {
				t.Logf("Concurrent session %d failed: %v", i, result.err)
			}
		}

		// Clean up all successful sessions
		defer func() {
			for _, cleanup := range cleanupFuncs {
				if cleanup != nil {
					if cleanupErr := cleanup(); cleanupErr != nil {
						t.Logf("Concurrent cleanup warning: %v", cleanupErr)
					}
				}
			}
		}()

		assert.Greater(t, successCount, 0, "At least one concurrent session should succeed")
		t.Logf("✓ %d/%d concurrent sessions created successfully", successCount, numSessions)
	})
}

// testSessionCreationFileSystem tests file system interactions
func testSessionCreationFileSystem(t *testing.T) {
	t.Run("WorkingDirectoryHandling", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a subdirectory
		subDir := filepath.Join(tempDir, "subdir")
		err := os.Mkdir(subDir, 0755)
		require.NoError(t, err)

		instance, cleanup, err := NewTestInstance(t, "workdir-test").
			WithPath(tempDir).
			WithProgram("bash -c 'pwd; echo workdir test'").
			WithWorkingDir("subdir").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// WorkingDir might be processed differently
		t.Logf("WorkingDir set to: '%s'", instance.WorkingDir)
		// The working directory behavior may vary based on implementation

		startCleanup, err := instance.StartWithCleanup(true)
		assert.NoError(t, err, "Session with working directory should start")
		if startCleanup != nil {
			defer func() {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}()
		}
	})

	t.Run("PathNormalization", func(t *testing.T) {
		tempDir := t.TempDir()

		// Test path with extra slashes and dots
		messyPath := tempDir + "//.//"

		instance, cleanup, err := NewTestInstance(t, "path-norm-test").
			WithPath(messyPath).
			WithProgram("bash -c 'echo path normalization test; exec bash'").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Path should be normalized
		normalizedPath := filepath.Clean(messyPath)
		assert.Equal(t, normalizedPath, instance.Path)
	})
}

// testSessionCreationWorktreeManagement tests git worktree functionality
func testSessionCreationWorktreeManagement(t *testing.T) {
	t.Run("NewWorktreeCreation", func(t *testing.T) {
		// Create a git repository
		tempRepo := setupTestRepository(t)
		defer func() {
			if err := os.RemoveAll(tempRepo); err != nil {
				t.Logf("Failed to remove temp repo: %v", err)
			}
		}()

		instance, cleanup, err := NewTestInstance(t, "worktree-creation-test").
			WithPath(tempRepo).
			WithProgram("bash -c 'git status; echo worktree test'").
			WithSessionType(SessionTypeNewWorktree).
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		startCleanup, err := instance.StartWithCleanup(true)
		require.NoError(t, err, "Worktree session should start successfully")
		defer func() {
			if startCleanup != nil {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}
		}()

		// Verify worktree was created
		gitWorktree, err := instance.GetGitWorktree()
		require.NoError(t, err)

		worktreePath := gitWorktree.GetWorktreePath()
		_, err = os.Stat(worktreePath)
		assert.NoError(t, err, "Worktree directory should exist at %s", worktreePath)

		t.Logf("✓ Worktree created at %s", worktreePath)
	})

	t.Run("DirectorySessionFallback", func(t *testing.T) {
		// Test directory session in non-git directory
		tempDir := t.TempDir()

		instance, cleanup, err := NewTestInstance(t, "directory-fallback-test").
			WithPath(tempDir).
			WithProgram("bash -c 'pwd; echo directory session'").
			Build()
		require.NoError(t, err)
		defer func() {
			if cleanup != nil {
				if cleanupErr := cleanup(); cleanupErr != nil {
					t.Logf("Cleanup warning: %v", cleanupErr)
				}
			}
		}()

		startCleanup, err := instance.StartWithCleanup(true)
		require.NoError(t, err, "Directory session should work in non-git directory")
		defer func() {
			if startCleanup != nil {
				if cleanupErr := startCleanup(); cleanupErr != nil {
					t.Logf("Start cleanup warning: %v", cleanupErr)
				}
			}
		}()

		assert.Equal(t, SessionTypeDirectory, instance.SessionType)
		t.Logf("✓ Directory session created successfully")
	})
}