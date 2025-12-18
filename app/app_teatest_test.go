package app

import (
	"claude-squad/testutil"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestTeatestBasicIntegration validates that teatest works with our minimal app
func TestTeatestBasicIntegration(t *testing.T) {
	// Test minimal model first to ensure teatest is working
	minimalModel := testutil.CreateMinimalApp(t)
	config := testutil.DefaultTUIConfig()

	tm := testutil.CreateTUITest(t, minimalModel, config)

	// Wait for content to appear using a single WaitFor call
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			content := string(bts)
			return strings.Contains(content, "Test App Started") && strings.Contains(content, "Press 'q' to quit")
		},
		teatest.WithCheckInterval(50*time.Millisecond),
		teatest.WithDuration(1*time.Second),
	)

	// Send quit command using proper key message
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	// Wait for program to finish with teatest's WaitFinished
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// TestTeatestTyping tests teatest's built-in Type method
func TestTeatestTyping(t *testing.T) {
	minimalModel := testutil.CreateMinimalApp(t)
	config := testutil.DefaultTUIConfig()

	tm := testutil.CreateTUITest(t, minimalModel, config)

	// Wait for initial render
	testutil.WaitForOutputContains(t, tm, "Test App Started", 500*time.Millisecond)

	// Test typing with teatest's Send method to send the "test" key
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	// The minimal model should change content when "test" key is sent
	testutil.WaitForOutputContains(t, tm, "Test key received", 500*time.Millisecond)

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// TestTeatestWindowResize tests window resize with teatest
func TestTeatestWindowResize(t *testing.T) {
	minimalModel := testutil.CreateMinimalApp(t)
	config := testutil.DefaultTUIConfig()

	tm := testutil.CreateTUITest(t, minimalModel, config)

	// Wait for initial render
	testutil.WaitForOutputContains(t, tm, "Test App Started", 500*time.Millisecond)

	// Test resize using teatest's Send method
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Verify the app still works after resize
	testutil.AssertOutputContains(t, tm, "Test App Started")

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// TestTeatestSendMessage tests sending arbitrary messages with teatest
func TestTeatestSendMessage(t *testing.T) {
	minimalModel := testutil.CreateMinimalApp(t)
	config := testutil.DefaultTUIConfig()

	tm := testutil.CreateTUITest(t, minimalModel, config)

	// Wait for initial render
	testutil.WaitForOutputContains(t, tm, "Test App Started", 500*time.Millisecond)

	// Test sending key messages using teatest's Send method
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	// The minimal model should change content when "test" key is sent
	testutil.WaitForOutputContains(t, tm, "Test key received", 500*time.Millisecond)

	// Clean exit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// Placeholder tests for actual app integration (to be implemented in subsequent tasks)

// TestAppConfirmationModalTeatest demonstrates teatest integration for confirmation modals
func TestAppConfirmationModalTeatest(t *testing.T) {
	// Create a simple confirmation modal test model
	// This validates that teatest can handle confirmation-style interactions
	model := &confirmationTestModel{
		message: "Delete session 'test'?",
		showing: true,
	}

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, model, config)

	// Wait for confirmation modal to render using the proven pattern
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			content := string(bts)
			return strings.Contains(content, "Delete session 'test'?") &&
				strings.Contains(content, "(y)es") &&
				strings.Contains(content, "(n)o")
		},
		teatest.WithCheckInterval(50*time.Millisecond),
		teatest.WithDuration(1*time.Second),
	)

	// Test 'n' key dismisses modal
	tm.Type("n")

	// Wait for modal to be dismissed using our fixed helper
	testutil.WaitForOutputContains(t, tm, "Modal dismissed", 500*time.Millisecond)

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// confirmationTestModel is a simple model that simulates confirmation modal behavior
type confirmationTestModel struct {
	message   string
	showing   bool
	dismissed bool
}

func (m *confirmationTestModel) Init() tea.Cmd {
	return nil
}

func (m *confirmationTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "y":
			m.showing = false
			m.dismissed = true
		case "n", "esc":
			m.showing = false
			m.dismissed = true
		}
	}
	return m, nil
}

func (m *confirmationTestModel) View() string {
	if m.showing {
		view := "┌─ Confirmation ──────┐\n│ " + m.message + " │\n│                     │\n│   (y)es    (n)o     │\n└─────────────────────┘\n\nPress 'q' to quit"
		return view
	}
	if m.dismissed {
		return "Modal dismissed\n\nPress 'q' to quit"
	}
	return "No modal\n\nPress 'q' to quit"
}

// TestAppNavigationTeatest demonstrates teatest integration for navigation
func TestAppNavigationTeatest(t *testing.T) {
	// Create a simple navigation test model with multiple items
	model := &navigationTestModel{
		items:    []string{"Session 1", "Session 2", "Session 3", "Session 4", "Session 5"},
		selected: 0,
	}

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, model, config)

	// Use the exact pattern from the working basic integration test
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			content := string(bts)
			return strings.Contains(content, "Session 1") && strings.Contains(content, "Selected: 0")
		},
		teatest.WithCheckInterval(50*time.Millisecond),
		teatest.WithDuration(1*time.Second),
	)

	// Test navigation step by step
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})

	// Wait for update
	teatest.WaitFor(
		t, tm.Output(),
		func(bts []byte) bool {
			content := string(bts)
			return strings.Contains(content, "Selected: 1")
		},
		teatest.WithCheckInterval(50*time.Millisecond),
		teatest.WithDuration(1*time.Second),
	)

	// Complete the navigation testing with working pattern
	// Note: We can only do a few key presses before teatest output gets exhausted
	// This is a known limitation of the current teatest integration

	// Test one more navigation step
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	testutil.WaitForOutputContains(t, tm, "Selected: 2", 500*time.Millisecond)

	// Test reverse navigation
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})
	testutil.WaitForOutputContains(t, tm, "Selected: 1", 500*time.Millisecond)

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// TestAppNavigationPageMovementTeatest tests page up/down navigation
func TestAppNavigationPageMovementTeatest(t *testing.T) {
	// Create model with many items to test page movement
	var items []string
	for i := 1; i <= 20; i++ {
		items = append(items, fmt.Sprintf("Session %d", i))
	}

	model := &navigationTestModel{
		items:    items,
		selected: 0,
	}

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, model, config)

	// Wait for initial render - should show Session 1 as selected
	testutil.WaitForOutputContains(t, tm, "Session 1 (selected)", 500*time.Millisecond)

	// Test Page Down (simulated with multiple down movements)
	tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	testutil.WaitForOutputContains(t, tm, "Session 11 (selected)", 200*time.Millisecond) // Page should move by 10

	// Test Page Up (simulated with multiple up movements)
	tm.Send(tea.KeyMsg{Type: tea.KeyPgUp})
	testutil.WaitForOutputContains(t, tm, "Session 1 (selected)", 200*time.Millisecond) // Should go back to start

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// TestAppNavigationMultipleSelectionsTeatest tests navigation with different selections
func TestAppNavigationMultipleSelectionsTeatest(t *testing.T) {
	model := &navigationTestModel{
		items:    []string{"First", "Second", "Third", "Fourth"},
		selected: 1, // Start at second item
	}

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, model, config)

	// Wait for initial render (starting at second item)
	testutil.WaitForOutputContains(t, tm, "Second (selected)", 500*time.Millisecond)

	// Navigate to first, then third, then last
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})
	testutil.WaitForOutputContains(t, tm, "First (selected)", 200*time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	testutil.WaitForOutputContains(t, tm, "Third (selected)", 200*time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	testutil.WaitForOutputContains(t, tm, "Fourth (selected)", 200*time.Millisecond)

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// navigationTestModel is a simple model that simulates list navigation behavior
type navigationTestModel struct {
	items    []string
	selected int
}

func (m *navigationTestModel) Init() tea.Cmd {
	return nil
}

func (m *navigationTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyRunes:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		case tea.KeyUp:
			if m.selected > 0 {
				m.selected--
			}
		case tea.KeyDown:
			if m.selected < len(m.items)-1 {
				m.selected++
			}
		case tea.KeyPgUp:
			// Simulate page up (move by 10 or to beginning)
			m.selected -= 10
			if m.selected < 0 {
				m.selected = 0
			}
		case tea.KeyPgDown:
			// Simulate page down (move by 10 or to end)
			m.selected += 10
			if m.selected >= len(m.items) {
				m.selected = len(m.items) - 1
			}
		}
	}
	return m, nil
}

func (m *navigationTestModel) View() string {
	if len(m.items) == 0 {
		return "No items\n\nPress 'q' to quit"
	}

	view := fmt.Sprintf("Navigation Test - Selected: %d\n\n", m.selected)

	for i, item := range m.items {
		if i == m.selected {
			view += fmt.Sprintf("> %s (selected)\n", item)
		} else {
			view += fmt.Sprintf("  %s\n", item)
		}
	}

	view += "\nUse arrow keys to navigate, PgUp/PgDn for page movement\nPress 'q' to quit"
	return view
}
