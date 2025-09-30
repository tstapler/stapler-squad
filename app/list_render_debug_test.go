package app

import (
	"testing"
)

// TestListRenderingDirectly - test if list renders sessions when called directly
func TestListRenderingDirectly(t *testing.T) {
	// Create app model with session
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "direct-render-test")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	// Debug: verify session was added
	t.Logf("Sessions in list: %d", appModel.getInstanceCount())
	if selected := appModel.getSelectedInstance(); selected != nil {
		t.Logf("Selected session: %s", selected.Title)
	}

	// Debug: check visible items
	visibleItems := appModel.list.GetVisibleItems()
	t.Logf("Visible items: %d", len(visibleItems))
	for i, item := range visibleItems {
		t.Logf("Visible item %d: %s (status: %d)", i, item.Title, int(item.Status))
	}

	// Debug: ensure categories are organized
	appModel.list.OrganizeByCategory()

	// Set a reasonable size for the list
	appModel.list.SetSize(50, 20)

	// CRITICAL FIX: Ensure the Uncategorized category is expanded in tests
	// This prevents the issue where state persistence loads collapsed state
	appModel.list.ExpandCategory("Uncategorized")

	// Call list.String() directly to see what it renders
	listOutput := appModel.list.String()
	t.Logf("List output length: %d characters", len(listOutput))

	if listOutput == "" {
		t.Errorf("List.String() returned empty string")
		return
	}

	// Check if the session appears in the direct list output
	if !containsSubstring(listOutput, "direct-render-test") {
		t.Errorf("Session 'direct-render-test' not found in list output")
		t.Logf("List output:\n%s", listOutput)
		return
	}

	t.Logf("✅ List renders session correctly when called directly")
}

// Helper function since we can't import strings in all cases
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}