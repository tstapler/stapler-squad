package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmationOverlay represents a confirmation dialog overlay
type ConfirmationOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	// Whether the overlay has been dismissed
	Dismissed bool
	// Message to display in the overlay
	message string
	// Callback function to be called when the user confirms (presses 'y')
	OnConfirm func()
	// Custom confirm key (defaults to 'y')
	ConfirmKey string
	// Custom cancel key (defaults to 'n')
	CancelKey string
	// Custom styling options
	borderColor lipgloss.Color
}

// NewConfirmationOverlay creates a new confirmation dialog overlay with the given message
func NewConfirmationOverlay(message string) *ConfirmationOverlay {
	overlay := &ConfirmationOverlay{
		Dismissed:   false,
		message:     message,
		ConfirmKey:  "y",
		CancelKey:   "n",
		borderColor: lipgloss.Color("#de613e"), // Red color for confirmations
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(50, 10)
	overlay.BaseOverlay.Focus()

	return overlay
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (c *ConfirmationOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Use BaseOverlay for Esc key handling
	if handled, shouldClose := c.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			c.Dismissed = true
			return true
		}
	}

	switch msg.String() {
	case c.ConfirmKey:
		c.Dismissed = true
		if c.OnConfirm != nil {
			c.OnConfirm()
		}
		return true
	case c.CancelKey:
		c.Dismissed = true
		if c.onCancel != nil {
			c.onCancel()
		}
		return true
	default:
		// Ignore other keys in confirmation state
		return false
	}
}

// Render renders the confirmation overlay
func (c *ConfirmationOverlay) Render(opts ...WhitespaceOption) string {
	// Use responsive width from BaseOverlay
	responsiveWidth := c.GetResponsiveWidth()
	hPadding, vPadding := c.GetResponsivePadding()

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.borderColor).
		Padding(vPadding, hPadding).
		MaxWidth(responsiveWidth)

	// Add the confirmation instructions
	content := c.message + "\n\n" +
		"Press " + lipgloss.NewStyle().Bold(true).Render(c.ConfirmKey) + " to confirm, " +
		lipgloss.NewStyle().Bold(true).Render(c.CancelKey) + " or " +
		lipgloss.NewStyle().Bold(true).Render("esc") + " to cancel"

	// Apply the border style and return
	return style.Render(content)
}

// View satisfies the tea.Model interface and renders the confirmation overlay
// This is needed for the TestRenderer to render the component
func (c *ConfirmationOverlay) View() string {
	return c.Render()
}

// SetBorderColor sets the border color of the confirmation overlay
func (c *ConfirmationOverlay) SetBorderColor(color lipgloss.Color) {
	c.borderColor = color
}

// SetConfirmKey sets the key used to confirm the action
func (c *ConfirmationOverlay) SetConfirmKey(key string) {
	c.ConfirmKey = key
}

// SetCancelKey sets the key used to cancel the action
func (c *ConfirmationOverlay) SetCancelKey(key string) {
	c.CancelKey = key
}
