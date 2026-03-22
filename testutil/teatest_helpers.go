package testutil

import (
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/session/tmux"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TUITestConfig contains configuration for TUI tests
type TUITestConfig struct {
	Width   int
	Height  int
	Timeout time.Duration
}

// DefaultTUIConfig returns sensible defaults for TUI testing
func DefaultTUIConfig() TUITestConfig {
	return TUITestConfig{
		Width:   200, // Increased from 80 to accommodate both list (30%=60 cols) and preview (70%=140 cols)
		Height:  40,  // Increased from 24 to ensure session list has enough space
		Timeout: 1 * time.Second,
	}
}

// CreateTUITest creates a new teatest instance with the given model
func CreateTUITest(t *testing.T, model tea.Model, config TUITestConfig) *teatest.TestModel {
	t.Helper()

	opts := []teatest.TestOption{
		teatest.WithInitialTermSize(config.Width, config.Height),
	}

	tm := teatest.NewTestModel(t, model, opts...)
	return tm
}

// ReadOutput reads all content from an io.Reader and returns it as a string
func ReadOutput(t *testing.T, r io.Reader) string {
	t.Helper()
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}
	return string(content)
}

// WaitForOutputContains waits until the output contains the expected text using teatest.WaitFor
func WaitForOutputContains(t *testing.T, tm *teatest.TestModel, expected string, timeout time.Duration) {
	t.Helper()

	// Create a fresh output reader for each call to avoid exhaustion
	output := tm.Output()
	teatest.WaitFor(
		t, output,
		func(bts []byte) bool {
			return strings.Contains(string(bts), expected)
		},
		teatest.WithDuration(timeout),
		teatest.WithCheckInterval(50*time.Millisecond),
	)
}

// WaitForOutputContainsAfterAction waits for an action to complete, then checks if output contains expected text
// This version avoids output stream consumption issues by using a single read approach
func WaitForOutputContainsAfterAction(t *testing.T, tm *teatest.TestModel, expected string, actionDelay time.Duration) {
	t.Helper()

	// Wait for the action to complete
	time.Sleep(actionDelay)

	// Get output and check if it contains the expected text
	output := tm.Output()
	outputStr := ReadOutput(t, output)

	if !strings.Contains(outputStr, expected) {
		t.Errorf("Expected output to contain %q, but got:\n%s", expected, outputStr)
	}
}

// AssertOutputContains checks if current output contains the expected text
func AssertOutputContains(t *testing.T, tm *teatest.TestModel, expected string) {
	t.Helper()

	// Use WaitFor with a short timeout to check current output without consuming it
	found := false
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			if strings.Contains(string(bts), expected) {
				found = true
				return true
			}
			return false
		},
		teatest.WithDuration(100*time.Millisecond),
		teatest.WithCheckInterval(10*time.Millisecond),
	)

	if !found {
		// Get a fresh output for error reporting
		output := tm.Output()
		outputStr := ReadOutput(t, output)
		t.Errorf("Expected output to contain %q, but got:\n%s", expected, outputStr)
	}
}

// CreateMinimalApp creates a minimal app model for testing basic functionality
func CreateMinimalApp(t *testing.T) tea.Model {
	t.Helper()

	return &minimalTestModel{
		content: "Test App Started",
	}
}

// minimalTestModel is a simple model for basic teatest validation
type minimalTestModel struct {
	content string
}

func (m *minimalTestModel) Init() tea.Cmd {
	return nil
}

func (m *minimalTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "test":
			m.content = "Test key received"
		}
	}
	return m, nil
}

func (m *minimalTestModel) View() string {
	return m.content + "\n\nPress 'q' to quit"
}

// CleanupTeatestTmuxServer cleans up the isolated tmux server for a specific test
func CleanupTeatestTmuxServer(t *testing.T) {
	t.Helper()

	serverSocket := "teatest_" + t.Name()
	cmdExec := executor.MakeExecutor()

	// Clean up any tmux sessions on the isolated server
	if err := tmux.CleanupSessionsOnServer(cmdExec, serverSocket); err != nil {
		// Log warning but don't fail test - cleanup is best effort
		t.Logf("Warning: failed to cleanup tmux server '%s': %v", serverSocket, err)
	}
}