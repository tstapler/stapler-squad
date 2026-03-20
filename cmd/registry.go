package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/cmd/interfaces"
	"github.com/tstapler/stapler-squad/log"
)

// Use types from interfaces package to avoid duplication
type CommandID = interfaces.CommandID
type ContextID = interfaces.ContextID
type Category = interfaces.Category

// CommandRegistry is the central registry for all commands and keybindings
type CommandRegistry struct {
	mu       sync.RWMutex
	commands map[CommandID]*Command
	contexts map[ContextID]*Context
	bindings map[ContextID]map[string]CommandID // context -> key -> command mapping
}

// Command represents a user action that can be triggered by keybindings
type Command struct {
	ID          CommandID
	Name        string
	Description string
	Category    Category
	Handler     CommandHandler
	Contexts    []ContextID

	// Optional fields
	Aliases       []string
	Deprecated    *DeprecationInfo
	Prerequisites []CommandID

	// Internal tracking
	keys []string // All keys bound to this command
}

// Context represents an application mode or state
type Context struct {
	ID          ContextID
	Name        string
	Parent      *ContextID
	Description string
}

// CommandHandler is the function signature for command implementations
type CommandHandler func(ctx *interfaces.CommandContext) error

// CommandContext is now defined in interfaces package to avoid import cycles

// DeprecationInfo tracks deprecated commands and their alternatives
type DeprecationInfo struct {
	Message     string
	Alternative CommandID
	RemoveIn    string
}

// KeyConflict represents a keybinding conflict within a context
type KeyConflict struct {
	Key      string
	Context  ContextID
	Commands []CommandID
}

// NewCommandRegistry creates a new command registry
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[CommandID]*Command),
		contexts: make(map[ContextID]*Context),
		bindings: make(map[ContextID]map[string]CommandID),
	}
}

// Global registry instance
var globalRegistry *CommandRegistry
var registryOnce sync.Once

// GetCommandRegistry returns the global command registry
func GetCommandRegistry() *CommandRegistry {
	registryOnce.Do(func() {
		globalRegistry = NewCommandRegistry()
	})
	return globalRegistry
}

// RegisterContext adds a new context to the registry
func (r *CommandRegistry) RegisterContext(ctx *Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.contexts[ctx.ID]; exists {
		return fmt.Errorf("context %s already registered", ctx.ID)
	}

	r.contexts[ctx.ID] = ctx

	// Initialize bindings map for this context
	if r.bindings[ctx.ID] == nil {
		r.bindings[ctx.ID] = make(map[string]CommandID)
	}

	return nil
}

// Register adds a command to the registry and returns a builder for further configuration
func (r *CommandRegistry) Register(cmd *Command) *CommandBuilder {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate required fields
	if cmd.ID == "" {
		panic("command ID cannot be empty")
	}
	if cmd.Handler == nil {
		panic("command handler cannot be nil")
	}

	// Store the command
	r.commands[cmd.ID] = cmd

	// Initialize keys slice
	cmd.keys = make([]string, 0)

	return &CommandBuilder{
		registry: r,
		command:  cmd,
	}
}

// CommandBuilder provides a fluent interface for configuring commands
type CommandBuilder struct {
	registry *CommandRegistry
	command  *Command
}

// BindKey binds a single key to the command in all its contexts
func (cb *CommandBuilder) BindKey(key string) *CommandBuilder {
	return cb.BindKeys(key)
}

// BindKeys binds multiple keys to the command in all its contexts
func (cb *CommandBuilder) BindKeys(keys ...string) *CommandBuilder {
	cb.registry.mu.Lock()
	defer cb.registry.mu.Unlock()

	for _, key := range keys {
		// Add to command's key list
		cb.command.keys = append(cb.command.keys, key)

		// Bind in all contexts where this command is available
		for _, contextID := range cb.command.Contexts {
			if cb.registry.bindings[contextID] == nil {
				cb.registry.bindings[contextID] = make(map[string]CommandID)
			}
			cb.registry.bindings[contextID][key] = cb.command.ID
		}
	}

	return cb
}

// BindKeyInContext binds a key to the command only in specific contexts
func (cb *CommandBuilder) BindKeyInContext(key string, contexts ...ContextID) *CommandBuilder {
	cb.registry.mu.Lock()
	defer cb.registry.mu.Unlock()

	cb.command.keys = append(cb.command.keys, key)

	for _, contextID := range contexts {
		if cb.registry.bindings[contextID] == nil {
			cb.registry.bindings[contextID] = make(map[string]CommandID)
		}
		cb.registry.bindings[contextID][key] = cb.command.ID
	}

	return cb
}

// DeprecateKey marks a specific key binding as deprecated
func (cb *CommandBuilder) DeprecateKey(key, message string) *CommandBuilder {
	// For now, just mark the whole command as deprecated if any key is deprecated
	// In the future, we could track per-key deprecation
	if cb.command.Deprecated == nil {
		cb.command.Deprecated = &DeprecationInfo{
			Message: message,
		}
	}
	return cb
}

// SetAlternative sets the alternative command for deprecated commands
func (cb *CommandBuilder) SetAlternative(altID CommandID) *CommandBuilder {
	if cb.command.Deprecated != nil {
		cb.command.Deprecated.Alternative = altID
	}
	return cb
}

// ResolveCommand finds the command bound to a key in a given context
func (r *CommandRegistry) ResolveCommand(contextID ContextID, key string) *Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	log.DebugLog.Printf("ResolveCommand: contextID=%s, key=%s", contextID, key)

	// First check the specific context
	if bindings, exists := r.bindings[contextID]; exists {
		log.DebugLog.Printf("ResolveCommand: found bindings for context %s, contains %d keys", contextID, len(bindings))
		if cmdID, found := bindings[key]; found {
			log.DebugLog.Printf("ResolveCommand: found command %s for key %s", cmdID, key)
			return r.commands[cmdID]
		}
		log.DebugLog.Printf("ResolveCommand: key %s not found in context %s bindings", key, contextID)
	} else {
		log.DebugLog.Printf("ResolveCommand: no bindings exist for context %s", contextID)
	}

	// If not found, check parent contexts
	if context, exists := r.contexts[contextID]; exists && context.Parent != nil {
		log.DebugLog.Printf("ResolveCommand: checking parent context %s", *context.Parent)
		return r.ResolveCommand(*context.Parent, key)
	}

	log.DebugLog.Printf("ResolveCommand: no command found for key %s in context %s", key, contextID)
	return nil
}

// GetCommandsForContext returns all commands available in a context (including inherited)
func (r *CommandRegistry) GetCommandsForContext(contextID ContextID) []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var commands []*Command
	seen := make(map[CommandID]bool)

	r.collectCommandsForContext(contextID, &commands, seen)
	return commands
}

// collectCommandsForContext recursively collects commands from context hierarchy
func (r *CommandRegistry) collectCommandsForContext(contextID ContextID, commands *[]*Command, seen map[CommandID]bool) {
	// Get commands from current context
	if bindings, exists := r.bindings[contextID]; exists {
		for _, cmdID := range bindings {
			if !seen[cmdID] {
				seen[cmdID] = true
				if cmd, exists := r.commands[cmdID]; exists {
					*commands = append(*commands, cmd)
				}
			}
		}
	}

	// Get commands from parent context
	if context, exists := r.contexts[contextID]; exists && context.Parent != nil {
		r.collectCommandsForContext(*context.Parent, commands, seen)
	}
}

// DetectConflicts finds keybinding conflicts within contexts
func (r *CommandRegistry) DetectConflicts() []KeyConflict {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var conflicts []KeyConflict

	for contextID, bindings := range r.bindings {
		keyCommands := make(map[string][]CommandID)

		// Group commands by key
		for key, cmdID := range bindings {
			keyCommands[key] = append(keyCommands[key], cmdID)
		}

		// Find conflicts (multiple commands for same key)
		for key, cmdIDs := range keyCommands {
			if len(cmdIDs) > 1 {
				conflicts = append(conflicts, KeyConflict{
					Key:      key,
					Context:  contextID,
					Commands: cmdIDs,
				})
			}
		}
	}

	return conflicts
}

// GetCommand retrieves a command by ID
func (r *CommandRegistry) GetCommand(id CommandID) (*Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, exists := r.commands[id]
	return cmd, exists
}

// GetContext retrieves a context by ID
func (r *CommandRegistry) GetContext(id ContextID) (*Context, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ctx, exists := r.contexts[id]
	return ctx, exists
}

// GetAllCommands returns all registered commands
func (r *CommandRegistry) GetAllCommands() map[CommandID]*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[CommandID]*Command)
	for id, cmd := range r.commands {
		result[id] = cmd
	}
	return result
}

// GetKeysForCommand returns all keys bound to a command
func (r *CommandRegistry) GetKeysForCommand(cmdID CommandID) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if cmd, exists := r.commands[cmdID]; exists {
		// Return a copy to prevent external modification
		keys := make([]string, len(cmd.keys))
		copy(keys, cmd.keys)
		return keys
	}
	return nil
}

// String returns a debug string representation of the registry
func (r *CommandRegistry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("CommandRegistry:\n")

	sb.WriteString("  Contexts:\n")
	for id, ctx := range r.contexts {
		sb.WriteString(fmt.Sprintf("    %s: %s\n", id, ctx.Name))
	}

	sb.WriteString("  Commands:\n")
	for id, cmd := range r.commands {
		sb.WriteString(fmt.Sprintf("    %s: %s (%v)\n", id, cmd.Name, cmd.keys))
	}

	return sb.String()
}
