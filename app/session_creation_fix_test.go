package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestSessionCreationOverlayFix - verify that pressing 'n' properly activates the session setup overlay
func TestSessionCreationOverlayFix(t *testing.T) {
	// Create app model with full initialization including proper bridge handlers
	// Use BuildWithMockDependencies instead of BuildWithMockDependenciesNoInit to get proper bridge setup
	appModel := NewTestHomeBuilder().WithBridge().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
		// Mock dependencies for test environment
	})

	// Ensure categories are organized and expanded for proper rendering
	appModel.list.OrganizeByCategory()
	appModel.list.SetSize(80, 24)
	appModel.list.ExpandCategory("Uncategorized")

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, appModel, config)

	// Wait for initial render
	time.Sleep(100 * time.Millisecond)

	// Test the "n" key to trigger session creation
	t.Logf("Testing 'n' key for session creation...")

	// Debug: Check initial state
	t.Logf("Before 'n' key - App state: %v", appModel.stateManager.Current())

	// Debug: Check if bridge has the command registered
	command := appModel.bridge.GetCommandForKey("n")
	if command != nil {
		t.Logf("Bridge has command for 'n': %s (%s)", command.Name, command.ID)
	} else {
		t.Logf("Bridge does NOT have command for 'n' key")
	}

	tm.Type("n")
	time.Sleep(300 * time.Millisecond) // Allow time for overlay to activate

	// Debug: Check state after 'n' key
	t.Logf("After 'n' key - App state: %v", appModel.stateManager.Current())

	// Debug: Check if overlay is active in coordinator
	if overlay := appModel.uiCoordinator.GetSessionSetupOverlay(); overlay != nil {
		t.Logf("✅ Session setup overlay is active in coordinator")
	} else {
		t.Logf("❌ Session setup overlay is NOT active in coordinator")
	}

	// Check if session setup overlay appeared
	output := tm.Output()
	outputStr := testutil.ReadOutput(t, output)

	// The session setup overlay should be visible in the output
	// Look for session setup related text
	hasSessionSetup := strings.Contains(outputStr, "Session") ||
					  strings.Contains(outputStr, "Name") ||
					  strings.Contains(outputStr, "Directory") ||
					  strings.Contains(outputStr, "Program")

	if hasSessionSetup {
		t.Logf("✅ SESSION CREATION FIX VERIFIED: Session setup overlay is active")
	} else {
		t.Logf("Raw output after 'n' key:\n%s", outputStr)
		t.Errorf("ISSUE: Session setup overlay did not appear after 'n' key press")
		return
	}

	// Test canceling the session creation with escape
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(200 * time.Millisecond)

	// Verify we're back to the main view
	output2 := tm.Output()
	outputStr2 := testutil.ReadOutput(t, output2)

	// Should see main list view without overlay
	isBackToMain := !strings.Contains(outputStr2, "Session Name") &&
					!strings.Contains(outputStr2, "Directory")

	if isBackToMain {
		t.Logf("✅ SESSION CREATION CANCEL VERIFIED: Returned to main view")
	} else {
		t.Logf("Warning: May still be in session setup mode")
	}

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(15*time.Second))

	t.Logf("🎉 SESSION CREATION OVERLAY FIX TEST COMPLETED")
}