package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRenderer captures the rendered output of UI components for testing purposes.
// It provides methods to save the output to files and compare with expected results.
type TestRenderer struct {
	// Path where snapshots will be stored
	SnapshotPath string
	// Whether to update existing snapshots
	UpdateSnapshots bool
	// Width of the terminal for rendering
	Width int
	// Height of the terminal for rendering
	Height int
	// Whether to strip ANSI color codes from output
	StripColors bool
}

// NewTestRenderer creates a new TestRenderer with default settings
func NewTestRenderer() *TestRenderer {
	updateSnapshots := os.Getenv("UPDATE_SNAPSHOTS") == "true"
	return &TestRenderer{
		SnapshotPath:    "test/ui/snapshots",
		UpdateSnapshots: updateSnapshots,
		Width:           80,
		Height:          24,
		StripColors:     false,
	}
}

// SetDimensions sets the terminal dimensions for rendering
func (r *TestRenderer) SetDimensions(width, height int) *TestRenderer {
	r.Width = width
	r.Height = height
	return r
}

// SetSnapshotPath sets the path where snapshots will be stored
func (r *TestRenderer) SetSnapshotPath(path string) *TestRenderer {
	r.SnapshotPath = path
	return r
}

// EnableUpdateSnapshots enables updating existing snapshots
func (r *TestRenderer) EnableUpdateSnapshots() *TestRenderer {
	r.UpdateSnapshots = true
	return r
}

// DisableColors strips ANSI color codes from output
func (r *TestRenderer) DisableColors() *TestRenderer {
	r.StripColors = true
	return r
}

// RenderToString renders a Bubble Tea model to a string without requiring a TTY
func (r *TestRenderer) RenderToString(model tea.Model) (string, error) {
	// Create a program with a custom output buffer
	var buf bytes.Buffer
	p := tea.NewProgram(model, tea.WithOutput(&buf), tea.WithAltScreen())

	// Run the program once to render the initial view
	err := p.Start()
	if err != nil {
		return "", fmt.Errorf("error rendering model: %w", err)
	}

	output := buf.String()

	// Strip ANSI color codes if requested
	if r.StripColors {
		output = removeANSIEscapeCodes(output)
	}

	return output, nil
}

// RenderAndSave renders a model and saves the output to a file
func (r *TestRenderer) RenderAndSave(model tea.Model, filename string) error {
	// Ensure the snapshot directory exists
	if err := os.MkdirAll(r.SnapshotPath, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Render the model to string
	output, err := r.RenderToString(model)
	if err != nil {
		return err
	}

	// Write to file
	path := filepath.Join(r.SnapshotPath, filename)
	return os.WriteFile(path, []byte(output), 0644)
}

// CompareWithSnapshot compares a rendered model with a saved snapshot
func (r *TestRenderer) CompareWithSnapshot(t *testing.T, model tea.Model, filename string) {
	t.Helper()

	// Render the model
	output, err := r.RenderToString(model)
	if err != nil {
		t.Fatalf("Failed to render model: %v", err)
	}

	snapshotPath := filepath.Join(r.SnapshotPath, filename)

	// Check if we need to update the snapshot
	if r.UpdateSnapshots {
		if err := os.MkdirAll(r.SnapshotPath, 0755); err != nil {
			t.Fatalf("Failed to create snapshot directory: %v", err)
		}

		if err := os.WriteFile(snapshotPath, []byte(output), 0644); err != nil {
			t.Fatalf("Failed to update snapshot: %v", err)
		}

		t.Logf("Updated snapshot: %s", filename)
		return
	}

	// Read the existing snapshot
	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Snapshot %s does not exist. Run with UpdateSnapshots=true to create it", filename)
		}
		t.Fatalf("Failed to read snapshot: %v", err)
	}

	// Compare the output with the snapshot
	if output != string(expected) {
		t.Errorf("Rendered output does not match snapshot %s", filename)
		t.Errorf("Diff:\n%s", diffStrings(string(expected), output))
	}
}

// RenderComponent renders any UI component that has a View() or Render() method
func (r *TestRenderer) RenderComponent(component interface{}) (string, error) {
	var output string

	// Check if the component has a View() method (BubbleTea style)
	if viewer, ok := component.(interface{ View() string }); ok {
		output = viewer.View()
	} else if renderer, ok := component.(interface{ Render() string }); ok {
		// Check if the component has a Render() method (Lipgloss style)
		output = renderer.Render()
	} else if stringer, ok := component.(fmt.Stringer); ok {
		// Check if the component has a String() method
		output = stringer.String()
	} else {
		return "", fmt.Errorf("component does not implement View(), Render(), or String()")
	}

	// Strip ANSI color codes if requested
	if r.StripColors {
		output = removeANSIEscapeCodes(output)
	}

	return output, nil
}

// SaveComponentOutput renders a component and saves its output to a file
func (r *TestRenderer) SaveComponentOutput(component interface{}, filename string) error {
	// Render the component
	output, err := r.RenderComponent(component)
	if err != nil {
		return err
	}

	// Ensure the snapshot directory exists
	if err := os.MkdirAll(r.SnapshotPath, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Write to file
	path := filepath.Join(r.SnapshotPath, filename)
	return os.WriteFile(path, []byte(output), 0644)
}

// CompareComponentWithSnapshot compares a rendered component with a saved snapshot
func (r *TestRenderer) CompareComponentWithSnapshot(t *testing.T, component interface{}, filename string) {
	t.Helper()

	// Render the component
	output, err := r.RenderComponent(component)
	if err != nil {
		t.Fatalf("Failed to render component: %v", err)
	}

	snapshotPath := filepath.Join(r.SnapshotPath, filename)

	// Check if we need to update the snapshot
	if r.UpdateSnapshots {
		if err := os.MkdirAll(r.SnapshotPath, 0755); err != nil {
			t.Fatalf("Failed to create snapshot directory: %v", err)
		}

		if err := os.WriteFile(snapshotPath, []byte(output), 0644); err != nil {
			t.Fatalf("Failed to update snapshot: %v", err)
		}

		t.Logf("Updated snapshot: %s", filename)
		return
	}

	// Read the existing snapshot
	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Snapshot %s does not exist. Run with UpdateSnapshots=true to create it", filename)
		}
		t.Fatalf("Failed to read snapshot: %v", err)
	}

	// Compare the output with the snapshot
	if output != string(expected) {
		t.Errorf("Rendered output does not match snapshot %s", filename)
		t.Errorf("Diff:\n%s", diffStrings(string(expected), output))
	}
}

// Create a simple text diff between two strings
func diffStrings(expected, actual string) string {
	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")

	var builder strings.Builder

	for i := 0; i < max(len(expectedLines), len(actualLines)); i++ {
		var expectedLine, actualLine string

		if i < len(expectedLines) {
			expectedLine = expectedLines[i]
		}

		if i < len(actualLines) {
			actualLine = actualLines[i]
		}

		if expectedLine != actualLine {
			builder.WriteString(fmt.Sprintf("Line %d:\n", i+1))
			builder.WriteString(fmt.Sprintf("  Expected: %q\n", expectedLine))
			builder.WriteString(fmt.Sprintf("  Actual:   %q\n", actualLine))
		}
	}

	return builder.String()
}

// Helper for max int
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// removeANSIEscapeCodes removes ANSI escape codes from a string
func removeANSIEscapeCodes(str string) string {
	// This is a simple approach that handles most common ANSI escape codes
	var result strings.Builder
	inEscapeSeq := false

	for _, r := range str {
		if inEscapeSeq {
			// End of escape sequence
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscapeSeq = false
			}
			continue
		}

		// Start of escape sequence
		if r == '\x1b' {
			inEscapeSeq = true
			continue
		}

		// Normal character
		result.WriteRune(r)
	}

	return result.String()
}

// MockProgram is a simpler way to execute Bubble Tea programs in tests
type MockProgram struct {
	model     tea.Model
	output    io.Writer
	initMsg   tea.Msg
	altScreen bool
	program   *tea.Program
}

// NewMockProgram creates a new MockProgram
func NewMockProgram(model tea.Model) *MockProgram {
	return &MockProgram{
		model:     model,
		output:    &bytes.Buffer{},
		altScreen: false,
	}
}

// WithOutput sets the output writer for the program
func (m *MockProgram) WithOutput(w io.Writer) *MockProgram {
	m.output = w
	return m
}

// WithInitMessage sets an initial message to send to the program
func (m *MockProgram) WithInitMessage(msg tea.Msg) *MockProgram {
	m.initMsg = msg
	return m
}

// WithAltScreen enables the alternate screen
func (m *MockProgram) WithAltScreen() *MockProgram {
	m.altScreen = true
	return m
}

// Start runs the program once and returns the output
func (m *MockProgram) Start() (string, error) {
	var opts []tea.ProgramOption

	opts = append(opts, tea.WithOutput(m.output))

	if m.altScreen {
		opts = append(opts, tea.WithAltScreen())
	}

	m.program = tea.NewProgram(m.model, opts...)

	if m.initMsg != nil {
		go func() {
			// Send the init message after the program has started
			m.program.Send(m.initMsg)
		}()
	}

	if _, err := m.program.Run(); err != nil {
		return "", err
	}

	if buf, ok := m.output.(*bytes.Buffer); ok {
		return buf.String(), nil
	}

	return "", fmt.Errorf("output is not a bytes.Buffer")
}

// SendMessage sends a message to the program
func (m *MockProgram) SendMessage(msg tea.Msg) error {
	if m.program == nil {
		return fmt.Errorf("program not started")
	}

	m.program.Send(msg)
	return nil
}

// GetOutputBuffer returns the output buffer if available
func (m *MockProgram) GetOutputBuffer() (*bytes.Buffer, error) {
	if buf, ok := m.output.(*bytes.Buffer); ok {
		return buf, nil
	}

	return nil, fmt.Errorf("output is not a bytes.Buffer")
}

// MockTerminal provides a simulated terminal environment for testing
type MockTerminal struct {
	Width  int
	Height int
}

// NewMockTerminal creates a new MockTerminal with default dimensions
func NewMockTerminal() *MockTerminal {
	return &MockTerminal{
		Width:  80,
		Height: 24,
	}
}

// SetSize sets the terminal dimensions
func (m *MockTerminal) SetSize(width, height int) *MockTerminal {
	m.Width = width
	m.Height = height
	return m
}

// SimulateKeyPress simulates a key press on a model
func (m *MockTerminal) SimulateKeyPress(model tea.Model, key string) (tea.Model, tea.Cmd) {
	var keyMsg tea.KeyMsg

	// Handle special keys
	switch key {
	case "enter":
		keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc", "escape":
		keyMsg = tea.KeyMsg{Type: tea.KeyEscape}
	case "space":
		keyMsg = tea.KeyMsg{Type: tea.KeySpace}
	case "tab":
		keyMsg = tea.KeyMsg{Type: tea.KeyTab}
	case "backtab":
		keyMsg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		keyMsg = tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		keyMsg = tea.KeyMsg{Type: tea.KeyDown}
	case "right":
		keyMsg = tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		keyMsg = tea.KeyMsg{Type: tea.KeyLeft}
	default:
		// Regular character key
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}

	return model.Update(keyMsg)
}

// SimulateWindowResize simulates a window resize event
func (m *MockTerminal) SimulateWindowResize(model tea.Model) (tea.Model, tea.Cmd) {
	return model.Update(tea.WindowSizeMsg{Width: m.Width, Height: m.Height})
}
