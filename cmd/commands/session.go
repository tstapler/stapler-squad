package commands

import (
	"claude-squad/cmd/interfaces"
	"claude-squad/log"
	tea "github.com/charmbracelet/bubbletea"
)

// SessionHandlers contains handlers for session management commands
type SessionHandlers struct {
	// These will be set by the application when integrating with the new system
	OnNewSession       func() (tea.Model, tea.Cmd)
	OnKillSession      func() (tea.Model, tea.Cmd)
	OnAttachSession    func() (tea.Model, tea.Cmd)
	OnCheckout         func() (tea.Model, tea.Cmd)
	OnResume           func() (tea.Model, tea.Cmd)
	OnClaudeSettings   func() (tea.Model, tea.Cmd)
	OnTagEditor        func() (tea.Model, tea.Cmd)
	OnHistoryBrowser   func() (tea.Model, tea.Cmd)
	OnConfigEditor     func() (tea.Model, tea.Cmd)
	OnRenameSession    func() (tea.Model, tea.Cmd)
	OnRestartSession   func() (tea.Model, tea.Cmd)
	OnWorkspaceStatus  func() (tea.Model, tea.Cmd)
	OnWorkspaceSwitch  func() (tea.Model, tea.Cmd)
}

var sessionHandlers = &SessionHandlers{}

// SetSessionHandlers configures the session command handlers
func SetSessionHandlers(handlers *SessionHandlers) {
	sessionHandlers = handlers
}

// NewSessionCommand creates a new session
func NewSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnNewSession != nil {
		log.InfoLog.Printf("NewSessionCommand: calling OnNewSession handler")
		model, teaCmd := sessionHandlers.OnNewSession()
		log.InfoLog.Printf("NewSessionCommand: OnNewSession returned model: %v, cmd: %v", model != nil, teaCmd != nil)
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("NewSessionCommand: sessionHandlers.OnNewSession is nil!")
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

// ClaudeSettingsCommand opens Claude Code settings configuration
func ClaudeSettingsCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnClaudeSettings != nil {
		model, teaCmd := sessionHandlers.OnClaudeSettings()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// TagEditorCommand opens the tag editor for the selected session
func TagEditorCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnTagEditor != nil {
		model, teaCmd := sessionHandlers.OnTagEditor()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// HistoryBrowserCommand opens the history browser overlay
func HistoryBrowserCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnHistoryBrowser != nil {
		model, teaCmd := sessionHandlers.OnHistoryBrowser()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ConfigEditorCommand opens the Claude config editor overlay
func ConfigEditorCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnConfigEditor != nil {
		model, teaCmd := sessionHandlers.OnConfigEditor()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// RenameSessionCommand handles renaming a session
func RenameSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnRenameSession != nil {
		model, teaCmd := sessionHandlers.OnRenameSession()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// RestartSessionCommand handles restarting a session
func RestartSessionCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnRestartSession != nil {
		model, teaCmd := sessionHandlers.OnRestartSession()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// WorkspaceStatusCommand opens the workspace status overlay
func WorkspaceStatusCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnWorkspaceStatus != nil {
		model, teaCmd := sessionHandlers.OnWorkspaceStatus()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// WorkspaceSwitchCommand opens the workspace switch overlay
func WorkspaceSwitchCommand(ctx *interfaces.CommandContext) error {
	if sessionHandlers.OnWorkspaceSwitch != nil {
		model, teaCmd := sessionHandlers.OnWorkspaceSwitch()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
