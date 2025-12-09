package app

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDebugVisibleWindow directly tests the visible window logic
func TestDebugVisibleWindow(t *testing.T) {
	h := SetupTestHomeWithSession(t, "test-session")

	// Get list state
	list := h.list

	fmt.Println("=== LIST STATE ===")
	fmt.Printf("Total items: %d\n", len(list.GetInstances()))
	selectedInst := h.list.GetSelectedInstance()
	if selectedInst != nil {
		fmt.Printf("Selected instance: %s\n", selectedInst.Title)
	} else {
		fmt.Println("Selected instance: nil")
	}

	// Check visible items
	visibleItems := list.GetVisibleItems()
	fmt.Printf("\n=== VISIBLE ITEMS (getVisibleItems) ===\n")
	fmt.Printf("Count: %d\n", len(visibleItems))
	for i, item := range visibleItems {
		fmt.Printf("  [%d] %s (Category: %s, Status: %v)\n", i, item.Title, item.Category, item.Status)
	}

	// Check rendered output
	rendered := list.String()
	fmt.Printf("\n=== RENDERED OUTPUT ===\n")
	fmt.Printf("Contains 'test-session': %v\n", containsText(rendered, "test-session"))
	fmt.Printf("Contains 'No sessions available': %v\n", containsText(rendered, "No sessions available"))

	// Print first 500 chars of rendered output
	if len(rendered) > 500 {
		fmt.Printf("First 500 chars:\n%s...\n", rendered[:500])
	} else {
		fmt.Printf("Full output:\n%s\n", rendered)
	}

	require.Greater(t, len(visibleItems), 0, "visibleItems should not be empty")
	require.Contains(t, rendered, "test-session", "Rendered output should contain session title")
}
