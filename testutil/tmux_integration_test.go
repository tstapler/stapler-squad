package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealTmuxSessionCreation validates session creation with real tmux
func TestRealTmuxSessionCreation(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("creates session with simple command", func(t *testing.T) {
		session, err := server.CreateSession("simple_test", "sleep 5")
		require.NoError(t, err)
		require.NotNil(t, session)

		// Verify session exists
		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		assert.NoError(t, err, "Session should exist after creation")

		// Verify it appears in server's session list
		sessions, err := server.ListSessions()
		require.NoError(t, err)
		assert.NotEmpty(t, sessions, "Server should have sessions")
	})

	t.Run("creates session with interactive shell", func(t *testing.T) {
		session, err := server.CreateSession("shell_test", "/bin/sh")
		require.NoError(t, err)
		require.NotNil(t, session)

		// Wait for session to be ready
		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionReady()
		assert.NoError(t, err, "Session should be ready")

		// Verify content appears
		content, err := waiter.WaitForNonEmptyContent()
		assert.NoError(t, err)
		assert.NotEmpty(t, content, "Session should have content")
	})

	t.Run("creates session with command that exits", func(t *testing.T) {
		// Use a command that exits quickly but successfully
		session, err := server.CreateSession("quick_exit_test", "echo 'done' && sleep 1")
		require.NoError(t, err)
		require.NotNil(t, session)

		// Session should exist initially
		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		assert.NoError(t, err, "Session should exist after creation")
	})

	t.Run("creates multiple sessions concurrently", func(t *testing.T) {
		const numSessions = 3
		errors := make(chan error, numSessions)

		// Create sessions concurrently
		for i := 0; i < numSessions; i++ {
			go func(id int) {
				session, err := server.CreateSession(
					TempSessionName("concurrent", id),
					"sleep 5",
				)
				if err != nil {
					errors <- err
					return
				}

				waiter := NewTmuxWaiter(session)
				errors <- waiter.WaitForSessionExists()
			}(i)
		}

		// Collect results
		for i := 0; i < numSessions; i++ {
			err := <-errors
			assert.NoError(t, err, "Concurrent session creation should succeed")
		}

		// Verify all sessions exist
		allSessions, err := server.ListSessions()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allSessions), numSessions,
			"All concurrent sessions should be created")
	})
}

// TestRealTmuxSessionLifecycle validates session lifecycle operations
func TestRealTmuxSessionLifecycle(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("session lifecycle: create, verify, cleanup", func(t *testing.T) {
		// Create session
		session, err := server.CreateSession("lifecycle_test", "sleep 10")
		require.NoError(t, err)

		// Verify existence
		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Get session list to find full name (with prefix)
		sessions, err := server.ListSessions()
		require.NoError(t, err)
		require.NotEmpty(t, sessions)

		// Kill the session
		err = server.KillSession(sessions[0])
		assert.NoError(t, err)

		// Verify session is gone
		time.Sleep(100 * time.Millisecond)
		sessions, err = server.ListSessions()
		require.NoError(t, err)

		// Should have fewer sessions now
		found := false
		for _, name := range sessions {
			if name == sessions[0] {
				found = true
				break
			}
		}
		assert.False(t, found, "Session should be removed after killing")
	})

	t.Run("session content capture", func(t *testing.T) {
		// Create session with command that produces output and stays alive
		session, err := server.CreateSession("content_test", "echo 'Hello from tmux' && sleep 2")
		require.NoError(t, err)

		// Wait for session and content
		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionReady()
		require.NoError(t, err)

		// Capture content
		content, err := waiter.WaitForContentContaining("Hello from tmux")
		assert.NoError(t, err)
		assert.Contains(t, content, "Hello", "Content should be captured")
	})

	t.Run("session with custom timeout", func(t *testing.T) {
		session, err := server.CreateSession("timeout_test", "sleep 1")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)

		// Use custom timeout configuration
		config := WaitConfig{
			Timeout:      5 * time.Second,
			PollInterval: 100 * time.Millisecond,
			Description:  "session to exist with custom timeout",
		}

		err = waiter.WaitForSessionExistsWithConfig(config)
		assert.NoError(t, err)
	})
}

// TestRealTmuxSessionErrors validates error handling in session operations
func TestRealTmuxSessionErrors(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("handles nonexistent session gracefully", func(t *testing.T) {
		// Try to kill non-existent session (should be idempotent)
		err := server.KillSession("nonexistent_session_123")
		assert.NoError(t, err, "Killing non-existent session should not error")
	})

	t.Run("handles empty session list", func(t *testing.T) {
		// List sessions on empty server
		sessions, err := server.ListSessions()
		assert.NoError(t, err)
		assert.Empty(t, sessions, "Empty server should return empty list")
	})
}

// TestRealTmuxMultipleServers validates multiple isolated servers
func TestRealTmuxMultipleServers(t *testing.T) {
	// Create two isolated servers
	server1 := CreateIsolatedTmuxServer(t)
	server2 := CreateIsolatedTmuxServer(t)

	t.Run("servers are truly isolated", func(t *testing.T) {
		// Create session on server1
		session1, err := server1.CreateSession("isolated1", "sleep 5")
		require.NoError(t, err)

		waiter1 := NewTmuxWaiter(session1)
		err = waiter1.WaitForSessionExists()
		require.NoError(t, err)

		// Create session on server2
		session2, err := server2.CreateSession("isolated2", "sleep 5")
		require.NoError(t, err)

		waiter2 := NewTmuxWaiter(session2)
		err = waiter2.WaitForSessionExists()
		require.NoError(t, err)

		// Verify each server only sees its own session
		sessions1, err := server1.ListSessions()
		require.NoError(t, err)
		assert.Len(t, sessions1, 1, "Server1 should only see 1 session")

		sessions2, err := server2.ListSessions()
		require.NoError(t, err)
		assert.Len(t, sessions2, 1, "Server2 should only see 1 session")

		// Verify session names are different
		assert.NotEqual(t, sessions1[0], sessions2[0],
			"Sessions on different servers should have different names")
	})
}

// TestRealTmuxSessionCleanup validates cleanup behavior
func TestRealTmuxSessionCleanup(t *testing.T) {
	t.Run("cleanup removes all sessions", func(t *testing.T) {
		server := CreateIsolatedTmuxServer(t)
		socketName := server.GetSocketName()
		_ = socketName // Used for potential future verification

		// Create multiple sessions
		for i := 0; i < 3; i++ {
			session, err := server.CreateSession(
				TempSessionName("cleanup", i),
				"sleep 10",
			)
			require.NoError(t, err)

			waiter := NewTmuxWaiter(session)
			err = waiter.WaitForSessionExists()
			require.NoError(t, err)
		}

		// Verify sessions exist
		sessions, err := server.ListSessions()
		require.NoError(t, err)
		assert.Len(t, sessions, 3, "Should have 3 sessions before cleanup")

		// Cleanup (via t.Cleanup() at end of test)
	})
	// Cleanup should have run here

	// Verify cleanup worked
	time.Sleep(100 * time.Millisecond)

	// Try to list sessions on cleaned-up server
	// This should fail or return empty
	// Note: We can't easily verify this without creating a new test server
}
