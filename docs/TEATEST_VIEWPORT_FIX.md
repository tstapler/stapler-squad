# Teatest Viewport Fix - Complete Solution

## Problem Summary

TUI tests using `teatest` were failing because sessions weren't appearing in the rendered output, showing "No sessions available" even though sessions were added to the model.

## Root Cause Analysis

The issue had multiple interrelated causes:

1. **Terminal Manager Override**: The app's `Init()` method sends `WindowSizeMsg` using `terminalManager.GetReliableSize()`, which returns 80x24 (default fallback in test environment)
2. **Dimension Override**: This 80x24 WindowSizeMsg overwrites teatest's configured dimensions (200x40)
3. **Narrow Viewport**: With only 80 columns and the list taking 30% (24 columns), there isn't enough space to properly render session titles
4. **Init() Timing**: BubbleTea processes Init() before the first render, so dimensions get overwritten before tea test can capture correct output

## The Complete Fix Pattern

### Recommended API (Preferred)

Use the `SetupTeatestApp()` helper which encapsulates all viewport fix steps:

```go
// Get config
config := testutil.DefaultTUIConfig()

// Create fully configured app (all 8 steps applied automatically)
appModel := SetupTeatestApp(t, config)

// Add sessions and configure test-specific behavior
session := CreateTestSession(t, "test-session")
_ = appModel.list.AddInstance(session)
appModel.list.SetSelectedInstance(0)

// Create teatest AFTER app is configured
tm := testutil.CreateTUITest(t, appModel, config)
```

**With Bridge Enabled (for command handling tests):**
```go
config := testutil.DefaultTUIConfig()
appModel := SetupTeatestApp(t, config, WithBridge())
// ... rest of test
```

### Manual Pattern (For Reference)

If you need custom setup, apply this pattern manually:

```go
// 1. Use BuildWithMockDependenciesNoInit (CRITICAL - prevents Init() override)
appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
    // Minimal setup
})

// 2. Add session and configure
session := CreateTestSession(t, "test-session")
_ = appModel.list.AddInstance(session)
appModel.list.SetSelectedInstance(0)

// 3. CRITICAL: Ensure category is expanded (sessions won't render if category collapsed)
appModel.list.OrganizeByCategory()
// Note: "Uncategorized" is transformed to "Squad Sessions" by OrganizeByStrategy
appModel.list.ExpandCategory("Squad Sessions")
// FORCE re-organization to ensure expansion state takes effect
appModel.list.OrganizeByCategory()

// 4. Get config BEFORE creating teatest
config := testutil.DefaultTUIConfig()  // Returns 200x40

// 5. PRE-CONFIGURE all component dimensions
listWidth := int(float32(config.Width) * 0.3)  // 30% of width
tabsWidth := config.Width - listWidth
menuHeight := 3
errorBoxHeight := 1
contentHeight := config.Height - menuHeight - errorBoxHeight

// Set each component's size directly
appModel.list.SetSize(listWidth, contentHeight)
appModel.tabbedWindow.SetSize(tabsWidth, contentHeight)
appModel.menu.SetSize(config.Width, menuHeight)

// 6. Set app-level dimensions
appModel.termWidth = config.Width
appModel.termHeight = config.Height

// 7. CRITICAL: Disable terminal manager to prevent 80x24 override
appModel.terminalManager = nil

// 8. NOW create teatest (dimensions already configured)
tm := testutil.CreateTUITest(t, appModel, config)
```

## Why Each Step Matters

### Step 1: Use NoInit
`BuildWithMockDependencies` calls `Init()` which sends the 80x24 WindowSizeMsg. Using `BuildWithMockDependenciesNoInit` prevents this.

### Step 3: Category Expansion
Sessions are organized into categories. If a category is collapsed, its sessions don't render even if they exist in the model. The three-step organization pattern ensures expansion takes effect:
1. First `OrganizeByCategory()` - creates category groups
2. `ExpandCategory()` - marks category as expanded
3. Second `OrganizeByCategory()` - applies expansion state

### Step 5-6: Pre-Configuration
Setting dimensions BEFORE creating teatest ensures the first render uses correct sizes. BubbleTea's teatest won't send WindowSizeMsg if dimensions are already set.

### Step 7: Disable Terminal Manager
Setting `terminalManager = nil` prevents any future attempts to send dimension messages from the terminal manager.

## Tests Successfully Fixed

✅ **Core Tests Fixed:**
1. `TestFixedConfirmationModalFlow` - app/fixed_confirmation_test.go:14
2. `TestDebugCancelBehavior` - app/debug_cancel_test.go:14
3. `TestRobustConfirmationModalFlow` - app/robust_confirmation_test.go:14
4. `TestSessionCreationOverlayFix` - app/session_creation_fix_test.go:14
5. `TestStateTransitionDebug` - app/state_debug_test.go:14

✅ **Helper Functions Fixed:**
1. `createTestAppWithSession()` - app/confirmation_modal_teatest_test.go:227
2. `createTestAppWithMultipleSessions()` - app/confirmation_modal_teatest_test.go:273

✅ **Configuration Updated:**
1. `testutil.DefaultTUIConfig()` - Width increased from 80 → 200

## Files Modified

### Test Files
- `app/fixed_confirmation_test.go`
- `app/confirmation_modal_teatest_test.go`
- `app/debug_cancel_test.go`
- `app/robust_confirmation_test.go`
- `app/session_creation_fix_test.go`
- `app/state_debug_test.go`

### Test Utilities
- `testutil/teatest_helpers.go` - DefaultTUIConfig width increased to 200

## Additional Considerations

### WindowSizeMsg in Tests
If your test explicitly sends `WindowSizeMsg`, use the config dimensions:
```go
// ❌ Wrong - hardcoded 80x24
tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})

// ✅ Correct - use config
tm.Send(tea.WindowSizeMsg{Width: config.Width, Height: config.Height})
```

### Tests with Bridge Handlers
If your test needs bridge command handlers, still use NoInit:
```go
// ✅ Correct - WithBridge + NoInit
appModel := NewTestHomeBuilder().WithBridge().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {})
```

### Multiple Test Runs
When running tests together, they may interfere with each other due to state persistence. Tests pass individually but may fail in suite. This is a separate issue from the viewport fix.

## Testing the Fix

### Individual Test
```bash
go test ./app -run "^TestFixedConfirmationModalFlow$" -v
```

### Multiple Tests
```bash
go test ./app -run "^(TestRobustConfirmationModalFlow|TestSessionCreationOverlayFix|TestStateTransitionDebug)$" -v
```

### Full Suite
```bash
go test ./app -timeout=60s
```

## Common Symptoms of Missing Fix

1. **"No sessions available"** appears even though sessions were added
2. Output shows `[80D]` (80 column width) instead of `[200D]`
3. Tests timeout waiting for sessions to appear
4. Direct `appModel.list.String()` shows session but teatest output doesn't
5. Log message: "Size mismatch detected! BubbleTea: 200x40 vs Detection: 80x24"

## Verification Checklist

When applying this fix to a new test:

- [ ] Uses `BuildWithMockDependenciesNoInit` (NOT `BuildWithMockDependencies`)
- [ ] Category organization with three-step pattern (organize → expand → organize)
- [ ] Gets `config` before creating teatest
- [ ] Pre-configures ALL component dimensions (list, tabbedWindow, menu)
- [ ] Sets `termWidth` and `termHeight`
- [ ] Sets `terminalManager = nil`
- [ ] Creates teatest AFTER all configuration
- [ ] Any WindowSizeMsg uses config dimensions (NOT hardcoded 80x24)

## API Improvements (Implemented)

To prevent future viewport issues, we've created the `SetupTeatestApp()` helper function that encapsulates the entire 8-step fix pattern. This is now the **recommended approach** for all new teatest tests.

### SetupTeatestApp() Function

**Location:** `app/test_helpers.go`

**Signature:**
```go
func SetupTeatestApp(t *testing.T, config testutil.TUITestConfig, options ...TeatestAppOption) *home
```

**What It Does:**
1. Uses `BuildWithMockDependenciesNoInit` to prevent Init() dimension override
2. Pre-configures all component dimensions based on config
3. Sets app-level termWidth/termHeight for viewport-aware rendering
4. Disables terminal manager to prevent 80x24 WindowSizeMsg
5. Organizes categories and expands default "Squad Sessions" category
6. Forces re-organization to ensure expansion takes effect
7. Initializes bridge if `WithBridge()` option is provided
8. Returns fully configured app ready for teatest creation

**Options:**
- `WithBridge()` - Enables bridge initialization for command handling tests

### Benefits

**Before (Manual Pattern - 60+ lines):**
```go
appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {})
session := CreateTestSession(t, "test-session")
_ = appModel.list.AddInstance(session)
appModel.list.SetSelectedInstance(0)

appModel.list.OrganizeByCategory()
appModel.list.ExpandCategory("Squad Sessions")
appModel.list.OrganizeByCategory()

config := testutil.DefaultTUIConfig()
listWidth := int(float32(config.Width) * 0.3)
tabsWidth := config.Width - listWidth
menuHeight := 3
errorBoxHeight := 1
contentHeight := config.Height - menuHeight - errorBoxHeight

appModel.list.SetSize(listWidth, contentHeight)
appModel.tabbedWindow.SetSize(tabsWidth, contentHeight)
appModel.menu.SetSize(config.Width, menuHeight)
appModel.termWidth = config.Width
appModel.termHeight = config.Height
appModel.terminalManager = nil

tm := testutil.CreateTUITest(t, appModel, config)
```

**After (Improved API - 8 lines):**
```go
config := testutil.DefaultTUIConfig()
appModel := SetupTeatestApp(t, config)

session := CreateTestSession(t, "test-session")
_ = appModel.list.AddInstance(session)
appModel.list.SetSelectedInstance(0)

tm := testutil.CreateTUITest(t, appModel, config)
```

### Migration Guide

**For Existing Tests:**
1. Replace manual 8-step pattern with `SetupTeatestApp(t, config)`
2. Add `WithBridge()` option if test needs command handling
3. Remove manual dimension configuration code
4. Keep session creation and test-specific logic

**For New Tests:**
Always use `SetupTeatestApp()` - it guarantees correct viewport dimensions and prevents the "No sessions available" issue.

### Example Migrations

**Standard Test:**
```go
// Old
appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {})
// ... 50+ lines of dimension configuration ...
tm := testutil.CreateTUITest(t, appModel, config)

// New
config := testutil.DefaultTUIConfig()
appModel := SetupTeatestApp(t, config)
tm := testutil.CreateTUITest(t, appModel, config)
```

**Test with Bridge:**
```go
// Old
appModel := NewTestHomeBuilder().WithBridge().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {})
// ... 50+ lines of dimension configuration ...
tm := testutil.CreateTUITest(t, appModel, config)

// New
config := testutil.DefaultTUIConfig()
appModel := SetupTeatestApp(t, config, WithBridge())
tm := testutil.CreateTUITest(t, appModel, config)
```

## References

- Original investigation: app/fixed_confirmation_test.go
- Debug script: /tmp/debug_expansion.go
- Test helper utilities: testutil/teatest_helpers.go
- App initialization: app/app.go:1418-1422 (Init method)
- Dimension handling: app/app.go:1307-1318 (WindowSizeMsg handling)
