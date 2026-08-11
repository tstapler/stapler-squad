package testutil

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestTmuxTestServer_Creation validates isolated server creation
func TestTmuxTestServer_Creation(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Verify socket name is unique and test-specific
	socketName := server.GetSocketName()
	assert.NotEmpty(t, socketName)
	assert.Contains(t, socketName, "test_")
	assert.Contains(t, socketName, "TestTmuxTestServer_Creation")
}

// TestTmuxTestServer_SessionCreation validates session creation on isolated server
func TestTmuxTestServer_SessionCreation(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Create a test session
	sessionName := "test_session"
	command := "sleep 10"
	session, err := server.CreateSession(sessionName, command)

	require.NoError(t, err)
	require.NotNil(t, session)

	// Wait for session to exist
	waiter := NewTmuxWaiter(session)
	err = waiter.WaitForSessionExists()
	assert.NoError(t, err, "Session should exist after creation")

	// Verify session appears in server's list
	sessions, err := server.ListSessions()
	require.NoError(t, err)

	// Session name will have prefix added
	foundSession := false
	for _, name := range sessions {
		if strings.Contains(name, sessionName) {
			foundSession = true
			break
		}
	}
	assert.True(t, foundSession, "Created session should appear in server list")
}

// TestTmuxTestServer_SessionExists validates session existence checking
func TestTmuxTestServer_SessionExists(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Create a session
	sessionName := "test_exists"
	session, err := server.CreateSession(sessionName, "sleep 10")
	require.NoError(t, err)

	// Wait for it to exist
	waiter := NewTmuxWaiter(session)
	err = waiter.WaitForSessionExists()
	require.NoError(t, err)

	// Check existence directly
	sessions, err := server.ListSessions()
	require.NoError(t, err)
	assert.NotEmpty(t, sessions, "Should have at least one session")

	// Non-existent session should not be found
	nonExistent := server.SessionExists("nonexistent_session")
	assert.False(t, nonExistent, "Non-existent session should return false")
}

// TestTmuxTestServer_KillSession validates session cleanup
func TestTmuxTestServer_KillSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Create two sessions
	session1, err := server.CreateSession("session1", "sleep 10")
	require.NoError(t, err)

	session2, err := server.CreateSession("session2", "sleep 10")
	require.NoError(t, err)

	// Wait for both to exist
	waiter1 := NewTmuxWaiter(session1)
	err = waiter1.WaitForSessionExists()
	require.NoError(t, err)

	waiter2 := NewTmuxWaiter(session2)
	err = waiter2.WaitForSessionExists()
	require.NoError(t, err)

	// Get initial session count
	sessionsBefore, err := server.ListSessions()
	require.NoError(t, err)
	require.Len(t, sessionsBefore, 2, "Should have exactly 2 sessions")

	// Kill one session
	fullSessionName := sessionsBefore[0]
	err = server.KillSession(fullSessionName)
	assert.NoError(t, err)

	// Verify only one session remains
	time.Sleep(100 * time.Millisecond) // Brief wait for cleanup
	sessionsAfter, err := server.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessionsAfter, 1, "Should have exactly 1 session after killing one")
}

// TestTmuxTestServer_KillAllSessions validates bulk cleanup
func TestTmuxTestServer_KillAllSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Create multiple sessions
	for i := 0; i < 3; i++ {
		sessionName := fmt.Sprintf("session_%d", i)
		session, err := server.CreateSession(sessionName, "sleep 10")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)
	}

	// Verify all sessions exist
	sessions, err := server.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 3, "Should have 3 sessions before cleanup")

	// Kill all sessions
	err = server.KillAllSessions()
	assert.NoError(t, err)

	// Verify no sessions remain
	time.Sleep(100 * time.Millisecond)
	sessions, err = server.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions, "Should have no sessions after KillAllSessions")
}

// TestTmuxTestServer_AutomaticCleanup validates t.Cleanup() integration
func TestTmuxTestServer_AutomaticCleanup(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	var socketName string

	// Run in subtest so cleanup happens at end of subtest
	t.Run("CreateAndVerify", func(t *testing.T) {
		server := CreateIsolatedTmuxServer(t)
		socketName = server.GetSocketName()

		// Create a session
		session, err := server.CreateSession("cleanup_test", "sleep 10")
		require.NoError(t, err)

		waiter := NewTmuxWaiter(session)
		err = waiter.WaitForSessionExists()
		require.NoError(t, err)

		// Verify session exists
		sessions, err := server.ListSessions()
		require.NoError(t, err)
		assert.NotEmpty(t, sessions, "Session should exist during test")
	})
	// t.Cleanup() should have run here

	// Verify server and sessions are cleaned up
	time.Sleep(100 * time.Millisecond)

	// Try to list sessions on the now-dead server (should fail)
	listCtx, listCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer listCancel()
	cmd := safeexec.CommandContext(listCtx, "tmux", "-L", socketName, "list-sessions")
	output, err := cmd.CombinedOutput()

	// Should get an error indicating the server is gone.
	// "no server running" = tmux server process exited cleanly.
	// "server exited unexpectedly" = socket present but server process gone.
	// "error connecting to" = socket file removed (our cleanup deleted it).
	// All three mean the server is not running.
	if err != nil {
		outputStr := string(output)
		isGone := strings.Contains(outputStr, "no server running") ||
			strings.Contains(outputStr, "server exited unexpectedly") ||
			strings.Contains(outputStr, "error connecting to")
		assert.True(t, isGone,
			"Server should be gone after cleanup, got: %q", outputStr)
	} else {
		// If no error, sessions list should be empty
		sessions := strings.TrimSpace(string(output))
		assert.Empty(t, sessions, "No sessions should remain after cleanup")
	}
}

// TestTmuxTestServer_Isolation validates that multiple test servers don't interfere
func TestTmuxTestServer_Isolation(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	// Create two isolated servers
	server1 := CreateIsolatedTmuxServer(t)
	server2 := CreateIsolatedTmuxServer(t)

	// Verify they have different socket names
	assert.NotEqual(t, server1.GetSocketName(), server2.GetSocketName(),
		"Isolated servers should have unique socket names")

	// Create sessions on each server
	session1, err := server1.CreateSession("server1_session", "sleep 10")
	require.NoError(t, err)

	session2, err := server2.CreateSession("server2_session", "sleep 10")
	require.NoError(t, err)

	// Wait for both to exist
	waiter1 := NewTmuxWaiter(session1)
	err = waiter1.WaitForSessionExists()
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

	// Verify sessions are different
	assert.NotEqual(t, sessions1[0], sessions2[0],
		"Isolated servers should have different sessions")
}

// TestTmuxTestServer_ConcurrentAccess validates thread safety
func TestTmuxTestServer_ConcurrentAccess(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Create multiple sessions concurrently
	const numConcurrent = 5
	var wg sync.WaitGroup
	errors := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sessionName := fmt.Sprintf("concurrent_%d", id)
			session, err := server.CreateSession(sessionName, "sleep 10")
			if err != nil {
				errors <- fmt.Errorf("session %d creation failed: %w", id, err)
				return
			}

			waiter := NewTmuxWaiter(session)
			if err := waiter.WaitForSessionExists(); err != nil {
				errors <- fmt.Errorf("session %d existence check failed: %w", id, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	var errorList []error
	for err := range errors {
		errorList = append(errorList, err)
	}
	assert.Empty(t, errorList, "No errors should occur during concurrent session creation")

	// Verify all sessions were created
	sessions, err := server.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, numConcurrent, "All concurrent sessions should be created")
}

// TestTmuxTestServer_KillNonExistentSession validates error handling
func TestTmuxTestServer_KillNonExistentSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// Try to kill non-existent session (should not error)
	err := server.KillSession("nonexistent_session")
	assert.NoError(t, err, "Killing non-existent session should be idempotent")
}

// TestTmuxTestServer_EmptyServerList validates listing with no sessions
func TestTmuxTestServer_EmptyServerList(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping test")
	}

	server := CreateIsolatedTmuxServer(t)

	// List sessions on empty server
	sessions, err := server.ListSessions()
	assert.NoError(t, err)
	assert.Empty(t, sessions, "Empty server should return empty session list")
}

// TestSanitizeTestName validates test name sanitization
func TestSanitizeTestName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "TestSimple",
			expected: "TestSimple",
		},
		{
			input:    "Test/With/Slashes",
			expected: "Test_With_Slashes",
		},
		{
			input:    "Test With Spaces",
			expected: "Test_With_Spaces",
		},
		{
			input:    "Test.With.Dots",
			expected: "Test_With_Dots",
		},
		{
			input:    "Test:With:Colons",
			expected: "Test_With_Colons",
		},
		{
			input:    "Test(With)Parens",
			expected: "Test_With_Parens",
		},
		{
			input:    "Complex/Test:With.All(Characters)",
			expected: "Complex_Test_With_All_Characters_",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeTestName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
