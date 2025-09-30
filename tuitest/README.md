# TUI Test Framework

A comprehensive testing framework for Terminal User Interface (TUI) applications, with first-class support for [BubbleTea](https://github.com/charmbracelet/bubbletea) applications.

## Overview

This framework provides end-to-end testing capabilities for TUI applications by:
- Creating virtual terminals with PTY support
- Simulating keyboard input and user interactions
- Capturing and analyzing terminal output
- Providing fluent assertion APIs for TUI testing
- Supporting both component-level and full application testing

## Quick Start

### Claude Squad Keyboard Testing

```go
func TestClaudeSquadKeyboards(t *testing.T) {
    // Build and start claude-squad with PTY
    tester, err := NewClaudeSquadTester(t, "./claude-squad", 30*time.Second)
    require.NoError(t, err)
    defer tester.Close()

    // Wait for app to load
    err = tester.WaitForText("Claude Squad", 5*time.Second)
    require.NoError(t, err)

    // Test 'n' key opens new session dialog
    err = tester.TestKeyboardShortcut("n", "Session", 3*time.Second)
    assert.NoError(t, err)
}
```

### Running Tests

```bash
# Test claude-squad keyboard shortcuts
cd tuitest
make test

# Run specific test
go test -v ./integration/claude_squad -run TestClaudeSquadKeyboardShortcuts

# Run with race detection
make test-race
```

## Current Status

This is an **initial implementation** focused on immediate keyboard testing needs for claude-squad. It implements:

✅ **Phase 1 (Partial)**: Project setup and basic PTY testing
🚧 **Phase 2**: BubbleTea integration (planned)
🚧 **Phase 3**: Advanced features (planned)
🚧 **Phase 4**: Full claude-squad integration (in progress)

## Architecture

The framework follows a 4-phase implementation plan:

### Phase 1: Core Infrastructure
- Virtual terminal abstraction
- PTY management for real terminal interaction
- Basic text locators and assertions
- Go testing integration

### Phase 2: BubbleTea Integration
- Model testing utilities
- Message passing simulation
- Component isolation testing
- Event simulation

### Phase 3: Advanced Features
- ANSI color matching
- Snapshot testing
- Complex selectors (regex, position-based)
- Performance testing utilities

### Phase 4: Claude-Squad Specific
- Session management helpers
- Git workflow testing
- tmux integration testing
- Complete E2E test suite

## Project Structure

```
tuitest/
├── pkg/                    # Core framework packages
│   ├── terminal/          # Terminal abstraction
│   ├── locator/           # Element finding
│   ├── expect/            # Assertions
│   ├── model/             # BubbleTea testing
│   └── framework/         # Test management
├── integration/           # Integration tests
│   └── claude_squad/      # Claude-squad specific tests
├── examples/              # Usage examples
├── testdata/              # Test fixtures
└── docs/                  # Documentation
```

## Features

### Current Features
- ✅ PTY-based terminal testing
- ✅ Keyboard input simulation
- ✅ Output capture and analysis
- ✅ Text-based assertions
- ✅ Claude-squad keyboard shortcut testing
- ✅ Session creation flow testing

### Planned Features
- 🚧 ANSI escape sequence parsing
- 🚧 Color matching
- 🚧 Snapshot testing
- 🚧 Performance benchmarking
- 🚧 Complex element selectors

## Dependencies

- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/creack/pty` - PTY management
- `github.com/stretchr/testify` - Test assertions

## Contributing

This framework is actively being developed following the detailed implementation plan in `../docs/tui-test-project/`. See the [implementation documentation](../docs/tui-test-implementation.md) for the complete roadmap.

### Development Workflow

1. Follow the INVEST ticket system in `../docs/tui-test-project/`
2. Implement tickets in dependency order
3. Ensure all tests pass before merging
4. Update documentation for new features

## License

Same as claude-squad project.