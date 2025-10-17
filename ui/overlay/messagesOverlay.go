package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusMessage represents a single status/error message
type StatusMessage struct {
	Timestamp time.Time
	Level     string // "INFO", "WARN", "ERROR"
	Message   string
}

// MessagesOverlay represents a scrollable messages overlay with vim-like navigation
type MessagesOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed
	OnDismiss func()

	// Message data
	messages []StatusMessage

	// Scrolling state
	scrollOffset int
	maxOffset    int

	// Styles
	infoStyle   lipgloss.Style
	warnStyle   lipgloss.Style
	errorStyle  lipgloss.Style
	headerStyle lipgloss.Style
}

// NewMessagesOverlay creates a new scrollable messages overlay
func NewMessagesOverlay(messages []StatusMessage) *MessagesOverlay {
	overlay := &MessagesOverlay{
		Dismissed:    false,
		messages:     messages,
		scrollOffset: 0,
		infoStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		warnStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		headerStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
	}

	// Initialize BaseOverlay with default size
	overlay.BaseOverlay.SetSize(80, 30)
	overlay.BaseOverlay.Focus()

	return overlay
}


// HandleKeyPress processes key presses for vim-like navigation
func (m *MessagesOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Use BaseOverlay for Esc key handling
	if handled, shouldClose := m.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			m.Dismissed = true
			if m.OnDismiss != nil {
				m.OnDismiss()
			}
			return true
		}
	}

	switch msg.String() {
	case "q":
		// Quit/close overlay
		m.Dismissed = true
		if m.OnDismiss != nil {
			m.OnDismiss()
		}
		return true

	case "j", "down":
		// Scroll down one line
		m.scrollDown(1)

	case "k", "up":
		// Scroll up one line
		m.scrollUp(1)

	case "ctrl+d":
		// Scroll down half page
		m.scrollDown(m.getVisibleHeight() / 2)

	case "ctrl+u":
		// Scroll up half page
		m.scrollUp(m.getVisibleHeight() / 2)

	case "ctrl+f", "pgdown":
		// Scroll down full page
		m.scrollDown(m.getVisibleHeight())

	case "ctrl+b", "pgup":
		// Scroll up full page
		m.scrollUp(m.getVisibleHeight())

	case "g":
		// Go to top
		m.scrollOffset = 0

	case "G":
		// Go to bottom
		m.scrollOffset = m.maxOffset

	case "home":
		// Go to top
		m.scrollOffset = 0

	case "end":
		// Go to bottom
		m.scrollOffset = m.maxOffset
	}

	return false
}

// scrollDown scrolls down by the specified number of lines
func (m *MessagesOverlay) scrollDown(lines int) {
	m.scrollOffset += lines
	if m.scrollOffset > m.maxOffset {
		m.scrollOffset = m.maxOffset
	}
}

// scrollUp scrolls up by the specified number of lines
func (m *MessagesOverlay) scrollUp(lines int) {
	m.scrollOffset -= lines
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// getVisibleHeight returns the height available for message display
func (m *MessagesOverlay) getVisibleHeight() int {
	// Account for header, separator, border, and padding
	return m.GetHeight() - 6
}

// formatMessage formats a single message with proper wrapping
func (m *MessagesOverlay) formatMessage(msg StatusMessage, maxWidth int) []string {
	timestamp := msg.Timestamp.Format("15:04:05")
	prefix := fmt.Sprintf("[%s] %s: ", timestamp, msg.Level)

	// Choose style based on level
	var style lipgloss.Style
	switch msg.Level {
	case "ERROR":
		style = m.errorStyle
	case "WARN":
		style = m.warnStyle
	default:
		style = m.infoStyle
	}

	// Calculate available width for message content
	prefixLen := len(prefix)
	contentWidth := maxWidth - prefixLen - 4 // Account for padding and border

	if contentWidth <= 0 {
		contentWidth = 20 // Minimum width
	}

	// Split message into lines that fit within the available width
	lines := []string{}
	message := msg.Message

	for len(message) > 0 {
		var line string
		if len(message) <= contentWidth {
			// Entire remaining message fits
			line = message
			message = ""
		} else {
			// Find a good break point (prefer word boundaries)
			breakPoint := contentWidth
			for breakPoint > 0 && message[breakPoint] != ' ' && message[breakPoint] != '\t' {
				breakPoint--
			}

			if breakPoint == 0 {
				// No good break point found, just cut at content width
				breakPoint = contentWidth
			}

			line = message[:breakPoint]
			message = strings.TrimLeft(message[breakPoint:], " \t")
		}

		// Format the line
		if len(lines) == 0 {
			// First line gets the full prefix
			lines = append(lines, style.Render(prefix+line))
		} else {
			// Subsequent lines get indented
			indent := strings.Repeat(" ", prefixLen)
			lines = append(lines, style.Render(indent+line))
		}
	}

	return lines
}

// updateScrollLimits calculates the maximum scroll offset based on content
func (m *MessagesOverlay) updateScrollLimits() {
	visibleHeight := m.getVisibleHeight()
	contentWidth := m.GetResponsiveWidth() - 4 // Account for padding and border

	// Count total lines needed for all messages
	totalLines := 0
	for _, msg := range m.messages {
		lines := m.formatMessage(msg, contentWidth)
		totalLines += len(lines)
	}

	// Calculate max offset
	m.maxOffset = totalLines - visibleHeight
	if m.maxOffset < 0 {
		m.maxOffset = 0
	}
}

// Render renders the messages overlay with scrolling support
func (m *MessagesOverlay) Render(opts ...WhitespaceOption) string {
	// Use responsive sizing from BaseOverlay
	responsiveWidth := m.GetResponsiveWidth()
	hPadding, vPadding := m.GetResponsivePadding()

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(vPadding, hPadding).
		MaxWidth(responsiveWidth)

	if len(m.messages) == 0 {
		content := m.headerStyle.Render("No messages")
		return borderStyle.Render(content)
	}

	// Update scroll limits based on current dimensions
	m.updateScrollLimits()

	// Ensure scroll offset is valid
	if m.scrollOffset > m.maxOffset {
		m.scrollOffset = m.maxOffset
	}

	contentWidth := responsiveWidth - 4 // Account for padding and border
	visibleHeight := m.getVisibleHeight()

	// Generate all content lines
	var allLines []string
	for _, msg := range m.messages {
		lines := m.formatMessage(msg, contentWidth)
		allLines = append(allLines, lines...)
	}

	// Apply scrolling - select the visible portion
	var visibleLines []string
	startLine := m.scrollOffset
	endLine := startLine + visibleHeight

	if startLine < len(allLines) {
		if endLine > len(allLines) {
			endLine = len(allLines)
		}
		visibleLines = allLines[startLine:endLine]
	}

	// Pad with empty lines if needed to maintain consistent height
	for len(visibleLines) < visibleHeight {
		visibleLines = append(visibleLines, "")
	}

	// Create header with scroll indicator
	totalLines := len(allLines)
	scrollInfo := ""
	if totalLines > visibleHeight {
		scrollInfo = fmt.Sprintf(" (%d-%d of %d)", startLine+1, endLine, totalLines)
	}
	header := fmt.Sprintf("Messages (%d total)%s", len(m.messages), scrollInfo)

	// Create help text
	helpText := "j/k: scroll, q/esc: quit, g/G: top/bottom, Ctrl+d/u: half page, Ctrl+f/b: full page"

	// Build content
	result := []string{
		m.headerStyle.Render(header),
		strings.Repeat("─", contentWidth),
	}
	result = append(result, visibleLines...)
	result = append(result, strings.Repeat("─", contentWidth))
	result = append(result, lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(helpText))

	content := strings.Join(result, "\n")

	return borderStyle.Render(content)
}

// View satisfies the tea.Model interface and renders the messages overlay
// This is needed for the TestRenderer to render the component
func (m *MessagesOverlay) View() string {
	return m.Render()
}

// SetDimensions sets the overlay dimensions
func (m *MessagesOverlay) SetDimensions(width, height int) {
	m.BaseOverlay.SetSize(width, height)
	m.updateScrollLimits()
}

// GetScrollPosition returns current scroll information for debugging
func (m *MessagesOverlay) GetScrollPosition() (current, max int) {
	return m.scrollOffset, m.maxOffset
}