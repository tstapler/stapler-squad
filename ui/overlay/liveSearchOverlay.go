package overlay

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LiveSearchOverlay represents a text input overlay that triggers search on every keystroke
type LiveSearchOverlay struct {
	input         textinput.Model
	Title         string
	FocusIndex    int
	Submitted     bool
	Canceled      bool
	OnSearchLive  func(string) // Called on every keystroke
	OnSubmit      func(string) // Called on Enter
	OnCancel      func()       // Called on Esc
	width, height int
}

// NewLiveSearchOverlay creates a new live search overlay
func NewLiveSearchOverlay(title string, initialValue string) *LiveSearchOverlay {
	ti := textinput.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 0

	return &LiveSearchOverlay{
		input:      ti,
		Title:      title,
		FocusIndex: 0,
		Submitted:  false,
		Canceled:   false,
	}
}

func (l *LiveSearchOverlay) SetSize(width, height int) {
	l.input.Width = width - 4 // Account for padding
	l.width = width
	l.height = height
}

// Init initializes the overlay
func (l *LiveSearchOverlay) Init() tea.Cmd {
	return textinput.Blink
}

// HandleKeyPress processes key press events
func (l *LiveSearchOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		l.Canceled = true
		if l.OnCancel != nil {
			l.OnCancel()
		}
		return true
	case tea.KeyEnter:
		l.Submitted = true
		if l.OnSubmit != nil {
			l.OnSubmit(l.input.Value())
		}
		return true
	default:
		// Update the input first
		l.input, _ = l.input.Update(msg)

		// Then trigger live search
		if l.OnSearchLive != nil {
			l.OnSearchLive(l.input.Value())
		}
		return false
	}
}

// GetValue returns the current input value
func (l *LiveSearchOverlay) GetValue() string {
	return l.input.Value()
}

// IsSubmitted returns whether the overlay was submitted
func (l *LiveSearchOverlay) IsSubmitted() bool {
	return l.Submitted
}

// IsCanceled returns whether the overlay was canceled
func (l *LiveSearchOverlay) IsCanceled() bool {
	return l.Canceled
}

// SetOnSearchLive sets the live search callback
func (l *LiveSearchOverlay) SetOnSearchLive(onSearchLive func(string)) {
	l.OnSearchLive = onSearchLive
}

// SetOnSubmit sets the submit callback
func (l *LiveSearchOverlay) SetOnSubmit(onSubmit func(string)) {
	l.OnSubmit = onSubmit
}

// SetOnCancel sets the cancel callback
func (l *LiveSearchOverlay) SetOnCancel(onCancel func()) {
	l.OnCancel = onCancel
}

// View renders the overlay
func (l *LiveSearchOverlay) View() string {
	var content string

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		MarginBottom(1)
	content += titleStyle.Render(l.Title) + "\n"

	// Input field
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		BorderForeground(lipgloss.Color("62"))
	content += inputStyle.Render(l.input.View()) + "\n"

	// Instructions
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)
	content += instructionStyle.Render("Type to search • Enter to confirm • Esc to cancel")

	// Center the content
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		BorderForeground(lipgloss.Color("62")).
		Background(lipgloss.Color("235")).
		Width(l.width - 4).
		Align(lipgloss.Center)

	return lipgloss.Place(l.width, l.height, lipgloss.Center, lipgloss.Center, overlayStyle.Render(content))
}