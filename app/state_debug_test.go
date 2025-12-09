package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestStateTransitionDebug - debug the state transitions during confirmation modal
func TestStateTransitionDebug(t *testing.T) {
	// Create app model with session that can be killed
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "debug-session")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	// CRITICAL: Ensure category is expanded so session is visible
	appModel.list.OrganizeByCategory()
	appModel.list.ExpandCategory("Squad Sessions")
	appModel.list.OrganizeByCategory()

	config := testutil.DefaultTUIConfig()

	// PRE-CONFIGURE component dimensions BEFORE creating teatest model
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
	appModel.terminalManager = nil

	tm := testutil.CreateTUITest(t, appModel, config)

	// Step 1: Initial state check
	time.Sleep(100 * time.Millisecond)
	t.Logf("=== STEP 1: Initial State ===")
	t.Logf("Sessions count: %d", appModel.getInstanceCount())
	t.Logf("Current state: %v", appModel.getState())
	t.Logf("Menu state: %v", appModel.menu.GetState())
	t.Logf("Visible items count: %d", len(appModel.list.GetInstances()))

	// Verify session is visible
	output1 := tm.Output()
	outputStr1 := testutil.ReadOutput(t, output1)
	containsSession := strings.Contains(outputStr1, "debug-session")
	t.Logf("UI contains session: %v", containsSession)

	// Step 2: Press 'D' to trigger confirmation
	tm.Type("D")
	time.Sleep(200 * time.Millisecond)

	t.Logf("=== STEP 2: After 'D' Keypress ===")
	t.Logf("Sessions count: %d", appModel.getInstanceCount())
	t.Logf("Current state: %v", appModel.getState())
	t.Logf("Menu state: %v", appModel.menu.GetState())

	output2 := tm.Output()
	outputStr2 := testutil.ReadOutput(t, output2)
	containsConfirmation := strings.Contains(outputStr2, "Kill session")
	t.Logf("UI contains confirmation: %v", containsConfirmation)

	// Step 3: Press 'n' to cancel
	tm.Type("n")
	time.Sleep(500 * time.Millisecond) // Increased delay to allow BubbleTea command processing

	t.Logf("=== STEP 3: After 'n' Keypress (Cancel) ===")
	t.Logf("Sessions count: %d", appModel.getInstanceCount())
	t.Logf("Current state: %v", appModel.getState())
	t.Logf("Menu state: %v", appModel.menu.GetState())

	// Check if visible cache is valid
	t.Logf("List cache valid: %v", appModel.list.IsVisibleCacheValid())
	t.Logf("Visible items from list: %d", len(appModel.list.GetVisibleItems()))

	// Check what the list is actually rendering
	listContent := appModel.list.String()
	t.Logf("List content length: %d", len(listContent))
	if len(listContent) < 200 {
		t.Logf("List content: %q", listContent)
	} else {
		t.Logf("List content (first 200 chars): %q", listContent[:200])
	}

	// Check what view directive the state manager is returning
	directive := appModel.stateManager.GetViewDirective()
	t.Logf("View directive type: %v", directive.Type)
	if directive.Type == 2 { // ViewOverlay enum value
		t.Logf("Overlay component: %s", directive.OverlayComponent)
		t.Logf("Should reset on nil: %v", directive.ShouldResetOnNil)
	}

	output3 := tm.Output()
	outputStr3 := testutil.ReadOutput(t, output3)
	containsSessionAfter := strings.Contains(outputStr3, "debug-session")
	t.Logf("UI contains session after cancel: %v", containsSessionAfter)

	// Step 4: Force a window resize to trigger re-render
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(100 * time.Millisecond)

	t.Logf("=== STEP 4: After Window Resize ===")
	output4 := tm.Output()
	outputStr4 := testutil.ReadOutput(t, output4)
	containsSessionAfterResize := strings.Contains(outputStr4, "debug-session")
	t.Logf("UI contains session after resize: %v", containsSessionAfterResize)

	if !containsSessionAfterResize {
		t.Logf("Final output content:\n%s", outputStr4)
	}

	// Clean exit - increased timeout to account for state isolation setup/teardown
	// Using 15s as this test creates/saves state which can be slow with workspace isolation
	quitStart := time.Now()
	t.Logf("=== QUIT: Sending quit command at %v ===", quitStart)
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(15*time.Second))
	quitDuration := time.Since(quitStart)
	t.Logf("=== QUIT: Completed in %v ===", quitDuration)

	// Warn if quit took longer than expected
	if quitDuration > 5*time.Second {
		t.Logf("⚠️  Quit took longer than 5s - this may indicate slow state saving with workspace isolation")
	}
}