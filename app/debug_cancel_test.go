package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestDebugCancelBehavior - debug what happens when we cancel a confirmation
func TestDebugCancelBehavior(t *testing.T) {
	// Create app model with session that can be killed
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "test-session")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	// CRITICAL: Ensure category is expanded so session is visible
	appModel.list.OrganizeByCategory()
	// Note: "Uncategorized" sessions are transformed to "Squad Sessions" by OrganizeByStrategy
	appModel.list.ExpandCategory("Squad Sessions")
	// FORCE re-organization to ensure expansion state takes effect
	appModel.list.OrganizeByCategory()

	config := testutil.DefaultTUIConfig()

	// PRE-CONFIGURE component dimensions BEFORE creating teatest model
	// This ensures the first render uses correct dimensions instead of waiting for WindowSizeMsg
	listWidth := int(float32(config.Width) * 0.3)
	tabsWidth := config.Width - listWidth
	menuHeight := 3
	errorBoxHeight := 1
	contentHeight := config.Height - menuHeight - errorBoxHeight

	// Set component dimensions directly
	appModel.list.SetSize(listWidth, contentHeight)
	appModel.tabbedWindow.SetSize(tabsWidth, contentHeight)
	appModel.menu.SetSize(config.Width, menuHeight)

	// CRITICAL: Set termWidth and termHeight to ensure View() renders correctly
	appModel.termWidth = config.Width
	appModel.termHeight = config.Height

	// CRITICAL FIX: Set terminalManager to nil to prevent Init() from sending 80x24 WindowSizeMsg
	// This allows BubbleTea's teatest framework to control terminal dimensions
	appModel.terminalManager = nil

	tm := testutil.CreateTUITest(t, appModel, config)

	// Step 1: Initial render
	time.Sleep(100 * time.Millisecond)
	output1 := tm.Output()
	outputStr1 := testutil.ReadOutput(t, output1)
	t.Logf("=== STEP 1: Initial render ===\n%s", outputStr1)
	t.Logf("Sessions count: %d", appModel.getInstanceCount())

	// Step 2: Press 'D'
	tm.Type("D")
	time.Sleep(200 * time.Millisecond)
	output2 := tm.Output()
	outputStr2 := testutil.ReadOutput(t, output2)
	t.Logf("=== STEP 2: After 'D' keypress ===\n%s", outputStr2)
	t.Logf("Sessions count: %d", appModel.getInstanceCount())
	t.Logf("Contains confirmation: %v", strings.Contains(outputStr2, "Kill session"))

	// Step 3: Press 'n' to cancel
	tm.Type("n")
	time.Sleep(200 * time.Millisecond)
	output3 := tm.Output()
	outputStr3 := testutil.ReadOutput(t, output3)
	t.Logf("=== STEP 3: After 'n' keypress (cancel) ===\n%s", outputStr3)
	t.Logf("Sessions count: %d", appModel.getInstanceCount())
	t.Logf("Contains test-session: %v", strings.Contains(outputStr3, "test-session"))

	// Step 4: Try pressing a navigation key to see if the session is still there
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	time.Sleep(100 * time.Millisecond)
	output4 := tm.Output()
	outputStr4 := testutil.ReadOutput(t, output4)
	t.Logf("=== STEP 4: After navigation key ===\n%s", outputStr4)
	t.Logf("Sessions count: %d", appModel.getInstanceCount())

	// Clean exit - increased timeout to account for state isolation setup/teardown
	// Using 10s to match other passing tests that save state on quit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}
