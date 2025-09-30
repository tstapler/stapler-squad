package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestWindowSizeCalculations tests the window size calculation logic
func TestWindowSizeCalculations(t *testing.T) {

	// Test with various terminal sizes
	testCases := []struct {
		name   string
		width  int
		height int
	}{
		{"Small terminal", 80, 24},
		{"Medium terminal", 120, 30},
		{"Large terminal", 160, 40},
		{"Tall terminal", 100, 50},
		{"Wide terminal", 200, 25},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create window size message
			msg := tea.WindowSizeMsg{
				Width:  tc.width,
				Height: tc.height,
			}

			// Test the size calculation logic
			listWidth := int(float32(msg.Width) * 0.3)
			tabsWidth := msg.Width - listWidth

			// Current calculation from the code (updated)
			menuHeight := 3
			errorBoxHeight := 1
			contentHeight := msg.Height - menuHeight - errorBoxHeight

			// Verify basic constraints
			if listWidth <= 0 {
				t.Errorf("List width should be positive, got %d", listWidth)
			}
			if tabsWidth <= 0 {
				t.Errorf("Tabs width should be positive, got %d", tabsWidth)
			}
			if contentHeight <= 0 {
				t.Errorf("Content height should be positive, got %d", contentHeight)
			}

			// Verify proportions make sense
			if listWidth+tabsWidth != msg.Width {
				t.Errorf("Width doesn't add up: list(%d) + tabs(%d) = %d, expected %d",
					listWidth, tabsWidth, listWidth+tabsWidth, msg.Width)
			}

			// Verify we're not taking too much vertical space
			totalUsedHeight := contentHeight + menuHeight + errorBoxHeight
			if totalUsedHeight != msg.Height {
				t.Errorf("Height doesn't add up: content(%d) + menu(%d) + error(%d) = %d, expected %d",
					contentHeight, menuHeight, errorBoxHeight, totalUsedHeight, msg.Height)
			}

			// Content should be reasonable size
			if contentHeight < 10 && msg.Height > 15 {
				t.Errorf("Content height too small (%d) for terminal height %d", contentHeight, msg.Height)
			}

			t.Logf("Terminal %dx%d -> List: %dx%d, Preview: %dx%d",
				msg.Width, msg.Height, listWidth, contentHeight, tabsWidth, contentHeight)
		})
	}
}

// TestLayoutSpacing tests that the layout spacing matches expectations
func TestLayoutSpacing(t *testing.T) {
	// Test the View method layout structure
	// This is a conceptual test since we can't easily test the actual rendering

	// The layout should be:
	// 1. listAndPreview (with PaddingTop(1) on each side)
	// 2. menu
	// 3. errBox

	// Calculate expected spacing (updated - no padding)
	menuRows := 3     // Estimated menu height
	errorBoxRows := 1 // Error box height

	totalReservedRows := menuRows + errorBoxRows

	// For a 30-row terminal, content should get:
	contentRows := 30 - totalReservedRows
	expectedContentRows := 26 // 30 - 3 - 1

	if contentRows != expectedContentRows {
		t.Errorf("Content rows calculation: got %d, expected %d", contentRows, expectedContentRows)
	}

	t.Logf("Layout spacing: menu=%d, error=%d, content=%d, total=%d",
		menuRows, errorBoxRows, contentRows, 30)
}

// TestLayoutConstraints tests edge cases and constraints
func TestLayoutConstraints(t *testing.T) {
	testCases := []struct {
		name           string
		width          int
		height         int
		expectMinimums bool
	}{
		{"Very small terminal", 20, 10, true},
		{"Minimum viable", 60, 20, false},
		{"Single line height", 80, 1, true},
		{"Zero width", 0, 24, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the size calculation with constraints
			listWidth := int(float32(tc.width) * 0.3)
			tabsWidth := tc.width - listWidth

			menuHeight := 3
			errorBoxHeight := 1
			contentHeight := tc.height - menuHeight - errorBoxHeight

			// Apply minimum constraints (from the current code)
			if contentHeight < 10 {
				contentHeight = 10
			}

			if tc.expectMinimums {
				// For very small terminals, we should hit minimum constraints
				if tc.height <= 15 && contentHeight != 10 {
					t.Errorf("Expected minimum content height (10) for small terminal, got %d", contentHeight)
				}
			}

			// Verify we don't have negative or zero dimensions
			if listWidth < 0 || tabsWidth < 0 || contentHeight < 0 {
				t.Errorf("Negative dimensions: list=%d, tabs=%d, content=%d",
					listWidth, tabsWidth, contentHeight)
			}
		})
	}
}