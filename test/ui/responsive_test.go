package ui

import (
	"claude-squad/ui/overlay"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestOverlayResponsiveness tests overlay rendering at different terminal sizes
func TestOverlayResponsiveness(t *testing.T) {
	// Test terminal sizes: narrow, medium, wide
	terminalSizes := []struct {
		name   string
		width  int
		height int
	}{
		{"narrow_40cols", 40, 24},
		{"medium_80cols", 80, 30},
		{"wide_120cols", 120, 40},
	}

	t.Run("ConfirmationOverlay", func(t *testing.T) {
		for _, size := range terminalSizes {
			t.Run(size.name, func(t *testing.T) {
				renderer := NewTestRenderer().
					SetSnapshotPath("snapshots/responsive/confirmation").
					SetDimensions(size.width, size.height).
					DisableColors()

				confirmOverlay := overlay.NewConfirmationOverlay("Are you sure you want to delete this session? This action cannot be undone.")
				confirmOverlay.SetSize(size.width, size.height)

				renderer.CompareComponentWithSnapshot(t, confirmOverlay, size.name+".txt")
			})
		}
	})

	t.Run("TextInputOverlay", func(t *testing.T) {
		for _, size := range terminalSizes {
			t.Run(size.name, func(t *testing.T) {
				renderer := NewTestRenderer().
					SetSnapshotPath("snapshots/responsive/textinput").
					SetDimensions(size.width, size.height).
					DisableColors()

				textInput := overlay.NewTextInputOverlay("Session Name", "my-test-session-with-a-very-long-name-that-should-wrap")
				textInput.SetSize(size.width, size.height)

				renderer.CompareComponentWithSnapshot(t, textInput, size.name+".txt")
			})
		}
	})

	t.Run("SessionSetupOverlay", func(t *testing.T) {
		for _, size := range terminalSizes {
			t.Run(size.name+"_basics", func(t *testing.T) {
				renderer := NewTestRenderer().
					SetSnapshotPath("snapshots/responsive/session_setup").
					SetDimensions(size.width, size.height).
					DisableColors()

				setupOverlay := overlay.NewSessionSetupOverlay()
				setupOverlay.SetSize(size.width, size.height)

				renderer.CompareComponentWithSnapshot(t, setupOverlay, size.name+"_basics.txt")
			})

			t.Run(size.name+"_location", func(t *testing.T) {
				renderer := NewTestRenderer().
					SetSnapshotPath("snapshots/responsive/session_setup").
					SetDimensions(size.width, size.height).
					DisableColors()

				setupOverlay := overlay.NewSessionSetupOverlay()
				setupOverlay.SetSize(size.width, size.height)

				// Advance to location step
				setupOverlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				setupOverlay.Update(tea.KeyMsg{Type: tea.KeyEnter})

				renderer.CompareComponentWithSnapshot(t, setupOverlay, size.name+"_location.txt")
			})

			t.Run(size.name+"_branch", func(t *testing.T) {
				renderer := NewTestRenderer().
					SetSnapshotPath("snapshots/responsive/session_setup").
					SetDimensions(size.width, size.height).
					DisableColors()

				setupOverlay := overlay.NewSessionSetupOverlay()
				setupOverlay.SetSize(size.width, size.height)

				// Advance to branch step (basics -> location -> branch)
				setupOverlay.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
				setupOverlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
				setupOverlay.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Current location

				renderer.CompareComponentWithSnapshot(t, setupOverlay, size.name+"_branch.txt")
			})
		}
	})
}

// TestResponsiveWidthCalculation verifies responsive width calculations
func TestResponsiveWidthCalculation(t *testing.T) {
	tests := []struct {
		name           string
		termWidth      int
		expectedWidth  int
		expectedHPad   int
		expectedVPad   int
	}{
		{"very_narrow", 30, 40, 1, 0},   // Clamps to minimum 40
		{"narrow", 50, 40, 1, 0},        // 80% = 40
		{"medium", 80, 64, 2, 1},        // 80% = 64
		{"wide", 100, 80, 3, 1},         // 80% = 80
		{"very_wide", 150, 100, 3, 1},   // Clamps to maximum 100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width := overlay.GetResponsiveWidth(tt.termWidth)
			if width != tt.expectedWidth {
				t.Errorf("GetResponsiveWidth(%d) = %d, expected %d", tt.termWidth, width, tt.expectedWidth)
			}

			hPad, vPad := overlay.GetResponsivePadding(tt.termWidth)
			if hPad != tt.expectedHPad || vPad != tt.expectedVPad {
				t.Errorf("GetResponsivePadding(%d) = (%d, %d), expected (%d, %d)",
					tt.termWidth, hPad, vPad, tt.expectedHPad, tt.expectedVPad)
			}
		})
	}
}

// TestDescriptionShortening verifies description shortening for narrow terminals
func TestDescriptionShortening(t *testing.T) {
	tests := []struct {
		name        string
		desc        string
		termWidth   int
		shouldTrunc bool
	}{
		{
			name:        "wide_no_truncation",
			desc:        "A new branch will be created based on your session name",
			termWidth:   120,
			shouldTrunc: false,
		},
		{
			name:        "medium_no_truncation",
			desc:        "Work on the repository's current branch",
			termWidth:   80,
			shouldTrunc: false,
		},
		{
			name:        "narrow_truncation",
			desc:        "A very long description that should be truncated on narrow terminals to fit",
			termWidth:   50,
			shouldTrunc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shortened := overlay.ShortenDescriptionForWidth(tt.desc, tt.termWidth)

			if tt.shouldTrunc {
				if len(shortened) >= len(tt.desc) {
					t.Errorf("Expected description to be shortened on %d cols, but it wasn't", tt.termWidth)
				}
			} else {
				if shortened != tt.desc {
					t.Errorf("Expected description to remain unchanged on %d cols, but it was modified", tt.termWidth)
				}
			}
		})
	}
}

// TestOverlayMinimumWidths verifies overlays maintain readable minimum widths
func TestOverlayMinimumWidths(t *testing.T) {
	// Test with extremely narrow terminal (30 columns)
	veryNarrow := 30

	t.Run("ConfirmationOverlay_readable_at_minimum", func(t *testing.T) {
		confirmOverlay := overlay.NewConfirmationOverlay("Confirm?")
		confirmOverlay.SetSize(veryNarrow, 10)

		output := confirmOverlay.View()
		if output == "" {
			t.Error("ConfirmationOverlay produces empty output at minimum width")
		}
		// Should still contain key elements
		if len(output) < 10 {
			t.Error("ConfirmationOverlay output too short at minimum width")
		}
	})

	t.Run("TextInputOverlay_readable_at_minimum", func(t *testing.T) {
		textInput := overlay.NewTextInputOverlay("Name", "test")
		textInput.SetSize(veryNarrow, 10)

		output := textInput.View()
		if output == "" {
			t.Error("TextInputOverlay produces empty output at minimum width")
		}
	})
}
