package overlay

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// GetResponsiveWidth calculates an appropriate width for content based on terminal width.
// This is a standalone helper that can be used without embedding BaseOverlay.
func GetResponsiveWidth(termWidth int) int {
	width := int(float64(termWidth) * 0.8)
	if width > 100 {
		width = 100
	}
	if width < 40 {
		width = 40
	}
	return width
}

// GetResponsivePadding returns appropriate horizontal and vertical padding
// based on terminal size. Smaller terminals get less padding.
// Takes both width and height into account.
func GetResponsivePadding(termWidth int) (horizontal, vertical int) {
	if termWidth < 60 {
		return 1, 0 // Minimal padding for narrow terminals
	} else if termWidth < 100 {
		return 2, 1 // Moderate padding for medium terminals
	}
	return 3, 1 // Comfortable padding for wide terminals
}

// GetResponsiveHeight calculates an appropriate height for content based on terminal height.
// This is a standalone helper that can be used without embedding BaseOverlay.
func GetResponsiveHeight(termHeight int) int {
	// Use 70% of terminal height for overlays to leave room for context
	height := int(float64(termHeight) * 0.7)

	// Clamp to reasonable bounds
	if height > 40 {
		height = 40 // Maximum height for large terminals
	}
	if height < 10 {
		height = 10 // Minimum height to show meaningful content
	}
	return height
}

// GetContentHeight calculates the available height for content inside an overlay,
// accounting for borders, padding, headers, and footers.
func GetContentHeight(overlayHeight int, hasBorder bool, hasHeader bool, hasFooter bool) int {
	available := overlayHeight

	// Account for borders (top and bottom)
	if hasBorder {
		available -= 2
	}

	// Account for padding (top and bottom)
	available -= 2

	// Account for header (title + separator)
	if hasHeader {
		available -= 2
	}

	// Account for footer (help text + separator)
	if hasFooter {
		available -= 2
	}

	// Ensure minimum content height
	if available < 3 {
		available = 3
	}

	return available
}

// ShouldShowDetailedContent determines if we have enough vertical space to show detailed content
// Short terminals should show compact versions of content.
func ShouldShowDetailedContent(termHeight int) bool {
	return termHeight >= 30 // 30+ lines = enough space for detailed content
}

// ShouldShowHelpText determines if we have enough vertical space to show help text
func ShouldShowHelpText(termHeight int) bool {
	return termHeight >= 20 // 20+ lines = enough space for help text
}

// GetMaxVisibleItems calculates how many items can be shown in a list given the available height
func GetMaxVisibleItems(termHeight int, itemHeight int) int {
	contentHeight := GetContentHeight(termHeight, true, true, true)
	maxItems := contentHeight / itemHeight
	if maxItems < 1 {
		maxItems = 1 // Always show at least one item
	}
	return maxItems
}

// TruncateForWidth truncates text to fit within a maximum width, adding "..." if truncated.
func TruncateForWidth(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxWidth {
		return text
	}
	return truncate.StringWithTail(text, uint(maxWidth), "...")
}

// CreateResponsiveStyle creates a lipgloss style that adapts to terminal width.
// The style uses MaxWidth to ensure content doesn't overflow on narrow terminals.
func CreateResponsiveStyle(termWidth int, border lipgloss.Border, borderColor lipgloss.Color) lipgloss.Style {
	contentWidth := GetResponsiveWidth(termWidth)
	hPadding, vPadding := GetResponsivePadding(termWidth)

	return lipgloss.NewStyle().
		Border(border).
		BorderForeground(borderColor).
		MaxWidth(contentWidth).
		Padding(vPadding, hPadding)
}

// AdaptTextForWidth adapts text content for display at a given width.
// - Truncates long lines
// - Shortens descriptions on narrow terminals
// - Preserves formatting where possible
func AdaptTextForWidth(text string, termWidth int) string {
	if termWidth >= 100 {
		return text // Wide terminals: show everything
	}

	lines := strings.Split(text, "\n")
	adaptedLines := make([]string, len(lines))

	maxLineWidth := GetResponsiveWidth(termWidth)
	for i, line := range lines {
		if utf8.RuneCountInString(line) > maxLineWidth {
			adaptedLines[i] = TruncateForWidth(line, maxLineWidth)
		} else {
			adaptedLines[i] = line
		}
	}

	return strings.Join(adaptedLines, "\n")
}

// ShortenDescriptionForWidth shortens description text for narrow terminals.
// Uses smart truncation to preserve meaning where possible.
func ShortenDescriptionForWidth(desc string, termWidth int) string {
	if termWidth >= 80 {
		return desc // Wide enough for full description
	}

	// For narrow terminals, truncate aggressively
	maxDescWidth := GetResponsiveWidth(termWidth) - 10 // Leave room for icons/borders
	if maxDescWidth < 20 {
		maxDescWidth = 20 // Minimum readable width
	}

	return TruncateForWidth(desc, maxDescWidth)
}
