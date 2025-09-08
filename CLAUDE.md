# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run
```bash
# Build the application
go build .

# Run the application
./claude-squad

# Run with flags
./claude-squad -y  # Auto-yes mode
./claude-squad -p "aider --model ollama_chat/gemma3:1b"  # Custom program
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests for specific packages
go test ./ui
go test ./app
go test ./session

# Run with coverage
go test -cover ./...

# Run specific test
go test ./ui -run TestSpecificFunction

# Run benchmarks (performance tests)
go test -bench=. -benchmem ./app

# Run specific benchmarks
go test -bench=BenchmarkNavigation -benchmem ./app
go test -bench=BenchmarkInstanceChangedComponents -benchmem ./app
go test -bench=BenchmarkListRendering -benchmem ./app

# Profile benchmarks for detailed performance analysis
go test -bench=BenchmarkNavigation -benchmem -cpuprofile=cpu.prof ./app
go tool pprof cpu.prof
```

### Code Quality
```bash
# Check for issues
go vet ./...

# Format code
go fmt ./...

# Build and check for compilation errors
go build .
```

## Architecture Overview

Claude Squad is a terminal-based session management application built with Go and the BubbleTea TUI framework. It manages multiple AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees.

### Core Architecture Layers

**1. Application Layer (`app/`)**
- `app.go` - Main BubbleTea application state machine and event handling
- `help.go` - Dynamic help system that pulls from centralized key definitions
- `handleAdvancedSessionSetup.go` - Advanced session creation wizard

**2. UI Components (`ui/`)**
- `list.go` - Main session list with filtering, search, and categorization
- `menu.go` - Bottom command bar that shows context-aware key bindings  
- `tabbed_window.go` - Preview/diff tab container
- `overlay/` - Modal overlays for session creation, search, confirmations

**3. Key Management (`keys/`)**
- `keys.go` - Centralized key binding definitions and mappings
- `help.go` - Key categorization system for dynamic help generation

**4. Session Management (`session/`)**
- `instance.go` - Session lifecycle management (create, start, pause, resume)
- `storage.go` - Session persistence and state management
- `tmux/` - tmux session integration
- `git/` - Git worktree management for isolated branches

**5. Configuration (`config/`)**
- JSON-based configuration with logging settings
- State persistence for UI preferences

### Key Design Patterns

**Event-Driven State Machine**: The app uses BubbleTea's event-driven pattern where user input generates messages that update the application state.

**Centralized Key Management**: All key bindings are defined in `keys/keys.go` with automatic help system integration. Adding new keys requires updating:
1. `KeyName` enum in `keys.go`
2. `GlobalKeyStringsMap` for key-to-enum mapping
3. `GlobalkeyBindings` for display strings
4. `KeyHelpMap` in `keys/help.go` for categorized help

**View Model Pattern**: The `List` component separates data model (`items`) from view model (`getVisibleItems()`, `searchResults`, `categoryGroups`) for filtering and organization.

**Overlay System**: Modal dialogs use a consistent overlay pattern in `ui/overlay/` for session setup, confirmations, and text input.

### Session Organization Features

**Filtering**: Sessions can be filtered by status (hide paused sessions with `f` key).

**Search**: Full-text search across session titles (activated with `s` key).

**Categorization**: Sessions are organized by category with expand/collapse functionality.

**Navigation**: Smart navigation that respects filtered views and category boundaries.

### Git Integration

Each session gets its own git worktree, allowing multiple concurrent branches without conflicts. The system handles:
- Worktree creation and cleanup
- Branch management and switching  
- Diff generation for preview pane
- Commit and push operations

### tmux Integration

Sessions run in isolated tmux sessions for:
- Process isolation and persistence
- Terminal multiplexing within each session
- Background execution capabilities
- Session attachment/detachment

### State Management

**Application State**: Managed through BubbleTea's state machine with states like `stateDefault`, `stateNew`, `statePrompt`.

**Session State**: Persisted in JSON format with status tracking (Running, Paused, Stopped).

**UI State**: Navigation indices, filter settings, and view preferences are maintained across operations.

## Adding New Features

### New Key Bindings
1. Add to `KeyName` enum in `keys/keys.go`
2. Add mapping in `GlobalKeyStringsMap`
3. Define binding in `GlobalkeyBindings`  
4. Add help entry in `keys/help.go`
5. Add to appropriate menu options in `ui/menu.go`
6. Handle in `app/app.go` key switch statement

### New Session Filters
1. Add filter field to `List` struct in `ui/list.go`
2. Update `getVisibleItems()` method to apply filter
3. Add toggle method similar to `TogglePausedFilter()`
4. Wire up key binding following pattern above

### New Overlay Dialogs
1. Create new overlay in `ui/overlay/`
2. Follow existing patterns like `textInput.go`
3. Implement `HandleKeyPress` and `View` methods
4. Integrate with main app state machine

## Important Implementation Notes

**Navigation Consistency**: Always use `getVisibleItems()` for user-facing navigation and `getVisibleIndex()` to translate between filtered and global indices.

**Key Handler Order**: Key handlers in `app.go` are processed in switch statement order - place more specific handlers before generic ones.

**Help System**: The help system automatically discovers keys by category. Use appropriate `HelpCategory` values in `keys/help.go`.

**Error Handling**: Use `handleError()` method in app for consistent error display.

**State Validation**: Always validate selection indices after filter changes to prevent out-of-bounds access.

## Performance Optimization

### Navigation Performance
The application implements several performance optimizations for smooth navigation:

**Debounced Updates**: Navigation operations use a 150ms debounce delay to batch rapid key presses and avoid expensive operations during fast scrolling.

**Smart Category Organization**: Category grouping only triggers when sessions are added/removed or filters change, not on every navigation.

**Repository Name Caching**: Git repository names are cached in the UI renderer to avoid repeated expensive git operations.

**Tab-Aware Updates**: Preview and diff panes skip expensive updates when not visible.

### Performance Benchmarks
Run benchmarks to measure navigation performance:

```bash
# Benchmark navigation with multiple sessions
go test -bench=BenchmarkNavigationPerformance -benchmem ./app

# Benchmark individual components
go test -bench=BenchmarkInstanceChangedComponents -benchmem ./app

# Benchmark list rendering
go test -bench=BenchmarkListRendering -benchmem ./app
```

### Tmux Session Isolation
Configure tmux session prefixes for process isolation:

```json
{
  "tmux_session_prefix": "myapp_"
}
```

This allows multiple claude-squad processes to run simultaneously without session conflicts.