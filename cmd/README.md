# Centralized Command & Help Management System

This package provides a unified, context-aware command registry and auto-generated help system for Stapler Squad. It replaces the scattered keybinding definitions with a centralized, maintainable approach.

## Architecture Overview

The system consists of several key components:

1. **Command Registry** (`registry.go`) - Central storage for all commands and keybindings
2. **Contexts** (`contexts.go`) - Define different application modes/states  
3. **Categories** (`categories.go`) - Organize commands for help display
4. **Commands** (`commands/`) - Individual command implementations
5. **Help Generator** (`help/generator.go`) - Auto-generate help content
6. **State Manager** (`state/manager.go`) - Manage modal contexts and command routing
7. **Migration Bridge** (`migration.go`) - Compatibility layer with existing code

## Key Benefits

- **Single Source of Truth**: All keybindings and help text in one place
- **Context Awareness**: Different keybindings per mode with inheritance
- **Auto-Generated Help**: Help screens and status lines generated automatically  
- **Conflict Detection**: Validate no duplicate keys within contexts
- **Easy Migration**: Built-in deprecation and migration path support
- **Type Safety**: Strongly typed command IDs and contexts

## Quick Integration Example

Here's how to integrate the new system with existing code:

```go
package main

import (
    "stapler-squad/cmd"
    "stapler-squad/cmd/commands" 
    "stapler-squad/cmd/interfaces"
    tea "github.com/charmbracelet/bubbletea"
)

func main() {
    // Get the migration bridge
    bridge := cmd.GetGlobalBridge()
    
    // Configure command handlers
    bridge.Initialize(
        &commands.SessionHandlers{
            OnNewSession: func() (tea.Model, tea.Cmd) {
                // Your existing new session logic
                return yourModel, yourCmd
            },
            OnAttachSession: func() (tea.Model, tea.Cmd) {
                // Your existing attach logic  
                return yourModel, yourCmd
            },
            // ... other handlers
        },
        &commands.GitHandlers{
            OnGitStatus: func() (tea.Model, tea.Cmd) {
                // Show git status overlay
                return yourGitModel, yourGitCmd
            },
            // ... other git handlers
        },
        &commands.NavigationHandlers{
            OnUp: func() (tea.Model, tea.Cmd) {
                // Handle up navigation
                return yourModel, yourCmd
            },
            // ... other navigation handlers
        },
        &commands.OrganizationHandlers{
            OnFilterPaused: func() (tea.Model, tea.Cmd) {
                // Handle filter toggle
                return yourModel, yourCmd
            },
            // ... other organization handlers
        },
        &commands.SystemHandlers{
            OnHelp: func() (tea.Model, tea.Cmd) {
                // Show auto-generated help
                helpContent := bridge.GetContextualHelp()
                return showHelpOverlay(helpContent), nil
            },
            OnQuit: func() (tea.Model, tea.Cmd) {
                return yourModel, tea.Quit
            },
            // ... other system handlers
        },
    )
}
```

## Using in Your Bubble Tea Model

### Handle Key Presses

Replace your existing key handling with:

```go
func (m YourModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        bridge := cmd.GetGlobalBridge()
        
        // Try to handle with new command system
        model, teaCmd, err := bridge.HandleKeyString(msg.String())
        if err == nil && model != nil {
            return model, teaCmd
        }
        
        // Fall back to legacy handling if needed
        // ... existing key handling code
    }
    return m, nil
}
```

### Context Management

Switch between different contexts as the user navigates:

```go
// Enter git status mode
bridge := cmd.GetGlobalBridge()
bridge.PushContext(interfaces.ContextGitStatus)

// Exit git status mode  
bridge.PopContext()

// Switch to list context
bridge.SetContext(interfaces.ContextList)
```

### Auto-Generated Status Line

Replace hardcoded status lines with:

```go
func (m YourModel) View() string {
    bridge := cmd.GetGlobalBridge()
    
    // Generate status line for current context
    statusLine := bridge.GetLegacyStatusLine()
    
    return fmt.Sprintf("%s\\n%s", yourContent, statusLine)
}
```

### Auto-Generated Help

Replace manual help screens with:

```go
func showHelpScreen() string {
    bridge := cmd.GetGlobalBridge()
    return bridge.GetContextualHelp()
}
```

## Available Contexts

- `ContextGlobal` - Commands available everywhere
- `ContextList` - Session list view  
- `ContextGitStatus` - Git status interface (fugitive-style)
- `ContextHelp` - Help screen
- `ContextPrompt` - Text input
- `ContextSearch` - Search mode
- `ContextConfirm` - Confirmation dialogs

## Adding New Commands

To add a new command:

1. **Create the handler function:**
```go
func MyNewCommand(ctx *interfaces.CommandContext) error {
    // Your command logic here
    return nil
}
```

2. **Register the command:**
```go
registry := cmd.GetGlobalRegistry()
registry.Register(&cmd.Command{
    ID:          "my.new_command",
    Name:        "My New Command", 
    Description: "Does something useful",
    Category:    cmd.CategorySession,
    Handler:     MyNewCommand,
    Contexts:    []cmd.ContextID{cmd.ContextList},
}).BindKey("x")
```

## Migration Strategy

The system provides full backward compatibility:

1. **Legacy Key Mapping**: Old `keys.KeyName` constants are automatically mapped
2. **Gradual Migration**: Can coexist with existing key handling  
3. **Deprecation Support**: Mark old commands as deprecated with alternatives
4. **Conflict Detection**: Validates no duplicate keybindings

## Validation

The system includes built-in validation:

```go
bridge := cmd.GetGlobalBridge()
issues := bridge.ValidateSetup()
for _, issue := range issues {
    log.Warn("Command system issue: %s", issue)
}
```

## File Organization

```
cmd/
├── registry.go          # Core registry system
├── contexts.go          # Context definitions  
├── categories.go        # Command categories
├── init.go              # Command registration
├── migration.go         # Legacy compatibility
├── interfaces/          # Type definitions
│   ├── types.go        # Basic types
│   └── registry.go     # Registry interfaces
├── commands/            # Command implementations
│   ├── session.go      # Session management
│   ├── git.go          # Git integration
│   ├── navigation.go   # Navigation commands
│   ├── organization.go # Filtering/organization  
│   └── system.go       # System commands
├── help/               # Auto-generated help
│   └── generator.go    # Help content generation
├── state/              # State management
│   └── manager.go      # Modal context management
└── README.md           # This documentation
```

This architecture eliminates the scattered keybinding definitions and provides a single, maintainable system for managing all commands and help text.