package cmd

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/cmd/interfaces"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// Bridge provides compatibility between old and new command systems
type Bridge struct {
	registry *CommandRegistry
	config   *config.Config

	// Context management (inline to avoid import cycles)
	contextStack []ContextID

	// Legacy mappings - disabled since keys package is removed
	initialized atomic.Bool

	// Performance optimization: cache expensive help generation per context
	cacheMutex         sync.RWMutex
	keyCategoriesCache map[ContextID]map[string][]string
}

// NewBridge creates a new migration bridge
func NewBridge() *Bridge {
	registry := GetGlobalRegistry()
	cfg := config.LoadConfig()

	// Debug: Check if registry is properly initialized
	allCommands := registry.GetAllCommands()
	log.Info("bridge initialized", "command_count", len(allCommands))

	// Check for navigation commands specifically
	upCmd := registry.ResolveCommand(ContextList, "up")
	downCmd := registry.ResolveCommand(ContextList, "down")
	log.Info("navigation commands resolved", "up", upCmd != nil, "down", downCmd != nil)

	bridge := &Bridge{
		registry:     registry,
		config:       cfg,
		contextStack: []ContextID{ContextGlobal}, // Start with global context

		keyCategoriesCache: make(map[ContextID]map[string][]string),
	}

	// Pre-warm the key categories cache in background to avoid delay on first help display
	go bridge.prewarmKeyCategories()

	return bridge
}

// GetCurrentContext returns the current context
func (b *Bridge) GetCurrentContext() ContextID {
	if len(b.contextStack) == 0 {
		return ContextGlobal
	}
	return b.contextStack[len(b.contextStack)-1]
}

// GetRegistry returns the command registry
func (b *Bridge) GetRegistry() *CommandRegistry {
	return b.registry
}

// HandleLegacyKey is disabled since legacy keys package has been removed
func (b *Bridge) HandleLegacyKey(keyName interface{}) error {
	// Legacy key handling disabled - use HandleKeyString directly
	return nil
}

// HandleKeyString processes a key string through the new command system
func (b *Bridge) HandleKeyString(key string) error {
	currentContext := b.GetCurrentContext()
	log.Debug("HandleKeyString", "key", key, "context", currentContext)
	command := b.registry.ResolveCommand(currentContext, key)
	log.Debug("HandleKeyString command resolved", "found", command != nil, "has_handler", command != nil && command.Handler != nil)

	if command != nil && command.Handler != nil {
		// Create command context
		ctx := &interfaces.CommandContext{
			Args: make(map[string]interface{}),
		}

		// Execute the command
		return command.Handler(ctx)
	}

	return nil
}

// GetLegacyStatusLine generates status line compatible with old menu system
func (b *Bridge) GetLegacyStatusLine() string {
	// TODO: Generate status line from current context commands
	return "Command system active"
}

// GetContextualHelp generates help for current context
func (b *Bridge) GetContextualHelp() string {
	return "Help system temporarily disabled - using legacy help"
}

// SetContext switches to a different application context
func (b *Bridge) SetContext(contextID ContextID) {
	log.Info("SetContext: changing context", "from", b.GetCurrentContext(), "to", contextID)
	// Clear stack and set new context
	b.contextStack = []ContextID{ContextGlobal}
	if contextID != ContextGlobal {
		b.contextStack = append(b.contextStack, contextID)
	}
	log.Info("SetContext: context stack updated", "stack", b.contextStack)
	// No need to invalidate cache - it's per-context
}

// PushContext adds a context to the stack (for modal operations)
func (b *Bridge) PushContext(contextID ContextID) {
	b.contextStack = append(b.contextStack, contextID)
	// No need to invalidate cache - it's per-context
}

// PopContext removes the top context from the stack
func (b *Bridge) PopContext() ContextID {
	if len(b.contextStack) <= 1 {
		return ContextGlobal
	}
	popped := b.contextStack[len(b.contextStack)-1]
	b.contextStack = b.contextStack[:len(b.contextStack)-1]
	// No need to invalidate cache - it's per-context
	return popped
}

// ValidateSetup checks if the bridge is properly configured
func (b *Bridge) ValidateSetup() []string {
	var issues []string

	if !b.initialized.Load() {
		issues = append(issues, "Bridge not initialized - call Initialize() first")
	}

	// Check for key conflicts across all contexts
	allConflicts := b.ValidateAllContexts()
	for contextID, conflicts := range allConflicts {
		for _, conflict := range conflicts {
			issues = append(issues, fmt.Sprintf("Context %s: %s", contextID, conflict))
		}
	}

	return issues
}

// GetAvailableKeys returns all keys available in the current context
func (b *Bridge) GetAvailableKeys() map[string]string {
	commands := b.registry.GetCommandsForContext(b.GetCurrentContext())
	keyMap := make(map[string]string)

	for _, command := range commands {
		keys := b.registry.GetKeysForCommand(command.ID)
		for _, key := range keys {
			keyMap[key] = command.Description
		}
	}

	return keyMap
}

// GetAvailableKeysForInstance returns keys available based on instance permissions
// This filters commands to only show what the user is allowed to execute for the given instance
func (b *Bridge) GetAvailableKeysForInstance(instance interfaces.Instance) map[string]string {
	// Get all commands for current context
	commands := b.registry.GetCommandsForContext(b.GetCurrentContext())

	// Get instance permissions
	perms := instance.GetPermissions()

	// Filter commands based on permissions
	filtered := FilterCommandsByPermissions(commands, perms)

	// Build key map from filtered commands
	keyMap := make(map[string]string)
	for _, command := range filtered {
		keys := b.registry.GetKeysForCommand(command.ID)
		for _, key := range keys {
			keyMap[key] = command.Description
		}
	}

	return keyMap
}

// IsKeyBound checks if a key is bound to any command in current context
func (b *Bridge) IsKeyBound(key string) bool {
	command := b.registry.ResolveCommand(b.GetCurrentContext(), key)
	return command != nil
}

// GetKeyCategories returns keys organized by category for dynamic help generation
// Uses per-context caching to avoid expensive registry lookups on every help display
func (b *Bridge) GetKeyCategories() map[string][]string {
	currentContext := b.GetCurrentContext()

	// Check if we have cached result for this context (thread-safe read)
	b.cacheMutex.RLock()
	if cached, exists := b.keyCategoriesCache[currentContext]; exists {
		b.cacheMutex.RUnlock()
		return cached
	}
	b.cacheMutex.RUnlock()

	// Rebuild cache for this context
	commands := b.registry.GetCommandsForContext(currentContext)
	categories := make(map[string][]string)

	for _, command := range commands {
		if command == nil {
			continue
		}
		keys := b.registry.GetKeysForCommand(command.ID)
		for _, key := range keys {
			keyDesc := fmt.Sprintf("%s - %s", key, command.Description)

			// Use configuration-based categorization first
			if b.config != nil && b.config.KeyCategories != nil {
				if categoryName, exists := b.config.KeyCategories[key]; exists {
					categories[categoryName] = append(categories[categoryName], keyDesc)
					continue
				}
			}

			// Fallback to command category
			categoryName := string(command.Category)
			if categoryName == "" {
				categoryName = "Other"
			}
			categories[categoryName] = append(categories[categoryName], keyDesc)
		}
	}

	// Cache the result for this context (thread-safe write)
	b.cacheMutex.Lock()
	b.keyCategoriesCache[currentContext] = categories
	b.cacheMutex.Unlock()

	return categories
}

// GetCommandForKey returns the command bound to a key
func (b *Bridge) GetCommandForKey(key string) *Command {
	return b.registry.ResolveCommand(b.GetCurrentContext(), key)
}

// ReloadConfig refreshes the configuration from disk
func (b *Bridge) ReloadConfig() {
	b.config = config.LoadConfig()
	// Invalidate cache since configuration may have changed key categories
	b.invalidateKeyCategories()
}

// invalidateKeyCategories clears all cached key categories
func (b *Bridge) invalidateKeyCategories() {
	b.cacheMutex.Lock()
	b.keyCategoriesCache = make(map[ContextID]map[string][]string)
	b.cacheMutex.Unlock()
}

// prewarmKeyCategories populates the key categories cache in background during startup
func (b *Bridge) prewarmKeyCategories() {
	// Wait for registry to be fully initialized
	// This ensures all commands and contexts are registered before we cache
	if !b.initialized.Load() {
		// If not initialized yet, we'll cache after Initialize() is called
		return
	}

	// Pre-populate cache for common contexts to avoid delay on first help display
	commonContexts := []ContextID{ContextGlobal, ContextList, ContextPrompt, ContextHelp, ContextSearch, ContextConfirm}

	// Build cache for each context WITHOUT modifying the bridge's active context
	// This avoids race conditions with the main thread's context management
	for _, contextID := range commonContexts {
		// Directly build the cache without changing contextStack
		// by calling GetCommandsForContext which doesn't depend on current context
		commands := b.registry.GetCommandsForContext(contextID)
		categories := make(map[string][]string)

		for _, command := range commands {
			if command == nil {
				continue
			}
			keys := b.registry.GetKeysForCommand(command.ID)
			for _, key := range keys {
				keyDesc := fmt.Sprintf("%s - %s", key, command.Description)

				// Use configuration-based categorization first
				if b.config != nil && b.config.KeyCategories != nil {
					if categoryName, exists := b.config.KeyCategories[key]; exists {
						categories[categoryName] = append(categories[categoryName], keyDesc)
						continue
					}
				}

				// Fallback to command category
				categoryName := string(command.Category)
				if categoryName == "" {
					categoryName = "Other"
				}
				categories[categoryName] = append(categories[categoryName], keyDesc)
			}
		}

		// Cache the result for this context (thread-safe write)
		b.cacheMutex.Lock()
		b.keyCategoriesCache[contextID] = categories
		b.cacheMutex.Unlock()
	}

	log.Debug("prewarmed key categories cache", "context_count", len(commonContexts))
}

// DetectKeyConflicts checks for duplicate key bindings within the current context
func (b *Bridge) DetectKeyConflicts() []string {
	var conflicts []string
	currentContext := b.GetCurrentContext()

	// Get all bindings for current context
	bindings := b.registry.bindings[currentContext]
	if bindings == nil {
		return conflicts // No bindings in this context
	}

	// Track keys that have multiple commands
	keyCommandCount := make(map[string][]CommandID)
	for key, commandID := range bindings {
		keyCommandCount[key] = append(keyCommandCount[key], commandID)
	}

	// Identify conflicts
	for key, commandIDs := range keyCommandCount {
		if len(commandIDs) > 1 {
			commandNames := make([]string, len(commandIDs))
			for i, cmdID := range commandIDs {
				if cmd := b.registry.commands[cmdID]; cmd != nil {
					commandNames[i] = cmd.Name
				} else {
					commandNames[i] = string(cmdID)
				}
			}
			conflicts = append(conflicts, fmt.Sprintf("Key conflict in %s: '%s' bound to [%s]",
				string(currentContext), key, strings.Join(commandNames, ", ")))
		}
	}

	return conflicts
}

// ValidateAllContexts checks for key conflicts across all contexts
func (b *Bridge) ValidateAllContexts() map[string][]string {
	allConflicts := make(map[string][]string)

	for contextID := range b.registry.contexts {
		// Temporarily switch context to check conflicts
		originalContext := b.GetCurrentContext()
		b.contextStack[len(b.contextStack)-1] = contextID

		conflicts := b.DetectKeyConflicts()
		if len(conflicts) > 0 {
			allConflicts[string(contextID)] = conflicts
		}

		// Restore original context
		b.contextStack[len(b.contextStack)-1] = originalContext
	}

	return allConflicts
}

// Legacy mapping removed - command registry now handles key to command mapping directly

// Global bridge instance
var globalBridge *Bridge

// GetGlobalBridge returns the global bridge instance
func GetGlobalBridge() *Bridge {
	if globalBridge == nil {
		globalBridge = NewBridge()
	}
	return globalBridge
}
