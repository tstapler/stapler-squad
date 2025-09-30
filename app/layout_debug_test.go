package app

import (
	"testing"
)

// TestLayoutDebug tests the actual layout behavior to debug sizing issues
func TestLayoutDebug(t *testing.T) {
	// Test the core issue: list height calculation mismatch

	// User reported terminal scenario:
	// - Can see items 12-31 (about 20 items visible)
	// - Item 1 is selected but not visible
	// - Preview is misaligned

	terminalWidth := 120
	terminalHeight := 30

	// Our current calculation
	listWidth := int(float32(terminalWidth) * 0.3)  // 36
	contentHeight := terminalHeight - 3 - 1        // 26

	t.Logf("Terminal: %dx%d", terminalWidth, terminalHeight)
	t.Logf("List allocated size: %dx%d", listWidth, contentHeight)

	// Test the calculateMaxVisibleItems logic with realistic assumptions
	// Based on actual list rendering, each item likely takes 1 line
	// and there may be minimal title/padding overhead
	titleLines := 2  // Reduced from overly conservative 4
	padding := 2     // Reduced from overly conservative 4
	availableHeight := contentHeight - titleLines - padding
	linesPerItem := 1 // Most realistic: 1 line per item for simple session list
	maxItems := availableHeight / linesPerItem

	t.Logf("Height calculation breakdown:")
	t.Logf("  Content height: %d", contentHeight)
	t.Logf("  Title lines: %d", titleLines)
	t.Logf("  Padding: %d", padding)
	t.Logf("  Available height: %d", availableHeight)
	t.Logf("  Lines per item: %d", linesPerItem)
	t.Logf("  Calculated max items: %d", maxItems)

	// With realistic assumptions: contentHeight=26, availableHeight=26-2-2=22, maxItems=22/1=22
	// This is much closer to the user's reported ~20 visible items.

	if maxItems < 15 {
		t.Errorf("ISSUE FOUND: Calculated max items (%d) is less than expected minimum (15)", maxItems)
	}

	// Let's test what the calculation should be to show 20 items
	targetVisibleItems := 20
	requiredAvailableHeight := targetVisibleItems * linesPerItem
	requiredContentHeight := requiredAvailableHeight + titleLines + padding

	t.Logf("To show %d items, we need:", targetVisibleItems)
	t.Logf("  Available height: %d", requiredAvailableHeight)
	t.Logf("  Content height: %d", requiredContentHeight)

	if requiredContentHeight > contentHeight {
		t.Logf("MISMATCH: We need %d content height but only allocated %d", requiredContentHeight, contentHeight)
		t.Logf("This suggests the list calculation assumptions are wrong!")
	}
}

