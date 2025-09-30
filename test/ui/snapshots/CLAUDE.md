# Snapshot Testing System - Technical Guide

## Overview

The snapshot testing system provides **visual regression testing** for UI components without requiring a TTY. This is critical for CI/CD environments and enables comprehensive testing of terminal user interfaces.

## Architecture

### Core Components

1. **TestRenderer** (`test/ui/testrender.go`)
   - Renders BubbleTea components to strings
   - Strips ANSI color codes for readable diffs
   - Compares renders with saved snapshots
   - Handles snapshot creation and updates

2. **Snapshot Files** (`test/ui/snapshots/`)
   - Plain text files containing rendered component output
   - Organized by component type and scenario
   - Version-controlled for tracking UI changes
   - Human-readable for code review

3. **Test Files** (`test/ui/*_test.go`)
   - Define test scenarios
   - Set up component state
   - Trigger renders
   - Compare with snapshots

## How Snapshot Testing Works

### The Testing Flow

```
1. Test creates and configures a component
2. TestRenderer renders component to string
3. Snapshot comparison:
   - If UPDATE_SNAPSHOTS=true: Save/update snapshot file
   - If UPDATE_SNAPSHOTS=false: Compare with existing snapshot
   - If no snapshot exists: Test fails with instructions
4. On mismatch: Show detailed diff
```

### Key Principles

**Isolation**: Each snapshot tests one specific UI state
- ✅ Good: `basics_initial_name_focused.txt`
- ❌ Bad: `entire_app_state.txt`

**Completeness**: Cover all reachable states
- All dialog steps
- All user choices
- All error conditions
- All terminal sizes

**Maintainability**: Organized directory structure
```
snapshots/
  session_setup/
    basics/              # Step-specific snapshots
    location/
    confirm/
    advanced/            # Advanced flows
    errors/              # Error states
    flows/               # Complete flows
    navigation/          # Navigation scenarios
    responsive/          # Terminal size variations
```

## Adding New Snapshot Tests

### Step 1: Create the Test

```go
func TestMyComponent_Snapshots(t *testing.T) {
    renderer := NewTestRenderer().
        SetSnapshotPath("snapshots/my_component").
        SetDimensions(80, 30).
        DisableColors()

    tests := []struct {
        name     string
        setup    func() *MyComponent
        snapshot string
    }{
        {
            name: "initial_state",
            setup: func() *MyComponent {
                c := NewMyComponent()
                c.SetSize(80, 30)
                return c
            },
            snapshot: "initial_state.txt",
        },
        {
            name: "after_user_input",
            setup: func() *MyComponent {
                c := NewMyComponent()
                c.SetSize(80, 30)
                c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("input")})
                return c
            },
            snapshot: "after_user_input.txt",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            component := tt.setup()
            renderer.CompareComponentWithSnapshot(t, component, tt.snapshot)
        })
    }
}
```

### Step 2: Generate Snapshots

```bash
# Create initial snapshots
UPDATE_SNAPSHOTS=true go test ./test/ui -run TestMyComponent

# Review the generated snapshots
ls -la test/ui/snapshots/my_component/

# Commit the snapshots
git add test/ui/snapshots/my_component/
git commit -m "Add snapshot tests for MyComponent"
```

### Step 3: Run Tests

```bash
# Normal test run (compares with snapshots)
go test ./test/ui -run TestMyComponent

# Update snapshots when component changes
UPDATE_SNAPSHOTS=true go test ./test/ui -run TestMyComponent
```

## Multi-Layer Component Testing

For components with multiple states/steps (like session setup dialog):

### Test Each Layer Individually

```go
{
    name: "step_one",
    setup: func() *Component {
        c := NewComponent()
        // Component starts at step one
        return c
    },
    snapshot: "layer1/step_one.txt",
},
{
    name: "step_two",
    setup: func() *Component {
        c := NewComponent()
        c.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Advance to step two
        return c
    },
    snapshot: "layer2/step_two.txt",
},
```

### Test Complete Flows

```go
{
    name: "complete_flow_option_a",
    setup: func() *Component {
        c := NewComponent()
        c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("input")})
        c.Update(tea.KeyMsg{Type: tea.KeyEnter})
        c.Update(tea.KeyMsg{Type: tea.KeyTab}) // Select option A
        return c
    },
    snapshot: "flows/complete_option_a.txt",
},
```

### Test Error States

```go
{
    name: "error_empty_input",
    setup: func() *Component {
        c := NewComponent()
        c.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Try to proceed without input
        return c
    },
    snapshot: "errors/empty_input.txt",
},
```

## Best Practices

### 1. Comprehensive Coverage

Test every state the user can reach:
- ✅ Initial state
- ✅ Each step in multi-step flows
- ✅ Each user choice/option
- ✅ Error conditions
- ✅ Edge cases (empty input, max length, special chars)
- ✅ Terminal size variations (small, large)

### 2. Descriptive Naming

Use clear, hierarchical names:
```
component/
  state/              # Component state
    variant.txt       # State variant
```

Examples:
- `session_setup/basics/initial_name_focused.txt`
- `session_setup/location/different_selected.txt`
- `session_setup/errors/empty_name_basics.txt`

### 3. Organized Structure

```
snapshots/
  component_name/
    basic_states/     # Simple scenarios
    advanced_flows/   # Complex scenarios
    errors/           # Error conditions
    responsive/       # Size variations
    navigation/       # Navigation tests
```

### 4. Test Isolation

Each test should be independent:
```go
setup: func() *Component {
    c := NewComponent()  // Fresh component
    c.SetSize(80, 30)    // Explicit size
    // Set up exact state needed
    return c
},
```

### 5. Meaningful Diffs

When snapshots change:
1. Review the diff carefully
2. Understand WHY it changed
3. Verify the change is intentional
4. Update snapshot if correct
5. Fix code if incorrect

## Debugging Snapshot Tests

### Test Fails: "Rendered output does not match snapshot"

```bash
# 1. Look at the diff
go test ./test/ui -run TestMyComponent -v

# 2. See what changed
git diff test/ui/snapshots/my_component/

# 3. If change is correct, update snapshot
UPDATE_SNAPSHOTS=true go test ./test/ui -run TestMyComponent

# 4. If change is wrong, fix your code
```

### Test Fails: "Snapshot does not exist"

```bash
# Create the missing snapshot
UPDATE_SNAPSHOTS=true go test ./test/ui -run TestMyComponent
```

### Snapshot Looks Wrong

```bash
# View the snapshot
cat test/ui/snapshots/my_component/my_state.txt

# Check your test setup
# Verify component configuration
# Ensure correct key sequences
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
- name: Run Snapshot Tests
  run: |
    go test ./test/ui -v

- name: Check for Snapshot Changes
  run: |
    if [[ $(git status --porcelain test/ui/snapshots/) ]]; then
      echo "Snapshots changed but not updated!"
      git diff test/ui/snapshots/
      exit 1
    fi
```

### Pre-commit Hook

```bash
#!/bin/bash
# Run snapshot tests before commit
go test ./test/ui -short
if [ $? -ne 0 ]; then
  echo "Snapshot tests failed!"
  echo "Run: UPDATE_SNAPSHOTS=true go test ./test/ui"
  exit 1
fi
```

## When to Update Snapshots

✅ **Update when:**
- Intentional UI changes
- Improved styling/layout
- New features added
- Bug fixes that change display

❌ **Don't update when:**
- Random changes you don't understand
- Test failures you haven't investigated
- Trying to make tests pass without understanding

## Complementary Testing

Snapshots test **visual output**. Also use:

1. **Unit Tests** - Test component logic
   ```go
   func TestComponent_Logic(t *testing.T) {
       c := NewComponent()
       c.HandleInput("test")
       if c.GetValue() != "test" {
           t.Error("Value not set correctly")
       }
   }
   ```

2. **Lifecycle Tests** - Test callbacks and state
   ```go
   func TestComponent_Callbacks(t *testing.T) {
       c := NewComponent()
       callbackFired := false
       c.SetOnComplete(func() { callbackFired = true })
       c.Complete()
       if !callbackFired {
           t.Error("Callback not fired")
       }
   }
   ```

3. **Integration Tests** - Test with full app (teatest)
   ```go
   func TestComponent_Integration(t *testing.T) {
       tm := testutil.CreateTUITest(t, app, config)
       tm.Type("input")
       tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
       // Verify behavior
   }
   ```

## Summary

Snapshot testing provides:
- ✅ Visual regression protection
- ✅ No PTY required
- ✅ CI/CD compatible
- ✅ Human-readable diffs
- ✅ Fast execution
- ✅ Comprehensive coverage

Follow this guide to maintain high-quality UI tests that catch regressions early and enable confident refactoring.
