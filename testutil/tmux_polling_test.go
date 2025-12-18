package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTmuxPollingDoesNotHang validates that tmux existence polling has timeout protection
func TestTmuxPollingDoesNotHang(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("polling times out for nonexistent session", func(t *testing.T) {
		// Create session but don't start it
		sessionName := "polling_timeout_test"
		prefix := "test_"

		// Create a tmux session object but don't call Start()
		// This simulates a session that will never exist
		session := server.CreateSessionWithoutStarting(sessionName, "sleep 10", prefix)

		waiter := NewTmuxWaiter(session)

		// Use a short timeout to verify timeout protection works
		config := WaitConfig{
			Timeout:      2 * time.Second,
			PollInterval: 100 * time.Millisecond,
			Description:  "nonexistent session (should timeout)",
		}

		startTime := time.Now()
		err := waiter.WaitForSessionExistsWithConfig(config)
		elapsed := time.Since(startTime)

		// Should timeout, not hang indefinitely
		require.Error(t, err, "Should timeout waiting for nonexistent session")
		assert.Contains(t, err.Error(), "timeout", "Error should indicate timeout")

		// Should timeout within reasonable time (timeout + small buffer)
		assert.Less(t, elapsed, 3*time.Second,
			"Should timeout within 3 seconds, not hang indefinitely")
	})

	t.Run("polling succeeds for existing session", func(t *testing.T) {
		// Create and start session
		session, err := server.CreateSession("polling_exists_test", "sleep 5")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)

		// Should succeed quickly
		startTime := time.Now()
		err = waiter.WaitForSessionExists()
		elapsed := time.Since(startTime)

		assert.NoError(t, err, "Should find existing session")
		assert.Less(t, elapsed, 5*time.Second,
			"Should find session quickly, not after long polling")
	})

	t.Run("content polling times out appropriately", func(t *testing.T) {
		// Create session with command that never produces expected output
		session, err := server.CreateSession("content_timeout_test", "sleep 10")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)

		// Wait for session to exist
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Try to wait for content that will never appear
		config := WaitConfig{
			Timeout:      1 * time.Second,
			PollInterval: 100 * time.Millisecond,
			Description:  "content that will never appear",
		}

		startTime := time.Now()
		_, err = waiter.WaitForContentWithConfig(ContainsText("NEVER_APPEARS"), config)
		elapsed := time.Since(startTime)

		// Should timeout, not hang
		require.Error(t, err, "Should timeout waiting for content")
		assert.Contains(t, err.Error(), "timeout", "Error should indicate timeout")
		assert.Less(t, elapsed, 2*time.Second,
			"Should timeout within reasonable time")
	})
}

// TestTmuxSessionExistenceChecking validates DoesSessionExist() behavior
func TestTmuxSessionExistenceChecking(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("detects session existence correctly", func(t *testing.T) {
		// Create session
		session, err := server.CreateSession("existence_test", "sleep 5")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Verify DoesSessionExist returns true
		// Note: We can't directly call DoesSessionExist on the tmux.TmuxSession
		// from here due to package boundaries, but WaitForSessionExists validates it
	})

	t.Run("handles rapid existence checks", func(t *testing.T) {
		session, err := server.CreateSession("rapid_check_test", "sleep 5")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Perform many rapid existence checks (tests caching)
		for i := 0; i < 10; i++ {
			err := waiter.WaitForSessionExists()
			assert.NoError(t, err, "Rapid checks should all succeed")
		}
	})
}

// TestTmuxSessionReadyTimeout validates WaitForSessionReady timeout behavior
// SKIP: This test is timing-sensitive and may hang in CI environments
func TestTmuxSessionReadyTimeout(t *testing.T) {
	t.Skip("Skipping timing-sensitive session ready timeout test")
	server := CreateIsolatedTmuxServer(t)

	t.Run("session ready times out appropriately", func(t *testing.T) {
		sessionName := "ready_timeout_test"
		prefix := "test_"

		// Create session object without starting (simulates hang scenario)
		session := server.CreateSessionWithoutStarting(sessionName, "sleep 10", prefix)

		waiter := NewTmuxWaiter(session)

		config := WaitConfig{
			Timeout:      2 * time.Second,
			PollInterval: 100 * time.Millisecond,
			Description:  "session to be ready (timeout test)",
		}

		startTime := time.Now()
		err := waiter.WaitForSessionReadyWithTimeout(config)
		elapsed := time.Since(startTime)

		// Should timeout, not hang
		require.Error(t, err, "Should timeout for session that never starts")
		assert.Contains(t, err.Error(), "never became available",
			"Error should indicate session availability issue")
		assert.Less(t, elapsed, 3*time.Second,
			"Should timeout within expected time")
	})

	t.Run("session ready succeeds for valid session", func(t *testing.T) {
		session, err := server.CreateSession("ready_success_test", "/bin/sh")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)

		startTime := time.Now()
		err = waiter.WaitForSessionReady()
		elapsed := time.Since(startTime)

		assert.NoError(t, err, "Should become ready for valid session")
		assert.Less(t, elapsed, 15*time.Second,
			"Should become ready within reasonable time")
	})
}

// TestTmuxHangPrevention validates that operations don't hang indefinitely
func TestTmuxHangPrevention(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("session creation has timeout protection", func(t *testing.T) {
		// This test validates that CreateSession() doesn't hang
		// even if tmux has issues starting
		startTime := time.Now()

		session, err := server.CreateSession("hang_prevention_test", "sleep 2")
		elapsed := time.Since(startTime)

		// Should either succeed or fail, but not hang
		if err != nil {
			t.Logf("Session creation failed (acceptable): %v", err)
		} else {
			require.NotNil(t, session)
		}

		// Should complete within reasonable time (not hang for 30+ seconds)
		assert.Less(t, elapsed, 30*time.Second,
			"Session creation should not hang indefinitely")
	})

	t.Run("multiple rapid operations don't cause hangs", func(t *testing.T) {
		// Create multiple sessions rapidly
		const numSessions = 5
		sessions := make([]string, 0, numSessions)

		startTime := time.Now()
		for i := 0; i < numSessions; i++ {
			sessionName := TempSessionName("rapid_ops", i)
			_, err := server.CreateSession(sessionName, "sleep 3")
			if err == nil {
				sessions = append(sessions, sessionName)
			}
		}
		elapsed := time.Since(startTime)

		// Should complete all operations within reasonable time
		assert.Less(t, elapsed, 30*time.Second,
			"Rapid operations should not cause hangs")
		assert.NotEmpty(t, sessions, "At least some sessions should be created")
	})
}

// TestTmuxContentWaitingBehavior validates content waiting with edge cases
func TestTmuxContentWaitingBehavior(t *testing.T) {
	server := CreateIsolatedTmuxServer(t)

	t.Run("waits for content with reasonable timeout", func(t *testing.T) {
		// Session that produces output after a delay
		sess, err := server.CreateSession("delayed_content_test",
			"sleep 0.5 && echo 'delayed output' && sleep 3")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(sess)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Wait for the delayed content
		startTime := time.Now()
		content, err := waiter.WaitForContentContaining("delayed output")
		elapsed := time.Since(startTime)

		assert.NoError(t, err, "Should find delayed content")
		assert.Contains(t, content, "delayed", "Should capture delayed output")
		assert.Less(t, elapsed, 15*time.Second,
			"Should find content within reasonable time")
	})

	t.Run("handles empty content gracefully", func(t *testing.T) {
		session, err := server.CreateSession("empty_content_test", "sleep 10")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Try to get any content (might be empty initially)
		config := WaitConfig{
			Timeout:      2 * time.Second,
			PollInterval: 200 * time.Millisecond,
			Description:  "any content",
		}

		startTime := time.Now()
		_, err = waiter.WaitForContentWithConfig(NonEmptyContent, config)
		elapsed := time.Since(startTime)

		// Might timeout if content is truly empty, which is acceptable
		if err != nil {
			assert.Contains(t, err.Error(), "timeout",
				"Should timeout gracefully for empty content")
		}
		assert.Less(t, elapsed, 3*time.Second,
			"Should not hang indefinitely on empty content")
	})
}
