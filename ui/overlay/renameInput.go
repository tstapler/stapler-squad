package overlay

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RenameInputOverlay represents a rename input overlay for renaming sessions
type RenameInputOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	textInput    textinput.Model
	oldTitle     string
	Title        string
	validator    func(string) error
	Submitted    bool
	Canceled     bool
	errorMessage string
	OnSubmit     func(newTitle string)
}

// NewRenameInputOverlay creates a new rename input overlay with the given old title and validator
func NewRenameInputOverlay(oldTitle string, validator func(string) error) *RenameInputOverlay {
	ti := textinput.New()
	ti.SetValue(oldTitle)
	ti.Focus()
	ti.CharLimit = 32
	ti.Placeholder = "Enter new session name..."
	ti.Width = 50

	overlay := &RenameInputOverlay{
		textInput: ti,
		oldTitle:  oldTitle,
		Title:     "Rename Session",
		validator: validator,
		Submitted: false,
		Canceled:  false,
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(60, 10)
	overlay.BaseOverlay.Focus()

	return overlay
}

// SetSize sets the size of the overlay
func (r *RenameInputOverlay) SetSize(width, height int) {
	// Update BaseOverlay size
	r.BaseOverlay.SetSize(width, height)

	// Update text input width
	r.textInput.Width = width - 10
}

// Init initializes the rename input overlay model
func (r *RenameInputOverlay) Init() tea.Cmd {
	return textinput.Blink
}

// View renders the model's view
func (r *RenameInputOverlay) View() string {
	return r.Render()
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (r *RenameInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Use BaseOverlay for common keys (Esc)
	if handled, shouldClose := r.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			r.Canceled = true
			return true
		}
	}

	switch msg.Type {
	case tea.KeyEnter:
		// Validate and submit the new name
		newTitle := r.textInput.Value()

		// Skip if the name hasn't changed
		if newTitle == r.oldTitle {
			r.Canceled = true
			return true
		}

		// Validate the new name if validator is provided
		if r.validator != nil {
			if err := r.validator(newTitle); err != nil {
				r.errorMessage = err.Error()
				return false
			}
		}

		// Check for empty title
		if len(newTitle) == 0 {
			r.errorMessage = "Session name cannot be empty"
			return false
		}

		// Check for max length
		if len(newTitle) > 32 {
			r.errorMessage = "Session name cannot be longer than 32 characters"
			return false
		}

		// Submit the new title
		r.Submitted = true
		if r.OnSubmit != nil {
			r.OnSubmit(newTitle)
		}
		return true

	case tea.KeyEsc:
		r.Canceled = true
		return true

	default:
		// Update the text input
		var cmd tea.Cmd
		r.textInput, cmd = r.textInput.Update(msg)
		// Clear error message when typing
		r.errorMessage = ""
		_ = cmd // Ignore the command for simplicity
	}

	return false
}

// Render returns the string representation of the overlay
func (r *RenameInputOverlay) Render() string {
	// Use responsive sizing from BaseOverlay
	responsiveWidth := r.GetResponsiveWidth()
	hPadding, vPadding := r.GetResponsivePadding()

	// Create box style with responsive sizing
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(vPadding, hPadding).
		MaxWidth(responsiveWidth)

	// Define styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginBottom(1)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		MarginTop(1)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)

	// Build the content
	content := titleStyle.Render(r.Title) + "\n\n"

	// Show the old title
	content += promptStyle.Render(fmt.Sprintf("Current: %s", r.oldTitle)) + "\n\n"

	// Show the text input
	content += r.textInput.View() + "\n"

	// Show error message if any
	if r.errorMessage != "" {
		content += errorStyle.Render("⚠ " + r.errorMessage) + "\n"
	}

	// Show instructions
	content += instructionStyle.Render("Press Enter to save, Esc to cancel")

	// Render with box style
	return boxStyle.Render(content)
}

// GetValue returns the current value of the text input
func (r *RenameInputOverlay) GetValue() string {
	return r.textInput.Value()
}

// SetValue sets the value of the text input
func (r *RenameInputOverlay) SetValue(value string) {
	r.textInput.SetValue(value)
}

// IsSubmitted returns true if the form was submitted
func (r *RenameInputOverlay) IsSubmitted() bool {
	return r.Submitted
}

// IsCanceled returns true if the form was canceled
func (r *RenameInputOverlay) IsCanceled() bool {
	return r.Canceled
}

// SetOnSubmit sets the callback to be called when the form is submitted
func (r *RenameInputOverlay) SetOnSubmit(fn func(newTitle string)) {
	r.OnSubmit = fn
}