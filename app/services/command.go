package services

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Command represents a framework-agnostic command that can be executed
// This abstraction allows services to return commands without coupling to BubbleTea
type Command interface {
	// Execute returns the underlying framework command
	// For BubbleTea, this returns tea.Cmd
	Execute() interface{}
}

// NoOpCommand represents a command that does nothing
type NoOpCommand struct{}

func (n NoOpCommand) Execute() interface{} {
	return nil
}

// BubbleTeaCommand wraps a BubbleTea command for framework compatibility
type BubbleTeaCommand struct {
	cmd tea.Cmd
}

func (b BubbleTeaCommand) Execute() interface{} {
	return b.cmd
}

// NewCommand creates a new framework-agnostic command from a BubbleTea command
func NewCommand(cmd tea.Cmd) Command {
	if cmd == nil {
		return NoOpCommand{}
	}
	return BubbleTeaCommand{cmd: cmd}
}

// ToTeaCmd converts a Command to a BubbleTea command
// This is used by the adapter layer when interfacing with BubbleTea
func ToTeaCmd(cmd Command) tea.Cmd {
	if cmd == nil {
		return nil
	}

	result := cmd.Execute()
	if result == nil {
		return nil
	}

	if teaCmd, ok := result.(tea.Cmd); ok {
		return teaCmd
	}

	return nil
}

// CommandFunc creates a Command from a function
// This allows for flexible command creation without direct BubbleTea coupling
type CommandFunc func() interface{}

func (f CommandFunc) Execute() interface{} {
	return f()
}

// Batch combines multiple commands into a single command
// Framework-agnostic version of tea.Batch
func Batch(cmds ...Command) Command {
	if len(cmds) == 0 {
		return NoOpCommand{}
	}

	return CommandFunc(func() interface{} {
		teaCmds := make([]tea.Cmd, 0, len(cmds))
		for _, cmd := range cmds {
			if cmd == nil {
				continue
			}
			if teaCmd := ToTeaCmd(cmd); teaCmd != nil {
				teaCmds = append(teaCmds, teaCmd)
			}
		}

		if len(teaCmds) == 0 {
			return nil
		}

		return tea.Batch(teaCmds...)
	})
}

// Sequence executes commands in sequence
// Each command waits for the previous to complete
func Sequence(cmds ...Command) Command {
	if len(cmds) == 0 {
		return NoOpCommand{}
	}

	return CommandFunc(func() interface{} {
		teaCmds := make([]tea.Cmd, 0, len(cmds))
		for _, cmd := range cmds {
			if cmd == nil {
				continue
			}
			if teaCmd := ToTeaCmd(cmd); teaCmd != nil {
				teaCmds = append(teaCmds, teaCmd)
			}
		}

		if len(teaCmds) == 0 {
			return nil
		}

		return tea.Sequence(teaCmds...)
	})
}
