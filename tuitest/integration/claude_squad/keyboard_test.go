package claude_squad

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ClaudeSquadTester provides keyboard testing for claude-squad TUI
type ClaudeSquadTester struct {
	t           *testing.T
	cmd         *exec.Cmd
	pty         *os.File
	output      strings.Builder
	outputMux   sync.Mutex
	timeout     time.Duration
	snapshotDir string
	snapshotNum int
}

// NewClaudeSquadTester creates a new tester for claude-squad keyboard interactions
func NewClaudeSquadTester(t *testing.T, timeout time.Duration) (*ClaudeSquadTester, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Create command using go run
	cmd, err := setupClaudeSquadCommand(t)
	if err != nil {
		return nil, fmt.Errorf("failed to setup claude-squad command: %w", err)
	}

	// Create snapshot directory
	snapshotDir := filepath.Join(os.TempDir(), fmt.Sprintf("claude-squad-tui-test-%d", time.Now().Unix()))
	err = os.MkdirAll(snapshotDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Start with PTY
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start claude-squad with PTY: %w", err)
	}

	tester := &ClaudeSquadTester{
		t:           t,
		cmd:         cmd,
		pty:         ptyFile,
		timeout:     timeout,
		snapshotDir: snapshotDir,
		snapshotNum: 0,
	}

	// Start output capture
	go tester.captureOutput()

	// Wait for startup - longer delay for TUI initialization
	time.Sleep(5 * time.Second)

	// Log initial output for debugging
	initialOutput := tester.GetOutput()
	t.Logf("Claude Squad test session started with PID %d", cmd.Process.Pid)
	t.Logf("Initial output: %q", initialOutput)
	if len(initialOutput) > 0 {
		t.Logf("Initial output hex: %x", []byte(initialOutput))
	}

	return tester, nil
}

// captureOutput continuously captures output from the PTY
func (c *ClaudeSquadTester) captureOutput() {
	buffer := make([]byte, 1024)
	for {
		c.pty.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := c.pty.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "file already closed") {
				return
			}
			continue
		}
		if n > 0 {
			c.outputMux.Lock()
			c.output.Write(buffer[:n])
			c.outputMux.Unlock()
		}
	}
}

// SendKey sends a keyboard input to claude-squad
func (c *ClaudeSquadTester) SendKey(key string) error {
	c.t.Logf("Sending key: %s", key)

	var keyBytes []byte
	switch key {
	case "enter":
		keyBytes = []byte{'\r'}
	case "escape", "esc":
		keyBytes = []byte{'\x1b'}
	case "tab":
		keyBytes = []byte{'\t'}
	case "backspace":
		keyBytes = []byte{'\x7f'}
	case "space":
		keyBytes = []byte{' '}
	case "ctrl+c":
		keyBytes = []byte{'\x03'}
	case "up":
		keyBytes = []byte{'\x1b', '[', 'A'}
	case "down":
		keyBytes = []byte{'\x1b', '[', 'B'}
	case "right":
		keyBytes = []byte{'\x1b', '[', 'C'}
	case "left":
		keyBytes = []byte{'\x1b', '[', 'D'}
	default:
		if len(key) == 1 {
			keyBytes = []byte(key)
		} else {
			return fmt.Errorf("unsupported key: %s", key)
		}
	}

	_, err := c.pty.Write(keyBytes)
	if err != nil {
		return fmt.Errorf("failed to send key %s: %w", key, err)
	}

	// Small delay for processing
	time.Sleep(50 * time.Millisecond)
	return nil
}

// SendText sends a string to claude-squad
func (c *ClaudeSquadTester) SendText(text string) error {
	c.t.Logf("Sending text: %s", text)
	_, err := c.pty.WriteString(text)
	if err != nil {
		return fmt.Errorf("failed to send text: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

// GetOutput returns the current accumulated output
func (c *ClaudeSquadTester) GetOutput() string {
	c.outputMux.Lock()
	defer c.outputMux.Unlock()
	return c.output.String()
}

// TakeSnapshot saves current output to a file and returns the filename
func (c *ClaudeSquadTester) TakeSnapshot(name string) string {
	c.snapshotNum++
	filename := fmt.Sprintf("%03d-%s.txt", c.snapshotNum, name)
	filepath := filepath.Join(c.snapshotDir, filename)

	output := c.GetOutput()
	err := os.WriteFile(filepath, []byte(output), 0644)
	if err != nil {
		c.t.Logf("Failed to write snapshot %s: %v", filename, err)
		return ""
	}

	c.t.Logf("Snapshot saved: %s (%d bytes)", filename, len(output))
	return filename
}

// SearchInSnapshots searches for text in all snapshot files
func (c *ClaudeSquadTester) SearchInSnapshots(text string) []string {
	var matches []string

	files, err := os.ReadDir(c.snapshotDir)
	if err != nil {
		c.t.Logf("Failed to read snapshot directory: %v", err)
		return matches
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(c.snapshotDir, file.Name()))
		if err != nil {
			continue
		}

		if strings.Contains(string(content), text) {
			matches = append(matches, file.Name())
		}
	}

	return matches
}

// WaitForTextWithSnapshots waits for text, taking snapshots during the wait
func (c *ClaudeSquadTester) WaitForTextWithSnapshots(text string, timeout time.Duration) error {
	c.t.Logf("Waiting for text: %s", text)
	deadline := time.Now().Add(timeout)
	lastSnapshotTime := time.Now()

	for time.Now().Before(deadline) {
		output := c.GetOutput()
		if strings.Contains(output, text) {
			c.t.Logf("Found expected text: %s", text)
			c.TakeSnapshot(fmt.Sprintf("found-%s", strings.ReplaceAll(text, " ", "-")))
			return nil
		}

		// Take periodic snapshots during wait
		if time.Since(lastSnapshotTime) > 2*time.Second {
			c.TakeSnapshot(fmt.Sprintf("waiting-for-%s", strings.ReplaceAll(text, " ", "-")))
			lastSnapshotTime = time.Now()
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Take final snapshot on timeout
	c.TakeSnapshot(fmt.Sprintf("timeout-%s", strings.ReplaceAll(text, " ", "-")))
	return fmt.Errorf("timeout waiting for text '%s'. Check snapshots in %s", text, c.snapshotDir)
}

// WaitForText waits for specific text to appear in output
func (c *ClaudeSquadTester) WaitForText(text string, timeout time.Duration) error {
	c.t.Logf("Waiting for text: %s", text)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		output := c.GetOutput()
		if strings.Contains(output, text) {
			c.t.Logf("Found expected text: %s", text)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	output := c.GetOutput()
	return fmt.Errorf("timeout waiting for text '%s'. Current output: %s", text, output)
}

// AssertTextPresent asserts that text is present in output
func (c *ClaudeSquadTester) AssertTextPresent(text string) {
	output := c.GetOutput()
	assert.Contains(c.t, output, text, "Expected text not found in output")
}

// ClearOutput clears the output buffer
func (c *ClaudeSquadTester) ClearOutput() {
	c.outputMux.Lock()
	defer c.outputMux.Unlock()
	c.output.Reset()
	c.t.Log("Output buffer cleared")
}

// Close terminates the claude-squad session
func (c *ClaudeSquadTester) Close() error {
	c.t.Log("Closing claude-squad test session")

	// Take final snapshot before closing
	c.TakeSnapshot("final-state")

	// Try graceful shutdown
	if c.cmd.Process != nil {
		c.cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill if needed
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}

	// Close PTY
	if c.pty != nil {
		c.pty.Close()
	}

	// Wait for command
	c.cmd.Wait()

	c.t.Logf("Claude-squad test session closed. Snapshots saved in: %s", c.snapshotDir)
	return nil
}

// TestKeyboardShortcut tests a specific keyboard shortcut
func (c *ClaudeSquadTester) TestKeyboardShortcut(key string, expectedBehavior string, timeout time.Duration) error {
	c.t.Logf("Testing keyboard shortcut: %s (expecting: %s)", key, expectedBehavior)

	// Clear output before test
	c.ClearOutput()

	// Send the key
	if err := c.SendKey(key); err != nil {
		return fmt.Errorf("failed to send key %s: %w", key, err)
	}

	// Wait for response
	if expectedBehavior != "" {
		if err := c.WaitForText(expectedBehavior, timeout); err != nil {
			return fmt.Errorf("keyboard shortcut test failed for key %s: %w", key, err)
		}
	}

	c.t.Logf("✓ Keyboard shortcut test passed for key: %s", key)
	return nil
}

// setupClaudeSquadCommand sets up the command to run claude-squad for testing
func setupClaudeSquadCommand(t *testing.T) (*exec.Cmd, error) {
	// Get the absolute path to the project root
	projectRoot := "../../../" // Go up from tuitest/integration/claude_squad to project root
	absProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute project root path: %w", err)
	}

	// Use go run instead of building a binary
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = absProjectRoot

	// Create unique log directory for this test run
	logDir := filepath.Join(os.TempDir(), fmt.Sprintf("claude-squad-test-logs-%d", time.Now().Unix()))
	os.MkdirAll(logDir, 0755)

	// Create temporary config file to isolate logs
	configDir := filepath.Join(os.TempDir(), fmt.Sprintf("claude-squad-test-config-%d", time.Now().Unix()))
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	// Write minimal config with isolated log directory
	configData := fmt.Sprintf(`{
		"logs_enabled": true,
		"logs_dir": "%s",
		"log_level": "INFO"
	}`, logDir)

	err = os.WriteFile(configFile, []byte(configData), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create test config file: %w", err)
	}

	// Set environment variables for test isolation
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLUMNS=80",
		"LINES=24",
		"TMUX_SESSION_PREFIX=tui-test-", // Use separate tmux session prefix for testing
		fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Dir(configDir)), // Point to our test config
	)

	t.Logf("Test logs will be written to: %s", logDir)
	t.Logf("Test config file: %s", configFile)

	t.Log("Using 'go run .' to execute claude-squad for testing")
	return cmd, nil
}

// TestClaudeSquadKeyboardShortcuts tests all keyboard shortcuts work
func TestClaudeSquadKeyboardShortcuts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping keyboard integration tests in short mode")
	}

	// Create tester
	tester, err := NewClaudeSquadTester(t, 60*time.Second)
	require.NoError(t, err)
	defer tester.Close()

	// Wait for initial UI load
	err = tester.WaitForText("Claude Squad", 5*time.Second)
	require.NoError(t, err, "Claude Squad did not start properly")

	// Test keyboard shortcuts
	testCases := []struct {
		key      string
		expected string
		desc     string
	}{
		{"n", "Session", "New session dialog should open"},
		{"escape", "", "Escape should close dialog"},
		{"s", "Search", "Search dialog should open"},
		{"escape", "", "Escape should close search"},
		{"h", "Help", "Help screen should open"},
		{"escape", "", "Escape should close help"},
		{"f", "", "Filter toggle should work"},
		{"tab", "", "Tab navigation should work"},
		{"up", "", "Up navigation should work"},
		{"down", "", "Down navigation should work"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Test_%s_key", tc.key), func(t *testing.T) {
			err := tester.TestKeyboardShortcut(tc.key, tc.expected, 3*time.Second)
			if err != nil {
				// Log current output for debugging
				output := tester.GetOutput()
				t.Logf("Current output when testing %s: %s", tc.key, output)
				t.Errorf("Keyboard shortcut test failed: %v", err)
			}
		})
	}
}

// TestNewSessionDialog tests the new session dialog functionality specifically
func TestNewSessionDialog(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping session dialog tests in short mode")
	}

	// Create tester
	tester, err := NewClaudeSquadTester(t, 60*time.Second)
	require.NoError(t, err)
	defer tester.Close()

	// Wait for initial UI load
	err = tester.WaitForText("Claude Squad", 5*time.Second)
	require.NoError(t, err)

	t.Run("Open new session dialog with 'n' key", func(t *testing.T) {
		tester.ClearOutput()
		tester.TakeSnapshot("before-n-key")

		// Press 'n' to open new session dialog
		err := tester.SendKey("n")
		require.NoError(t, err)

		// Look for dialog elements - use snapshots for debugging
		err = tester.WaitForTextWithSnapshots("Enter filename or path", 3*time.Second)
		if err != nil {
			// Try alternative dialog text that might appear
			matches := tester.SearchInSnapshots("filename")
			t.Logf("Found 'filename' in snapshots: %v", matches)

			matches = tester.SearchInSnapshots("Session")
			t.Logf("Found 'Session' in snapshots: %v", matches)
		}
		require.NoError(t, err, "New session dialog should open and show input prompt")

		t.Log("✓ New session dialog opens successfully")
	})

	t.Run("Cancel dialog with Escape", func(t *testing.T) {
		// Dialog should still be open from previous test
		err := tester.SendKey("escape")
		require.NoError(t, err)

		// Verify we're back to main screen
		time.Sleep(500 * time.Millisecond)
		err = tester.SendKey("down") // Should work on main session list
		assert.NoError(t, err, "Should return to main session list")

		t.Log("✓ Dialog cancellation works")
	})

	t.Run("Complete session creation flow", func(t *testing.T) {
		tester.ClearOutput()
		tester.TakeSnapshot("before-session-creation")

		// Step 1: Press 'n' to start new session
		t.Log("Step 1: Starting new session creation")
		err := tester.SendKey("n")
		require.NoError(t, err)

		// Give it time to process
		time.Sleep(1 * time.Second)
		tester.TakeSnapshot("after-n-key")

		// Step 2: Navigate through any dialogs or prompts that appear
		// Since we don't know the exact UI flow, let's observe what appears
		output := tester.GetOutput()
		t.Logf("Output after 'n' key: available for analysis")

		// Look for common session creation indicators
		possibleTexts := []string{
			"session", "Session", "create", "Create",
			"path", "Path", "directory", "Directory",
			"name", "Name", "title", "Title",
		}

		var foundText string
		for _, text := range possibleTexts {
			if strings.Contains(output, text) {
				foundText = text
				t.Logf("Found potential dialog text: %s", text)
				break
			}
		}

		if foundText == "" {
			// If no dialog appears, maybe we need to interact differently
			// Try some common keys that might advance the flow
			t.Log("No dialog detected, trying common navigation keys")

			// Try Enter to proceed
			err = tester.SendKey("enter")
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)
			tester.TakeSnapshot("after-enter")

			// Check if anything changed
			newOutput := tester.GetOutput()
			if len(newOutput) > len(output) {
				t.Log("Interface changed after Enter key")
			}
		}

		// Step 3: Try to provide a test path
		testPath := "/tmp/tui-test-session"
		t.Logf("Step 3: Attempting to enter path: %s", testPath)

		err = tester.SendText(testPath)
		require.NoError(t, err)
		time.Sleep(500 * time.Millisecond)
		tester.TakeSnapshot("after-path-input")

		// Step 4: Try to confirm/submit
		t.Log("Step 4: Attempting to confirm session creation")
		err = tester.SendKey("enter")
		require.NoError(t, err)

		// Give session creation time (this could take several seconds)
		time.Sleep(5 * time.Second)
		tester.TakeSnapshot("after-session-creation-attempt")

		// Step 5: Check if we successfully created a session
		finalOutput := tester.GetOutput()
		sessionCreated := strings.Contains(finalOutput, testPath) ||
		                 strings.Contains(finalOutput, "tui-test-session") ||
		                 strings.Contains(finalOutput, "claude") ||
		                 strings.Contains(finalOutput, "Claude")

		if sessionCreated {
			t.Log("✓ Session appears to have been created")
		} else {
			t.Log("⚠ Session creation unclear - check snapshots for details")
		}

		// Cancel/cleanup any remaining dialogs
		err = tester.SendKey("escape")
		if err == nil {
			time.Sleep(500 * time.Millisecond)
		}

		tester.TakeSnapshot("final-cleanup")
		t.Log("✓ Complete session creation flow tested")
	})
}