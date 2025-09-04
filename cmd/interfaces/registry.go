package interfaces

// RegistryInterface defines the interface for command registries
type RegistryInterface interface {
	// Command management
	GetCommand(id CommandID) (CommandInterface, bool)
	GetAllCommands() map[CommandID]CommandInterface
	GetKeysForCommand(cmdID CommandID) []string
	ResolveCommand(contextID ContextID, key string) CommandInterface

	// Context management
	GetContext(id ContextID) (ContextInterface, bool)
	GetCommandsForContext(contextID ContextID) []CommandInterface

	// Validation
	DetectConflicts() []KeyConflict
}

// CommandInterface defines the interface for commands
type CommandInterface interface {
	GetID() CommandID
	GetName() string
	GetDescription() string
	GetCategory() Category
	GetContexts() []ContextID
	GetKeys() []string
	IsDeprecated() bool
	GetDeprecationInfo() *DeprecationInfo
	Execute(ctx *CommandContext) error
}

// ContextInterface defines the interface for contexts
type ContextInterface interface {
	GetID() ContextID
	GetName() string
	GetDescription() string
	GetParent() *ContextID
}

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
