package ui

import (
	"testing"
)

// TestStatusBarViewWithSmallWidth tests that View handles small widths gracefully
func TestStatusBarViewWithSmallWidth(t *testing.T) {
	sb := NewStatusBar()
	sb.SetInfo("This is a test message")

	tests := []struct {
		name  string
		width int
	}{
		{"zero width", 0},
		{"width 1", 1},
		{"width 5", 5},
		{"width 9", 9},
		{"width 10", 10},
		{"width 20", 20},
		{"width 100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := sb.View(tt.width)

			// For small widths, should return empty string
			if tt.width < 10 && result != "" {
				t.Errorf("Expected empty string for width %d, got: %q", tt.width, result)
			}

			// For larger widths, should return something
			if tt.width >= 10 && result == "" {
				t.Errorf("Expected non-empty string for width %d", tt.width)
			}
		})
	}
}

// TestGetMessagesViewWithSmallWidth tests that GetMessagesView handles small widths gracefully
func TestGetMessagesViewWithSmallWidth(t *testing.T) {
	sb := NewStatusBar()
	sb.SetInfo("Test message 1")
	sb.SetInfo("Test message 2")

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 10},
		{"width 1", 1, 10},
		{"width 5", 5, 10},
		{"width 9", 9, 10},
		{"width 10", 10, 10},
		{"width 50", 50, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := sb.GetMessagesView(tt.width, tt.height)

			// For small widths, should return "Terminal too narrow"
			if tt.width < 10 && result != "Terminal too narrow" {
				t.Errorf("Expected 'Terminal too narrow' for width %d, got: %q", tt.width, result)
			}

			// For larger widths, should return something useful
			if tt.width >= 10 && len(result) == 0 {
				t.Errorf("Expected non-empty result for width %d", tt.width)
			}
		})
	}
}
