package app

import (
	"claude-squad/cmd"
	"claude-squad/ui"
	"testing"
)

// TestMenuShortcutIntegration verifies that menu shortcuts are properly populated from bridge commands
func TestMenuShortcutIntegration(t *testing.T) {
	// Initialize bridge directly for testing
	bridge := cmd.GetGlobalBridge()

	// Initialize with empty handlers (sufficient for menu testing)
	bridge.Initialize(nil, nil, nil, nil, nil)
	bridge.SetContext(cmd.ContextList)

	// Get available keys from bridge
	availableKeys := bridge.GetAvailableKeys()

	// Verify we have some keys
	if len(availableKeys) == 0 {
		t.Error("Bridge should provide available keys")
	}

	// Check for critical keys that should be available in list context
	expectedKeys := []string{"n", "q"}
	for _, key := range expectedKeys {
		if description, exists := availableKeys[key]; !exists {
			t.Errorf("Expected key '%s' not available from bridge", key)
		} else if description == "" {
			t.Errorf("Key '%s' has empty description", key)
		}
	}

	// Verify 'n' key is mapped to new session
	if desc, exists := availableKeys["n"]; exists {
		if desc != "Create a new session" {
			t.Errorf("Expected 'n' key to be 'Create a new session', got: %s", desc)
		}
	}
}

// TestUpdateMenuFromContext verifies that the updateMenuFromContext helper works
func TestUpdateMenuFromContext(t *testing.T) {
	// Initialize bridge directly for testing
	bridge := cmd.GetGlobalBridge()
	bridge.Initialize(nil, nil, nil, nil, nil)
	bridge.SetContext(cmd.ContextList)

	// Create a simple home model with menu
	homeModel := &home{
		bridge: bridge,
		menu:   ui.NewMenu(),
	}

	// Test that updateMenuFromContext doesn't panic
	homeModel.updateMenuFromContext()

	// Verify menu was updated
	menuOptions := homeModel.menu.GetOptions()
	if len(menuOptions) == 0 {
		t.Error("Menu should have options after updateMenuFromContext call")
	}

	// Check for at least one expected command
	hasExpectedCommand := false
	for _, option := range menuOptions {
		if option.Key == "n" || option.Key == "q" {
			hasExpectedCommand = true
			break
		}
	}

	if !hasExpectedCommand {
		t.Error("Menu should contain at least one expected command after update")
	}
}
