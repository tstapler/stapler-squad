package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
)

// BaseOverlay provides common functionality for all overlay components.
// Overlays should embed this struct to inherit shared behavior like:
// - Escape key handling
// - Focus management
// - Responsive sizing
// - Cancel callbacks
type BaseOverlay struct {
	width, height int
	focused       bool
	onCancel      func()
}

// SetSize sets the dimensions for the overlay.
// This is called by the parent component when terminal size changes.
func (b *BaseOverlay) SetSize(width, height int) {
	b.width = width
	b.height = height
}

// GetWidth returns the current width of the overlay.
func (b *BaseOverlay) GetWidth() int {
	return b.width
}

// GetHeight returns the current height of the overlay.
func (b *BaseOverlay) GetHeight() int {
	return b.height
}

// Focus gives focus to the overlay.
func (b *BaseOverlay) Focus() {
	b.focused = true
}

// Blur removes focus from the overlay.
func (b *BaseOverlay) Blur() {
	b.focused = false
}

// IsFocused returns whether the overlay is focused.
func (b *BaseOverlay) IsFocused() bool {
	return b.focused
}

// SetOnCancel sets the callback function to be called when the overlay is cancelled (Esc pressed).
func (b *BaseOverlay) SetOnCancel(callback func()) {
	b.onCancel = callback
}

// HandleCommonKeys handles keyboard input common to all overlays.
// Returns (handled, shouldClose) where:
// - handled: true if this handler processed the key
// - shouldClose: true if the overlay should be closed
func (b *BaseOverlay) HandleCommonKeys(msg tea.KeyMsg) (handled bool, shouldClose bool) {
	switch msg.Type {
	case tea.KeyEsc:
		if b.onCancel != nil {
			b.onCancel()
		}
		return true, true
	}
	return false, false
}

// GetResponsiveWidth calculates an appropriate width for overlay content
// based on terminal width. Returns a width that is:
// - 80% of terminal width
// - Maximum 100 columns
// - Minimum 40 columns
func (b *BaseOverlay) GetResponsiveWidth() int {
	width := int(float64(b.width) * 0.8)
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
func (b *BaseOverlay) GetResponsivePadding() (horizontal, vertical int) {
	if b.width < 60 {
		return 1, 0 // Minimal padding for narrow terminals
	} else if b.width < 100 {
		return 2, 1 // Moderate padding for medium terminals
	}
	return 3, 1 // Comfortable padding for wide terminals
}

// GetResponsiveHeight calculates an appropriate height for overlay content
// based on terminal height. Returns a height that is:
// - 70% of terminal height (to leave room for context)
// - Maximum 40 lines
// - Minimum 10 lines
func (b *BaseOverlay) GetResponsiveHeight() int {
	height := int(float64(b.height) * 0.7)
	if height > 40 {
		height = 40
	}
	if height < 10 {
		height = 10
	}
	return height
}

// GetContentHeight calculates the available height for content inside the overlay,
// accounting for borders, padding, headers, and footers.
func (b *BaseOverlay) GetContentHeight(hasBorder bool, hasHeader bool, hasFooter bool) int {
	return GetContentHeight(b.height, hasBorder, hasHeader, hasFooter)
}

// ShouldShowDetailedContent determines if we have enough vertical space for detailed content.
// Short terminals should show compact versions.
func (b *BaseOverlay) ShouldShowDetailedContent() bool {
	return b.height >= 30
}

// ShouldShowHelpText determines if we have enough vertical space for help text.
func (b *BaseOverlay) ShouldShowHelpText() bool {
	return b.height >= 20
}

// GetMaxVisibleItems calculates how many items can be shown in a list.
func (b *BaseOverlay) GetMaxVisibleItems(itemHeight int) int {
	return GetMaxVisibleItems(b.height, itemHeight)
}
