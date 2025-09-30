package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestRobustConfirmationModalFlow - test confirmation modal with robust rendering approach
func TestRobustConfirmationModalFlow(t *testing.T) {
	// Create app model with session that can be killed
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "test-session")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	// Debug: verify session was added
	t.Logf("Sessions in list: %d", appModel.getInstanceCount())
	if selected := appModel.getSelectedInstance(); selected != nil {
		t.Logf("Selected session: %s", selected.Title)
	} else {
		t.Logf("No selected session")
	}

	// CRITICAL FIX: Ensure the Uncategorized category is expanded in tests
	// This prevents the issue where state persistence loads collapsed state
	appModel.list.ExpandCategory("Uncategorized")

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, appModel, config)

	// Wait for initial render and verify session is visible
	time.Sleep(200 * time.Millisecond)
	output1 := tm.Output()
	outputStr1 := testutil.ReadOutput(t, output1)
	if !strings.Contains(outputStr1, "test-session") {
		t.Errorf("Expected to see 'test-session' in initial render, got:\n%s", outputStr1)
		// Debug: check if sessions are filtered out
		t.Logf("Debug - Sessions in model: %d", appModel.getInstanceCount())
		if searchMode, query := appModel.list.GetSearchState(); searchMode {
			t.Logf("Debug - Search mode active with query: '%s'", query)
		}

		// Debug: check visible items
		visibleItems := appModel.list.GetVisibleItems()
		t.Logf("Debug - Visible items: %d", len(visibleItems))
		for i, item := range visibleItems {
			t.Logf("Debug - Visible item %d: %s (status: %d)", i, item.Title, int(item.Status))
		}
		return
	}
	t.Logf("✅ Initial render OK - session visible")

	// Press 'D' to trigger kill confirmation
	tm.Type("D")

	// Wait for confirmation modal to appear
	testutil.WaitForOutputContainsAfterAction(t, tm, "Kill session 'test-session'?", 200*time.Millisecond)
	t.Logf("✅ Confirmation modal appeared")

	// Press 'n' to cancel
	tm.Type("n")
	time.Sleep(200 * time.Millisecond)

	// CRITICAL: Force a window resize to ensure UI refresh (workaround for teatest timing)
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(100 * time.Millisecond)

	// Now check that modal is closed and session still exists
	output2 := tm.Output()
	outputStr2 := testutil.ReadOutput(t, output2)

	// Check that modal is closed (no longer contains confirmation text)
	if strings.Contains(outputStr2, "Kill session 'test-session'?") {
		t.Errorf("Expected confirmation modal to be closed, but it's still visible:\n%s", outputStr2)
		return
	}

	// Check that session still exists (should be back to normal view)
	if !strings.Contains(outputStr2, "test-session") {
		t.Errorf("Expected session to still exist after canceling, got:\n%s", outputStr2)
		return
	}

	t.Logf("✅ Modal canceled successfully - session still exists")

	// Verify data integrity by checking the actual model state
	if appModel.getInstanceCount() != 1 {
		t.Errorf("Expected 1 session in model, got %d", appModel.getInstanceCount())
		return
	}

	selected := appModel.getSelectedInstance()
	if selected == nil || selected.Title != "test-session" {
		t.Errorf("Expected selected session to be 'test-session', got %v", selected)
		return
	}

	t.Logf("✅ Data integrity verified - session preserved in model")

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second)) // Increased timeout for state saving operations
}