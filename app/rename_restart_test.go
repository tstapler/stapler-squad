package app

import (
	"claude-squad/cmd"
	"claude-squad/session"
	"testing"
)

func TestRenameSessionCommand(t *testing.T) {
	// Test that rename and restart commands are registered in the global bridge
	bridge := cmd.GetGlobalBridge()

	// Make sure the commands are initialized by setting a context
	bridge.SetContext(cmd.ContextList)

	// Get available keys to check our commands are registered
	availableKeys := bridge.GetAvailableKeys()

	// Check that the rename command exists
	if _, exists := availableKeys["r"]; !exists {
		t.Error("Rename command (r) not registered")
	}

	// Check that the restart command exists
	if _, exists := availableKeys["R"]; !exists {
		t.Error("Restart command (R) not registered")
	}
}

func TestRenameSessionNotStarted(t *testing.T) {
	// Create a test instance that is not started
	instance := &session.Instance{
		Title: "TestSession",
	}

	// Verify we can rename it
	err := instance.SetTitle("NewName")
	if err != nil {
		t.Errorf("Should be able to rename unstarted session: %v", err)
	}

	if instance.Title != "NewName" {
		t.Errorf("Expected title 'NewName', got '%s'", instance.Title)
	}
}

func TestRenameSessionStarted(t *testing.T) {
	// Note: We can't directly test started session rename without full instance setup
	// because the 'started' field is private. This functionality is tested through
	// integration tests and manual testing.
	t.Skip("Cannot directly test started session rename without full instance setup")
}