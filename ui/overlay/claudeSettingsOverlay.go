package overlay

import (
	"claude-squad/session"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ClaudeSettingsOverlay represents a settings configuration overlay for Claude Code integration
type ClaudeSettingsOverlay struct {
	// Current settings being edited
	settings session.ClaudeSettings
	// Available Claude sessions to choose from
	availableSessions []session.ClaudeSession
	// Currently selected field
	selectedField int
	// Whether the overlay has been dismissed
	dismissed bool
	// Width and height of the overlay
	width  int
	height int
	// Callback functions
	OnComplete func(settings session.ClaudeSettings, selectedSessionID string)
	OnCancel   func()
	// Currently selected session ID (if any)
	selectedSessionID string
	// Field states for editing
	editingField   bool
	editingValue   string
	sessionListOpen bool
}

// Field definitions for the settings form
type settingField struct {
	name        string
	description string
	fieldType   string // "bool", "string", "int"
}

var settingFields = []settingField{
	{"Auto Reattach", "Automatically reattach to last Claude session on resume", "bool"},
	{"Preferred Session Name", "Preferred naming pattern for new sessions", "string"},
	{"Create New on Missing", "Create new session if previous one is missing", "bool"},
	{"Show Session Selector", "Show session selection menu on resume", "bool"},
	{"Session Timeout (minutes)", "Consider sessions stale after this time (0 = no timeout)", "int"},
	{"Select Claude Session", "Choose specific Claude session to attach to", "session"},
}

// NewClaudeSettingsOverlay creates a new Claude settings overlay
func NewClaudeSettingsOverlay(currentSettings session.ClaudeSettings, availableSessions []session.ClaudeSession) *ClaudeSettingsOverlay {
	return &ClaudeSettingsOverlay{
		settings:          currentSettings,
		availableSessions: availableSessions,
		selectedField:     0,
		dismissed:         false,
		width:             80,
		height:            20,
		editingField:      false,
		sessionListOpen:   false,
	}
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (c *ClaudeSettingsOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	if c.editingField {
		return c.handleEditingKeyPress(msg)
	}

	if c.sessionListOpen {
		return c.handleSessionListKeyPress(msg)
	}

	switch msg.String() {
	case "up", "k":
		if c.selectedField > 0 {
			c.selectedField--
		}
	case "down", "j":
		if c.selectedField < len(settingFields)-1 {
			c.selectedField++
		}
	case "enter", " ":
		return c.handleFieldSelection()
	case "s", "ctrl+s":
		// Save settings
		c.dismissed = true
		if c.OnComplete != nil {
			c.OnComplete(c.settings, c.selectedSessionID)
		}
		return true
	case "esc", "q":
		c.dismissed = true
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return true
	}

	return false
}

// handleFieldSelection handles when a field is selected for editing
func (c *ClaudeSettingsOverlay) handleFieldSelection() bool {
	field := settingFields[c.selectedField]

	switch field.fieldType {
	case "bool":
		// Toggle boolean values directly
		switch field.name {
		case "Auto Reattach":
			c.settings.AutoReattach = !c.settings.AutoReattach
		case "Create New on Missing":
			c.settings.CreateNewOnMissing = !c.settings.CreateNewOnMissing
		case "Show Session Selector":
			c.settings.ShowSessionSelector = !c.settings.ShowSessionSelector
		}
	case "string", "int":
		// Enter editing mode for string/int fields
		c.editingField = true
		switch field.name {
		case "Preferred Session Name":
			c.editingValue = c.settings.PreferredSessionName
		case "Session Timeout (minutes)":
			c.editingValue = strconv.Itoa(c.settings.SessionTimeoutMinutes)
		}
	case "session":
		// Open session selection list
		c.sessionListOpen = true
	}

	return false
}

// handleEditingKeyPress handles key presses while editing a field
func (c *ClaudeSettingsOverlay) handleEditingKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter":
		// Save the edited value
		field := settingFields[c.selectedField]
		switch field.name {
		case "Preferred Session Name":
			c.settings.PreferredSessionName = c.editingValue
		case "Session Timeout (minutes)":
			if val, err := strconv.Atoi(c.editingValue); err == nil && val >= 0 {
				c.settings.SessionTimeoutMinutes = val
			}
		}
		c.editingField = false
		c.editingValue = ""
	case "esc":
		// Cancel editing
		c.editingField = false
		c.editingValue = ""
	case "backspace":
		if len(c.editingValue) > 0 {
			c.editingValue = c.editingValue[:len(c.editingValue)-1]
		}
	default:
		// Add character to editing value
		if len(msg.Runes) > 0 {
			c.editingValue += string(msg.Runes)
		}
	}

	return false
}

// handleSessionListKeyPress handles key presses while session list is open
func (c *ClaudeSettingsOverlay) handleSessionListKeyPress(msg tea.KeyMsg) bool {
	// TODO: Implement session list navigation
	switch msg.String() {
	case "esc":
		c.sessionListOpen = false
	case "enter":
		// For now, just close the session list
		c.sessionListOpen = false
	}
	return false
}

// SetSize sets the size of the overlay
func (c *ClaudeSettingsOverlay) SetSize(width, height int) {
	c.width = width
	c.height = height
}

// View renders the Claude settings overlay
func (c *ClaudeSettingsOverlay) View() string {
	if c.dismissed {
		return ""
	}

	if c.sessionListOpen {
		return c.renderSessionList()
	}

	return c.renderSettings()
}

// renderSettings renders the main settings form
func (c *ClaudeSettingsOverlay) renderSettings() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED")).
		PaddingBottom(1)

	fieldStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2)

	selectedFieldStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).
		Background(lipgloss.Color("#3B82F6")).
		Foreground(lipgloss.Color("#FFFFFF"))

	editingFieldStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).
		Background(lipgloss.Color("#10B981")).
		Foreground(lipgloss.Color("#FFFFFF"))

	title := titleStyle.Render("🔧 Claude Code Settings")

	var fields []string

	for i, field := range settingFields {
		var value string
		var style lipgloss.Style

		// Determine the display value
		switch field.fieldType {
		case "bool":
			var boolVal bool
			switch field.name {
			case "Auto Reattach":
				boolVal = c.settings.AutoReattach
			case "Create New on Missing":
				boolVal = c.settings.CreateNewOnMissing
			case "Show Session Selector":
				boolVal = c.settings.ShowSessionSelector
			}
			if boolVal {
				value = "✓ Enabled"
			} else {
				value = "✗ Disabled"
			}
		case "string":
			switch field.name {
			case "Preferred Session Name":
				value = c.settings.PreferredSessionName
				if value == "" {
					value = "(default)"
				}
			}
		case "int":
			switch field.name {
			case "Session Timeout (minutes)":
				if c.settings.SessionTimeoutMinutes == 0 {
					value = "No timeout"
				} else {
					value = fmt.Sprintf("%d minutes", c.settings.SessionTimeoutMinutes)
				}
			}
		case "session":
			if c.selectedSessionID != "" {
				value = fmt.Sprintf("Session: %s", c.selectedSessionID)
			} else {
				value = fmt.Sprintf("%d sessions available", len(c.availableSessions))
			}
		}

		// Apply appropriate styling
		if c.editingField && i == c.selectedField {
			style = editingFieldStyle
			if field.fieldType == "string" || field.fieldType == "int" {
				value = c.editingValue + "│" // Show cursor
			}
		} else if i == c.selectedField {
			style = selectedFieldStyle
		} else {
			style = fieldStyle
		}

		fieldText := fmt.Sprintf("%-25s %s", field.name+":", value)
		fields = append(fields, style.Render(fieldText))

		// Add description line
		if i == c.selectedField {
			desc := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")).
				Italic(true).
				PaddingLeft(4).
				Render("  " + field.description)
			fields = append(fields, desc)
		}
	}

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		PaddingTop(1).
		Render("Navigation: ↑/↓  Edit: Enter/Space  Save: Ctrl+S  Cancel: Esc")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		strings.Join(fields, "\n"),
		instructions,
	)

	// Add border
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(c.width - 4)

	return borderStyle.Render(content)
}

// renderSessionList renders the Claude session selection list
func (c *ClaudeSettingsOverlay) renderSessionList() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED")).
		PaddingBottom(1)

	title := titleStyle.Render("📋 Available Claude Sessions")

	if len(c.availableSessions) == 0 {
		noSessions := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Italic(true).
			Render("No Claude sessions found")

		instructions := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			PaddingTop(1).
			Render("Press Esc to return to settings")

		content := lipgloss.JoinVertical(lipgloss.Left, title, noSessions, instructions)

		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Width(c.width - 4)

		return borderStyle.Render(content)
	}

	var sessions []string
	for _, session := range c.availableSessions {
		status := "Inactive"
		if session.IsActive {
			status = "Active"
		}

		sessionText := fmt.Sprintf("• %s (%s) - %s",
			session.ProjectName,
			session.ID[:8],
			status)
		sessions = append(sessions, sessionText)
	}

	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		PaddingTop(1).
		Render("Select: Enter  Cancel: Esc")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		strings.Join(sessions, "\n"),
		instructions,
	)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 2).
		Width(c.width - 4)

	return borderStyle.Render(content)
}