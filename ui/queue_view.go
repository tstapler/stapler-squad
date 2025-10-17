package ui

import (
	"fmt"
	"strings"
	"time"

	"claude-squad/session"

	"github.com/charmbracelet/lipgloss"
)

// Priority display styles
var (
	urgentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Bold(true)

	highStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa00"))

	mediumStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffff00"))

	lowStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))
)

// QueueView displays the review queue with sessions needing attention
type QueueView struct {
	queue         *session.ReviewQueue
	selectedIndex int
	width         int
	height        int
	scrollOffset  int

	// Display options
	showContext bool // Whether to show context snippets
}

// NewQueueView creates a new queue view
func NewQueueView(queue *session.ReviewQueue) *QueueView {
	return &QueueView{
		queue:        queue,
		showContext:  true,
		scrollOffset: 0,
	}
}

// SetSize updates the display dimensions
func (qv *QueueView) SetSize(width, height int) {
	qv.width = width
	qv.height = height
}

// MoveUp moves selection up
func (qv *QueueView) MoveUp() {
	if qv.selectedIndex > 0 {
		qv.selectedIndex--
		qv.adjustScrollOffset()
	}
}

// MoveDown moves selection down
func (qv *QueueView) MoveDown() {
	items := qv.queue.List()
	if qv.selectedIndex < len(items)-1 {
		qv.selectedIndex++
		qv.adjustScrollOffset()
	}
}

// GetSelected returns the currently selected review item
func (qv *QueueView) GetSelected() *session.ReviewItem {
	items := qv.queue.List()
	if qv.selectedIndex >= 0 && qv.selectedIndex < len(items) {
		return items[qv.selectedIndex]
	}
	return nil
}

// adjustScrollOffset adjusts scroll to keep selection visible
func (qv *QueueView) adjustScrollOffset() {
	// Reserve lines for header (3) and footer (2)
	visibleLines := qv.height - 5
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Each item takes 2 lines minimum (title + metadata)
	// Add 1 if showing context
	linesPerItem := 2
	if qv.showContext {
		linesPerItem = 3
	}

	visibleItems := visibleLines / linesPerItem

	// Scroll down if selection is below visible area
	if qv.selectedIndex >= qv.scrollOffset+visibleItems {
		qv.scrollOffset = qv.selectedIndex - visibleItems + 1
	}

	// Scroll up if selection is above visible area
	if qv.selectedIndex < qv.scrollOffset {
		qv.scrollOffset = qv.selectedIndex
	}

	// Ensure scroll offset is valid
	if qv.scrollOffset < 0 {
		qv.scrollOffset = 0
	}
}

// View renders the queue view
func (qv *QueueView) View() string {
	items := qv.queue.List()

	if len(items) == 0 {
		return qv.renderEmpty()
	}

	var b strings.Builder

	// Header
	b.WriteString(qv.renderHeader(len(items)))
	b.WriteString("\n\n")

	// Calculate visible items
	visibleLines := qv.height - 5
	if visibleLines < 1 {
		visibleLines = 1
	}

	linesPerItem := 2
	if qv.showContext {
		linesPerItem = 3
	}

	visibleItems := visibleLines / linesPerItem
	endIdx := qv.scrollOffset + visibleItems
	if endIdx > len(items) {
		endIdx = len(items)
	}

	// Render visible items
	for i := qv.scrollOffset; i < endIdx; i++ {
		item := items[i]
		selected := i == qv.selectedIndex
		b.WriteString(qv.renderItem(item, selected, i+1))
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(items) > visibleItems {
		totalItems := len(items)
		showing := fmt.Sprintf("Showing %d-%d of %d", qv.scrollOffset+1, endIdx, totalItems)
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(showing))
	}

	return b.String()
}

// renderHeader renders the queue header
func (qv *QueueView) renderHeader(count int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffffff")).
		Render(fmt.Sprintf("📥 Review Queue (%d items)", count))

	subtitle := lipgloss.NewStyle().
		Faint(true).
		Render("Sessions needing your attention")

	return title + "\n" + subtitle
}

// renderEmpty renders the empty state
func (qv *QueueView) renderEmpty() string {
	style := lipgloss.NewStyle().
		Faint(true).
		Align(lipgloss.Center).
		Width(qv.width)

	return style.Render("\n\n✓ All caught up!\n\nNo sessions need attention.")
}

// renderItem renders a single queue item
func (qv *QueueView) renderItem(item *session.ReviewItem, selected bool, number int) string {
	var b strings.Builder

	// Get priority icon and style
	icon, style := qv.getPriorityStyle(item.Priority)

	// Number and icon
	prefix := fmt.Sprintf("%d. %s ", number, icon)

	// Session name (truncate if needed)
	maxNameWidth := qv.width - 40 // Reserve space for metadata
	if maxNameWidth < 20 {
		maxNameWidth = 20
	}
	name := truncateString(item.SessionName, maxNameWidth)

	// Priority badge
	priorityBadge := qv.formatPriority(item.Priority)

	// Time elapsed
	elapsed := time.Since(item.DetectedAt)
	timeStr := formatDuration(elapsed)

	// Reason
	reason := qv.formatReason(item.Reason)

	// Build first line (title + metadata)
	titleStyle := titleStyle
	if selected {
		titleStyle = selectedTitleStyle
	}

	firstLine := fmt.Sprintf("%s%s [%s] - %s - %s ago",
		prefix, name, priorityBadge, reason, timeStr)

	b.WriteString(style.Render(titleStyle.Render(firstLine)))
	b.WriteString("\n")

	// Context snippet (if enabled and available)
	if qv.showContext && item.Context != "" {
		contextStyle := listDescStyle
		if selected {
			contextStyle = selectedDescStyle
		}

		// Truncate context to fit width
		maxContextWidth := qv.width - 6 // Indent + padding
		if maxContextWidth < 20 {
			maxContextWidth = 20
		}
		context := truncateString(strings.TrimSpace(item.Context), maxContextWidth)

		b.WriteString(contextStyle.Render(fmt.Sprintf("   %s", context)))
		b.WriteString("\n")
	}

	return b.String()
}

// getPriorityStyle returns icon and style for a priority level
func (qv *QueueView) getPriorityStyle(priority session.Priority) (string, lipgloss.Style) {
	switch priority {
	case session.PriorityUrgent:
		return "🔴", urgentStyle
	case session.PriorityHigh:
		return "🟡", highStyle
	case session.PriorityMedium:
		return "🟠", mediumStyle
	case session.PriorityLow:
		return "⚪", lowStyle
	default:
		return "⚪", lowStyle
	}
}

// formatPriority formats priority as text
func (qv *QueueView) formatPriority(priority session.Priority) string {
	switch priority {
	case session.PriorityUrgent:
		return "Urgent"
	case session.PriorityHigh:
		return "High"
	case session.PriorityMedium:
		return "Medium"
	case session.PriorityLow:
		return "Low"
	default:
		return "Unknown"
	}
}

// formatReason formats attention reason as text
func (qv *QueueView) formatReason(reason session.AttentionReason) string {
	switch reason {
	case session.ReasonApprovalPending:
		return "Approval Pending"
	case session.ReasonInputRequired:
		return "Input Required"
	case session.ReasonErrorState:
		return "Error State"
	case session.ReasonIdleTimeout:
		return "Idle Timeout"
	case session.ReasonTaskComplete:
		return "Task Complete"
	default:
		return "Unknown"
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm", mins)
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh", hours)
	} else {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
}

// truncateString truncates a string to maxLen, adding ellipsis if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// ToggleContext toggles context display
func (qv *QueueView) ToggleContext() {
	qv.showContext = !qv.showContext
}
