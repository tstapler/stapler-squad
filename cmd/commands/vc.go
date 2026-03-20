package commands

import (
	"github.com/tstapler/stapler-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// VCHandlers contains handlers for version control commands
type VCHandlers struct {
	OnVCStageFile       func() (tea.Model, tea.Cmd)
	OnVCUnstageFile     func() (tea.Model, tea.Cmd)
	OnVCStageAll        func() (tea.Model, tea.Cmd)
	OnVCUnstageAll      func() (tea.Model, tea.Cmd)
	OnVCOpenTerminal    func() (tea.Model, tea.Cmd)
	OnVCToggleHelp      func() (tea.Model, tea.Cmd)
	OnVCNavigateUp      func() (tea.Model, tea.Cmd)
	OnVCNavigateDown    func() (tea.Model, tea.Cmd)
	OnVCCommandPalette  func() (tea.Model, tea.Cmd)
}

var vcHandlers = &VCHandlers{}

// SetVCHandlers configures the VC command handlers
func SetVCHandlers(handlers *VCHandlers) {
	vcHandlers = handlers
}

// VCStageFileCommand stages the selected file
func VCStageFileCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCStageFile != nil {
		model, teaCmd := vcHandlers.OnVCStageFile()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCUnstageFileCommand unstages the selected file
func VCUnstageFileCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCUnstageFile != nil {
		model, teaCmd := vcHandlers.OnVCUnstageFile()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCStageAllCommand stages all files
func VCStageAllCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCStageAll != nil {
		model, teaCmd := vcHandlers.OnVCStageAll()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCUnstageAllCommand unstages all files
func VCUnstageAllCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCUnstageAll != nil {
		model, teaCmd := vcHandlers.OnVCUnstageAll()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCOpenTerminalCommand opens an interactive terminal for VCS operations
func VCOpenTerminalCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCOpenTerminal != nil {
		model, teaCmd := vcHandlers.OnVCOpenTerminal()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCToggleHelpCommand toggles help visibility in VC pane
func VCToggleHelpCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCToggleHelp != nil {
		model, teaCmd := vcHandlers.OnVCToggleHelp()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCNavigateUpCommand navigates up in VC file list
func VCNavigateUpCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCNavigateUp != nil {
		model, teaCmd := vcHandlers.OnVCNavigateUp()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCNavigateDownCommand navigates down in VC file list
func VCNavigateDownCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCNavigateDown != nil {
		model, teaCmd := vcHandlers.OnVCNavigateDown()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// VCCommandPaletteCommand opens the VC command palette
func VCCommandPaletteCommand(ctx *interfaces.CommandContext) error {
	if vcHandlers.OnVCCommandPalette != nil {
		model, teaCmd := vcHandlers.OnVCCommandPalette()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
