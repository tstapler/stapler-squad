package interfaces

import (
	tea "github.com/charmbracelet/bubbletea"
)

// CommandContext provides context and state to command handlers
type CommandContext struct {
	Command  interface{} // The actual command (opaque to avoid circular import)
	Args     map[string]interface{}
	AppState interface{}
	UIState  interface{}
	Key      string
}

// CommandHandler is the function signature for command implementations
type CommandHandler func(ctx *CommandContext) error

// ContextID identifies different application contexts/modes
type ContextID string

// CommandID uniquely identifies a command
type CommandID string

// Category groups related commands for help display
type Category string

// Standard application contexts
const (
	ContextGlobal    ContextID = "global"
	ContextList      ContextID = "list"
	ContextGitStatus ContextID = "git-status"
	ContextHelp      ContextID = "help"
	ContextPrompt    ContextID = "prompt"
	ContextSearch    ContextID = "search"
	ContextConfirm   ContextID = "confirm"
)

// Standard command categories
const (
	CategorySession      Category = "Session Management"
	CategoryGit          Category = "Git Integration"
	CategoryNavigation   Category = "Navigation"
	CategoryOrganization Category = "Organization"
	CategorySystem       Category = "System"
	CategoryLegacy       Category = "Legacy"
	CategorySpecial      Category = "Special" // Hidden from main help
)
