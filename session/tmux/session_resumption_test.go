package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSessionResumption tests the new behavior where existing sessions are reused
// instead of failing with "tmux session already exists" error
func TestSessionResumption(t *testing.T) {
	t.Run("ExistingSession_ShouldReuse_NotFail", func(t *testing.T) {
		testExistingSessionReuse(t)
	})

	t.Run("ExistingSession_WithCleanup_ShouldSetupProperly", func(t *testing.T) {
		testExistingSessionWithCleanup(t)
	})

	t.Run("NewSession_ShouldCreateNormally", func(t *testing.T) {
		testNewSessionCreation(t)
	})
}

// testExistingSessionReuse tests that when a session already exists,
// the Start() method reuses it instead of failing
func testExistingSessionReuse(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Mock executor that simulates an existing session
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			if strings.Contains(cmdStr, "has-session") {
				// Simulate existing session
				return nil
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()
			// Handle DoesSessionExist() which uses list-sessions
			if strings.Contains(cmdStr, "list-sessions") && strings.Contains(cmdStr, "#{session_name}") {
				// Return the session name to indicate it exists
				return []byte("claudesquad_existing-session"), nil
			}
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("existing-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// This should succeed instead of failing with "tmux session already exists"
	err := session.Start("/tmp")
	require.NoError(t, err, "Starting existing session should succeed (reuse)")

	// Verify no new-session command was called since session already exists
	newSessionCalled := false
	for _, cmd := range ptyFactory.cmds {
		cmdStr := fmt.Sprintf("%s", cmd.Args)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCalled = true
			break
		}
	}
	require.False(t, newSessionCalled, "Should not create new session when one already exists")

	t.Logf("✓ Existing session was reused successfully without creating a new one")
}

// testExistingSessionWithCleanup tests that cleanup functions are properly set up
// even when reusing existing sessions
func testExistingSessionWithCleanup(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Mock executor that simulates an existing session
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			if strings.Contains(cmdStr, "has-session") {
				// Simulate existing session
				return nil
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()
			// Handle DoesSessionExist() which uses list-sessions
			if strings.Contains(cmdStr, "list-sessions") && strings.Contains(cmdStr, "#{session_name}") {
				// Return the session name to indicate it exists
				return []byte("claudesquad_existing-with-cleanup"), nil
			}
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("existing-with-cleanup", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// StartWithCleanup should succeed and return a valid cleanup function
	cleanup, err := session.StartWithCleanup("/tmp")
	require.NoError(t, err, "StartWithCleanup on existing session should succeed")
	require.NotNil(t, cleanup, "Cleanup function should be returned for existing sessions")

	// Test that cleanup function works (shouldn't panic)
	cleanupErr := cleanup()
	// In mock, Close() will likely return nil or a mock error - either is fine
	t.Logf("Cleanup function executed with result: %v", cleanupErr)

	t.Logf("✓ Cleanup function properly set up for existing session")
}

// testNewSessionCreation tests that new sessions are still created normally
// when no existing session is found
func testNewSessionCreation(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Mock executor that simulates no existing session initially, then exists after creation
	sessionCreated := false
	newSessionCommandCalled := false
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			if strings.Contains(cmdStr, "new-session") {
				sessionCreated = true
				newSessionCommandCalled = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()
			// Handle DoesSessionExist() which uses list-sessions
			if strings.Contains(cmdStr, "list-sessions") && strings.Contains(cmdStr, "#{session_name}") {
				if sessionCreated {
					// Session exists after creation
					return []byte("claudesquad_new-session"), nil
				}
				// No session initially
				return nil, fmt.Errorf("no server running")
			}
			return []byte("output"), nil
		},
	}

	session := newTmuxSession("new-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// This should create a new session
	err := session.Start("/tmp")
	require.NoError(t, err, "Starting new session should succeed")

	// Verify new-session command was called via cmdExec.Run()
	require.True(t, newSessionCommandCalled, "Should create new session when none exists")

	t.Logf("✓ New session was created successfully")
}

// TestSessionResumptionBehaviorComparison compares old vs new behavior
func TestSessionResumptionBehaviorComparison(t *testing.T) {
	t.Run("OLD_Behavior_Would_Fail", func(t *testing.T) {
		// This test documents what the OLD behavior would have been
		// Before the fix: session.Start() would return an error like:
		// "tmux session already exists: claudesquad_test-session"

		t.Logf("OLD behavior: session.Start() would fail with 'tmux session already exists' error")
		t.Logf("This would cause the entire session creation to fail, leading to:")
		t.Logf("  - User sees hanging behavior")
		t.Logf("  - No session gets created")
		t.Logf("  - User has to manually clean up tmux sessions")
	})

	t.Run("NEW_Behavior_Succeeds", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)

		// Simulate existing session
		cmdExec := MockCmdExec{
			RunFunc: func(cmd *exec.Cmd) error {
				cmdStr := cmd.String()
				if strings.Contains(cmdStr, "has-session") {
					return nil // Session exists
				}
				return nil
			},
			OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				cmdStr := cmd.String()
				// Handle DoesSessionExist() which uses list-sessions
				if strings.Contains(cmdStr, "list-sessions") && strings.Contains(cmdStr, "#{session_name}") {
					// Return the session name to indicate it exists
					return []byte("claudesquad_resumption-test"), nil
				}
				return []byte("output"), nil
			},
		}

		session := newTmuxSession("resumption-test", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		// NEW behavior: this succeeds by reusing existing session
		err := session.Start("/tmp")
		require.NoError(t, err, "NEW behavior: existing session should be reused successfully")

		t.Logf("✓ NEW behavior: session.Start() succeeds by reusing existing session")
		t.Logf("✓ This provides seamless user experience:")
		t.Logf("  - No hanging or errors")
		t.Logf("  - Existing session is reused")
		t.Logf("  - User can continue working immediately")
	})
}

// TestSessionResumptionIntegration tests the complete integration scenario
// that reproduces the original user-reported issue
func TestSessionResumptionIntegration(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Simulate the exact scenario from the error logs:
	// 1. User tries to create session for 'postgres-connection-pooling'
	// 2. Session already exists (maybe from previous attempt)
	// 3. Should reuse instead of failing

	listSessionsCalled := 0
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()
			// Handle DoesSessionExist() which uses list-sessions
			if strings.Contains(cmdStr, "list-sessions") && strings.Contains(cmdStr, "#{session_name}") {
				listSessionsCalled++
				// Return the session name to indicate it exists
				return []byte("claudesquad_postgres-connection-pooling"), nil
			}
			return []byte("output"), nil
		},
	}

	// Use the exact session name from the error logs
	session := newTmuxSession("postgres-connection-pooling", "/Users/tylerstapler/.asdf/shims/claude", ptyFactory, cmdExec, TmuxPrefix)

	// This should succeed (reproducing the fix for the reported issue)
	err := session.Start("/Users/tylerstapler/IdeaProjects/pgbouncer-images")
	require.NoError(t, err, "Session creation should succeed for existing session")

	// Verify session existence was checked
	require.Greater(t, listSessionsCalled, 0, "Should have checked for existing session")

	// Verify no new session was created (since one already exists)
	newSessionCreated := false
	for _, cmd := range ptyFactory.cmds {
		cmdStr := fmt.Sprintf("%s", cmd.Args)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCreated = true
			break
		}
	}
	require.False(t, newSessionCreated, "Should not create new session when existing one can be reused")

	t.Logf("✓ Integration test passed: postgres-connection-pooling session reused successfully")
	t.Logf("✓ This validates the fix for the original user-reported issue")
}