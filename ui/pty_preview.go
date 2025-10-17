package ui

import (
	"fmt"
	"strings"
	"time"

	"claude-squad/session"

	"github.com/charmbracelet/lipgloss"
)

// PTYPreview displays output from a selected PTY
type PTYPreview struct {
	connection   *session.PTYConnection
	outputBuffer []string // Recent output lines
	maxLines     int
	width        int
	height       int
	scrollOffset int
}

// NewPTYPreview creates a new PTY preview pane
func NewPTYPreview() *PTYPreview {
	return &PTYPreview{
		outputBuffer: make([]string, 0),
		maxLines:     1000, // Keep last 1000 lines
		scrollOffset: 0,
	}
}

// SetConnection updates the displayed PTY connection
func (pp *PTYPreview) SetConnection(conn *session.PTYConnection) {
	pp.connection = conn
	pp.scrollOffset = 0

	// Load output if controller is available
	if conn != nil && conn.Controller != nil {
		pp.loadOutput()
	} else {
		pp.outputBuffer = make([]string, 0)
	}
}

// SetSize updates the display dimensions
func (pp *PTYPreview) SetSize(width, height int) {
	pp.width = width
	pp.height = height
}

// ScrollUp scrolls the view up
func (pp *PTYPreview) ScrollUp() {
	if pp.scrollOffset > 0 {
		pp.scrollOffset--
	}
}

// ScrollDown scrolls the view down
func (pp *PTYPreview) ScrollDown() {
	maxScroll := len(pp.outputBuffer) - pp.getDisplayLines()
	if pp.scrollOffset < maxScroll && maxScroll > 0 {
		pp.scrollOffset++
	}
}

// PageUp scrolls up by a page
func (pp *PTYPreview) PageUp() {
	pageSize := pp.getDisplayLines()
	pp.scrollOffset -= pageSize
	if pp.scrollOffset < 0 {
		pp.scrollOffset = 0
	}
}

// PageDown scrolls down by a page
func (pp *PTYPreview) PageDown() {
	pageSize := pp.getDisplayLines()
	maxScroll := len(pp.outputBuffer) - pp.getDisplayLines()
	pp.scrollOffset += pageSize
	if pp.scrollOffset > maxScroll {
		pp.scrollOffset = maxScroll
	}
	if pp.scrollOffset < 0 {
		pp.scrollOffset = 0
	}
}

// Render generates the preview pane view
func (pp *PTYPreview) Render() string {
	if pp.connection == nil {
		return pp.renderEmpty()
	}

	var lines []string

	// Header
	lines = append(lines, pp.renderHeader())
	lines = append(lines, pp.renderSeparator())

	// Output content
	lines = append(lines, pp.renderOutput()...)

	return strings.Join(lines, "\n")
}

// renderHeader renders the PTY information header
func (pp *PTYPreview) renderHeader() string {
	if pp.connection == nil {
		return ""
	}

	// Title
	title := fmt.Sprintf("PTY Output: %s", pp.connection.Path)
	titleStyle := lipgloss.NewStyle().Bold(true)

	// Status
	status := fmt.Sprintf("[%s %s]", pp.connection.GetStatusIcon(), pp.connection.Status.String())
	statusColor := pp.connection.GetStatusColor()
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))

	header := fmt.Sprintf("%s %s", titleStyle.Render(title), statusStyle.Render(status))
	return header
}

// renderSeparator renders a separator line
func (pp *PTYPreview) renderSeparator() string {
	if pp.width <= 0 {
		return strings.Repeat("─", 80)
	}
	return strings.Repeat("─", pp.width)
}

// renderOutput renders the PTY output buffer
func (pp *PTYPreview) renderOutput() []string {
	if len(pp.outputBuffer) == 0 {
		return []string{pp.renderNoOutput()}
	}

	displayLines := pp.getDisplayLines()
	start := pp.scrollOffset
	end := start + displayLines

	if end > len(pp.outputBuffer) {
		end = len(pp.outputBuffer)
	}

	if start >= len(pp.outputBuffer) {
		start = len(pp.outputBuffer) - displayLines
		if start < 0 {
			start = 0
		}
	}

	return pp.outputBuffer[start:end]
}

// renderEmpty renders an empty state
func (pp *PTYPreview) renderEmpty() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	return style.Render("No PTY selected")
}

// renderNoOutput renders a no-output state
func (pp *PTYPreview) renderNoOutput() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	if pp.connection != nil && pp.connection.Status == session.PTYError {
		return style.Render("PTY error - cannot read output")
	}

	return style.Render("No output available")
}

// renderMetadata renders metadata about the PTY
func (pp *PTYPreview) renderMetadata() []string {
	if pp.connection == nil {
		return []string{}
	}

	var lines []string
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	// PID
	lines = append(lines, style.Render(fmt.Sprintf("PID: %d", pp.connection.PID)))

	// Command
	lines = append(lines, style.Render(fmt.Sprintf("Command: %s", pp.connection.Command)))

	// Session name (if available)
	if pp.connection.SessionName != "" {
		lines = append(lines, style.Render(fmt.Sprintf("Session: %s", pp.connection.SessionName)))
	}

	// Last activity
	if !pp.connection.LastActivity.IsZero() {
		ago := time.Since(pp.connection.LastActivity)
		lines = append(lines, style.Render(fmt.Sprintf("Last Activity: %s ago", pp.formatDuration(ago))))
	}

	return lines
}

// loadOutput loads output from the PTY controller
func (pp *PTYPreview) loadOutput() {
	if pp.connection == nil || pp.connection.Controller == nil {
		return
	}

	// Get recent output from controller (last 4KB)
	recentOutput := pp.connection.Controller.GetRecentOutput(4096)
	if len(recentOutput) == 0 {
		pp.outputBuffer = make([]string, 0)
		return
	}

	// Split into lines
	lines := strings.Split(string(recentOutput), "\n")

	// Limit to maxLines
	if len(lines) > pp.maxLines {
		lines = lines[len(lines)-pp.maxLines:]
	}

	pp.outputBuffer = lines
}

// RefreshOutput updates the output buffer
func (pp *PTYPreview) RefreshOutput() {
	pp.loadOutput()
}

// getDisplayLines calculates how many lines can be displayed
func (pp *PTYPreview) getDisplayLines() int {
	if pp.height <= 0 {
		return 20 // Default
	}

	// Reserve space for header (2 lines) and metadata (4 lines)
	available := pp.height - 6
	if available < 5 {
		available = 5
	}

	return available
}

// formatDuration formats a duration in human-readable form
func (pp *PTYPreview) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// GetTitle returns the preview pane title
func (pp *PTYPreview) GetTitle() string {
	if pp.connection == nil {
		return "PTY Preview"
	}

	displayName := pp.connection.GetDisplayName()
	if displayName == "" {
		displayName = pp.connection.GetPTYBasename()
	}

	return fmt.Sprintf("PTY Preview: %s", displayName)
}

// GetScrollInfo returns scroll position information
func (pp *PTYPreview) GetScrollInfo() string {
	if len(pp.outputBuffer) == 0 {
		return ""
	}

	totalLines := len(pp.outputBuffer)
	displayLines := pp.getDisplayLines()
	currentLine := pp.scrollOffset + 1

	if totalLines <= displayLines {
		return "All"
	}

	percentage := (pp.scrollOffset * 100) / (totalLines - displayLines)
	return fmt.Sprintf("%d%% (%d/%d)", percentage, currentLine, totalLines)
}
