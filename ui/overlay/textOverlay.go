package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextOverlay represents a text screen overlay
type TextOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed
	OnDismiss func()
	// Content to display in the overlay
	content string
}

// NewTextOverlay creates a new text screen overlay with the given title and content
func NewTextOverlay(content string) *TextOverlay {
	overlay := &TextOverlay{
		Dismissed: false,
		content:   content,
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(60, 20)
	overlay.BaseOverlay.Focus()

	return overlay
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (t *TextOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Use BaseOverlay for Esc key handling
	if handled, shouldClose := t.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			t.Dismissed = true
			if t.OnDismiss != nil {
				t.OnDismiss()
			}
			return true
		}
	}

	// Close on any other key too
	t.Dismissed = true
	if t.OnDismiss != nil {
		t.OnDismiss()
	}
	return true
}

// Render renders the text overlay
func (t *TextOverlay) Render(opts ...WhitespaceOption) string {
	// Use responsive sizing from BaseOverlay
	responsiveWidth := t.GetResponsiveWidth()
	hPadding, vPadding := t.GetResponsivePadding()

	// Create responsive styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(vPadding, hPadding).
		MaxWidth(responsiveWidth)

	// Adapt content for terminal width
	adaptedContent := AdaptTextForWidth(t.content, t.GetWidth())

	// Apply the border style and return
	return style.Render(adaptedContent)
}

// View satisfies the tea.Model interface and renders the text overlay
// This is needed for the TestRenderer to render the component
func (t *TextOverlay) View() string {
	return t.Render()
}
