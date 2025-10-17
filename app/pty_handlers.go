package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"claude-squad/cmd"
	"claude-squad/log"
	"github.com/charmbracelet/lipgloss"
)

// PTY View Handler Methods
// These methods handle PTY list view operations

// handleTogglePTYView toggles between session list and PTY list view
func (h *home) handleTogglePTYView() (tea.Model, tea.Cmd) {
	if h.viewMode == ViewModeSessions {
		// Switch to PTY view
		h.viewMode = ViewModePTYs

		// Update PTY list with current sessions
		h.updatePTYList()

		// Update context for key bindings
		h.bridge.SetContext(cmd.ContextPTYList)
		h.updateMenuFromContext()

		log.InfoLog.Println("Switched to PTY list view")
	} else {
		// Switch back to session view
		h.viewMode = ViewModeSessions

		// Update context for key bindings
		h.bridge.SetContext(cmd.ContextList)
		h.updateMenuFromContext()

		log.InfoLog.Println("Switched to session list view")
	}

	return h, nil
}

// handleAttachPTY attaches to the selected PTY
func (h *home) handleAttachPTY() (tea.Model, tea.Cmd) {
	selected := h.ptyList.GetSelected()
	if selected == nil {
		return h, nil
	}

	log.InfoLog.Printf("Attaching to PTY: %s", selected.Path)

	// TODO: Implement actual PTY attachment
	// For now, just show a message
	return h, h.handleError(fmt.Errorf("PTY attachment not yet implemented"))
}

// handleSendCommandToPTY opens command input overlay for selected PTY
func (h *home) handleSendCommandToPTY() (tea.Model, tea.Cmd) {
	selected := h.ptyList.GetSelected()
	if selected == nil {
		return h, nil
	}

	log.InfoLog.Printf("Opening command input for PTY: %s", selected.Path)

	// TODO: Open CommandInputOverlay
	// For now, just show a message
	return h, h.handleError(fmt.Errorf("Command input not yet implemented"))
}

// handleDisconnectPTY disconnects from current PTY
func (h *home) handleDisconnectPTY() (tea.Model, tea.Cmd) {
	log.InfoLog.Println("Disconnecting from PTY")

	// TODO: Implement PTY disconnection
	return h, h.handleError(fmt.Errorf("PTY disconnection not yet implemented"))
}

// handleRefreshPTYs manually refreshes the PTY list
func (h *home) handleRefreshPTYs() (tea.Model, tea.Cmd) {
	log.InfoLog.Println("Refreshing PTY list")

	h.updatePTYList()

	return h, nil
}

// updatePTYList updates the PTY list with current data
func (h *home) updatePTYList() {
	// Update PTY discovery with current sessions
	sessions := h.getAllInstances()
	h.ptyDiscovery.SetSessions(sessions)

	// Trigger refresh
	if err := h.ptyDiscovery.Refresh(); err != nil {
		log.ErrorLog.Printf("Failed to refresh PTY list: %v", err)
		return
	}

	// Update PTY list component
	connections := h.ptyDiscovery.GetConnections()
	h.ptyList.SetConnections(connections)

	// Update preview with selected PTY
	if selected := h.ptyList.GetSelected(); selected != nil {
		h.ptyPreview.SetConnection(selected)
	}

	log.DebugLog.Printf("Updated PTY list: %d connections found", len(connections))
}

// renderReviewQueueView renders the review queue view
func (h *home) renderReviewQueueView() string {
	queueContent := h.queueView.View()

	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		queueContent,
		h.menu.String(),
		h.statusBar.View(80), // TODO: use actual terminal width
		h.errBox.String(),
	)

	return mainView
}

// renderPTYView renders the PTY list view with split-pane layout
func (h *home) renderPTYView() string {
	// Get rendered content from components (they use their configured sizes from updateHandleWindowSizeEvent)
	ptyListContent := h.ptyList.Render()
	ptyPreviewContent := h.ptyPreview.Render()

	// Join horizontally for split view
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, ptyListContent, ptyPreviewContent)

	// Add menu, status bar, and error box at bottom
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		listAndPreview,
		h.menu.String(),
		h.statusBar.View(80), // TODO: use actual terminal width
		h.errBox.String(),
	)

	return mainView
}
