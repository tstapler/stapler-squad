package app

import (
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
	
	// Create the session setup overlay if it doesn't exist
	if m.sessionSetupOverlay == nil {
		m.sessionSetupOverlay = overlay.NewSessionSetupOverlay()
		
		// Set callbacks
		m.sessionSetupOverlay.SetOnComplete(func(opts session.InstanceOptions) {
			// Create the instance with the configured options
			instance, err := session.NewInstance(opts)
			if err != nil {
				m.errBox.SetError(err)
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				return
			}
			
			// Add the instance to the list
			m.newInstanceFinalizer = m.list.AddInstance(instance)
			m.list.SetSelectedInstance(m.list.NumInstances() - 1)
			
			// Switch to creating session state
			m.state = stateCreatingSession
			m.asyncSessionCreationActive = true
			m.menu.SetState(ui.StateCreatingInstance)
			
			// Start session creation in a background goroutine
			go func() {
				// Start the instance
				err := instance.Start(true)
				
				// Store the error and let the status update mechanism handle it
				// Set an error message if there was a problem
				if err != nil {
					m.errBox.SetError(err)
				}
			}()
		})
		
		m.sessionSetupOverlay.SetOnCancel(func() {
			// Cancel the advanced setup and go back to default state
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
		})
	}
	
	// Use a reasonable default size for the overlay
	// We can't access the internal width/height directly
	// The app.go already handles window resize events and updates the overlay size
	m.sessionSetupOverlay.SetSize(
		80, 
		24)
	
	// Focus the overlay
	m.sessionSetupOverlay.Focus()
	
	// Set the state
	m.state = stateAdvancedNew
	m.menu.SetState(ui.StateAdvancedNew)
	
	return m, nil
}

// handleAdvancedSessionSetupUpdate handles updates to the advanced session setup overlay
func (m *home) handleAdvancedSessionSetupUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward the message to the session setup overlay
	cmd := m.sessionSetupOverlay.Update(msg)
	return m, cmd
}