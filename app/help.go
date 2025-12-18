package app

import (
	"claude-squad/app/state"
	"claude-squad/cmd"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpText interface {
	// toContent returns the help UI content.
	toContent() string
	// toContentWithBridge returns the help UI content with access to the command bridge for dynamic generation.
	toContentWithBridge(bridge *cmd.Bridge) string
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
	// Fallback static help when bridge is not available
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Claude Squad"),
		"",
		"A terminal UI that manages multiple Claude Code (and other local agents) in separate workspaces.",
		"",
		"Press ? for context-specific help.",
	)
}

func (h helpTypeGeneral) toContentWithBridge(bridge *cmd.Bridge) string {
	// Generate help dynamically from bridge's key categories
	currentContext := bridge.GetCurrentContext()

	// Create header
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Claude Squad"),
		"",
		"A terminal UI that manages multiple Claude Code (and other local agents) in separate workspaces.",
		"",
		headerStyle.Render(fmt.Sprintf("Context: %s", string(currentContext))),
		"",
	)

	// Get categories from bridge with configuration-based categorization
	categories := bridge.GetKeyCategories()

	// Style the keys in the category descriptions
	for categoryName, commands := range categories {
		styledCommands := make([]string, len(commands))
		for i, command := range commands {
			// Commands already formatted as "key - description", just need to style the key part
			parts := strings.SplitN(command, " - ", 2)
			if len(parts) == 2 {
				styledCommands[i] = fmt.Sprintf("%s - %s", keyStyle.Render(parts[0]), parts[1])
			} else {
				styledCommands[i] = command
			}
		}
		categories[categoryName] = styledCommands
	}

	// Add each category
	categoryOrder := []string{"Session Management", "Git Integration", "Navigation", "Organization", "System"}
	for _, category := range categoryOrder {
		if commands, exists := categories[category]; exists {
			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				headerStyle.Render(category+":"),
			)
			for _, command := range commands {
				content = lipgloss.JoinVertical(lipgloss.Left, content, "  "+command)
			}
			content = lipgloss.JoinVertical(lipgloss.Left, content, "")
		}
	}

	return content
}

func (h helpTypeInstanceStart) toContentWithBridge(bridge *cmd.Bridge) string {
	return h.toContent() // Instance help doesn't need dynamic generation
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

func (h helpTypeInstanceAttach) toContentWithBridge(bridge *cmd.Bridge) string {
	return h.toContent() // Instance help doesn't need dynamic generation
}

func (h helpTypeInstanceAttach) toContent() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Attaching to Instance"),
		"",
		descStyle.Render("To detach from a session, press ")+keyStyle.Render("ctrl-q"),
	)
	return content
}

func (h helpTypeInstanceCheckout) toContentWithBridge(bridge *cmd.Bridge) string {
	return h.toContent() // Instance help doesn't need dynamic generation
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

		var content string
		if m.bridge != nil {
			content = helpType.toContentWithBridge(m.bridge)
		} else {
			content = helpType.toContent()
		}

		m.textOverlay = overlay.NewTextOverlay(content)
		m.textOverlay.OnDismiss = onDismiss
		m.transitionToState(state.Help)
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
	// Handle messages overlay with vim-like navigation
	if m.messagesOverlay != nil {
		shouldClose := m.messagesOverlay.HandleKeyPress(msg)
		if shouldClose {
			m.messagesOverlay = nil
			m.transitionToDefault()
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

	// Handle text overlay (legacy help/text display)
	if m.textOverlay != nil {
		shouldClose := m.textOverlay.HandleKeyPress(msg)
		if shouldClose {
			m.textOverlay = nil
			m.transitionToDefault()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}
	}

	return m, nil
}
