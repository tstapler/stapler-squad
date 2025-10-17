package ui

import (
	"claude-squad/session"
	"claude-squad/ui/overlay"
	"testing"
)

// TestResponsiveHeightCalculation tests height-responsive calculations
func TestResponsiveHeightCalculation(t *testing.T) {
	tests := []struct {
		name           string
		termHeight     int
		expectedHeight int
		shouldDetail   bool
		shouldHelp     bool
	}{
		{"very_short", 15, 10, false, false},  // Clamps to minimum 10
		{"short", 20, 14, false, true},        // 70% = 14, no detail, yes help
		{"medium", 30, 21, true, true},        // 70% = 21, detail + help
		{"tall", 50, 35, true, true},          // 70% = 35
		{"very_tall", 100, 40, true, true},    // Clamps to maximum 40
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			height := overlay.GetResponsiveHeight(tt.termHeight)
			if height != tt.expectedHeight {
				t.Errorf("GetResponsiveHeight(%d) = %d, expected %d", tt.termHeight, height, tt.expectedHeight)
			}

			showDetail := overlay.ShouldShowDetailedContent(tt.termHeight)
			if showDetail != tt.shouldDetail {
				t.Errorf("ShouldShowDetailedContent(%d) = %v, expected %v", tt.termHeight, showDetail, tt.shouldDetail)
			}

			showHelp := overlay.ShouldShowHelpText(tt.termHeight)
			if showHelp != tt.shouldHelp {
				t.Errorf("ShouldShowHelpText(%d) = %v, expected %v", tt.termHeight, showHelp, tt.shouldHelp)
			}
		})
	}
}

// TestContentHeightCalculation tests available content height calculations
func TestContentHeightCalculation(t *testing.T) {
	tests := []struct {
		name            string
		overlayHeight   int
		hasBorder       bool
		hasHeader       bool
		hasFooter       bool
		expectedContent int
	}{
		{
			name:            "full_decoration",
			overlayHeight:   20,
			hasBorder:       true,
			hasHeader:       true,
			hasFooter:       true,
			expectedContent: 12, // 20 - 2(border) - 2(padding) - 2(header) - 2(footer) = 12
		},
		{
			name:            "minimal_decoration",
			overlayHeight:   20,
			hasBorder:       false,
			hasHeader:       false,
			hasFooter:       false,
			expectedContent: 18, // 20 - 2(padding) = 18
		},
		{
			name:            "border_only",
			overlayHeight:   20,
			hasBorder:       true,
			hasHeader:       false,
			hasFooter:       false,
			expectedContent: 16, // 20 - 2(border) - 2(padding) = 16
		},
		{
			name:            "very_short_overlay",
			overlayHeight:   8,
			hasBorder:       true,
			hasHeader:       true,
			hasFooter:       true,
			expectedContent: 3, // Clamps to minimum 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := overlay.GetContentHeight(tt.overlayHeight, tt.hasBorder, tt.hasHeader, tt.hasFooter)
			if content != tt.expectedContent {
				t.Errorf("GetContentHeight(%d, %v, %v, %v) = %d, expected %d",
					tt.overlayHeight, tt.hasBorder, tt.hasHeader, tt.hasFooter, content, tt.expectedContent)
			}
		})
	}
}

// TestMaxVisibleItems tests visible item calculations
func TestMaxVisibleItems(t *testing.T) {
	tests := []struct {
		name         string
		termHeight   int
		itemHeight   int
		expectedMax  int
	}{
		{"short_terminal_single_line", 20, 1, 12},  // Content height ~12
		{"short_terminal_multi_line", 20, 3, 4},    // 12 / 3 = 4
		{"medium_terminal_single_line", 30, 1, 22}, // Content height ~22
		{"tall_terminal_single_line", 50, 1, 42},   // Content height ~42
		{"very_small_terminal", 10, 1, 3},          // Minimum content height
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxItems := overlay.GetMaxVisibleItems(tt.termHeight, tt.itemHeight)
			if maxItems != tt.expectedMax {
				t.Errorf("GetMaxVisibleItems(%d, %d) = %d, expected %d",
					tt.termHeight, tt.itemHeight, maxItems, tt.expectedMax)
			}
		})
	}
}

// TestSessionSetupHeightResponsiveness tests SessionSetupOverlay at different heights
func TestSessionSetupHeightResponsiveness(t *testing.T) {
	heightVariations := []struct {
		name   string
		height int
	}{
		{"very_short_15_lines", 15},
		{"short_20_lines", 20},
		{"medium_30_lines", 30},
		{"tall_50_lines", 50},
	}

	for _, hv := range heightVariations {
		t.Run(hv.name, func(t *testing.T) {
			renderer := NewTestRenderer().
				SetSnapshotPath("snapshots/responsive_height/session_setup").
				SetDimensions(80, hv.height).
				DisableColors()

			setupOverlay := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
				OnComplete: func(session.InstanceOptions) {},
				OnCancel:   func() {},
			})
			setupOverlay.SetSize(80, hv.height)

			renderer.CompareComponentWithSnapshot(t, setupOverlay, hv.name+"_basics.txt")
		})
	}
}

// TestMessagesOverlayHeightResponsiveness tests MessagesOverlay with varying heights
func TestMessagesOverlayHeightResponsiveness(t *testing.T) {
	// Create sample messages
	messages := []overlay.StatusMessage{
		{Level: "INFO", Message: "System started successfully"},
		{Level: "WARN", Message: "Configuration file not found, using defaults"},
		{Level: "ERROR", Message: "Failed to connect to database"},
		{Level: "INFO", Message: "Retrying connection..."},
		{Level: "INFO", Message: "Connected to database successfully"},
		{Level: "WARN", Message: "Memory usage is high"},
		{Level: "ERROR", Message: "Request timeout after 30 seconds"},
		{Level: "INFO", Message: "Request completed successfully"},
	}

	heightVariations := []struct {
		name   string
		height int
	}{
		{"compact_15_lines", 15},
		{"short_20_lines", 20},
		{"medium_30_lines", 30},
		{"tall_50_lines", 50},
	}

	for _, hv := range heightVariations {
		t.Run(hv.name, func(t *testing.T) {
			renderer := NewTestRenderer().
				SetSnapshotPath("snapshots/responsive_height/messages").
				SetDimensions(80, hv.height).
				DisableColors()

			messagesOverlay := overlay.NewMessagesOverlay(messages)
			messagesOverlay.SetDimensions(80, hv.height)

			renderer.CompareComponentWithSnapshot(t, messagesOverlay, hv.name+".txt")
		})
	}
}

// TestConfirmationOverlayHeightAdaptation tests confirmation overlay at different heights
func TestConfirmationOverlayHeightAdaptation(t *testing.T) {
	heightVariations := []struct {
		name   string
		height int
	}{
		{"minimal_10_lines", 10},
		{"short_15_lines", 15},
		{"comfortable_25_lines", 25},
	}

	for _, hv := range heightVariations {
		t.Run(hv.name, func(t *testing.T) {
			renderer := NewTestRenderer().
				SetSnapshotPath("snapshots/responsive_height/confirmation").
				SetDimensions(80, hv.height).
				DisableColors()

			confirmOverlay := overlay.NewConfirmationOverlay("Are you sure you want to proceed with this operation?")
			confirmOverlay.SetSize(80, hv.height)

			renderer.CompareComponentWithSnapshot(t, confirmOverlay, hv.name+".txt")
		})
	}
}

// TestBaseOverlayHeightMethods tests BaseOverlay height-aware methods
func TestBaseOverlayHeightMethods(t *testing.T) {
	base := &overlay.BaseOverlay{}

	tests := []struct {
		height          int
		expectedResp    int
		expectedDetail  bool
		expectedHelp    bool
		expectedContent int // with full decoration (uses raw height, not responsive)
	}{
		{15, 10, false, false, 7},   // Very short: 15 - 2(border) - 2(padding) - 2(header) - 2(footer) = 7
		{20, 14, false, true, 12},   // Short: 20 - 8 = 12
		{30, 21, true, true, 22},    // Medium: 30 - 8 = 22
		{50, 35, true, true, 42},    // Tall: 50 - 8 = 42
	}

	for _, tt := range tests {
		t.Run("height_"+string(rune(tt.height)), func(t *testing.T) {
			base.SetSize(80, tt.height)

			respHeight := base.GetResponsiveHeight()
			if respHeight != tt.expectedResp {
				t.Errorf("GetResponsiveHeight() = %d, expected %d", respHeight, tt.expectedResp)
			}

			showDetail := base.ShouldShowDetailedContent()
			if showDetail != tt.expectedDetail {
				t.Errorf("ShouldShowDetailedContent() = %v, expected %v", showDetail, tt.expectedDetail)
			}

			showHelp := base.ShouldShowHelpText()
			if showHelp != tt.expectedHelp {
				t.Errorf("ShouldShowHelpText() = %v, expected %v", showHelp, tt.expectedHelp)
			}

			contentHeight := base.GetContentHeight(true, true, true)
			if contentHeight != tt.expectedContent {
				t.Errorf("GetContentHeight() = %d, expected %d", contentHeight, tt.expectedContent)
			}
		})
	}
}
