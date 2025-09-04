package state

import (
	"fmt"

	"claude-squad/cmd"
	"claude-squad/cmd/help"
	tea "github.com/charmbracelet/bubbletea"
)

// Manager manages the modal context stack and command routing
type Manager struct {
	contextStack []cmd.ContextID
	registry     *cmd.CommandRegistry
	helpGen      *help.Generator

	// State that commands can access
	appState interface{}
	uiState  interface{}
}

// NewManager creates a new state manager
func NewManager(registry *cmd.CommandRegistry) *Manager {
	return &Manager{
		contextStack: []cmd.ContextID{cmd.ContextGlobal},
		registry:     registry,
		helpGen:      help.NewGenerator(registry),
	}
}

// SetAppState sets the application state that commands can access
func (sm *Manager) SetAppState(state interface{}) {
	sm.appState = state
}

// SetUIState sets the UI state that commands can access
func (sm *Manager) SetUIState(state interface{}) {
	sm.uiState = state
}

// PushContext adds a new context to the stack
func (sm *Manager) PushContext(ctx cmd.ContextID) {
	sm.contextStack = append(sm.contextStack, ctx)
}

// PopContext removes the top context from the stack and returns it
func (sm *Manager) PopContext() cmd.ContextID {
	if len(sm.contextStack) <= 1 {
		// Always keep at least the global context
		return cmd.ContextGlobal
	}

	ctx := sm.contextStack[len(sm.contextStack)-1]
	sm.contextStack = sm.contextStack[:len(sm.contextStack)-1]
	return ctx
}

// GetCurrentContext returns the current (top) context
func (sm *Manager) GetCurrentContext() cmd.ContextID {
	if len(sm.contextStack) == 0 {
		return cmd.ContextGlobal
	}
	return sm.contextStack[len(sm.contextStack)-1]
}

// GetContextStack returns a copy of the context stack
func (sm *Manager) GetContextStack() []cmd.ContextID {
	stack := make([]cmd.ContextID, len(sm.contextStack))
	copy(stack, sm.contextStack)
	return stack
}

// HandleKey processes a key press and executes the appropriate command
func (sm *Manager) HandleKey(key string) (tea.Model, tea.Cmd, error) {
	currentCtx := sm.GetCurrentContext()
	command := sm.registry.ResolveCommand(currentCtx, key)

	if command == nil {
		return nil, nil, fmt.Errorf("unknown key: '%s' in context %s", key, currentCtx)
	}

	// Create command context
	cmdCtx := &cmd.CommandContext{
		Command:  command,
		Args:     make(map[string]interface{}),
		AppState: sm.appState,
		UIState:  sm.uiState,
		Key:      key,
	}

	// Execute the command
	err := command.Handler(cmdCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("command %s failed: %w", command.ID, err)
	}

	// Commands can return tea.Model and tea.Cmd through the context
	var model tea.Model
	var teaCmd tea.Cmd

	if m, ok := cmdCtx.Args["model"]; ok {
		if teaModel, ok := m.(tea.Model); ok {
			model = teaModel
		}
	}

	if c, ok := cmdCtx.Args["cmd"]; ok {
		if tCmd, ok := c.(tea.Cmd); ok {
			teaCmd = tCmd
		}
	}

	return model, teaCmd, nil
}

// GetStatusLine generates the status line for the current context
func (sm *Manager) GetStatusLine() string {
	return sm.helpGen.GenerateStatusLine(sm.GetCurrentContext())
}

// GetHelpContent generates help content for the current context
func (sm *Manager) GetHelpContent() string {
	return sm.helpGen.GenerateContextHelp(sm.GetCurrentContext())
}

// GetQuickHelp generates quick help for overlays
func (sm *Manager) GetQuickHelp(maxItems int) []string {
	return sm.helpGen.GenerateQuickHelp(sm.GetCurrentContext(), maxItems)
}

// ValidateCommands validates the current command registry
func (sm *Manager) ValidateCommands() []string {
	return sm.helpGen.ValidateRegistry()
}

// IsKeyAvailable checks if a key is bound to a command in the current context
func (sm *Manager) IsKeyAvailable(key string) bool {
	currentCtx := sm.GetCurrentContext()
	command := sm.registry.ResolveCommand(currentCtx, key)
	return command != nil
}

// GetAvailableCommands returns all commands available in the current context
func (sm *Manager) GetAvailableCommands() []*cmd.Command {
	return sm.registry.GetCommandsForContext(sm.GetCurrentContext())
}

// GetCommandForKey returns the command bound to a key in the current context
func (sm *Manager) GetCommandForKey(key string) *cmd.Command {
	currentCtx := sm.GetCurrentContext()
	return sm.registry.ResolveCommand(currentCtx, key)
}

// ContextInfo provides information about a context
type ContextInfo struct {
	ID          cmd.ContextID
	Name        string
	Description string
	Parent      *cmd.ContextID
	Commands    []*cmd.Command
}

// GetCurrentContextInfo returns detailed information about the current context
func (sm *Manager) GetCurrentContextInfo() *ContextInfo {
	currentCtx := sm.GetCurrentContext()
	context, exists := sm.registry.GetContext(currentCtx)

	info := &ContextInfo{
		ID:       currentCtx,
		Commands: sm.registry.GetCommandsForContext(currentCtx),
	}

	if exists {
		info.Name = context.Name
		info.Description = context.Description
		info.Parent = context.Parent
	} else {
		info.Name = string(currentCtx)
	}

	return info
}
