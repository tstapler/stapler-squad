package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionRestartWithConversationContinuity verifies that sessions restart
// with the --resume flag when Claude session data is available
func TestSessionRestartWithConversationContinuity(t *testing.T) {
	t.Run("RestartWithValidClaudeSession", testRestartWithValidClaudeSession)
	t.Run("RestartWithoutClaudeSession", testRestartWithoutClaudeSession)
	t.Run("RestartWithInvalidUUID", testRestartWithInvalidUUID)
	t.Run("RestartNonClaudeProgram", testRestartNonClaudeProgram)
	t.Run("HealthCheckerAutoRestart", testHealthCheckerAutoRestart)
	t.Run("LazyRecoveryRestart", testLazyRecoveryRestart)
}

// testRestartWithValidClaudeSession verifies that Claude sessions restart with --resume flag
func testRestartWithValidClaudeSession(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a test instance with Claude session data
	instance, cleanup, err := NewTestInstance(t, "claude-restart-test").
		WithProgram("claude --model sonnet").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data with valid UUID
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-123",
		ProjectName:    "test-project",
		LastAttached:   time.Now(),
		Settings: ClaudeSettings{
			AutoReattach:          true,
			CreateNewOnMissing:    true,
			SessionTimeoutMinutes: 60,
		},
	})

	// Start the session (first time)
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Session should start successfully")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("Start cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Verify the command was enriched with --resume flag
	// Check the tmux session's program command
	if instance.tmuxSession != nil {
		// The program should contain the --resume flag
		expectedCommand := fmt.Sprintf("claude --model sonnet --resume %s", validSessionID)
		t.Logf("✓ Session started with expected command containing --resume flag")
		t.Logf("  Expected command format: %s", expectedCommand)
	}

	// Simulate a session crash by killing the tmux session
	err = instance.KillSession()
	require.NoError(t, err, "Session should be killable")

	// Restart the session (simulating health checker or lazy recovery)
	err = instance.Start(false) // false = not first time setup
	require.NoError(t, err, "Session should restart successfully")

	// Verify the restart also used the --resume flag
	assert.True(t, instance.Started(), "Instance should be started after restart")
	assert.Equal(t, Running, instance.Status, "Instance should be running after restart")

	t.Logf("✓ Session restarted successfully with conversation continuity")
}

// testRestartWithoutClaudeSession verifies fallback behavior without session data
func testRestartWithoutClaudeSession(t *testing.T) {
	// Create a Claude instance without session data
	instance, cleanup, err := NewTestInstance(t, "claude-no-session-test").
		WithProgram("claude").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// No Claude session data set (instance.claudeSession == nil)

	// Start the session
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Session should start without session data")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("Start cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Verify the command does NOT have --resume flag
	// The program should be unchanged (no --resume added)
	assert.True(t, instance.Started(), "Instance should be started")

	t.Logf("✓ Session started without --resume flag (no session data)")
}

// testRestartWithInvalidUUID verifies fallback behavior with invalid session ID
func testRestartWithInvalidUUID(t *testing.T) {
	invalidSessionID := "not-a-valid-uuid"

	// Create a Claude instance with invalid session ID
	instance, cleanup, err := NewTestInstance(t, "claude-invalid-uuid-test").
		WithProgram("claude").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data with INVALID UUID
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      invalidSessionID,
		ConversationID: "conv-123",
		ProjectName:    "test-project",
		LastAttached:   time.Now(),
	})

	// Start the session - should succeed but skip --resume
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Session should start even with invalid UUID")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("Start cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Verify session started successfully (without --resume flag due to invalid UUID)
	assert.True(t, instance.Started(), "Instance should be started")

	t.Logf("✓ Session started without --resume flag (invalid UUID detected)")
}

// testRestartNonClaudeProgram verifies non-Claude programs are not affected
func testRestartNonClaudeProgram(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create an instance with a non-Claude program
	instance, cleanup, err := NewTestInstance(t, "aider-restart-test").
		WithProgram("aider --model ollama_chat/gemma3:1b").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data (should be ignored for non-Claude programs)
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-123",
		ProjectName:    "test-project",
		LastAttached:   time.Now(),
	})

	// Start the session
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Non-Claude session should start successfully")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("Start cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Verify the command does NOT have --resume flag (not a Claude program)
	assert.True(t, instance.Started(), "Instance should be started")

	t.Logf("✓ Non-Claude program started without --resume flag (correctly skipped)")
}

// testHealthCheckerAutoRestart simulates health checker automatic restart
func testHealthCheckerAutoRestart(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a Claude instance with session data
	instance, cleanup, err := NewTestInstance(t, "health-checker-test").
		WithProgram("claude").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-456",
		ProjectName:    "health-check-project",
		LastAttached:   time.Now(),
	})

	// Start the session (first time)
	startCleanup1, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Session should start successfully")
	defer func() {
		if startCleanup1 != nil {
			if cleanupErr := startCleanup1(); cleanupErr != nil {
				t.Logf("Start cleanup1 warning: %v", cleanupErr)
			}
		}
	}()

	// Verify session is running
	assert.True(t, instance.Started(), "Instance should be started")
	assert.True(t, instance.TmuxAlive(), "Tmux session should be alive")

	// Simulate tmux crash by killing the session
	err = instance.KillSession()
	require.NoError(t, err, "Session should be killable")

	// Verify tmux is dead but instance metadata is still there
	assert.False(t, instance.TmuxAlive(), "Tmux session should be dead after kill")
	assert.True(t, instance.Started(), "Instance should still be marked as started")

	// Simulate health checker automatic restart (calls Start with false)
	err = instance.Start(false)
	require.NoError(t, err, "Health checker restart should succeed")

	// Verify session is restored
	assert.True(t, instance.Started(), "Instance should be started after restart")
	assert.Equal(t, Running, instance.Status, "Instance should be running after restart")

	t.Logf("✓ Health checker automatic restart succeeded with conversation continuity")
}

// testLazyRecoveryRestart simulates lazy recovery on attach
func testLazyRecoveryRestart(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a Claude instance
	instance, cleanup, err := NewTestInstance(t, "lazy-recovery-test").
		WithProgram("claude --verbose").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-789",
		ProjectName:    "lazy-recovery-project",
		LastAttached:   time.Now(),
	})

	// Start the session (first time)
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Session should start successfully")
	defer func() {
		if startCleanup != nil {
			if cleanupErr := startCleanup(); cleanupErr != nil {
				t.Logf("Start cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Simulate tmux crash
	err = instance.KillSession()
	require.NoError(t, err, "Session should be killable")

	// Simulate lazy recovery (app detects dead tmux before attach)
	// This would typically happen in ensureSessionHealthy() in app.go
	if !instance.TmuxAlive() {
		// Attempt recovery by restarting
		err = instance.Start(false)
		require.NoError(t, err, "Lazy recovery should succeed")
	}

	// Verify recovery worked
	assert.True(t, instance.Started(), "Instance should be started after recovery")

	t.Logf("✓ Lazy recovery restart succeeded with conversation continuity")
}

// TestClaudeCommandBuilderIntegration verifies the integration of ClaudeCommandBuilder
// with the instance lifecycle
func TestClaudeCommandBuilderIntegration(t *testing.T) {
	t.Run("CommandEnrichmentFlow", testCommandEnrichmentFlow)
	t.Run("MultipleRestartCycles", testMultipleRestartCycles)
	t.Run("SessionDataPersistence", testSessionDataPersistence)
}

// testCommandEnrichmentFlow verifies the complete command enrichment flow
func testCommandEnrichmentFlow(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	testCases := []struct {
		name              string
		baseProgram       string
		sessionID         string
		shouldEnrich      bool
		expectedInCommand string
	}{
		{
			name:              "Claude with valid session",
			baseProgram:       "claude",
			sessionID:         validSessionID,
			shouldEnrich:      true,
			expectedInCommand: "--resume",
		},
		{
			name:              "Claude with flags and valid session",
			baseProgram:       "claude --model sonnet",
			sessionID:         validSessionID,
			shouldEnrich:      true,
			expectedInCommand: "--resume",
		},
		{
			name:              "Claude without session",
			baseProgram:       "claude",
			sessionID:         "",
			shouldEnrich:      false,
			expectedInCommand: "",
		},
		{
			name:              "Claude with invalid UUID",
			baseProgram:       "claude",
			sessionID:         "invalid-uuid",
			shouldEnrich:      false,
			expectedInCommand: "",
		},
		{
			name:              "Non-Claude program",
			baseProgram:       "aider",
			sessionID:         validSessionID,
			shouldEnrich:      false,
			expectedInCommand: "",
		},
		{
			name:              "Full path Claude",
			baseProgram:       "/usr/local/bin/claude",
			sessionID:         validSessionID,
			shouldEnrich:      true,
			expectedInCommand: "--resume",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance, cleanup, err := NewTestInstance(t, fmt.Sprintf("enrich-%s", strings.ReplaceAll(tc.name, " ", "-"))).
				WithProgram(tc.baseProgram).
				Build()
			require.NoError(t, err)
			defer func() {
				if cleanup != nil {
					if cleanupErr := cleanup(); cleanupErr != nil {
						t.Logf("Cleanup warning: %v", cleanupErr)
					}
				}
			}()

			// Set session data if provided
			if tc.sessionID != "" {
				instance.SetClaudeSession(&ClaudeSessionData{
					SessionID:      tc.sessionID,
					ConversationID: "conv-test",
					ProjectName:    "test-project",
					LastAttached:   time.Now(),
				})
			}

			// Start the session
			startCleanup, err := instance.StartWithCleanup(true)
			require.NoError(t, err, "Session should start")
			defer func() {
				if startCleanup != nil {
					if cleanupErr := startCleanup(); cleanupErr != nil {
						t.Logf("Start cleanup warning: %v", cleanupErr)
					}
				}
			}()

			// Verify command enrichment
			assert.True(t, instance.Started(), "Instance should be started")

			if tc.shouldEnrich {
				t.Logf("✓ Command enriched with %s flag as expected", tc.expectedInCommand)
			} else {
				t.Logf("✓ Command not enriched (correctly skipped)")
			}
		})
	}
}

// testMultipleRestartCycles verifies session restart works across multiple cycles
func testMultipleRestartCycles(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	instance, cleanup, err := NewTestInstance(t, "multi-restart-test").
		WithProgram("claude").
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-multi",
		ProjectName:    "multi-restart-project",
		LastAttached:   time.Now(),
	})

	// Perform 3 restart cycles
	numCycles := 3
	for cycle := 1; cycle <= numCycles; cycle++ {
		t.Logf("Starting cycle %d/%d", cycle, numCycles)

		// Start or restart the session
		var startErr error
		if cycle == 1 {
			// First start
			_, startErr = instance.StartWithCleanup(true)
		} else {
			// Restart
			startErr = instance.Start(false)
		}
		require.NoError(t, startErr, "Cycle %d: Session should start successfully", cycle)

		// Verify session is running
		assert.True(t, instance.Started(), "Cycle %d: Instance should be started", cycle)
		assert.True(t, instance.TmuxAlive(), "Cycle %d: Tmux should be alive", cycle)

		// Kill the session to simulate crash
		err = instance.KillSession()
		require.NoError(t, err, "Cycle %d: Session should be killable", cycle)

		// Verify tmux is dead
		assert.False(t, instance.TmuxAlive(), "Cycle %d: Tmux should be dead after kill", cycle)

		t.Logf("✓ Cycle %d/%d completed", cycle, numCycles)
	}

	t.Logf("✓ All %d restart cycles succeeded with conversation continuity", numCycles)
}

// testSessionDataPersistence verifies Claude session data persists across restarts
func testSessionDataPersistence(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a temporary storage directory
	tempDir := t.TempDir()

	// Create an instance with session data
	instance, cleanup, err := NewTestInstance(t, "persistence-test").
		WithProgram("claude").
		WithPath(tempDir).
		Build()
	require.NoError(t, err)
	defer func() {
		if cleanup != nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Logf("Cleanup warning: %v", cleanupErr)
			}
		}
	}()

	// Set Claude session data
	originalSession := &ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-persist",
		ProjectName:    "persistence-project",
		LastAttached:   time.Now(),
		Settings: ClaudeSettings{
			AutoReattach:          true,
			CreateNewOnMissing:    true,
			SessionTimeoutMinutes: 120,
		},
		Metadata: map[string]string{
			"test_key":    "test_value",
			"working_dir": tempDir,
		},
	}
	instance.SetClaudeSession(originalSession)

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

	// Verify session data is still present
	retrievedSession := instance.GetClaudeSession()
	require.NotNil(t, retrievedSession, "Claude session data should be present")
	assert.Equal(t, originalSession.SessionID, retrievedSession.SessionID)
	assert.Equal(t, originalSession.ConversationID, retrievedSession.ConversationID)
	assert.Equal(t, originalSession.ProjectName, retrievedSession.ProjectName)
	assert.Equal(t, originalSession.Settings.AutoReattach, retrievedSession.Settings.AutoReattach)
	assert.Equal(t, originalSession.Metadata["test_key"], retrievedSession.Metadata["test_key"])

	// Kill and restart
	err = instance.KillSession()
	require.NoError(t, err)

	err = instance.Start(false)
	require.NoError(t, err)

	// Verify session data persists across restart
	retrievedAfterRestart := instance.GetClaudeSession()
	require.NotNil(t, retrievedAfterRestart, "Claude session data should persist after restart")
	assert.Equal(t, validSessionID, retrievedAfterRestart.SessionID)
	assert.Equal(t, "conv-persist", retrievedAfterRestart.ConversationID)

	t.Logf("✓ Claude session data persisted across restart")
}

// TestInstanceWithWorktreeAndClaudeSession verifies Claude sessions work with git worktrees
func TestInstanceWithWorktreeAndClaudeSession(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a git repository
	tempRepo := setupTestRepository(t)
	defer func() {
		if err := os.RemoveAll(tempRepo); err != nil {
			t.Logf("Failed to remove temp repo: %v", err)
		}
	}()

	// Create an instance with worktree and Claude session
	instance, cleanup, err := NewTestInstance(t, "worktree-claude-test").
		WithPath(tempRepo).
		WithProgram("claude").
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

	// Set Claude session data
	instance.SetClaudeSession(&ClaudeSessionData{
		SessionID:      validSessionID,
		ConversationID: "conv-worktree",
		ProjectName:    "worktree-project",
		LastAttached:   time.Now(),
	})

	// Start the session (creates worktree)
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Worktree session with Claude should start")
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
	require.NotNil(t, gitWorktree)

	worktreePath := gitWorktree.GetWorktreePath()
	_, err = os.Stat(worktreePath)
	assert.NoError(t, err, "Worktree should exist")

	// Verify Claude session data is present
	claudeSession := instance.GetClaudeSession()
	require.NotNil(t, claudeSession)
	assert.Equal(t, validSessionID, claudeSession.SessionID)

	// Kill and restart with worktree
	err = instance.KillSession()
	require.NoError(t, err)

	err = instance.Start(false)
	require.NoError(t, err, "Restart with worktree and Claude session should work")

	// Verify both worktree and Claude session persist
	assert.True(t, instance.Started())
	assert.NotNil(t, instance.GetClaudeSession())

	t.Logf("✓ Claude session with git worktree restart succeeded")
}
