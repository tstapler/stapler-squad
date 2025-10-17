package ui

import (
	"fmt"
	"strings"

	"claude-squad/session"

	"github.com/charmbracelet/lipgloss"
)

// PTYList displays a list of available PTY connections
type PTYList struct {
	connections      []*session.PTYConnection
	selectedIndex    int
	categoryExpanded map[session.PTYCategory]bool
	width            int
	height           int

	// Category organization
	categories map[session.PTYCategory][]*session.PTYConnection
}

// NewPTYList creates a new PTY list
func NewPTYList() *PTYList {
	return &PTYList{
		connections:      make([]*session.PTYConnection, 0),
		selectedIndex:    0,
		categoryExpanded: map[session.PTYCategory]bool{
			session.PTYCategorySquad:    true,
			session.PTYCategoryOrphaned: true,
			session.PTYCategoryOther:    true,
		},
		categories: make(map[session.PTYCategory][]*session.PTYConnection),
	}
}

// SetConnections updates the PTY list
func (pl *PTYList) SetConnections(connections []*session.PTYConnection) {
	pl.connections = connections
	pl.organizeByCategory()

	// Adjust selection if out of bounds
	visibleCount := pl.getVisibleCount()
	if pl.selectedIndex >= visibleCount {
		pl.selectedIndex = visibleCount - 1
	}
	if pl.selectedIndex < 0 {
		pl.selectedIndex = 0
	}
}

// SetSize updates the display dimensions
func (pl *PTYList) SetSize(width, height int) {
	pl.width = width
	pl.height = height
}

// MoveUp moves selection up
func (pl *PTYList) MoveUp() {
	if pl.selectedIndex > 0 {
		pl.selectedIndex--
	}
}

// MoveDown moves selection down
func (pl *PTYList) MoveDown() {
	visibleCount := pl.getVisibleCount()
	if pl.selectedIndex < visibleCount-1 {
		pl.selectedIndex++
	}
}

// ToggleCategory expands/collapses a category
func (pl *PTYList) ToggleCategory(category session.PTYCategory) {
	pl.categoryExpanded[category] = !pl.categoryExpanded[category]
}

// GetSelected returns the currently selected PTY connection
func (pl *PTYList) GetSelected() *session.PTYConnection {
	visible := pl.getVisibleItems()
	if pl.selectedIndex >= 0 && pl.selectedIndex < len(visible) {
		return visible[pl.selectedIndex]
	}
	return nil
}

// GetSelectedCategory returns the category of the selected item
func (pl *PTYList) GetSelectedCategory() session.PTYCategory {
	visibleIdx := 0

	for _, category := range []session.PTYCategory{
		session.PTYCategorySquad,
		session.PTYCategoryOrphaned,
		session.PTYCategoryOther,
	} {
		// Category header
		if visibleIdx == pl.selectedIndex {
			return category
		}
		visibleIdx++

		// Category items (if expanded)
		if pl.categoryExpanded[category] {
			items := pl.categories[category]
			if pl.selectedIndex < visibleIdx+len(items) {
				return category
			}
			visibleIdx += len(items)
		}
	}

	return session.PTYCategorySquad
}

// Render generates the PTY list view
func (pl *PTYList) Render() string {
	if len(pl.connections) == 0 {
		return pl.renderEmpty()
	}

	var lines []string
	visibleIdx := 0

	for _, category := range []session.PTYCategory{
		session.PTYCategorySquad,
		session.PTYCategoryOrphaned,
		session.PTYCategoryOther,
	} {
		items := pl.categories[category]
		if len(items) == 0 {
			continue
		}

		// Render category header
		lines = append(lines, pl.renderCategoryHeader(category, visibleIdx))
		visibleIdx++

		// Render items if expanded
		if pl.categoryExpanded[category] {
			for _, conn := range items {
				lines = append(lines, pl.renderConnection(conn, visibleIdx))
				visibleIdx++
			}
		}
	}

	// Join lines and fit to height
	content := strings.Join(lines, "\n")
	return content
}

// renderCategoryHeader renders a category header
func (pl *PTYList) renderCategoryHeader(category session.PTYCategory, index int) string {
	items := pl.categories[category]
	count := len(items)

	// Expansion indicator
	indicator := "▼"
	if !pl.categoryExpanded[category] {
		indicator = "▶"
	}

	// Selection indicator
	prefix := "  "
	if index == pl.selectedIndex {
		prefix = "> "
	}

	style := lipgloss.NewStyle().Bold(true)
	if index == pl.selectedIndex {
		style = style.Foreground(lipgloss.Color("39"))
	}

	header := fmt.Sprintf("%s%s %s (%d)", prefix, indicator, category.String(), count)
	return style.Render(header)
}

// renderConnection renders a single PTY connection
func (pl *PTYList) renderConnection(conn *session.PTYConnection, index int) string {
	// Selection indicator
	prefix := "    "
	if index == pl.selectedIndex {
		prefix = "  > "
	}

	// Status icon with color
	statusIcon := conn.GetStatusIcon()
	statusColor := conn.GetStatusColor()
	styledIcon := lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusColor)).
		Render(statusIcon)

	// PTY path
	ptyPath := conn.GetPTYBasename()

	// Display name
	displayName := conn.GetDisplayName()
	if displayName == "" {
		displayName = "(unnamed)"
	}

	// Build line
	line := fmt.Sprintf("%s%s /%s  %s", prefix, styledIcon, ptyPath, displayName)

	// Highlight if selected
	if index == pl.selectedIndex {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
		return style.Render(line)
	}

	return line
}

// renderEmpty renders an empty state
func (pl *PTYList) renderEmpty() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	return style.Render("No PTYs discovered")
}

// organizeByCategory groups connections by category
func (pl *PTYList) organizeByCategory() {
	pl.categories = map[session.PTYCategory][]*session.PTYConnection{
		session.PTYCategorySquad:    make([]*session.PTYConnection, 0),
		session.PTYCategoryOrphaned: make([]*session.PTYConnection, 0),
		session.PTYCategoryOther:    make([]*session.PTYConnection, 0),
	}

	for _, conn := range pl.connections {
		category := pl.categorizeConnection(conn)
		pl.categories[category] = append(pl.categories[category], conn)
	}
}

// categorizeConnection determines the category for a connection
func (pl *PTYList) categorizeConnection(conn *session.PTYConnection) session.PTYCategory {
	if conn.SessionName != "" {
		return session.PTYCategorySquad
	}

	if strings.Contains(strings.ToLower(conn.Command), "claude") {
		return session.PTYCategoryOrphaned
	}

	return session.PTYCategoryOther
}

// getVisibleCount returns the number of visible items (including headers)
func (pl *PTYList) getVisibleCount() int {
	count := 0

	for _, category := range []session.PTYCategory{
		session.PTYCategorySquad,
		session.PTYCategoryOrphaned,
		session.PTYCategoryOther,
	} {
		items := pl.categories[category]
		if len(items) == 0 {
			continue
		}

		// Count header
		count++

		// Count items if expanded
		if pl.categoryExpanded[category] {
			count += len(items)
		}
	}

	return count
}

// getVisibleItems returns all visible PTY connections (flattened)
func (pl *PTYList) getVisibleItems() []*session.PTYConnection {
	var result []*session.PTYConnection

	for _, category := range []session.PTYCategory{
		session.PTYCategorySquad,
		session.PTYCategoryOrphaned,
		session.PTYCategoryOther,
	} {
		if pl.categoryExpanded[category] {
			result = append(result, pl.categories[category]...)
		}
	}

	return result
}

// IsOnCategoryHeader checks if the current selection is on a category header
func (pl *PTYList) IsOnCategoryHeader() bool {
	visibleIdx := 0

	for _, category := range []session.PTYCategory{
		session.PTYCategorySquad,
		session.PTYCategoryOrphaned,
		session.PTYCategoryOther,
	} {
		items := pl.categories[category]
		if len(items) == 0 {
			continue
		}

		// Check if on header
		if visibleIdx == pl.selectedIndex {
			return true
		}
		visibleIdx++

		// Skip items
		if pl.categoryExpanded[category] {
			visibleIdx += len(items)
		}
	}

	return false
}

// GetTitle returns the list title with count
func (pl *PTYList) GetTitle() string {
	return fmt.Sprintf("Available PTYs (%d)", len(pl.connections))
}
