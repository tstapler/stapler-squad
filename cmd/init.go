package cmd

import (
	"github.com/tstapler/stapler-squad/cmd/interfaces"
)

// Import category constants from interfaces
const (
	CategoryView = interfaces.CategoryView
	CategoryPTY  = interfaces.CategoryPTY
)

// InitializeCommands sets up all standard commands in the registry
func InitializeCommands(registry *CommandRegistry) error {
	// Initialize contexts first
	if err := InitializeContexts(registry); err != nil {
		return err
	}
	// TUI commands removed in Phase 5 (TUI removal)
	return nil
}

// GetGlobalRegistry returns the initialized global registry
func GetGlobalRegistry() *CommandRegistry {
	registry := GetCommandRegistry()

	// Initialize once
	if len(registry.GetAllCommands()) == 0 {
		if err := InitializeCommands(registry); err != nil {
			panic("Failed to initialize commands: " + err.Error())
		}
	}

	return registry
}
