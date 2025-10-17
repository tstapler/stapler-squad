package overlay

import (
	"claude-squad/session"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewCommandInputOverlay(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)

	overlay := NewCommandInputOverlay("test-session", controller)

	if overlay == nil {
		t.Fatal("NewCommandInputOverlay() returned nil")
	}

	if overlay.sessionName != "test-session" {
		t.Errorf("sessionName = %q, expected %q", overlay.sessionName, "test-session")
	}

	if overlay.FocusIndex != 0 {
		t.Errorf("FocusIndex = %d, expected 0", overlay.FocusIndex)
	}

	if overlay.Submitted {
		t.Error("Submitted should be false initially")
	}

	if overlay.Canceled {
		t.Error("Canceled should be false initially")
	}
}

func TestCommandInputOverlay_GetCommand(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.textarea.SetValue("test command")

	command := overlay.GetCommand()
	if command != "test command" {
		t.Errorf("GetCommand() = %q, expected %q", command, "test command")
	}
}

func TestCommandInputOverlay_GetPriority(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	priority := overlay.GetPriority()
	if priority != 100 {
		t.Errorf("GetPriority() = %d, expected 100", priority)
	}
}

func TestCommandInputOverlay_SetPriority(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.SetPriority(150)
	if overlay.GetPriority() != 150 {
		t.Errorf("Priority = %d, expected 150", overlay.GetPriority())
	}

	// Test bounds
	overlay.SetPriority(250)
	if overlay.GetPriority() != 200 {
		t.Errorf("Priority = %d, expected 200 (capped)", overlay.GetPriority())
	}

	overlay.SetPriority(-10)
	if overlay.GetPriority() != 0 {
		t.Errorf("Priority = %d, expected 0 (capped)", overlay.GetPriority())
	}
}

func TestCommandInputOverlay_IsImmediate(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	if overlay.IsImmediate() {
		t.Error("IsImmediate() should be false by default")
	}
}

func TestCommandInputOverlay_SetImmediate(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.SetImmediate(true)
	if !overlay.IsImmediate() {
		t.Error("IsImmediate() should be true after SetImmediate(true)")
	}

	overlay.SetImmediate(false)
	if overlay.IsImmediate() {
		t.Error("IsImmediate() should be false after SetImmediate(false)")
	}
}

func TestCommandInputOverlay_HandleKeyPressEsc(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	shouldClose := overlay.HandleKeyPress(msg)

	if !shouldClose {
		t.Error("HandleKeyPress(Esc) should return true")
	}

	if !overlay.Canceled {
		t.Error("Canceled should be true after Esc")
	}
}

func TestCommandInputOverlay_HandleKeyPressTab(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	initialFocus := overlay.FocusIndex

	msg := tea.KeyMsg{Type: tea.KeyTab}
	shouldClose := overlay.HandleKeyPress(msg)

	if shouldClose {
		t.Error("HandleKeyPress(Tab) should not close overlay")
	}

	if overlay.FocusIndex == initialFocus {
		t.Error("FocusIndex should change after Tab")
	}
}

func TestCommandInputOverlay_HandleKeyPressShiftTab(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.FocusIndex = 2

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	overlay.HandleKeyPress(msg)

	if overlay.FocusIndex != 1 {
		t.Errorf("FocusIndex = %d, expected 1 after ShiftTab from 2", overlay.FocusIndex)
	}
}

func TestCommandInputOverlay_HandleKeyPressPriorityAdjust(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.FocusIndex = 1 // Focus on priority control
	initialPriority := overlay.GetPriority()

	// Increase priority
	upMsg := tea.KeyMsg{Type: tea.KeyUp}
	overlay.HandleKeyPress(upMsg)

	if overlay.GetPriority() <= initialPriority {
		t.Error("Priority should increase after KeyUp")
	}

	// Decrease priority
	downMsg := tea.KeyMsg{Type: tea.KeyDown}
	overlay.HandleKeyPress(downMsg)
	overlay.HandleKeyPress(downMsg)

	if overlay.GetPriority() >= initialPriority {
		t.Error("Priority should decrease after KeyDown")
	}
}

func TestCommandInputOverlay_HandleKeyPressImmediateToggle(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	overlay.FocusIndex = 2 // Focus on immediate control
	initialImmediate := overlay.IsImmediate()

	// Toggle immediate
	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	overlay.HandleKeyPress(spaceMsg)

	if overlay.IsImmediate() == initialImmediate {
		t.Error("Immediate should toggle after Space key")
	}
}

func TestCommandInputOverlay_SendCommandEmpty(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	// Empty command
	overlay.textarea.SetValue("")

	shouldClose := overlay.sendCommand()

	if shouldClose {
		t.Error("sendCommand() should not close on empty command")
	}

	if overlay.errorMessage == "" {
		t.Error("Error message should be set for empty command")
	}
}

func TestCommandInputOverlay_SendCommandNoController(t *testing.T) {
	overlay := NewCommandInputOverlay("test-session", nil)

	overlay.textarea.SetValue("test command")

	shouldClose := overlay.sendCommand()

	if shouldClose {
		t.Error("sendCommand() should not close without controller")
	}

	if overlay.errorMessage == "" {
		t.Error("Error message should be set when controller is nil")
	}
}

func TestCommandInputOverlay_GetLastCommandID(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	commandID := overlay.GetLastCommandID()
	if commandID != "" {
		t.Error("GetLastCommandID() should return empty string initially")
	}
}

func TestCommandInputOverlay_Render(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	view := overlay.Render()

	if view == "" {
		t.Error("Render() should return non-empty string")
	}

	// Check that title appears
	if !contains(view, "Send Command") {
		t.Error("Render() should contain title")
	}
}

func TestCommandInputOverlay_View(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	view := overlay.View()

	if view == "" {
		t.Error("View() should return non-empty string")
	}
}

func TestCommandInputOverlay_Init(t *testing.T) {
	instance := &session.Instance{Title: "test-session"}
	controller, _ := session.NewClaudeController(instance)
	overlay := NewCommandInputOverlay("test-session", controller)

	cmd := overlay.Init()

	if cmd == nil {
		t.Error("Init() should return a command")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
