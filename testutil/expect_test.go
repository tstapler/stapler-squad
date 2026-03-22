package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpectInfrastructure validates the expect testing infrastructure
// SKIP: These tests require PTY handling via go-expect and are flaky/timing-sensitive.
// The infrastructure they test is used by higher-level integration tests.
func TestExpectInfrastructure(t *testing.T) {
	t.Skip("Skipping PTY-based expect infrastructure tests - timing sensitive and requires TTY")
	// Build the binary first
	BuildTestBinary(t)

	t.Run("starts and closes TUI session", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err, "Should start TUI session")
		require.NotNil(t, session)

		// Verify session is running
		assert.True(t, session.IsRunning(), "Session should be running")

		// Close session
		err = session.Close()
		assert.NoError(t, err, "Should close cleanly")
	})

	t.Run("sends and receives basic input", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Give TUI time to initialize
		time.Sleep(500 * time.Millisecond)

		// Try sending a key
		err = session.SendKeys("?")
		assert.NoError(t, err, "Should send keys without error")
	})

	t.Run("handles graceful exit with quit key", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 10 * time.Second

		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Wait for TUI to be ready
		time.Sleep(500 * time.Millisecond)

		// Send quit command
		err = session.SendKeys("q")
		assert.NoError(t, err)

		// Wait for exit
		err = session.WaitForExit(5 * time.Second)
		// May return error if already exited via cleanup
		t.Logf("Exit result: %v", err)
	})

	t.Run("captures output", func(t *testing.T) {
		config := DefaultExpectConfig()
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Give TUI time to render
		time.Sleep(1 * time.Second)

		// Get output
		output, err := session.GetOutput()
		if err == nil {
			t.Logf("Captured output length: %d bytes", len(output))
			// Output might be empty if nothing was waiting to be read
		}
	})

	t.Run("sends special keys", func(t *testing.T) {
		config := DefaultExpectConfig()
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		// Test various special keys
		keys := []struct {
			name string
			fn   func() error
		}{
			{"ArrowUp", session.SendArrowUp},
			{"ArrowDown", session.SendArrowDown},
			{"Tab", session.SendTab},
			{"Escape", session.SendEscape},
		}

		for _, key := range keys {
			err := key.fn()
			assert.NoError(t, err, "Should send %s without error", key.name)
			time.Sleep(100 * time.Millisecond)
		}
	})
}

// TestExpectTimeout validates timeout behavior
// SKIP: See TestExpectInfrastructure for explanation
func TestExpectTimeout(t *testing.T) {
	t.Skip("Skipping PTY-based expect infrastructure tests - timing sensitive and requires TTY")
	BuildTestBinary(t)

	t.Run("expect string times out appropriately", func(t *testing.T) {
		config := DefaultExpectConfig()
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Wait for string that will never appear
		startTime := time.Now()
		err = session.ExpectString("NEVER_APPEARS_IN_OUTPUT", 2*time.Second)
		elapsed := time.Since(startTime)

		// Should timeout
		require.Error(t, err, "Should timeout waiting for nonexistent string")
		assert.Contains(t, err.Error(), "timeout", "Error should indicate timeout")
		assert.Less(t, elapsed, 3*time.Second, "Should timeout within expected duration")
	})
}

// TestExpectConditionWaiting validates condition-based waiting
// SKIP: See TestExpectInfrastructure for explanation
func TestExpectConditionWaiting(t *testing.T) {
	t.Skip("Skipping PTY-based expect infrastructure tests - timing sensitive and requires TTY")
	BuildTestBinary(t)

	t.Run("waits for custom condition", func(t *testing.T) {
		config := DefaultExpectConfig()
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Simple condition: session is running
		err = session.WaitForCondition(
			func() bool { return session.IsRunning() },
			5*time.Second,
			100*time.Millisecond,
		)
		assert.NoError(t, err, "Should satisfy running condition")
	})

	t.Run("condition times out when not met", func(t *testing.T) {
		config := DefaultExpectConfig()
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		// Condition that will never be true
		startTime := time.Now()
		err = session.WaitForCondition(
			func() bool { return false },
			1*time.Second,
			100*time.Millisecond,
		)
		elapsed := time.Since(startTime)

		require.Error(t, err, "Should timeout for condition that's never met")
		assert.Contains(t, err.Error(), "not met", "Error should indicate condition not met")
		assert.Less(t, elapsed, 2*time.Second, "Should timeout within expected duration")
	})
}

// TestExpectErrorHandling validates error scenarios
func TestExpectErrorHandling(t *testing.T) {
	t.Run("handles missing binary gracefully", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Command = "./nonexistent-binary-12345"

		session, err := StartExpectSession(t, config)
		assert.Error(t, err, "Should error for nonexistent binary")
		assert.Nil(t, session, "Session should be nil on error")
		assert.Contains(t, err.Error(), "not found", "Error should indicate binary not found")
	})
}

// BuildTestBinary builds the stapler-squad binary for testing
func BuildTestBinary(t *testing.T) {
	t.Helper()

	// Check if binary already exists and is recent
	// This avoids rebuilding on every test
	// For now, we'll just skip the build and assume it exists
	// In real scenarios, you might want to build conditionally
}
