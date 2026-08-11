package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTUISessionCreation validates real TUI session creation flow
// SKIP: These tests require PTY handling via go-expect and are flaky/timing-sensitive.
// They may hang in CI environments or when TTY is not available.
func TestTUISessionCreation(t *testing.T) {
	t.Skip("Skipping PTY-based TUI session creation tests - timing sensitive and requires TTY")
	// Skip if binary doesn't exist
	BuildTestBinary(t)

	// Create isolated tmux server for this test
	tmuxServer := CreateIsolatedTmuxServer(t)

	t.Run("starts TUI and shows main menu", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err, "Should start TUI")

		// Give TUI time to initialize and render
		time.Sleep(1 * time.Second)

		// Verify TUI is running
		assert.True(t, session.IsRunning(), "TUI should be running")

		// Try to capture some output
		output, err := session.GetOutput()
		if err == nil && len(output) > 0 {
			t.Logf("TUI output: %s", output)
		}
	})

	t.Run("navigates to session creation", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Wait for TUI to be ready
		time.Sleep(1 * time.Second)

		// Send 'n' to trigger new session creation
		err = session.SendKeys("n")
		assert.NoError(t, err, "Should send 'n' key without error")

		// Give time for session creation wizard to appear
		time.Sleep(500 * time.Millisecond)

		// Check if session is still running (didn't crash)
		assert.True(t, session.IsRunning(), "TUI should still be running after 'n' press")
	})

	t.Run("escapes from session creation", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Wait for TUI to be ready
		time.Sleep(1 * time.Second)

		// Send 'n' to trigger new session creation
		err = session.SendKeys("n")
		require.NoError(t, err)

		// Wait for wizard to appear
		time.Sleep(500 * time.Millisecond)

		// Send escape to cancel
		err = session.SendEscape()
		assert.NoError(t, err, "Should send escape key")

		// Wait for return to main view
		time.Sleep(500 * time.Millisecond)

		// TUI should still be running
		assert.True(t, session.IsRunning(), "TUI should still be running after escape")
	})

	t.Run("creates simple session with auto-yes mode", func(t *testing.T) {
		// Use auto-yes mode to skip all prompts
		config := DefaultExpectConfig()
		config.Args = []string{"-y"} // Auto-yes mode
		config.Timeout = 15 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Wait for TUI to be ready
		time.Sleep(1 * time.Second)

		// List sessions before creation
		sessionsBefore, err := tmuxServer.ListSessions()
		if err != nil {
			sessionsBefore = []string{}
		}
		t.Logf("Sessions before: %d", len(sessionsBefore))

		// Send 'n' to trigger new session creation
		err = session.SendKeys("n")
		require.NoError(t, err)

		// In auto-yes mode, session creation should proceed automatically
		// Give it time to create the session
		time.Sleep(3 * time.Second)

		// Check if TUI is still running
		if session.IsRunning() {
			t.Log("TUI still running after session creation attempt")
		}

		// List sessions after creation
		sessionsAfter, err := tmuxServer.ListSessions()
		if err != nil {
			t.Logf("Could not list sessions after creation: %v", err)
			sessionsAfter = []string{}
		}
		t.Logf("Sessions after: %d", len(sessionsAfter))

		// Note: Session count comparison is informational only
		// The actual session might be created on the default tmux server
		// rather than our isolated test server
	})
}

// TestTUIHandling validates TUI doesn't hang during operations
// SKIP: See TestTUISessionCreation for explanation
func TestTUIHandling(t *testing.T) {
	t.Skip("Skipping PTY-based TUI handling tests - timing sensitive and requires TTY")
	BuildTestBinary(t)

	t.Run("TUI responds to rapid key presses", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Send rapid key presses to test responsiveness
		keys := []string{"?", "j", "k", "j", "k"}
		for _, key := range keys {
			err := session.SendKeys(key)
			assert.NoError(t, err, "Should handle key %s", key)
			time.Sleep(50 * time.Millisecond)
		}

		// TUI should still be running
		assert.True(t, session.IsRunning(), "TUI should handle rapid keys without hanging")
	})

	t.Run("TUI doesn't hang on invalid operations", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Try various operations that might cause hangs
		operations := []struct {
			name string
			keys string
		}{
			{"Help toggle", "?"},
			{"Search empty", "s"},
			{"Escape", "\x1b"},
			{"Filter", "f"},
		}

		for _, op := range operations {
			err := session.SendKeys(op.keys)
			assert.NoError(t, err, "Should handle %s", op.name)
			time.Sleep(200 * time.Millisecond)
		}

		// TUI should still be responsive
		assert.True(t, session.IsRunning(), "TUI should not hang on invalid operations")
	})
}
