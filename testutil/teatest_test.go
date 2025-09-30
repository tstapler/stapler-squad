package testutil

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCreateMinimalApp tests that our minimal app model works
func TestCreateMinimalApp(t *testing.T) {
	model := CreateMinimalApp(t)

	// Test that it implements tea.Model interface
	var _ tea.Model = model

	// Test that Init() returns nil (no initial commands)
	cmd := model.Init()
	if cmd != nil {
		t.Errorf("Expected Init() to return nil, got %v", cmd)
	}

	// Test basic View() output
	view := model.View()
	if view == "" {
		t.Error("Expected non-empty View() output")
	}

	// Test that it contains expected text
	if !strings.Contains(view, "Test App Started") {
		t.Errorf("Expected view to contain 'Test App Started', got: %s", view)
	}
}

// TestTUITestConfig tests the configuration struct
func TestTUITestConfig(t *testing.T) {
	config := DefaultTUIConfig()

	if config.Width <= 0 {
		t.Errorf("Expected positive width, got %d", config.Width)
	}

	if config.Height <= 0 {
		t.Errorf("Expected positive height, got %d", config.Height)
	}

	if config.Timeout <= 0 {
		t.Errorf("Expected positive timeout, got %v", config.Timeout)
	}
}