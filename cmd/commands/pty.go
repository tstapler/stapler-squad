package commands

import (
	"github.com/tstapler/stapler-squad/cmd/interfaces"
	"github.com/tstapler/stapler-squad/log"
	tea "github.com/charmbracelet/bubbletea"
)

// PTYHandlers contains handlers for PTY management commands
type PTYHandlers struct {
	OnTogglePTYView func() (tea.Model, tea.Cmd)
	OnAttachPTY     func() (tea.Model, tea.Cmd)
	OnSendCommand   func() (tea.Model, tea.Cmd)
	OnDisconnectPTY func() (tea.Model, tea.Cmd)
	OnRefreshPTYs   func() (tea.Model, tea.Cmd)
}

var ptyHandlers = &PTYHandlers{}

// SetPTYHandlers configures the PTY command handlers
func SetPTYHandlers(handlers *PTYHandlers) {
	ptyHandlers = handlers
}

// TogglePTYViewCommand toggles between session list and PTY list view
func TogglePTYViewCommand(ctx *interfaces.CommandContext) error {
	if ptyHandlers.OnTogglePTYView != nil {
		log.InfoLog.Printf("TogglePTYViewCommand: toggling PTY view")
		model, teaCmd := ptyHandlers.OnTogglePTYView()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("TogglePTYViewCommand: handler not set")
	}
	return nil
}

// AttachPTYCommand attaches to the selected PTY
func AttachPTYCommand(ctx *interfaces.CommandContext) error {
	if ptyHandlers.OnAttachPTY != nil {
		log.InfoLog.Printf("AttachPTYCommand: attaching to PTY")
		model, teaCmd := ptyHandlers.OnAttachPTY()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("AttachPTYCommand: handler not set")
	}
	return nil
}

// SendCommandToPTYCommand opens the command input overlay for the selected PTY
func SendCommandToPTYCommand(ctx *interfaces.CommandContext) error {
	if ptyHandlers.OnSendCommand != nil {
		log.InfoLog.Printf("SendCommandToPTYCommand: opening command input")
		model, teaCmd := ptyHandlers.OnSendCommand()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("SendCommandToPTYCommand: handler not set")
	}
	return nil
}

// DisconnectPTYCommand disconnects from the current PTY
func DisconnectPTYCommand(ctx *interfaces.CommandContext) error {
	if ptyHandlers.OnDisconnectPTY != nil {
		log.InfoLog.Printf("DisconnectPTYCommand: disconnecting PTY")
		model, teaCmd := ptyHandlers.OnDisconnectPTY()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("DisconnectPTYCommand: handler not set")
	}
	return nil
}

// RefreshPTYsCommand manually refreshes the PTY list
func RefreshPTYsCommand(ctx *interfaces.CommandContext) error {
	if ptyHandlers.OnRefreshPTYs != nil {
		log.InfoLog.Printf("RefreshPTYsCommand: refreshing PTY list")
		model, teaCmd := ptyHandlers.OnRefreshPTYs()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	} else {
		log.InfoLog.Printf("RefreshPTYsCommand: handler not set")
	}
	return nil
}
