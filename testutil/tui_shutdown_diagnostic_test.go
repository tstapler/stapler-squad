package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTUIShutdownDiagnostic measures shutdown timing to diagnose the delay
// SKIP: This test requires PTY handling via go-expect and is timing-sensitive.
// It may hang in CI environments or when TTY is not available.
func TestTUIShutdownDiagnostic(t *testing.T) {
	t.Skip("Skipping PTY-based TUI shutdown diagnostic tests - timing sensitive and requires TTY")
	BuildTestBinary(t)

	t.Run("measure shutdown components", func(t *testing.T) {
		config := DefaultExpectConfig()
		config.Timeout = 15 * time.Second

		startTotal := time.Now()

		// Start TUI
		session, err := StartExpectSession(t, config)
		require.NoError(t, err)

		time.Sleep(1 * time.Second) // Let TUI initialize

		// Measure quit sequence
		quitStart := time.Now()

		// Send quit
		err = session.SendKeys("q")
		if err != nil {
			t.Logf("Error sending q: %v", err)
		}

		// Wait for exit with detailed timing
		exitCheckStart := time.Now()
		err = session.WaitForExit(10 * time.Second)
		exitCheckElapsed := time.Since(exitCheckStart)

		quitElapsed := time.Since(quitStart)
		totalElapsed := time.Since(startTotal)

		// Log detailed timing
		t.Logf("=== SHUTDOWN TIMING DIAGNOSTIC ===")
		t.Logf("Total elapsed: %v", totalElapsed)
		t.Logf("Quit sequence: %v", quitElapsed)
		t.Logf("Exit check: %v", exitCheckElapsed)
		t.Logf("Wait result: %v", err)

		// The problem: shutdown takes 5+ seconds
		if quitElapsed > 5*time.Second {
			t.Logf("⚠️  SLOW SHUTDOWN DETECTED: %v (expected <2s)", quitElapsed)
			t.Logf("This indicates a blocking operation in handleQuit()")
			t.Logf("Likely causes:")
			t.Logf("  1. StateService.Shutdown() timing out (5s)")
			t.Logf("  2. storage.SaveInstancesSync() blocking")
			t.Logf("  3. storage.Close() blocking")
		}

		// Check logs for shutdown timing
		// Note: Would need to read ~/.stapler-squad/logs/claude-squad.log
		// to see the actual handleQuit timing logs
	})

	t.Run("check if StateService is the culprit", func(t *testing.T) {
		// This test documents the hypothesis that StateService.Shutdown()
		// is timing out after 5 seconds, causing the slow exit.

		// The StateService waits for its goroutine with this code:
		//   select {
		//   case <-done:
		//       return nil
		//   case <-time.After(5 * time.Second):
		//       return fmt.Errorf("StateService shutdown timed out")
		//   }

		// If the goroutine is blocked or not responding to stopChan,
		// this will wait the full 5 seconds before timing out.

		t.Log("StateService.Shutdown() timeout hypothesis:")
		t.Log("- StateService goroutine may not be responding to stopChan")
		t.Log("- Shutdown waits 5 seconds for goroutine to finish")
		t.Log("- This matches the observed ~8 second exit time")
		t.Log("  (3s misc + 5s StateService timeout)")
	})
}
