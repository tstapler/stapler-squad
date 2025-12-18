package app

import (
	"claude-squad/app/state"
	appui "claude-squad/app/ui"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
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

	// Create the session setup overlay WITH callbacks at construction time
	// This is the proper way - callbacks are required and cannot be nil
	if err := m.uiCoordinator.CreateSessionSetupOverlay(overlay.SessionSetupCallbacks{
		OnComplete: func(options session.InstanceOptions) {
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
		},
		OnCancel: func() {
			m.transitionToDefault()
			m.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)
		},
	}); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create session setup overlay: %w", err))
	}

	// Get the overlay for focus control
	sessionSetupOverlay := m.uiCoordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay == nil {
		return m, m.handleError(fmt.Errorf("failed to get session setup overlay after creation"))
	}

	// Ensure the overlay is focused so it can receive key events
	sessionSetupOverlay.Focus()

	// NOW show the overlay after callbacks are configured
	// This marks it as active in the coordinator so RenderOverlay will work
	if err := m.uiCoordinator.ShowOverlay(appui.ComponentSessionSetupOverlay); err != nil {
		return m, m.handleError(fmt.Errorf("failed to show session setup overlay: %w", err))
	}

	// Transition to advanced new state with overlay configuration
	// IMPORTANT: Use transitionToOverlay to properly set up the view directive
	// that will call RenderOverlay on the now-active overlay
	if err := m.transitionToOverlay(state.AdvancedNew, "Default", "sessionSetup"); err != nil {
		return m, m.handleError(fmt.Errorf("failed to transition to session setup: %w", err))
	}

	// Menu state is set by transitionToOverlay's PostTransitionAction, but we
	// need to explicitly set it to StateNewInstance for proper command availability
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
