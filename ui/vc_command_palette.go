package ui

import (
	"claude-squad/session/vc"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// VCCommand represents a version control command
type VCCommand struct {
	ID          string
	Name        string
	Description string
	KeyBinding  string
	Category    string
	Handler     func() error
}

// VCCommandPalette provides a fuzzy-searchable command palette for VCS operations
type VCCommandPalette struct {
	// UI Components
	input textinput.Model

	// State
	title         string
	commands      []VCCommand
	filtered      []VCCommand
	selectedIndex int
	width         int
	height        int
	visible       bool
	vcsType       vc.VCSType

	// Visual styles
	titleStyle       lipgloss.Style
	inputStyle       lipgloss.Style
	commandStyle     lipgloss.Style
	selectedStyle    lipgloss.Style
	keyStyle         lipgloss.Style
	descriptionStyle lipgloss.Style
	categoryStyle    lipgloss.Style
	borderStyle      lipgloss.Style

	// Callbacks
	OnSelect func(cmd VCCommand)
	OnCancel func()
}

// NewVCCommandPalette creates a new command palette for VCS operations
func NewVCCommandPalette() *VCCommandPalette {
	// Initialize the text input component
	ti := textinput.New()
	ti.Placeholder = "Search commands..."
	ti.Focus()

	// Define UI styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		MarginBottom(1)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	commandStyle := lipgloss.NewStyle().
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	descriptionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	categoryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	p := &VCCommandPalette{
		input:            ti,
		title:            "VC Commands",
		commands:         []VCCommand{},
		filtered:         []VCCommand{},
		selectedIndex:    0,
		width:            60,
		height:           20,
		visible:          false,
		vcsType:          vc.VCSUnknown,
		titleStyle:       titleStyle,
		inputStyle:       inputStyle,
		commandStyle:     commandStyle,
		selectedStyle:    selectedStyle,
		keyStyle:         keyStyle,
		descriptionStyle: descriptionStyle,
		categoryStyle:    categoryStyle,
		borderStyle:      borderStyle,
	}

	// Initialize default commands
	p.initializeCommands()

	return p
}

// initializeCommands sets up the default VCS commands
func (p *VCCommandPalette) initializeCommands() {
	// Common commands (available for all VCS)
	commonCommands := []VCCommand{
		{ID: "stage", Name: "Stage File", Description: "Stage selected file", KeyBinding: "s", Category: "Staging"},
		{ID: "unstage", Name: "Unstage File", Description: "Unstage selected file", KeyBinding: "u", Category: "Staging"},
		{ID: "stage_all", Name: "Stage All", Description: "Stage all changed files", KeyBinding: "S", Category: "Staging"},
		{ID: "unstage_all", Name: "Unstage All", Description: "Unstage all files", KeyBinding: "U", Category: "Staging"},
		{ID: "terminal", Name: "Open Terminal", Description: "Open interactive VCS terminal", KeyBinding: "t", Category: "Terminal"},
		{ID: "help", Name: "Toggle Help", Description: "Show/hide help", KeyBinding: "?", Category: "Help"},
	}

	// Git-specific commands
	gitCommands := []VCCommand{
		{ID: "commit", Name: "Commit", Description: "Create a new commit", KeyBinding: "cc", Category: "Commit"},
		{ID: "commit_amend", Name: "Amend Commit", Description: "Amend the last commit", KeyBinding: "ca", Category: "Commit"},
		{ID: "push", Name: "Push", Description: "Push to remote", KeyBinding: "p", Category: "Remote"},
		{ID: "pull", Name: "Pull", Description: "Pull from remote", KeyBinding: "P", Category: "Remote"},
		{ID: "fetch", Name: "Fetch", Description: "Fetch from remote", KeyBinding: "F", Category: "Remote"},
		{ID: "stash", Name: "Stash", Description: "Stash changes", KeyBinding: "z", Category: "Stash"},
		{ID: "stash_pop", Name: "Stash Pop", Description: "Pop stashed changes", KeyBinding: "Z", Category: "Stash"},
		{ID: "diff", Name: "Show Diff", Description: "Show diff for selected file", KeyBinding: "d", Category: "View"},
		{ID: "log", Name: "View Log", Description: "View commit history", KeyBinding: "l", Category: "View"},
		{ID: "blame", Name: "Blame", Description: "Show file blame", KeyBinding: "b", Category: "View"},
	}

	// Jujutsu-specific commands
	jjCommands := []VCCommand{
		{ID: "describe", Name: "Describe", Description: "Edit change description", KeyBinding: "D", Category: "Changes"},
		{ID: "new", Name: "New Change", Description: "Create a new change", KeyBinding: "n", Category: "Changes"},
		{ID: "squash", Name: "Squash", Description: "Squash changes", KeyBinding: "q", Category: "Changes"},
		{ID: "split", Name: "Split", Description: "Split current change", KeyBinding: "sp", Category: "Changes"},
		{ID: "abandon", Name: "Abandon", Description: "Abandon current change", KeyBinding: "A", Category: "Changes"},
		{ID: "edit", Name: "Edit Change", Description: "Edit a change", KeyBinding: "e", Category: "Changes"},
		{ID: "bookmark", Name: "Bookmark", Description: "Manage bookmarks", KeyBinding: "B", Category: "Bookmarks"},
		{ID: "git_push", Name: "Git Push", Description: "Push to git remote", KeyBinding: "gp", Category: "Remote"},
		{ID: "git_fetch", Name: "Git Fetch", Description: "Fetch from git remote", KeyBinding: "gf", Category: "Remote"},
	}

	// Combine commands based on VCS type
	p.commands = commonCommands
	switch p.vcsType {
	case vc.VCSGit:
		p.commands = append(p.commands, gitCommands...)
	case vc.VCSJujutsu:
		p.commands = append(p.commands, jjCommands...)
	default:
		// Include both for unknown - will be filtered on actual use
		p.commands = append(p.commands, gitCommands...)
		p.commands = append(p.commands, jjCommands...)
	}

	p.filtered = p.commands
}

// SetVCSType sets the VCS type to filter available commands
func (p *VCCommandPalette) SetVCSType(vcsType vc.VCSType) {
	p.vcsType = vcsType
	p.initializeCommands()
	p.filterCommands()
}

// SetSize sets the dimensions of the palette
func (p *VCCommandPalette) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.inputStyle = p.inputStyle.Width(width - 10)
}

// Show makes the palette visible
func (p *VCCommandPalette) Show() {
	p.visible = true
	p.input.SetValue("")
	p.input.Focus()
	p.selectedIndex = 0
	p.filterCommands()
}

// Hide hides the palette
func (p *VCCommandPalette) Hide() {
	p.visible = false
	p.input.Blur()
}

// IsVisible returns whether the palette is visible
func (p *VCCommandPalette) IsVisible() bool {
	return p.visible
}

// filterCommands filters commands based on current input
func (p *VCCommandPalette) filterCommands() {
	query := strings.ToLower(strings.TrimSpace(p.input.Value()))
	if query == "" {
		p.filtered = p.commands
		return
	}

	var filtered []VCCommand
	for _, cmd := range p.commands {
		// Match against name, description, key binding, or category
		name := strings.ToLower(cmd.Name)
		desc := strings.ToLower(cmd.Description)
		key := strings.ToLower(cmd.KeyBinding)
		cat := strings.ToLower(cmd.Category)

		if strings.Contains(name, query) ||
			strings.Contains(desc, query) ||
			strings.Contains(key, query) ||
			strings.Contains(cat, query) ||
			fuzzyMatch(name, query) {
			filtered = append(filtered, cmd)
		}
	}

	p.filtered = filtered
	// Reset selection if out of bounds
	if p.selectedIndex >= len(p.filtered) {
		p.selectedIndex = 0
	}
}

// fuzzyMatch performs simple fuzzy matching
func fuzzyMatch(text, query string) bool {
	if query == "" {
		return true
	}

	queryIdx := 0
	for i := 0; i < len(text) && queryIdx < len(query); i++ {
		if text[i] == query[queryIdx] {
			queryIdx++
		}
	}
	return queryIdx == len(query)
}

// HandleKeyPress handles keyboard input
func (p *VCCommandPalette) HandleKeyPress(msg tea.KeyMsg) bool {
	if !p.visible {
		return false
	}

	switch msg.Type {
	case tea.KeyEnter:
		// Execute selected command
		if len(p.filtered) > 0 && p.selectedIndex >= 0 && p.selectedIndex < len(p.filtered) {
			if p.OnSelect != nil {
				p.OnSelect(p.filtered[p.selectedIndex])
			}
		}
		p.Hide()
		return true

	case tea.KeyEsc, tea.KeyCtrlC:
		if p.OnCancel != nil {
			p.OnCancel()
		}
		p.Hide()
		return true

	case tea.KeyUp, tea.KeyCtrlP:
		if p.selectedIndex > 0 {
			p.selectedIndex--
		} else if len(p.filtered) > 0 {
			p.selectedIndex = len(p.filtered) - 1
		}
		return true

	case tea.KeyDown, tea.KeyCtrlN:
		if p.selectedIndex < len(p.filtered)-1 {
			p.selectedIndex++
		} else {
			p.selectedIndex = 0
		}
		return true

	default:
		// Update text input
		p.input, _ = p.input.Update(msg)
		p.filterCommands()
		return true
	}
}

// View renders the command palette
func (p *VCCommandPalette) View() string {
	if !p.visible {
		return ""
	}

	var sb strings.Builder

	// Title with VCS indicator
	vcsIndicator := ""
	switch p.vcsType {
	case vc.VCSGit:
		vcsIndicator = " (Git)"
	case vc.VCSJujutsu:
		vcsIndicator = " (Jujutsu)"
	}
	sb.WriteString(p.titleStyle.Render(p.title + vcsIndicator))
	sb.WriteString("\n\n")

	// Search input
	sb.WriteString(p.inputStyle.Render(p.input.View()))
	sb.WriteString("\n\n")

	// Results
	maxVisible := p.height - 8
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if p.selectedIndex >= maxVisible {
		startIdx = p.selectedIndex - maxVisible + 1
	}

	endIdx := startIdx + maxVisible
	if endIdx > len(p.filtered) {
		endIdx = len(p.filtered)
	}

	if len(p.filtered) == 0 {
		sb.WriteString(p.descriptionStyle.Render("No commands found"))
		sb.WriteString("\n")
	} else {
		currentCategory := ""
		for i := startIdx; i < endIdx; i++ {
			cmd := p.filtered[i]

			// Show category header when it changes
			if cmd.Category != currentCategory {
				currentCategory = cmd.Category
				sb.WriteString(p.categoryStyle.Render("── " + currentCategory + " ──"))
				sb.WriteString("\n")
			}

			// Render command
			keyPart := p.keyStyle.Render("[" + cmd.KeyBinding + "]")
			namePart := cmd.Name
			descPart := p.descriptionStyle.Render(cmd.Description)

			line := keyPart + " " + namePart + " " + descPart

			if i == p.selectedIndex {
				sb.WriteString(p.selectedStyle.Render(line))
			} else {
				sb.WriteString(p.commandStyle.Render(line))
			}
			sb.WriteString("\n")
		}

		// Show scroll indicator if needed
		if len(p.filtered) > maxVisible {
			remaining := len(p.filtered) - endIdx
			if remaining > 0 {
				sb.WriteString(p.descriptionStyle.Render("... and " + string(rune('0'+remaining)) + " more"))
				sb.WriteString("\n")
			}
		}
	}

	// Help footer
	sb.WriteString("\n")
	sb.WriteString(p.descriptionStyle.Render("↑↓ navigate • Enter select • Esc cancel"))

	return p.borderStyle.Width(p.width).Render(sb.String())
}

// GetSelectedCommand returns the currently selected command
func (p *VCCommandPalette) GetSelectedCommand() *VCCommand {
	if len(p.filtered) > 0 && p.selectedIndex >= 0 && p.selectedIndex < len(p.filtered) {
		return &p.filtered[p.selectedIndex]
	}
	return nil
}
