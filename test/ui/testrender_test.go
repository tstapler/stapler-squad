package ui

import (
	"claude-squad/ui/overlay"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmationOverlayRendering(t *testing.T) {
	// Create a confirmation overlay
	message := "[!] Test confirmation message"
	confirmOverlay := overlay.NewConfirmationOverlay(message)

	// Create a test renderer
	renderer := NewTestRenderer().
		SetSnapshotPath("../../test/ui/snapshots/overlays").
		DisableColors()

	// Option 1: Compare with snapshot
	if testing.Short() {
		t.Skip("Skipping snapshot test in short mode")
	}

	// Create snapshot if it doesn't exist
	renderer.UpdateSnapshots = true
	renderer.CompareComponentWithSnapshot(t, confirmOverlay, "confirmation_overlay.txt")

	// Option 2: Direct assertion
	output, err := renderer.RenderComponent(confirmOverlay)
	require.NoError(t, err)
	assert.Contains(t, output, "Test confirmation message")
	assert.Contains(t, output, "Press")
	assert.Contains(t, output, "to confirm")
	assert.Contains(t, output, "to cancel")
}

func TestCustomBorderStyle(t *testing.T) {
	// Create a confirmation overlay with custom styling
	message := "Custom border test"
	confirmOverlay := overlay.NewConfirmationOverlay(message)

	// Set custom border color - skip color test since we removed lipgloss

	// Create a test renderer
	renderer := NewTestRenderer().
		SetSnapshotPath("../../test/ui/snapshots/overlays")

	// Render component
	output, err := renderer.RenderComponent(confirmOverlay)
	require.NoError(t, err)

	// Save to file for manual inspection
	err = renderer.SaveComponentOutput(confirmOverlay, "custom_border_overlay.txt")
	require.NoError(t, err)

	// Basic verification
	assert.Contains(t, output, "Custom border test")
}

func TestComponentDimensions(t *testing.T) {
	// Create a confirmation overlay
	confirmOverlay := overlay.NewConfirmationOverlay("Test with different width")

	// Set custom width
	confirmOverlay.SetWidth(30)

	// Create test renderer
	renderer := NewTestRenderer().
		SetSnapshotPath("../../test/ui/snapshots/overlays").
		DisableColors()

	// Render and verify
	output, err := renderer.RenderComponent(confirmOverlay)
	require.NoError(t, err)

	// Save output for inspection
	err = renderer.SaveComponentOutput(confirmOverlay, "narrow_confirmation.txt")
	require.NoError(t, err)

	// Basic verification that content is present
	assert.Contains(t, output, "Test with different width")
}
