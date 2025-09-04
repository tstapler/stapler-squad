package app

import (
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpText interface {
	// toContent returns the help UI content.
	toContent() string
	// mask returns the bit mask for this help text. These are used to track which help screens
	// have been seen in the config and app state.
	mask() uint32
}

type helpTypeGeneral struct{}
type helpTypeSessionOrganization struct{}

type helpTypeInstanceStart struct {
	instance *session.Instance
}

type helpTypeInstanceAttach struct{}

type helpTypeInstanceCheckout struct{}

func helpStart(instance *session.Instance) helpText {
	return helpTypeInstanceStart{instance: instance}
}

func (h helpTypeGeneral) toContent() string {
	// Get all categories except special
	allCategories := keys.GetAllCategories()
	
	// Sort categories in a specific order
	sort.Slice(allCategories, func(i, j int) bool {
		// Define category order for display
		order := map[keys.HelpCategory]int{
			keys.HelpCategoryManaging:    1,
			keys.HelpCategoryHandoff:     2,
			keys.HelpCategoryOrganize:    3,
			keys.HelpCategoryNavigation:  4,
			keys.HelpCategoryOther:       5,
			keys.HelpCategoryUncategory:  6, // Always last if present
		}
		return order[allCategories[i]] < order[allCategories[j]]
	})
	
	// Start with title and description
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Claude Squad"),
		"",
		"A terminal UI that manages multiple Claude Code (and other local agents) in separate workspaces.",
		"",
	)
	
	// Add sections for each category
	for _, category := range allCategories {
		// Get keys in this category
		categoryKeys := keys.GetKeysInCategory(category)
		
		// Skip empty categories
		if len(categoryKeys) == 0 {
			continue
		}
		
		// Add category header
		content = lipgloss.JoinVertical(lipgloss.Left, 
			content, 
			headerStyle.Render(string(category)+":"),
		)
		
		// Add each key binding in this category
		for _, keyName := range categoryKeys {
			// Get the key binding
			keyBinding := keys.GlobalkeyBindings[keyName]
			
			// Get help info
			helpInfo := keys.GetKeyHelp(keyName)
			
			// Format and add the key help
			keyText := keyBinding.Help().Key
			descText := helpInfo.Description
			
			// Calculate padding (to align descriptions)
			padding := ""
			padLen := 12 - len(keyText) // Assuming max key length of 12
			if padLen > 0 {
				for i := 0; i < padLen; i++ {
					padding += " "
				}
			}
			
			keyLine := keyStyle.Render(keyText) + padding + descStyle.Render("- " + descText)
			content = lipgloss.JoinVertical(lipgloss.Left, content, keyLine)
		}
		
		// Add spacing between categories
		content = lipgloss.JoinVertical(lipgloss.Left, content, "")
	}
	
	return content
}

func (h helpTypeInstanceStart) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Instance Created"),
		"",
		descStyle.Render("New session created:"),
		descStyle.Render(fmt.Sprintf("• Git branch: %s (isolated worktree)",
			lipgloss.NewStyle().Bold(true).Render(h.instance.Branch))),
		descStyle.Render(fmt.Sprintf("• %s running in background tmux session",
			lipgloss.NewStyle().Bold(true).Render(h.instance.Program))),
		"",
		headerStyle.Render("Managing:"),
		keyStyle.Render("↵")+descStyle.Render("   - Attach to the session to interact with it directly"),
		keyStyle.Render("tab")+descStyle.Render("   - Switch preview panes to view session diff"),
		keyStyle.Render("D")+descStyle.Render("     - Kill (delete) the selected session"),
		"",
		headerStyle.Render("Git Integration:"),
		keyStyle.Render("g")+descStyle.Render("     - Open git status (fugitive-style)"),
		keyStyle.Render("c")+descStyle.Render("     - Checkout this instance's branch"),
	)
	return content
}

func (h helpTypeInstanceAttach) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Attaching to Instance"),
		"",
		descStyle.Render("To detach from a session, press ")+keyStyle.Render("ctrl-q"),
	)
	return content
}

func (h helpTypeInstanceCheckout) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Checkout Instance"),
		"",
		"Changes will be committed locally. The branch name has been copied to your clipboard for you to checkout.",
		"",
		"Feel free to make changes to the branch and commit them. When resuming, the session will continue from where you left off.",
		"",
		headerStyle.Render("Commands:"),
		keyStyle.Render("c")+descStyle.Render(" - Checkout: commit changes locally and pause session"),
		keyStyle.Render("g")+descStyle.Render(" - Git status: manage staging and commits (fugitive-style)"),
		keyStyle.Render("r")+descStyle.Render(" - Resume a paused session"),
	)
	return content
}
func (h helpTypeGeneral) mask() uint32 {
	return 1
}

func (h helpTypeInstanceStart) mask() uint32 {
	return 1 << 1
}
func (h helpTypeInstanceAttach) mask() uint32 {
	return 1 << 2
}
func (h helpTypeInstanceCheckout) mask() uint32 {
	return 1 << 3
}

func (h helpTypeSessionOrganization) mask() uint32 {
	return 1 << 4
}

func (h helpTypeSessionOrganization) toContent() string {
	// Get organization category keys
	organizationKeys := keys.GetKeysInCategory(keys.HelpCategoryOrganize)
	// Get navigation keys related to organization
	navigationKeys := keys.GetKeysInCategory(keys.HelpCategoryNavigation)
	
	// Start with title and introduction
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Session Organization"),
		"",
		descStyle.Render("Claude Squad organizes your sessions by category for easier management."),
		"",
		headerStyle.Render("Categories:"),
		descStyle.Render("Sessions are organized by category, with uncategorized sessions in their own group."),
		"",
	)
	
	// Add Organization section
	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		headerStyle.Render("Organization:"),
	)
	
	// Add each organization key
	for _, keyName := range organizationKeys {
		keyBinding := keys.GlobalkeyBindings[keyName]
		helpInfo := keys.GetKeyHelp(keyName)
		
		// Format key and description
		keyText := keyBinding.Help().Key
		descText := helpInfo.Description
		
		// Add padding for alignment
		padding := ""
		padLen := 10 - len(keyText) // Assume max key text length of 10
		if padLen > 0 {
			for i := 0; i < padLen; i++ {
				padding += " "
			}
		}
		
		keyLine := keyStyle.Render(keyText) + padding + descStyle.Render("- " + descText)
		content = lipgloss.JoinVertical(lipgloss.Left, content, keyLine)
	}
	
	// Add Navigation section
	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		headerStyle.Render("Navigation:"),
	)
	
	// Add each navigation key
	for _, keyName := range navigationKeys {
		keyBinding := keys.GlobalkeyBindings[keyName]
		helpInfo := keys.GetKeyHelp(keyName)
		
		// Format key and description
		keyText := keyBinding.Help().Key
		descText := helpInfo.Description
		
		// Add padding for alignment
		padding := ""
		padLen := 10 - len(keyText) // Assume max key text length of 10
		if padLen > 0 {
			for i := 0; i < padLen; i++ {
				padding += " "
			}
		}
		
		keyLine := keyStyle.Render(keyText) + padding + descStyle.Render("- " + descText)
		content = lipgloss.JoinVertical(lipgloss.Left, content, keyLine)
	}
	
	// Add Search section
	content = lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		headerStyle.Render("Search:"),
		descStyle.Render("Type your search query and press Enter to find matching sessions."),
		descStyle.Render("Press Escape to cancel search mode."),
	)
	
	return content
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("#7D56F4"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#36CFC9"))
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFCC00"))
	descStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
)

// showHelpScreen displays the help screen overlay if it hasn't been shown before
func (m *home) showHelpScreen(helpType helpText, onDismiss func()) (tea.Model, tea.Cmd) {
	// Get the flag for this help type
	var alwaysShow bool
	switch helpType.(type) {
	case helpTypeGeneral:
		alwaysShow = true
	}

	flag := helpType.mask()

	// Check if this help screen has been seen before
	// Only show if we're showing the general help screen or the corresponding flag is not set
	// in the seen bitmask.
	if alwaysShow || (m.appState.GetHelpScreensSeen()&flag) == 0 {
		// Mark this help screen as seen and save state
		if err := m.appState.SetHelpScreensSeen(m.appState.GetHelpScreensSeen() | flag); err != nil {
			log.WarningLog.Printf("Failed to save help screen state: %v", err)
		}

		content := helpType.toContent()

		m.textOverlay = overlay.NewTextOverlay(content)
		m.textOverlay.OnDismiss = onDismiss
		m.state = stateHelp
		return m, nil
	}

	// Skip displaying the help screen
	if onDismiss != nil {
		onDismiss()
	}
	return m, nil
}

// handleHelpState handles key events when in help state
func (m *home) handleHelpState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key press will close the help overlay
	shouldClose := m.textOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.state = stateDefault
		return m, tea.Sequence(
			tea.WindowSize(),
			func() tea.Msg {
				m.menu.SetState(ui.StateDefault)
				return nil
			},
		)
	}

	return m, nil
}
