package commands

import (
	"claude-squad/cmd/interfaces"
	tea "github.com/charmbracelet/bubbletea"
)

// NavigationHandlers contains handlers for navigation commands
type NavigationHandlers struct {
	OnUp       func() (tea.Model, tea.Cmd)
	OnDown     func() (tea.Model, tea.Cmd)
	OnLeft     func() (tea.Model, tea.Cmd)
	OnRight    func() (tea.Model, tea.Cmd)
	OnPageUp   func() (tea.Model, tea.Cmd)
	OnPageDown func() (tea.Model, tea.Cmd)
	OnSearch   func() (tea.Model, tea.Cmd)
}

var navigationHandlers = &NavigationHandlers{}

// SetNavigationHandlers configures the navigation command handlers
func SetNavigationHandlers(handlers *NavigationHandlers) {
	navigationHandlers = handlers
}

// UpCommand moves selection up
func UpCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnUp != nil {
		model, teaCmd := navigationHandlers.OnUp()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// DownCommand moves selection down
func DownCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnDown != nil {
		model, teaCmd := navigationHandlers.OnDown()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// LeftCommand moves selection left or collapses categories
func LeftCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnLeft != nil {
		model, teaCmd := navigationHandlers.OnLeft()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// RightCommand moves selection right or expands categories
func RightCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnRight != nil {
		model, teaCmd := navigationHandlers.OnRight()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// PageUpCommand scrolls up by page
func PageUpCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnPageUp != nil {
		model, teaCmd := navigationHandlers.OnPageUp()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// PageDownCommand scrolls down by page
func PageDownCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnPageDown != nil {
		model, teaCmd := navigationHandlers.OnPageDown()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}

// SearchCommand enters search mode
func SearchCommand(ctx *interfaces.CommandContext) error {
	if navigationHandlers.OnSearch != nil {
		model, teaCmd := navigationHandlers.OnSearch()
		ctx.Args["model"] = model
		ctx.Args["cmd"] = teaCmd
	}
	return nil
}
