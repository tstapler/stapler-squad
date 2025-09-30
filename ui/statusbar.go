package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusMessage represents a single status/error message
type StatusMessage struct {
	Timestamp time.Time
	Level     string // "INFO", "WARN", "ERROR"
	Message   string
}

// StatusBar implements a vim-style status bar with message history
type StatusBar struct {
	// Current status line message
	currentMessage string
	currentLevel   string

	// Message history for :messages command
	messageHistory []StatusMessage
	maxMessages    int

	// Command mode for :messages
	commandMode bool
	commandInput textinput.Model

	// Styles
	infoStyle    lipgloss.Style
	warnStyle    lipgloss.Style
	errorStyle   lipgloss.Style
	commandStyle lipgloss.Style
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	input := textinput.New()
	input.Prompt = ":"
	input.CharLimit = 50

	return &StatusBar{
		messageHistory: make([]StatusMessage, 0),
		maxMessages:    100, // Keep last 100 messages
		commandInput:   input,
		infoStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		warnStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		errorStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		commandStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
	}
}

// AddMessage adds a new message to the status bar and history
func (s *StatusBar) AddMessage(level, message string) {
	s.currentMessage = message
	s.currentLevel = level

	// Add to history
	msg := StatusMessage{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	s.messageHistory = append(s.messageHistory, msg)

	// Trim history if too long
	if len(s.messageHistory) > s.maxMessages {
		s.messageHistory = s.messageHistory[len(s.messageHistory)-s.maxMessages:]
	}
}

// SetInfo sets an info message
func (s *StatusBar) SetInfo(message string) {
	s.AddMessage("INFO", message)
}

// SetWarning sets a warning message
func (s *StatusBar) SetWarning(message string) {
	s.AddMessage("WARN", message)
}

// SetError sets an error message
func (s *StatusBar) SetError(message string) {
	s.AddMessage("ERROR", message)
}

// Clear clears the current status message
func (s *StatusBar) Clear() {
	s.currentMessage = ""
	s.currentLevel = ""
}

// EnterCommandMode enters vim-style command mode
func (s *StatusBar) EnterCommandMode() {
	s.commandMode = true
	s.commandInput.Focus()
	s.commandInput.SetValue("")
}

// ExitCommandMode exits command mode
func (s *StatusBar) ExitCommandMode() {
	s.commandMode = false
	s.commandInput.Blur()
}

// IsInCommandMode returns true if in command mode
func (s *StatusBar) IsInCommandMode() bool {
	return s.commandMode
}

// GetCommand returns the current command input
func (s *StatusBar) GetCommand() string {
	return s.commandInput.Value()
}

// HandleCommandInput handles input in command mode
func (s *StatusBar) HandleCommandInput(msg interface{}) {
	if s.commandMode {
		var cmd tea.Cmd
		s.commandInput, cmd = s.commandInput.Update(msg)
		_ = cmd // Ignore command for now
	}
}

// Update handles textinput updates
func (s *StatusBar) Update() {
	if s.commandMode {
		s.commandInput, _ = s.commandInput.Update(nil)
	}
}

// GetMessagesView returns the :messages view (deprecated - use GetMessageHistory instead)
func (s *StatusBar) GetMessagesView(width, height int) string {
	if len(s.messageHistory) == 0 {
		return "No messages"
	}

	var lines []string

	// Show recent messages (fit within height)
	start := 0
	if len(s.messageHistory) > height-2 {
		start = len(s.messageHistory) - (height - 2)
	}

	for i := start; i < len(s.messageHistory); i++ {
		msg := s.messageHistory[i]
		timestamp := msg.Timestamp.Format("15:04:05")

		var style lipgloss.Style
		switch msg.Level {
		case "ERROR":
			style = s.errorStyle
		case "WARN":
			style = s.warnStyle
		default:
			style = s.infoStyle
		}

		line := fmt.Sprintf("[%s] %s: %s", timestamp, msg.Level, msg.Message)

		// Truncate if too long
		if len(line) > width-2 {
			line = line[:width-5] + "..."
		}

		lines = append(lines, style.Render(line))
	}

	// Add header
	header := fmt.Sprintf("Messages (%d total):", len(s.messageHistory))
	result := []string{
		lipgloss.NewStyle().Bold(true).Render(header),
		strings.Repeat("─", width),
	}
	result = append(result, lines...)

	return strings.Join(result, "\n")
}

// GetMessageHistory returns a copy of the message history for use with MessagesOverlay
func (s *StatusBar) GetMessageHistory() []StatusMessage {
	// Create a copy to avoid external modification
	history := make([]StatusMessage, len(s.messageHistory))
	copy(history, s.messageHistory)
	return history
}


// View renders the status bar
func (s *StatusBar) View(width int) string {
	if s.commandMode {
		// Show command input
		return s.commandStyle.Render(s.commandInput.View())
	}

	if s.currentMessage == "" {
		return ""
	}

	// Choose style based on level
	var style lipgloss.Style
	switch s.currentLevel {
	case "ERROR":
		style = s.errorStyle
	case "WARN":
		style = s.warnStyle
	default:
		style = s.infoStyle
	}

	message := s.currentMessage
	if len(message) > width-4 {
		message = message[:width-7] + "..."
	}

	return style.Render(message)
}