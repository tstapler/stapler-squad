package commands

import (
	"claude-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// SessionHandlers contains handlers for session management commands
type SessionHandlers struct {
	// These will be set by the application when integrating with the new system
	OnNewSession    func() (tea.Model, tea.Cmd)
	OnKillSession   func() (tea.Model, tea.Cmd)
	OnAttachSession func() (tea.Model, tea.Cmd)
	OnCheckout      func() (tea.Model, tea.Cmd)
	OnResume        func() (tea.Model, tea.Cmd)
}

var sessionHandlers = &SessionHandlers{}

// SetSessionHandlers configures the session command handlers
func SetSessionHandlers(handlers *SessionHandlers) {
	sessionHandlers = handlers
}

// NewSessionCommand creates a new session
func NewSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnNewSession != nil {
		model, teaCmd := sessionHandlers.OnNewSession()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// KillSessionCommand kills/deletes the selected session
func KillSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnKillSession != nil {
		model, teaCmd := sessionHandlers.OnKillSession()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// AttachSessionCommand attaches to the selected session
func AttachSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnAttachSession != nil {
		model, teaCmd := sessionHandlers.OnAttachSession()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// CheckoutCommand commits changes and pauses the session
func CheckoutCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnCheckout != nil {
		model, teaCmd := sessionHandlers.OnCheckout()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ResumeCommand resumes a paused session
func ResumeCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnResume != nil {
		model, teaCmd := sessionHandlers.OnResume()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
