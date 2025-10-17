package overlay

import (
	"claude-squad/session"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandInputOverlay provides an interface for sending commands to Claude.
type CommandInputOverlay struct {
	BaseOverlay

	textarea      textarea.Model
	sessionName   string
	controller    *session.ClaudeController
	priority      int
	immediate     bool // Send immediately vs queued
	FocusIndex    int  // 0=textarea, 1=priority, 2=immediate, 3=send button
	Submitted     bool
	Canceled      bool
	lastCommandID string
	errorMessage  string
}

// NewCommandInputOverlay creates a new command input overlay.
func NewCommandInputOverlay(sessionName string, controller *session.ClaudeController) *CommandInputOverlay {
	ti := textarea.New()
	ti.Placeholder = "Enter command to send to Claude..."
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = "│ "
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.CharLimit = 0
	ti.MaxHeight = 8

	overlay := &CommandInputOverlay{
		textarea:    ti,
		sessionName: sessionName,
		controller:  controller,
		priority:    100, // Default priority
		immediate:   false,
		FocusIndex:  0,
		Submitted:   false,
		Canceled:    false,
	}

	overlay.BaseOverlay.SetSize(80, 20)
	overlay.BaseOverlay.Focus()

	return overlay
}

// Init initializes the overlay.
func (c *CommandInputOverlay) Init() tea.Cmd {
	return textarea.Blink
}

// View renders the overlay.
func (c *CommandInputOverlay) View() string {
	return c.Render()
}

// HandleKeyPress processes key input.
func (c *CommandInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Common keys (Esc)
	if handled, shouldClose := c.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			c.Canceled = true
			return true
		}
	}

	switch msg.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		// Cycle through focusable elements
		if msg.Type == tea.KeyTab {
			c.FocusIndex = (c.FocusIndex + 1) % 4
		} else {
			c.FocusIndex = (c.FocusIndex - 1 + 4) % 4
		}

		// Update textarea focus
		if c.FocusIndex == 0 {
			c.textarea.Focus()
		} else {
			c.textarea.Blur()
		}
		return false

	case tea.KeyEnter:
		if c.FocusIndex == 3 || (c.FocusIndex == 0 && msg.Alt) {
			// Send button or Alt+Enter from textarea
			return c.sendCommand()
		}
		// Otherwise, let textarea handle it (newline)
		if c.FocusIndex == 0 {
			var cmd tea.Cmd
			c.textarea, cmd = c.textarea.Update(msg)
			_ = cmd
		}
		return false

	case tea.KeyUp, tea.KeyDown:
		// Adjust priority when focused on priority control
		if c.FocusIndex == 1 {
			if msg.Type == tea.KeyUp && c.priority < 200 {
				c.priority += 10
			} else if msg.Type == tea.KeyDown && c.priority > 0 {
				c.priority -= 10
			}
			return false
		}

	case tea.KeySpace:
		// Toggle immediate when focused on immediate control
		if c.FocusIndex == 2 {
			c.immediate = !c.immediate
			return false
		}

	case tea.KeyCtrlC:
		c.Canceled = true
		return true
	}

	// Pass to textarea if focused
	if c.FocusIndex == 0 {
		var cmd tea.Cmd
		c.textarea, cmd = c.textarea.Update(msg)
		_ = cmd
	}

	return false
}

// sendCommand sends the command via the controller.
func (c *CommandInputOverlay) sendCommand() bool {
	command := strings.TrimSpace(c.textarea.Value())
	if command == "" {
		c.errorMessage = "Command cannot be empty"
		return false
	}

	if c.controller == nil {
		c.errorMessage = "Controller not initialized"
		return false
	}

	if c.immediate {
		// Send immediately, bypassing queue
		result, execErr := c.controller.SendCommandImmediate(command)
		if execErr != nil {
			c.errorMessage = fmt.Sprintf("Failed to send: %v", execErr)
			return false
		}
		c.lastCommandID = result.Command.ID
	} else {
		// Send via queue
		commandID, queueErr := c.controller.SendCommand(command, c.priority)
		if queueErr != nil {
			c.errorMessage = fmt.Sprintf("Failed to queue: %v", queueErr)
			return false
		}
		c.lastCommandID = commandID
	}

	c.Submitted = true
	return true
}

// Render generates the overlay view.
func (c *CommandInputOverlay) Render() string {
	width := c.BaseOverlay.GetWidth()
	height := c.BaseOverlay.GetHeight()

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Render(fmt.Sprintf("Send Command to %s", c.sessionName))

	// Command input area
	textareaStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	if c.FocusIndex == 0 {
		textareaStyle = textareaStyle.BorderForeground(lipgloss.Color("39"))
	}

	c.textarea.SetWidth(width - 4)
	commandInput := textareaStyle.Render(c.textarea.View())

	// Priority control
	priorityLabel := "Priority:"
	priorityValue := fmt.Sprintf("%d", c.priority)
	priorityStyle := lipgloss.NewStyle()
	if c.FocusIndex == 1 {
		priorityStyle = priorityStyle.Bold(true).Foreground(lipgloss.Color("39"))
	}
	priorityControl := priorityStyle.Render(fmt.Sprintf("%s %s (↑/↓)", priorityLabel, priorityValue))

	// Immediate toggle
	immediateLabel := "Immediate:"
	immediateValue := "No"
	if c.immediate {
		immediateValue = "Yes"
	}
	immediateStyle := lipgloss.NewStyle()
	if c.FocusIndex == 2 {
		immediateStyle = immediateStyle.Bold(true).Foreground(lipgloss.Color("39"))
	}
	immediateControl := immediateStyle.Render(fmt.Sprintf("%s %s (Space)", immediateLabel, immediateValue))

	// Send button
	sendButtonStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 2)

	if c.FocusIndex == 3 {
		sendButtonStyle = sendButtonStyle.
			Bold(true).
			Foreground(lipgloss.Color("39")).
			BorderForeground(lipgloss.Color("39"))
	} else {
		sendButtonStyle = sendButtonStyle.BorderForeground(lipgloss.Color("240"))
	}

	sendButton := sendButtonStyle.Render("Send (Enter)")

	// Controls row
	controlsRow := lipgloss.JoinHorizontal(
		lipgloss.Left,
		priorityControl,
		"  ",
		immediateControl,
		"  ",
		sendButton,
	)

	// Help text
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpText := helpStyle.Render("Tab: cycle • Alt+Enter: send • Esc: cancel")

	// Error message
	errorText := ""
	if c.errorMessage != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		errorText = errorStyle.Render(fmt.Sprintf("Error: %s", c.errorMessage))
	}

	// Status info
	statusText := ""
	if c.controller != nil && c.controller.IsStarted() {
		status, _ := c.controller.GetCurrentStatus()
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		statusText = statusStyle.Render(fmt.Sprintf("Claude Status: %s", status))
	}

	// Assemble content
	contentLines := []string{
		title,
		"",
		commandInput,
		"",
		controlsRow,
		"",
		helpText,
	}

	if errorText != "" {
		contentLines = append(contentLines, "", errorText)
	}

	if statusText != "" {
		contentLines = append(contentLines, "", statusText)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, contentLines...)

	// Center in overlay
	contentHeight := lipgloss.Height(content)
	contentWidth := lipgloss.Width(content)

	paddingTop := (height - contentHeight) / 2
	paddingLeft := (width - contentWidth) / 2

	if paddingTop < 0 {
		paddingTop = 0
	}
	if paddingLeft < 0 {
		paddingLeft = 0
	}

	// Apply padding
	paddedContent := lipgloss.NewStyle().
		PaddingTop(paddingTop).
		PaddingLeft(paddingLeft).
		Width(width).
		Height(height).
		Render(content)

	// Wrap in overlay border
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Width(width).
		Height(height)

	return overlayStyle.Render(paddedContent)
}

// GetCommand returns the entered command text.
func (c *CommandInputOverlay) GetCommand() string {
	return strings.TrimSpace(c.textarea.Value())
}

// GetLastCommandID returns the ID of the last sent command.
func (c *CommandInputOverlay) GetLastCommandID() string {
	return c.lastCommandID
}

// GetPriority returns the current priority setting.
func (c *CommandInputOverlay) GetPriority() int {
	return c.priority
}

// IsImmediate returns whether immediate execution is enabled.
func (c *CommandInputOverlay) IsImmediate() bool {
	return c.immediate
}

// SetPriority sets the command priority.
func (c *CommandInputOverlay) SetPriority(priority int) {
	if priority < 0 {
		priority = 0
	}
	if priority > 200 {
		priority = 200
	}
	c.priority = priority
}

// SetImmediate sets immediate execution mode.
func (c *CommandInputOverlay) SetImmediate(immediate bool) {
	c.immediate = immediate
}
