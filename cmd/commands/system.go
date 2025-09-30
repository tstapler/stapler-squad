package commands

import (
	"claude-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// SystemHandlers contains handlers for system commands
type SystemHandlers struct {
	OnHelp        func() (tea.Model, tea.Cmd)
	OnQuit        func() (tea.Model, tea.Cmd)
	OnEscape      func() (tea.Model, tea.Cmd)
	OnTab         func() (tea.Model, tea.Cmd)
	OnConfirm     func() (tea.Model, tea.Cmd)
	OnCommandMode func() (tea.Model, tea.Cmd)
}

var systemHandlers = &SystemHandlers{}

// SetSystemHandlers configures the system command handlers
func SetSystemHandlers(handlers *SystemHandlers) {
	systemHandlers = handlers
}

// HelpCommand shows help
func HelpCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnHelp != nil {
		model, teaCmd := systemHandlers.OnHelp()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// QuitCommand quits the application
func QuitCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnQuit != nil {
		model, teaCmd := systemHandlers.OnQuit()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// EscapeCommand handles escape key
func EscapeCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnEscape != nil {
		model, teaCmd := systemHandlers.OnEscape()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// TabCommand handles tab key
func TabCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnTab != nil {
		model, teaCmd := systemHandlers.OnTab()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// ConfirmCommand handles confirmation in dialogs
func ConfirmCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnConfirm != nil {
		model, teaCmd := systemHandlers.OnConfirm()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// CommandModeCommand enters vim-style command mode
func CommandModeCommand(ctx *interfaces.CommandContext) error {
	if systemHandlers.OnCommandMode != nil {
		model, teaCmd := systemHandlers.OnCommandMode()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
