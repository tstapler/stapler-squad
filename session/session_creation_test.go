package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionCreationDoesNotHang tests that session creation completes within reasonable time
func TestSessionCreationDoesNotHang(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	tempDir := t.TempDir()

	t.Run("DirectorySessionCreation", func(t *testing.T) {
		t.Parallel()
		// Create a simple directory session with a command that stays alive
		// Using sleep to keep the session alive long enough for tmux to register it
		instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
			Title:            "test-session-no-hang",
			Path:             tempDir,
			Program:          "bash -c 'echo test session created; sleep 1'",
			SessionType:      SessionTypeDirectory,
			AutoYes:          true,
			TmuxServerSocket: testTmuxSocket(t),
		})
		require.NoError(t, err)
		defer func() {
			if err := cleanup(); err != nil {
				t.Logf("Warning: cleanup failed: %v", err)
			}
		}()

		// Start the session with a timeout to ensure it doesn't hang
		startTime := time.Now()
		startCleanup, err := instance.StartWithCleanup(true)
		elapsed := time.Since(startTime)

		// The session should start well under the previous 30-45 second wait.
		// 15s headroom accounts for preconfigureServerBeforeSession's
		// ~9.1s worst-case retry/backoff (serverStartAttempts=8, up to
		// serverStartBackoffMax=3s each) when this subtest's dedicated tmux
		// server contends with sibling t.Parallel() subtests' own server
		// spawns for CPU under load.
		require.NoError(t, err, "Session creation should succeed")
		require.Less(t, elapsed, 15*time.Second, "Session creation should complete within 15 seconds, took %v", elapsed)

		defer func() {
			if err := startCleanup(); err != nil {
				t.Logf("Warning: startCleanup failed: %v", err)
			}
		}()

		// Verify the session is running
		assert.Equal(t, Running, instance.Status)
		t.Logf("✓ Session created successfully in %v", elapsed)
	})

	t.Run("ClaudeSessionCreation", func(t *testing.T) {
		t.Parallel()
		// Test with Claude program (but use a simple command instead of actual claude)
		instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
			Title:            "test-claude-session",
			Path:             tempDir,
			Program:          "bash -c 'echo \"Starting claude session\"; sleep 1; echo \"Ready\"'",
			SessionType:      SessionTypeDirectory,
			AutoYes:          true,
			TmuxServerSocket: testTmuxSocket(t),
		})
		require.NoError(t, err)
		defer func() {
			if err := cleanup(); err != nil {
				t.Logf("Warning: cleanup failed: %v", err)
			}
		}()

		// Start the session with a timeout
		startTime := time.Now()
		startCleanup, err := instance.StartWithCleanup(true)
		elapsed := time.Since(startTime)

		// Should complete quickly without waiting for trust prompts.
		// See the DirectorySessionCreation subtest above for why 15s.
		require.NoError(t, err, "Claude session creation should succeed")
		require.Less(t, elapsed, 15*time.Second, "Claude session creation should complete within 15 seconds, took %v", elapsed)

		defer func() {
			if err := startCleanup(); err != nil {
				t.Logf("Warning: startCleanup failed: %v", err)
			}
		}()

		assert.Equal(t, Running, instance.Status)
		t.Logf("✓ Claude session created successfully in %v", elapsed)
	})
}

// TestSessionCreationWithRealPrograms tests session creation with actual programs if available
func TestSessionCreationWithRealPrograms(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	tempDir := t.TempDir()

	// Test cases for different programs
	testCases := []struct {
		name     string
		program  string
		skipTest func() bool
	}{
		{
			name:    "Bash",
			program: "bash",
			skipTest: func() bool {
				// Always available on Unix systems
				return false
			},
		},
		{
			name:    "Claude",
			program: "claude",
			skipTest: func() bool {
				// Skip if claude command not available
				return !commandExists("claude")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.skipTest() {
				t.Skipf("Skipping %s test - program not available", tc.name)
				return
			}

			instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
				Title:            "test-" + strings.ToLower(tc.name) + "-real",
				Path:             tempDir,
				Program:          tc.program,
				SessionType:      SessionTypeDirectory,
				AutoYes:          true,
				TmuxServerSocket: testTmuxSocket(t),
			})
			require.NoError(t, err)
			defer func() {
				if err := cleanup(); err != nil {
					t.Logf("Warning: cleanup failed: %v", err)
				}
			}()

			// Start session with reasonable timeout
			startTime := time.Now()
			startCleanup, err := instance.StartWithCleanup(true)
			elapsed := time.Since(startTime)

			// 20s headroom: see the DirectorySessionCreation subtest in
			// TestSessionCreationDoesNotHang for the retry/backoff/contention
			// rationale.
			require.NoError(t, err, "%s session should start successfully", tc.name)
			require.Less(t, elapsed, 20*time.Second, "%s session should start within 20 seconds, took %v", tc.name, elapsed)

			defer func() {
				if err := startCleanup(); err != nil {
					t.Logf("Warning: startCleanup failed: %v", err)
				}
			}()

			assert.Equal(t, Running, instance.Status)
			t.Logf("✓ %s session created successfully in %v", tc.name, elapsed)
		})
	}
}

// TestSessionCreationWithWorktree tests session creation with git worktrees
func TestSessionCreationWithWorktree(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            "test-worktree-session",
		Path:             tempRepo,
		Program:          "bash -c 'pwd; echo Worktree session ready; sleep 1'",
		SessionType:      SessionTypeNewWorktree,
		AutoYes:          true,
		TmuxServerSocket: testTmuxSocket(t),
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("Warning: cleanup failed: %v", err)
		}
	}()

	// Start session with timeout
	startTime := time.Now()
	startCleanup, err := instance.StartWithCleanup(true)
	elapsed := time.Since(startTime)

	require.NoError(t, err, "Worktree session should start successfully")
	require.Less(t, elapsed, 10*time.Second, "Worktree session should start within 10 seconds, took %v", elapsed)

	defer func() {
		if err := startCleanup(); err != nil {
			t.Logf("Warning: startCleanup failed: %v", err)
		}
	}()

	assert.Equal(t, Running, instance.Status)

	// Verify worktree was created
	gitWorktree, err := instance.GetGitWorktree()
	require.NoError(t, err)
	worktreePath := gitWorktree.GetWorktreePath()

	// Check that the worktree directory exists
	_, err = os.Stat(worktreePath)
	assert.NoError(t, err, "Worktree directory should exist at %s", worktreePath)

	t.Logf("✓ Worktree session created successfully in %v at %s", elapsed, worktreePath)
}

// testTmuxSocket derives a per-test tmux server socket name that is also
// unique per process (via PID). These tests set "exit-empty off" so the
// isolated tmux server outlives the session and the test process itself;
// a bare test-name-derived socket (no PID) is a fixed path that collides
// with an orphaned server left behind by an earlier run of the same test
// on the same machine -- surfacing as "server exited unexpectedly" when
// that orphan has since died but its socket file lingers. See
// testSocketOnce in session/tmux/tmux.go for the same PID-based pattern
// used for the shared "" -> isolated-socket case.
func testTmuxSocket(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test_%s_%d", strings.ReplaceAll(t.Name(), "/", "_"), os.Getpid())
}

// commandExists checks if a command is available in PATH
func commandExists(cmd string) bool {
	_, err := filepath.Abs(cmd)
	if err == nil {
		// If we can get absolute path, check if file exists
		if _, err := os.Stat(cmd); err == nil {
			return true
		}
	}

	// Check in PATH
	path := os.Getenv("PATH")
	for _, dir := range strings.Split(path, ":") {
		if dir == "" {
			continue
		}
		cmdPath := filepath.Join(dir, cmd)
		if _, err := os.Stat(cmdPath); err == nil {
			return true
		}
	}
	return false
}
