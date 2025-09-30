# UI Snapshots Directory

This directory contains snapshot files for visual regression testing of UI components.

## Quick Start

```bash
# Run all snapshot tests
go test ./test/ui

# Run specific component tests
go test ./test/ui -run TestSessionSetup

# Update snapshots after intentional UI changes
UPDATE_SNAPSHOTS=true go test ./test/ui

# View a specific snapshot
cat session_setup/basics/initial_name_focused.txt
```

## Directory Structure

```
snapshots/
├── CLAUDE.md                    # Technical guide (READ THIS FIRST)
├── README.md                    # This file
├── session_setup/               # Session creation dialog
│   ├── basics/                  # Name + Program input step
│   ├── location/                # Location choice step
│   ├── confirm/                 # Final confirmation step
│   ├── advanced/                # Advanced flows (repo/worktree/branch selectors)
│   ├── errors/                  # Error states
│   ├── flows/                   # Complete user journeys
│   ├── navigation/              # Navigation scenarios
│   └── responsive/              # Terminal size variations
├── session_list/                # Main session list
├── git_status/                  # Git status overlay
└── overlays/                    # Generic overlays

```

## What Are Snapshots?

Snapshots are **plain text files** containing the rendered output of UI components. They serve as:

1. **Visual Regression Tests** - Detect unintended UI changes
2. **Documentation** - Show what the UI actually looks like
3. **Change Review** - Git diffs reveal UI modifications

## Common Workflows

### Adding a New Component

1. Create test file: `test/ui/my_component_test.go`
2. Write test using `TestRenderer`
3. Generate snapshots: `UPDATE_SNAPSHOTS=true go test ./test/ui -run TestMyComponent`
4. Review snapshots: `ls test/ui/snapshots/my_component/`
5. Commit: `git add test/ui/snapshots/my_component/`

### After Changing UI

1. Run tests: `go test ./test/ui`
2. If tests fail, review diff: `git diff test/ui/snapshots/`
3. If change is correct: `UPDATE_SNAPSHOTS=true go test ./test/ui`
4. Commit updated snapshots

### Investigating Test Failures

```bash
# See detailed diff
go test ./test/ui -v -run TestMyComponent

# View expected snapshot
cat test/ui/snapshots/my_component/my_state.txt

# Run single test
go test ./test/ui -run TestMyComponent/specific_test
```

## Guidelines

### When to Update Snapshots

✅ **Update when:**
- You intentionally changed the UI
- You improved styling or layout
- You added new features
- You fixed a bug that changes what's displayed

❌ **Don't update when:**
- Tests fail and you don't know why
- You're trying to make tests pass without understanding the change
- Random diffs appear that you didn't cause

### Snapshot Best Practices

1. **One state per snapshot** - Don't combine multiple scenarios
2. **Descriptive names** - `basics/initial_name_focused.txt` not `test1.txt`
3. **Organized structure** - Use subdirectories for related snapshots
4. **Complete coverage** - Test all reachable states
5. **Review changes** - Always inspect snapshot diffs before committing

## Example Snapshot

```
🚀 Create New Session

◐ ○ ○ ○

📝 Session Details

► Session Name: (active)
╭──────────────────────────────────────────────────────────╮
│                                                          │
│  Session Name                                            │
│                                                          │
│                                                          │
│                                                          │
╰──────────────────────────────────────────────────────────╯

Program:
╭──────────────────────────────────────────────────────────╮
│                                                          │
│  Program                                                 │
│                                                          │
│  /Users/user/.asdf/shims/claude                          │
│                                                          │
╰──────────────────────────────────────────────────────────╯

💡 Tab to switch fields • Enter to continue

Esc: Cancel, ↑/↓: Navigate
```

## Related Documentation

- **Technical Guide**: [`CLAUDE.md`](./CLAUDE.md) - Detailed implementation guide
- **Test Utilities**: [`test/ui/README.md`](../README.md) - TestRenderer usage
- **Integration Tests**: [`app/*_test.go`](../../app/) - Full app testing with teatest

## Troubleshooting

**Problem**: Tests fail with "Snapshot does not exist"
**Solution**: Run `UPDATE_SNAPSHOTS=true go test ./test/ui -run YourTest`

**Problem**: Diffs are hard to read
**Solution**: Snapshots use plain text. Use `git diff` or any diff tool

**Problem**: Snapshots contain ANSI codes
**Solution**: Tests use `.DisableColors()` - check your test configuration

**Problem**: CI fails but local tests pass
**Solution**: Ensure you committed the snapshots: `git add test/ui/snapshots/`

## CI/CD Integration

Snapshots work automatically in CI/CD:
- No PTY required
- Fast execution
- Git tracks changes
- Diffs are reviewable

GitHub Actions example:
```yaml
- name: Test UI
  run: go test ./test/ui -v
```

## Questions?

- Read the [Technical Guide](./CLAUDE.md)
- Check existing tests in `test/ui/`
- See `test/ui/testrender.go` for utilities
