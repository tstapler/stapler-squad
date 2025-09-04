package ui

import (
	"claude-squad/session"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TagFilter handles filtering sessions by tags
type TagFilter struct {
	width         int
	height        int
	activeTag     string
	availableTags map[string]int // Map of tag to count of sessions with that tag
	focused       bool
}

// NewTagFilter creates a new tag filter component
func NewTagFilter() *TagFilter {
	return &TagFilter{
		width:         0,
		height:        0,
		activeTag:     "",
		availableTags: make(map[string]int),
		focused:       false,
	}
}

// SetSize sets the size of the component
func (t *TagFilter) SetSize(width, height int) {
	t.width = width
	t.height = height
}

// Focus gives focus to the component
func (t *TagFilter) Focus() {
	t.focused = true
}

// Blur removes focus from the component
func (t *TagFilter) Blur() {
	t.focused = false
}

// HandleKeyPress processes key presses when filter is focused
func (t *TagFilter) HandleKeyPress(msg tea.KeyMsg) bool {
	if !t.focused {
		return false
	}

	switch msg.Type {
	case tea.KeyEsc:
		t.activeTag = ""
		t.Blur()
		return true
	case tea.KeyEnter:
		// Confirm selected tag
		t.Blur()
		return true
	}

	return false
}

// SetActiveTag sets the active tag filter
func (t *TagFilter) SetActiveTag(tag string) {
	t.activeTag = tag
}

// GetActiveTag returns the currently active tag filter
func (t *TagFilter) GetActiveTag() string {
	return t.activeTag
}

// ClearTagFilter clears the active tag filter
func (t *TagFilter) ClearTagFilter() {
	t.activeTag = ""
}

// UpdateAvailableTags updates the available tags from a list of sessions
func (t *TagFilter) UpdateAvailableTags(instances []*session.Instance) {
	// Clear existing tags
	t.availableTags = make(map[string]int)

	// Count tags across all sessions
	for _, instance := range instances {
		if instance.Tags != nil {
			for _, tag := range instance.Tags {
				t.availableTags[tag]++
			}
		}
	}
}

// ApplyFilter filters a list of sessions by the active tag
func (t *TagFilter) ApplyFilter(instances []*session.Instance) []*session.Instance {
	// If no active tag, return all instances
	if t.activeTag == "" {
		return instances
	}

	// Filter instances by the active tag
	filtered := make([]*session.Instance, 0)
	for _, instance := range instances {
		if instance.Tags != nil {
			for _, tag := range instance.Tags {
				if tag == t.activeTag {
					filtered = append(filtered, instance)
					break
				}
			}
		}
	}

	return filtered
}

// View renders the tag filter component
func (t *TagFilter) View() string {
	if len(t.availableTags) == 0 {
		return ""
	}

	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))
	sb.WriteString(titleStyle.Render("Filter by Tags"))
	sb.WriteString("\n")

	// Tag list
	tags := make([]string, 0, len(t.availableTags))
	for tag := range t.availableTags {
		tags = append(tags, tag)
	}

	// Sort tags by name alphabetically
	sort.Strings(tags)

	// Render available tags
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#0000FF")).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#DDDDDD")).
		Padding(0, 1)

	for _, tag := range tags {
		count := t.availableTags[tag]

		// Truncate tag if too long (max 15 chars)
		displayTag := tag
		if len(tag) > 15 {
			displayTag = tag[:12] + "..."
		}

		tagText := fmt.Sprintf("%s (%d)", displayTag, count)

		if tag == t.activeTag {
			sb.WriteString(activeStyle.Render(tagText))
		} else {
			sb.WriteString(inactiveStyle.Render(tagText))
		}
		sb.WriteString(" ")
	}

	return sb.String()
}
