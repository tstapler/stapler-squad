package app

import (
	"claude-squad/terminal"

	tea "github.com/charmbracelet/bubbletea"
)

// setupSIGWINCHHandler sets up proper SIGWINCH signal handling for terminal resize events
// DEPRECATED: Use terminal.SignalManager.SetupResizeHandler() instead
func setupSIGWINCHHandler() tea.Cmd {
	// Create a temporary signal manager for backward compatibility
	termManager := terminal.NewManager()
	sigManager := terminal.NewSignalManager(termManager)
	return sigManager.SetupResizeHandler()
}

// sigwinchMsg is deprecated - use terminal.ResizeMsg instead
type sigwinchMsg = terminal.ResizeMsg

