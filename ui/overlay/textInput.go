package overlay

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextInputOverlay represents a text input overlay with state management.
type TextInputOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	textarea   textarea.Model
	Title      string
	FocusIndex int // 0 for text input, 1 for enter button
	Submitted  bool
	Canceled   bool
	OnSubmit   func()
}

// NewTextInputOverlay creates a new text input overlay with the given title and initial value.
func NewTextInputOverlay(title string, initialValue string) *TextInputOverlay {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

	// Ensure no character limit
	ti.CharLimit = 0
	// Ensure no maximum height limit
	ti.MaxHeight = 0

	overlay := &TextInputOverlay{
		textarea:   ti,
		Title:      title,
		FocusIndex: 0,
		Submitted:  false,
		Canceled:   false,
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(60, 15)
	overlay.BaseOverlay.Focus()

	return overlay
}

func (t *TextInputOverlay) SetSize(width, height int) {
	// Update BaseOverlay size
	t.BaseOverlay.SetSize(width, height)

	// Update textarea dimensions
	t.textarea.SetHeight(height)
}

// Init initializes the text input overlay model
func (t *TextInputOverlay) Init() tea.Cmd {
	return textarea.Blink
}

// View renders the model's view
func (t *TextInputOverlay) View() string {
	return t.Render()
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (t *TextInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Use BaseOverlay for common keys (Esc)
	if handled, shouldClose := t.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			t.Canceled = true
			return true
		}
	}

	switch msg.Type {
	case tea.KeyTab:
		// Toggle focus between input and enter button.
		t.FocusIndex = (t.FocusIndex + 1) % 2
		if t.FocusIndex == 0 {
			t.textarea.Focus()
		} else {
			t.textarea.Blur()
		}
		return false
	case tea.KeyShiftTab:
		// Toggle focus in reverse.
		t.FocusIndex = (t.FocusIndex + 1) % 2
		if t.FocusIndex == 0 {
			t.textarea.Focus()
		} else {
			t.textarea.Blur()
		}
		return false
	case tea.KeyEnter:
		if t.FocusIndex == 1 || msg.Type == tea.KeyEnter {
			// Enter button is focused or Enter key is pressed, so submit.
			t.Submitted = true
			if t.OnSubmit != nil {
				t.OnSubmit()
			}
			return true
		}
		fallthrough // Send enter key to textarea
	default:
		if t.FocusIndex == 0 {
			t.textarea, _ = t.textarea.Update(msg)
		}
		return false
	}
}

// GetValue returns the current value of the text input.
func (t *TextInputOverlay) GetValue() string {
	return t.textarea.Value()
}

// IsSubmitted returns whether the form was submitted.
func (t *TextInputOverlay) IsSubmitted() bool {
	return t.Submitted
}

// IsCanceled returns whether the form was canceled.
func (t *TextInputOverlay) IsCanceled() bool {
	return t.Canceled
}

// SetOnSubmit sets a callback function for form submission.
func (t *TextInputOverlay) SetOnSubmit(onSubmit func()) {
	t.OnSubmit = onSubmit
}

// SetOnSubmit sets a callback function that receives the input value.
func (t *TextInputOverlay) SetOnSubmitWithValue(onSubmit func(string)) {
	t.OnSubmit = func() {
		if onSubmit != nil {
			onSubmit(t.GetValue())
		}
	}
}

// Render renders the text input overlay.
func (t *TextInputOverlay) Render() string {
	// Use responsive sizing from BaseOverlay
	responsiveWidth := t.GetResponsiveWidth()
	hPadding, vPadding := t.GetResponsivePadding()

	// Create styles with responsive sizing
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(vPadding, hPadding).
		MaxWidth(responsiveWidth)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)

	buttonStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	focusedButtonStyle := buttonStyle
	focusedButtonStyle = focusedButtonStyle.
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0"))

	// Set textarea width to fit within the responsive overlay
	// Account for padding and borders
	textareaWidth := responsiveWidth - (hPadding * 2) - 4
	if textareaWidth < 20 {
		textareaWidth = 20 // Minimum readable width
	}
	t.textarea.SetWidth(textareaWidth)

	// Build the view
	content := titleStyle.Render(t.Title) + "\n"
	content += t.textarea.View() + "\n\n"

	// Render enter button with appropriate style
	enterButton := " Enter "
	if t.FocusIndex == 1 {
		enterButton = focusedButtonStyle.Render(enterButton)
	} else {
		enterButton = buttonStyle.Render(enterButton)
	}
	content += enterButton

	return style.Render(content)
}
