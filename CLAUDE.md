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
# CRITICAL: Benchmarks take 5-30 minutes and MUST be run in background with &
# Do NOT run benchmarks without & as they will block your terminal
go test -bench=. -benchmem ./app -timeout=30m &

# Run specific benchmark categories
go test -bench=BenchmarkNavigation -benchmem ./app -timeout=10m &
go test -bench=BenchmarkInstanceChangedComponents -benchmem ./app -timeout=10m &
go test -bench=BenchmarkListRendering -benchmem ./app -timeout=10m &

# New comprehensive performance benchmarks
go test -bench=BenchmarkLargeSessionNavigation -benchmem ./app -timeout=20m &
go test -bench=BenchmarkAttachDetachPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkFilteringPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkCategoryOrganization -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRenderingPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkMemoryUsage -benchmem ./app -timeout=15m &
go test -bench=BenchmarkStartupPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRealtimeUpdates -benchmem ./app -timeout=10m &

# Git integration and contextual discovery benchmarks
go test -bench=BenchmarkGitRepositoryDiscovery -benchmem ./ui/overlay -timeout=5m &
go test -bench=BenchmarkContextualDiscovery -benchmem ./ui/overlay -timeout=5m &

# Path validation benchmarks
go test -bench=BenchmarkValidatePath -benchmem ./ui/overlay -timeout=2m &

# Profile benchmarks for detailed performance analysis
go test -bench=BenchmarkLargeSessionNavigation -benchmem -cpuprofile=cpu.prof ./app -timeout=20m
go tool pprof cpu.prof

# Memory profiling for large session counts
go test -bench=BenchmarkMemoryUsage -benchmem -memprofile=mem.prof ./app -timeout=15m
go tool pprof mem.prof

# Trace profiling for detailed execution analysis
go test -bench=BenchmarkAttachDetachPerformance -trace=trace.out ./app -timeout=15m
go tool trace trace.out
```

### Testing Interactive TUI Components

**TTY Requirement**: Claude Squad is a terminal-based application that requires a TTY to run. This creates special considerations for testing interactive components.

**Testing Strategies:**

1. **Unit Tests for Business Logic** (Recommended)
   ```bash
   # Test core logic without TUI interaction
   go test ./ui/overlay -run TestContextualDiscovery
   go test ./ui/overlay -run TestSessionSetup
   go test ./session -run TestInstance
   ```

2. **Mock TTY for Integration Tests**
   ```bash
   # For testing that requires terminal interaction, consider:
   # - github.com/creack/pty (pseudo-terminal)
   # - github.com/Netflix/go-expect (terminal automation)
   # - Headless terminal testing with script automation
   ```

3. **Isolated Component Testing**
   ```bash
   # Test individual TUI components in isolation
   go test ./ui -run TestFuzzyInputOverlay
   go test ./ui -run TestMenuHandling
   go test ./ui -run TestNavigationLogic
   ```

**Manual Testing Protocol:**
```bash
# Build and run for interactive testing
go build .
./claude-squad

# Test specific flows:
# 1. Session creation (n key) - test contextual path discovery
# 2. Navigation (arrow keys) - test performance with multiple sessions
# 3. Filtering (f key) - test session visibility
# 4. Search (s key) - test fuzzy matching
# 5. Menu shortcuts - verify bottom menu displays active key bindings
```

**Menu Shortcut Testing:**
Since the menu shortcuts cannot be tested interactively in TTY-less environments, use integration tests:

```bash
# Test menu shortcut integration
go test ./app -run TestMenuShortcutIntegration -v

# Test menu context updates
go test ./app -run TestUpdateMenuFromContext -v

# Manual verification checklist:
# - Bottom menu shows key shortcuts (n, D, g, q, etc.)
# - Shortcuts correspond to actual available commands
# - Menu updates when context changes (if applicable)
# - All registered commands appear with correct descriptions
```

**TTY-Related Issue Debugging:**
```bash
# Common TTY issues and solutions:

# Issue: "could not open a new TTY: open /dev/tty: device not configured"
# Solution: Run in actual terminal, not programmatically

# Issue: Menu shortcuts not displaying
# Root cause: SetAvailableCommands not called with bridge commands
# Fix: Ensure bridge.GetAvailableKeys() is passed to menu.SetAvailableCommands()

# Issue: Terminal size detection problems
# Solution: Use reliable terminal size detection methods (see app/app.go:791)
```

**CI/CD Considerations:**
- Unit tests run in CI without TTY
- Integration tests may need headless terminal or be run manually
- Performance benchmarks should be run in background (`&`)
- Menu functionality tested via integration tests, not manual interaction

### Code Quality and Analysis

**Using Makefile (Recommended):**
```bash
# Install all development tools
make install-tools

# Quick development validation
make quick-check

# Comprehensive analysis (all static analysis tools)
make analyze

# Individual analysis tools
make nil-safety     # Comprehensive nil safety analysis
make staticcheck    # Advanced static analysis
make security       # Security vulnerability scanning
make lint          # Comprehensive linting

# Pre-commit validation
make pre-commit

# Dead code detection
make deadcode       # Find unreachable code
deadcode -test ./...  # Include test files in analysis
```

**Manual Commands:**
```bash
# Check for issues
go vet ./...

# Format code
go fmt ./...

# Build and check for compilation errors
go build .
```

### Nil Safety Analysis

Claude Squad includes comprehensive nil safety analysis tools to prevent panic-causing nil pointer dereferences:

```bash
# Run all nil safety tools
make nil-safety

# Individual tools
make nilaway        # Advanced nil flow analysis (Uber)
go vet -nilness ./... # Built-in Go nilness analyzer
go-nilcheck ./...   # Function pointer validation
```

**Required Tools Installation:**
```bash
make install-tools
# Or manually:
go install go.uber.org/nilaway/cmd/nilaway@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

**Nil Safety Best Practices:**
1. Always run `make nil-safety` before committing code
2. Use NilAway for the most comprehensive nil flow analysis
3. Include nil checks before pointer dereferences
4. Use defensive programming in overlay rendering (see app/app.go:1225)

### Static Analysis Tools

**Comprehensive Toolchain:**
- **NilAway** - Advanced nil pointer safety analysis (Uber)
- **Staticcheck** - Production-grade static analyzer  
- **golangci-lint** - Meta-linter with multiple analyzers
- **gosec** - Security-focused static analysis
- **go vet** - Built-in Go static analysis

## Application Data Directory

Claude Squad stores application data, logs, and git worktrees in the `~/.claude-squad` directory:

```
~/.claude-squad/
├── logs/                    # Application logs for debugging
│   ├── claude-squad.log     # Main application log
│   └── debug.log           # Detailed debug information
├── worktrees/              # Git worktrees for isolated sessions
│   ├── session-name_hash/  # Individual worktree directories
│   └── ...
├── config.json            # Application configuration
└── sessions.json          # Session state persistence
```

**Debugging Session Creation Issues:**
- Check `~/.claude-squad/logs/claude-squad.log` for session creation attempts
- Look for tmux command executions, git operations, and timeout messages
- Debug logs show detailed command execution and timing information

**Key Log Patterns to Look For:**
- `Starting tmux session` - Session creation initiation
- `timed out waiting for tmux session` - Session creation hangs
- `which claude` - External command execution that may block
- `git worktree` operations - Git integration issues
- `DoesSessionExist()` polling - Session existence checking loops

## State File Isolation and Multi-Instance Support

Claude Squad implements hierarchical state file isolation to prevent conflicts between tests, benchmarks, and multiple production instances. This ensures safe concurrent execution and data integrity.

### Isolation Hierarchy

State files are automatically isolated based on a priority hierarchy:

1. **Explicit Instance ID** (Highest Priority)
   ```bash
   # Named instance with dedicated state
   CLAUDE_SQUAD_INSTANCE=work ./claude-squad

   # Backward compatibility - shared global state
   CLAUDE_SQUAD_INSTANCE=shared ./claude-squad
   ```
   - State location: `~/.claude-squad/instances/{INSTANCE_ID}/`
   - Use case: Named instances for different projects/contexts

2. **Test Mode Auto-Detection**
   - Automatically activated when running `go test` or benchmarks
   - State location: `~/.claude-squad/test/test-{PID}/`
   - Use case: Prevents tests from corrupting production state
   - No configuration needed - works automatically

3. **Workspace-Based Isolation** (Production Default)
   ```bash
   # Automatic per-directory state (default behavior)
   cd ~/my-project && ./claude-squad

   # Disable workspace isolation if needed
   CLAUDE_SQUAD_WORKSPACE_MODE=false ./claude-squad
   ```
   - State location: `~/.claude-squad/workspaces/{WORKSPACE_HASH}/`
   - Use case: Different state per git repository/working directory
   - Each directory gets a stable, unique workspace ID (SHA256 hash)

4. **Global Shared State** (Fallback)
   - State location: `~/.claude-squad/`
   - Use case: Backward compatibility, explicit sharing
   - Activated when workspace mode is disabled or detection fails

### Instance Identification in Logs

All log messages include instance identifiers for debugging multi-instance scenarios:

```bash
# Named instance logs
[work] INFO: Session created

# PID-based logs (with timestamp to prevent PID reuse confusion)
[pid-12345-1704132000] INFO: Session started

# Daemon logs
[work][DAEMON] INFO: Polling sessions
```

The instance identifier prevents confusion when:
- Multiple instances run simultaneously
- Analyzing historical logs with reused PIDs
- Debugging concurrent test execution

### Common Usage Patterns

**Development Workflow:**
```bash
# Default: Workspace isolation (safest, per-project state)
cd ~/my-feature && ./claude-squad

# Different project, different state automatically
cd ~/other-project && ./claude-squad
```

**Testing and Benchmarks:**
```bash
# Automatic isolation - no configuration needed
go test ./...
go test -bench=. ./app

# Tests never interfere with production state
# Each test/benchmark gets isolated directories
```

**Multiple Named Instances:**
```bash
# Work sessions
CLAUDE_SQUAD_INSTANCE=work ./claude-squad

# Personal projects
CLAUDE_SQUAD_INSTANCE=personal ./claude-squad

# Completely isolated state files
```

**Legacy Shared State:**
```bash
# Use pre-isolation behavior if needed
CLAUDE_SQUAD_INSTANCE=shared ./claude-squad

# Or disable workspace mode
CLAUDE_SQUAD_WORKSPACE_MODE=false ./claude-squad
```

### Migration Notes

**Existing Users:**
- Workspace isolation is now the default (per-directory state)
- To use old shared behavior: `CLAUDE_SQUAD_INSTANCE=shared`
- Or disable workspace mode: `CLAUDE_SQUAD_WORKSPACE_MODE=false`
- Your existing `~/.claude-squad/` state is preserved

**Test Authors:**
- Tests automatically get isolated state - no code changes needed
- Benchmarks no longer corrupt production state files
- Test mode is auto-detected from command-line arguments

**Multi-Instance Users:**
- Safe concurrent execution is now supported by default
- Each workspace/instance gets its own state file
- File locking prevents corruption within each instance
- Log messages include instance IDs for debugging

### Troubleshooting

**Issue: "Can't find my sessions after restart"**
- Check if you're in a different directory (workspace isolation active)
- Use `CLAUDE_SQUAD_WORKSPACE_MODE=false` for directory-independent state
- Or set `CLAUDE_SQUAD_INSTANCE=shared` for global shared state

**Issue: "Tests are modifying production state"**
- Should not happen with auto-detection
- Verify tests are run with `go test` command
- Check test binary names contain `.test` suffix

**Issue: "Multiple instances conflicting"**
- Each instance should have its own workspace automatically
- Check instance identifiers in logs to verify isolation
- Use explicit `CLAUDE_SQUAD_INSTANCE` for named instances

**Issue: "Want to share state across multiple directories"**
- Use named instance: `CLAUDE_SQUAD_INSTANCE=shared`
- Or disable workspace mode: `CLAUDE_SQUAD_WORKSPACE_MODE=false`
- Both approaches give you global shared state

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

## Makefile Usage

The project includes a comprehensive Makefile for development workflows:

### Quick Start
```bash
make help          # Show all available commands
make dev-setup     # Set up development environment
make validate-env  # Check if tools are installed
```

### Development Workflows
```bash
make build         # Build the application
make test         # Run tests
make quick-check   # Build + test + lint (fast validation)
make pre-commit    # Full pre-commit validation
make all          # Complete workflow: clean + build + test + analyze
```

### Analysis and Quality
```bash
make analyze       # Run all static analysis tools
make nil-safety    # Comprehensive nil safety analysis
make security      # Security vulnerability scanning
make lint         # Code style and quality checks
```

### Performance Testing
```bash
make benchmark          # Full benchmarks (runs in background)
make benchmark-quick    # Fast subset for validation
make benchmark-navigation # Navigation performance only
make profile-cpu       # CPU profiling
```

### Tool Management
```bash
make install-tools # Install all development tools
make clean        # Clean build artifacts  
make clean-tools  # Remove installed tools (caution)
```

The Makefile handles long-running benchmarks automatically and provides comprehensive development automation.