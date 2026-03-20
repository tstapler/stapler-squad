package cmd

import (
	"github.com/tstapler/stapler-squad/cmd/interfaces"
	"github.com/tstapler/stapler-squad/config"
	"testing"
)

func TestConfigurationBasedCategorization(t *testing.T) {
	// Create a bridge with custom configuration
	bridge := NewBridge()

	// Override the config with custom key categories
	bridge.config = &config.Config{
		KeyCategories: map[string]string{
			"n":     "Custom Session",
			"g":     "Custom Git",
			"test":  "Custom Category",
		},
	}

	// Create test commands
	cmd1 := &Command{
		ID:          "test.command1",
		Name:        "Test Command 1",
		Description: "First test command",
		Category:    interfaces.CategorySession, // This should be overridden by config
		Contexts:    []ContextID{ContextList},
		Handler:     func(ctx *interfaces.CommandContext) error { return nil },
	}

	cmd2 := &Command{
		ID:          "test.command2",
		Name:        "Test Command 2",
		Description: "Second test command",
		Category:    interfaces.CategoryGit, // This should be overridden by config
		Contexts:    []ContextID{ContextList},
		Handler:     func(ctx *interfaces.CommandContext) error { return nil },
	}

	cmd3 := &Command{
		ID:          "test.command3",
		Name:        "Test Command 3",
		Description: "Third test command",
		Category:    interfaces.CategorySystem, // This should remain as System (not in config)
		Contexts:    []ContextID{ContextList},
		Handler:     func(ctx *interfaces.CommandContext) error { return nil },
	}

	// Switch to ContextList to match command contexts
	bridge.SetContext(ContextList)

	// Register commands with keys
	bridge.registry.Register(cmd1).BindKey("n")
	bridge.registry.Register(cmd2).BindKey("g")
	bridge.registry.Register(cmd3).BindKey("q")

	// Get categories
	categories := bridge.GetKeyCategories()

	// Check that configuration-based categorization works
	if commands, exists := categories["Custom Session"]; !exists {
		t.Error("Expected 'Custom Session' category to exist")
	} else {
		found := false
		for _, command := range commands {
			if command == "n - First test command" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'n - First test command' in 'Custom Session' category")
		}
	}

	if commands, exists := categories["Custom Git"]; !exists {
		t.Error("Expected 'Custom Git' category to exist")
	} else {
		found := false
		for _, command := range commands {
			if command == "g - Second test command" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'g - Second test command' in 'Custom Git' category")
		}
	}

	// Check that fallback to command category works for keys not in config
	if commands, exists := categories["System"]; !exists {
		t.Error("Expected 'System' category to exist for fallback")
	} else {
		found := false
		for _, command := range commands {
			if command == "q - Third test command" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected 'q - Third test command' in 'System' category (fallback)")
		}
	}
}

func TestConfigReload(t *testing.T) {
	bridge := NewBridge()

	// Verify initial config is loaded
	if bridge.config == nil {
		t.Error("Expected bridge to have config loaded")
	}

	// Check that config has default key categories
	if bridge.config.KeyCategories == nil {
		t.Error("Expected bridge config to have key categories")
	}

	// Test reload
	bridge.ReloadConfig()
	if bridge.config == nil {
		t.Error("Expected config to still be loaded after reload")
	}
}