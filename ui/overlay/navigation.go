package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
)

// NavigationHandler provides reusable navigation logic for overlays with multiple options.
// Handles up/down arrow keys and optional vim-style j/k keys.
type NavigationHandler struct {
	currentIndex int
	itemCount    int
	allowVimKeys bool
	wrapAround   bool // Whether to wrap from last to first item
}

// NewNavigationHandler creates a new navigation handler.
func NewNavigationHandler(itemCount int, allowVim bool) *NavigationHandler {
	return &NavigationHandler{
		currentIndex: 0,
		itemCount:    itemCount,
		allowVimKeys: allowVim,
		wrapAround:   true, // Default to wrapping
	}
}

// SetItemCount updates the number of items to navigate through.
func (n *NavigationHandler) SetItemCount(count int) {
	n.itemCount = count
	// Clamp current index if it's now out of bounds
	if n.currentIndex >= count {
		n.currentIndex = count - 1
	}
	if n.currentIndex < 0 && count > 0 {
		n.currentIndex = 0
	}
}

// GetCurrentIndex returns the currently selected index.
func (n *NavigationHandler) GetCurrentIndex() int {
	return n.currentIndex
}

// SetCurrentIndex sets the currently selected index.
func (n *NavigationHandler) SetCurrentIndex(index int) {
	if index >= 0 && index < n.itemCount {
		n.currentIndex = index
	}
}

// SetWrapAround controls whether navigation wraps from last to first item.
func (n *NavigationHandler) SetWrapAround(wrap bool) {
	n.wrapAround = wrap
}

// HandleNavigation processes navigation keys (up/down arrows and optionally j/k).
// Returns true if the key was handled and the index changed.
func (n *NavigationHandler) HandleNavigation(msg tea.KeyMsg) (changed bool) {
	if n.itemCount <= 0 {
		return false
	}

	var delta int
	switch msg.Type {
	case tea.KeyUp:
		delta = -1
	case tea.KeyDown:
		delta = 1
	case tea.KeyRunes:
		if !n.allowVimKeys || len(msg.Runes) != 1 {
			return false
		}
		switch string(msg.Runes) {
		case "k":
			delta = -1
		case "j":
			delta = 1
		default:
			return false
		}
	default:
		return false
	}

	if delta != 0 {
		newIndex := n.currentIndex + delta
		if n.wrapAround {
			// Wrap around using modulo
			newIndex = (newIndex + n.itemCount) % n.itemCount
		} else {
			// Clamp to bounds without wrapping
			if newIndex < 0 {
				newIndex = 0
			} else if newIndex >= n.itemCount {
				newIndex = n.itemCount - 1
			}
		}

		if newIndex != n.currentIndex {
			n.currentIndex = newIndex
			return true
		}
	}
	return false
}

// HandleTabNavigation processes Tab and Shift+Tab for cycling through options.
// Returns true if the key was handled.
func (n *NavigationHandler) HandleTabNavigation(msg tea.KeyMsg) (changed bool) {
	if n.itemCount <= 0 {
		return false
	}

	switch msg.Type {
	case tea.KeyTab:
		// Forward cycle
		n.currentIndex = (n.currentIndex + 1) % n.itemCount
		return true
	case tea.KeyShiftTab:
		// Backward cycle
		n.currentIndex = (n.currentIndex - 1 + n.itemCount) % n.itemCount
		return true
	}
	return false
}
