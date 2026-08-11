# Go TUI-Test Framework Implementation Plan

## Overview

This document outlines the complete implementation plan for a Go-based TUI testing framework, inspired by Microsoft's TUI-Test library and specifically designed to integrate with stapler-squad's BubbleTea architecture.

## Project Goals

- Provide comprehensive end-to-end testing for TUI applications
- Support both isolated component testing and full application testing
- Integrate seamlessly with stapler-squad's existing test infrastructure
- Create a reusable framework for the broader Go/BubbleTea ecosystem

## Architecture Overview

### Core Components

1. **Terminal Abstraction Layer** - Virtual terminal for testing
2. **Test Framework Integration** - Go testing integration with lifecycle management
3. **Locator System** - Playwright-style element finding
4. **Expectations/Assertions** - Fluent assertion API
5. **Model Testing** - BubbleTea-specific testing utilities

### Technology Stack

- **Core Language**: Go 1.21+
- **TUI Framework**: github.com/charmbracelet/bubbletea
- **Testing**: Standard library `testing` package
- **PTY Management**: github.com/creack/pty
- **Assertions**: github.com/stretchr/testify (optional)

## Implementation Phases

### Phase 1: Core Infrastructure (Weeks 1-2)
**Goal**: Establish foundational testing framework

**Deliverables**:
- Terminal buffer abstraction
- Basic test framework structure
- Simple text locators
- Basic visibility assertions

### Phase 2: BubbleTea Integration (Weeks 2-3)
**Goal**: Enable comprehensive BubbleTea model testing

**Deliverables**:
- ModelTester implementation
- Key event simulation
- Message passing system
- Component isolation testing

### Phase 3: Advanced Features (Weeks 3-4)
**Goal**: Add sophisticated testing capabilities

**Deliverables**:
- Color matching and ANSI parsing
- Snapshot testing system
- Complex selectors (regex, position-based)
- Performance testing utilities

### Phase 4: Claude-Squad Integration (Weeks 4-5)
**Goal**: Provide stapler-squad specific testing utilities

**Deliverables**:
- Session management test helpers
- Git workflow testing utilities
- tmux integration testing
- Performance benchmarking extensions

## Directory Structure

```
tuitest/
├── pkg/
│   ├── terminal/
│   │   ├── buffer.go        # Terminal buffer management
│   │   ├── renderer.go      # ANSI parsing and rendering
│   │   ├── simulator.go     # Input simulation
│   │   └── cell.go         # Terminal cell representation
│   ├── locator/
│   │   ├── locator.go      # Main locator interface
│   │   ├── selector.go      # Element selection interfaces
│   │   ├── text.go         # Text-based selectors
│   │   ├── position.go     # Position-based selectors
│   │   └── regex.go        # Regex-based selectors
│   ├── expect/
│   │   ├── expectation.go   # Main expectation struct
│   │   ├── assertions.go    # Assertion implementations
│   │   ├── matchers.go     # Matcher utilities
│   │   ├── snapshot.go     # Snapshot testing
│   │   └── color.go        # Color matching
│   ├── model/
│   │   ├── tester.go       # BubbleTea model testing
│   │   ├── simulator.go    # Event simulation
│   │   └── mock.go         # Mock utilities
│   ├── framework/
│   │   ├── test.go         # Individual test management
│   │   ├── suite.go        # Test suite management
│   │   ├── runner.go       # Test execution engine
│   │   ├── context.go      # Test context
│   │   └── hooks.go        # Lifecycle hooks
│   └── utils/
│       ├── ansi.go         # ANSI escape code utilities
│       ├── diff.go         # Text diff utilities
│       └── color.go        # Color parsing utilities
├── examples/
│   ├── basic_test.go       # Basic usage examples
│   ├── component_test.go   # Component testing examples
│   ├── e2e_test.go        # End-to-end testing examples
│   └── snapshot_test.go    # Snapshot testing examples
├── integration/
│   ├── claude_squad_test.go # Claude-squad specific tests
│   ├── session_test.go     # Session management tests
│   └── navigation_test.go  # Navigation testing
├── testdata/
│   ├── snapshots/          # Test snapshots
│   └── fixtures/           # Test fixtures
├── docs/
│   ├── api.md             # API documentation
│   ├── examples.md        # Usage examples
│   └── integration.md     # Integration guide
├── go.mod
├── go.sum
├── README.md
├── LICENSE
└── Makefile
```

## API Design

### Core API

```go
// Terminal represents a virtual terminal for testing
type Terminal struct {
    width  int
    height int
    buffer *TerminalBuffer
    opts   *TerminalOptions
}

// Test represents a single TUI test case
type Test struct {
    name     string
    fn       TestFunction
    options  *TestOptions
    timeout  time.Duration
    suite    *TestSuite
}

// Locator finds elements in the terminal buffer
type Locator struct {
    terminal *Terminal
    selector Selector
    options  *LocatorOptions
}

// Expectation provides fluent assertion API
type Expectation struct {
    locator *Locator
    not     bool
}

// ModelTester provides utilities for testing BubbleTea models
type ModelTester struct {
    model   tea.Model
    program *tea.Program
    options *ModelTestOptions
}
```

### Usage Examples

#### Basic Test
```go
func TestClaudeSquadBasicFlow(t *testing.T) {
    suite := tuitest.NewSuite("Stapler Squad E2E")

    suite.Test("create session", func(ctx *tuitest.TestContext) error {
        terminal := ctx.Terminal()

        // Start application
        err := terminal.Start("./stapler-squad")
        if err != nil {
            return err
        }

        // Create new session
        terminal.SendKey('n')
        prompt := terminal.GetByText("Session title:")
        return tuitest.Expect(prompt).ToBeVisible()
    })

    suite.Run(t)
}
```

#### Component Test
```go
func TestListComponent(t *testing.T) {
    list := &ui.List{
        Items: generateTestSessions(10),
    }

    tester := tuitest.NewModelTester(list, nil)
    output := tester.Render()

    assert.Contains(t, output, "10 sessions")
}
```

## Integration with Claude-Squad

### Test Integration Points

1. **UI Components**: List, Menu, Overlays
2. **Session Management**: Instance lifecycle, state transitions
3. **Navigation**: Key handling, state updates
4. **Git Integration**: Worktree management, diff display
5. **Performance**: Rendering benchmarks, memory usage

### Existing Test Enhancement

The framework will enhance stapler-squad's existing test infrastructure:
- Extend `test/ui/testrender.go` with TUI-specific capabilities
- Add snapshot testing to component tests
- Provide utilities for integration testing

## Success Metrics

### Phase 1 Success Criteria
- [ ] Virtual terminal can capture and parse ANSI output
- [ ] Basic text locators work correctly
- [ ] Simple visibility assertions pass
- [ ] Framework integrates with Go testing

### Phase 2 Success Criteria
- [ ] BubbleTea models can be tested in isolation
- [ ] Key events are simulated correctly
- [ ] Message passing works reliably
- [ ] Component state changes are testable

### Phase 3 Success Criteria
- [ ] Color matching works for ANSI colors
- [ ] Snapshot testing generates stable output
- [ ] Complex selectors find elements accurately
- [ ] Performance tests provide meaningful metrics

### Phase 4 Success Criteria
- [ ] Claude-squad E2E tests pass consistently
- [ ] Session management is fully testable
- [ ] Git workflow tests work reliably
- [ ] Performance benchmarks show improvements

## Risk Mitigation

### Technical Risks
1. **ANSI Parsing Complexity**: Mitigate with incremental implementation and extensive testing
2. **BubbleTea Integration**: Work closely with existing patterns in stapler-squad
3. **Cross-platform Compatibility**: Test on multiple platforms early

### Project Risks
1. **Scope Creep**: Stick to defined phases and deliverables
2. **Performance Impact**: Profile early and optimize incrementally
3. **Maintenance Burden**: Design for extensibility and clear documentation

## Next Steps

1. Create project structure and initial files
2. Implement Phase 1 core infrastructure
3. Set up CI/CD pipeline
4. Begin integration with stapler-squad tests
5. Gather feedback and iterate

## Dependencies

### External Dependencies
- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/creack/pty` - PTY management
- `github.com/stretchr/testify` - Assertions (optional)

### Internal Dependencies
- Integration with stapler-squad's existing test infrastructure
- Compatibility with current build and CI systems
- Documentation alignment with project standards

---

*This document will be updated as implementation progresses and requirements evolve.*