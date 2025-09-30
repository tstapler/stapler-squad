package app

import (
	"claude-squad/app/state"
	appui "claude-squad/app/ui"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// handleAdvancedSessionSetup handles entering the advanced session setup state
func (m *home) handleAdvancedSessionSetup() (tea.Model, tea.Cmd) {
	// Check if we're already at the instance limit
	if m.list.NumInstances() >= GlobalInstanceLimit {
		return m, m.handleError(
			fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
	}

	// Create the session setup overlay using coordinator
	if err := m.uiCoordinator.CreateSessionSetupOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create session setup overlay: %w", err))
	}

	// Set up the callbacks for the session setup overlay
	sessionSetupOverlay := m.uiCoordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay != nil {
		// Set up the cancel callback for escape key handling
		sessionSetupOverlay.SetOnCancel(func() {
			m.transitionToDefault()
			m.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)
		})

		// Set up the completion callback for enter key handling
		sessionSetupOverlay.SetOnComplete(func(options session.InstanceOptions) {
			// Set the tmux prefix from configuration
			cfg := config.LoadConfig()
			options.TmuxPrefix = cfg.TmuxSessionPrefix

			// Create the instance with the configured options
			instance, err := session.NewInstance(options)
			if err != nil {
				// Handle error - show error and stay in overlay state
				m.handleError(err)
				return
			}

			// Set pending session for async creation using existing pattern
			m.pendingSessionInstance = instance
			m.pendingAutoYes = false

			// Hide the overlay and transition to creating session state to show spinner
			m.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)
			m.transitionToCreatingSession()
		})

		// Ensure the overlay is focused so it can receive key events
		sessionSetupOverlay.Focus()
	}

	// Transition to advanced new state
	m.transitionToState(state.AdvancedNew)
	m.menu.SetState(ui.StateNewInstance)

	return m, nil
}

// handleAdvancedSessionSetupUpdate handles updates when in advanced session setup state
func (m *home) handleAdvancedSessionSetupUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Delegate to the session setup overlay
	sessionSetupOverlay := m.uiCoordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	// Let the overlay handle the key press
	// Note: SessionSetupOverlay uses callback pattern, not return values
	cmd := sessionSetupOverlay.Update(msg)
	return m, cmd
}