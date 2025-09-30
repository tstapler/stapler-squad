package session

import (
	"claude-squad/session"
	"claude-squad/ui/overlay"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// NewSession creates a new session through the advanced setup overlay
func (c *controller) NewSession() SessionOperation {
	result := SessionOperation{
		Type:    OpNewSession,
		Success: false,
	}

	// Validate that a new session can be created
	if err := c.ValidateNewSession(); err != nil {
		result.Error = err
		result.Cmd = c.deps.ErrorHandler(err)
		return result
	}

	// Create the new instance overlay
	newInstanceOverlay := overlay.NewSessionSetupOverlay()

	// Set completion callback
	newInstanceOverlay.SetOnComplete(func(options session.InstanceOptions) {
		// Set the tmux prefix from configuration
		options.TmuxPrefix = c.deps.TmuxPrefix

		// Create the instance with the configured options
		instance, err := session.NewInstance(options)
		if err != nil {
			c.deps.ErrorHandler(err)()
			c.deps.StateTransition("Default")
			c.deps.SetSessionSetupOverlay(nil)
			return
		}

		// Add the instance to the list
		finalizer := c.deps.List.AddInstance(instance)
		c.deps.SetNewInstanceFinalizer(finalizer)
		c.deps.List.SetSelectedInstance(c.deps.List.NumInstances() - 1)

		// Switch to creating session state
		c.deps.StateTransition("CreatingSession")

		// Close the setup overlay
		c.deps.SetSessionSetupOverlay(nil)

		// Set up the state for async session creation
		c.deps.SetPendingSession(instance, c.deps.AutoYes)
	})

	// Set cancellation callback
	newInstanceOverlay.SetOnCancel(func() {
		c.deps.SetSessionSetupOverlay(nil)
		c.deps.StateTransition("Default")
	})

	// Set the overlay and transition state
	c.deps.SetSessionSetupOverlay(newInstanceOverlay)
	c.deps.StateTransition("AdvancedNew")

	result.Success = true
	result.Cmd = tea.WindowSize()
	return result
}

// KillSession terminates the selected session after confirmation
func (c *controller) KillSession() SessionOperation {
	result := SessionOperation{
		Type:    OpKillSession,
		Success: false,
	}

	selected := c.GetSelectedSession()
	if selected == nil {
		result.Success = true // No-op if no selection
		return result
	}

	// Create kill action
	killAction := func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}

		checkedOut, err := worktree.IsBranchCheckedOut()
		if err != nil {
			return err
		}

		if checkedOut {
			return fmt.Errorf("instance %s is currently checked out", selected.Title)
		}

		if err := c.deps.Storage.DeleteInstance(selected.Title); err != nil {
			return err
		}

		c.deps.List.Kill()
		return instanceChangedMsg{}
	}

	// Show confirmation dialog
	message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
	result.Cmd = c.deps.ConfirmAction(message, killAction)
	result.Success = true
	return result
}

// AttachSession attaches to the selected session
func (c *controller) AttachSession() SessionOperation {
	result := SessionOperation{
		Type:    OpAttachSession,
		Success: false,
	}

	if !c.CanPerformOperation(OpAttachSession) {
		result.Success = true // No-op if conditions not met
		return result
	}

	// Show help screen and perform attach
	c.deps.ShowHelpScreen(helpTypeInstanceAttach{}, func() {
		ch, err := c.deps.List.Attach()
		if err != nil {
			c.deps.ErrorHandler(err)()
			return
		}
		<-ch
		c.deps.StateTransition("Default")
	})

	result.Success = true
	return result
}

// CheckoutSession pauses (checks out) the selected session
func (c *controller) CheckoutSession() SessionOperation {
	result := SessionOperation{
		Type:    OpCheckoutSession,
		Success: false,
	}

	selected := c.GetSelectedSession()
	if selected == nil {
		result.Success = true // No-op if no selection
		return result
	}

	// Show help screen and perform checkout
	c.deps.ShowHelpScreen(helpTypeInstanceCheckout{}, func() {
		if err := selected.Pause(); err != nil {
			c.deps.ErrorHandler(err)()
		}
		c.deps.InstanceChanged()()
	})

	result.Success = true
	return result
}

// ResumeSession resumes the selected paused session
func (c *controller) ResumeSession() SessionOperation {
	result := SessionOperation{
		Type:    OpResumeSession,
		Success: false,
	}

	selected := c.GetSelectedSession()
	if selected == nil {
		result.Success = true // No-op if no selection
		return result
	}

	if err := selected.Resume(); err != nil {
		result.Error = &SessionError{
			Operation: OpResumeSession,
			Message:   "failed to resume session",
			Cause:     err,
		}
		result.Cmd = c.deps.ErrorHandler(result.Error)
		return result
	}

	result.Success = true
	result.Cmd = tea.WindowSize()
	return result
}

// instanceChangedMsg is a message type for instance change notifications
type instanceChangedMsg struct{}

// helpTypeInstanceAttach represents the attach help screen type
type helpTypeInstanceAttach struct{}

// helpTypeInstanceCheckout represents the checkout help screen type
type helpTypeInstanceCheckout struct{}