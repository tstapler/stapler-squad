# BUG-008: Category Rendering in Tests - Sessions Don't Render Despite Existing [SEVERITY: Critical]

**Status**: 🐛 Open
**Discovered**: 2025-12-05 during test stabilization work
**Impact**: Test suite completely broken, prevents verification of rendering logic

## Problem Description

Despite fixing BUG-004 through BUG-007, the category rendering test still fails. Sessions exist in the test setup but **do not render** in the category groups:

**Test output**:
```
=== RUN   TestCategoryRenderingWithSessions
    list_test.go:123:
        Expected sessions in rendered output but none were found.

        Rendered output (length 1039):
        ┌──────────────────────────────────────────────┐
        │ Sessions                                     │
        ├──────────────────────────────────────────────┤
        │                                              │
        │ > ▼ All (1)                                  │  <- Category shows count!
        │                                              │  <- But no sessions rendered!
        │                                              │
        │                                              │
        ├──────────────────────────────────────────────┤
        │ n: New  D: Delete  g: Group Sessions        │
        │ 0/0 sessions                                 │  <- Counter shows 0!
        └──────────────────────────────────────────────┘
```

**Key mystery**:
- Category header shows "▼ All (1)" = 1 session in category ✅
- Session counter shows "0/0 sessions" = no sessions rendered ❌
- Test setup creates session successfully (verified)
- Session exists in `list.items` (verified)
- But `getVisibleItems()` returns empty array

## Reproduction

```bash
# Run the category rendering test
go test ./ui -run TestCategoryRenderingWithSessions -v

# Output shows:
# - Category count: 1 session
# - Visible items: 0 sessions
# - Rendered output: empty list
```

**Expected**:
- Session renders under "All" category
- Visible items count matches session count
- Test passes with session in output

**Actual**:
- Category shows count but no sessions render
- Visible items returns empty
- Test fails

## Root Cause Analysis

**Potential issues** (requires investigation):

### 1. Filtering Logic Bug
```go
// ui/list.go - getVisibleItems() method
func (l *List) getVisibleItems() []session.Instance {
    // Complex filtering logic:
    // - Category filtering
    // - Status filtering (showPaused)
    // - Search filtering
    // - Tag filtering

    // BUG: One of these filters incorrectly excludes sessions?
}
```

**Investigation needed**:
- Add debug logging to each filter stage
- Check if sessions pass category filter
- Verify status filter logic (showPaused flag)
- Check search filter (empty search should match all)

### 2. Category Group Membership Bug
```go
// Session might not be in correct category group
// Check: Does getGroupForSession() return expected group?
```

**Investigation needed**:
- Verify session category assignment in test
- Check getGroupForSession() return value
- Validate category group construction

### 3. Visibility State Bug
```go
// Session might be marked as hidden/filtered in test state
```

**Investigation needed**:
- Check initial visibility state
- Verify no filters active in test setup
- Validate showPaused flag state

### 4. Rendering vs Model Mismatch
```go
// Category count might be from cached model
// But rendering uses filtered/visible items
```

**Investigation needed**:
- Check where "(1)" count comes from
- Verify count matches getVisibleItems() result
- Look for model/view synchronization bugs

## Files Affected (2-4 files)

1. **ui/list.go** (primary suspect) - getVisibleItems() filtering logic
2. **ui/list_test.go** - Test setup and assertions
3. **grouping/grouping.go** - Category group construction (possibly)
4. **app/app.go** - Rendering coordination (possibly)

**Context boundary**: ⚠️ Potentially complex (multiple files, filtering logic)

## Investigation Steps

### Phase 1: Isolate the Issue (1 hour)

1. **Add debug logging to getVisibleItems()**:
   ```go
   func (l *List) getVisibleItems() []session.Instance {
       log.Printf("DEBUG: Total items: %d", len(l.items))
       log.Printf("DEBUG: Category filter: %v", l.categoryFilter)
       log.Printf("DEBUG: Show paused: %v", l.showPaused)
       log.Printf("DEBUG: Search filter: %v", l.searchFilter)

       // Log each filter stage
       filtered := l.items
       log.Printf("DEBUG: After category filter: %d", len(filtered))
       // ... etc

       return filtered
   }
   ```

2. **Add debug logging to test**:
   ```go
   func TestCategoryRenderingWithSessions(t *testing.T) {
       // After setup
       t.Logf("Created session: %+v", session)
       t.Logf("List items count: %d", len(list.items))
       t.Logf("Visible items count: %d", len(list.getVisibleItems()))
       t.Logf("Category groups: %+v", list.categoryGroups)
   }
   ```

3. **Check category group construction**:
   ```go
   // Verify session is in correct group
   for _, group := range list.categoryGroups {
       t.Logf("Group %s: %d sessions", group.Name, len(group.Items))
       for _, item := range group.Items {
           t.Logf("  - Session: %s", item.Title)
       }
   }
   ```

### Phase 2: Fix the Root Cause (1-2 hours)

Based on investigation findings, apply targeted fix:

**If filtering bug**:
- Fix getVisibleItems() filter logic
- Add unit tests for each filter stage
- Verify filter combinations work correctly

**If category assignment bug**:
- Fix getGroupForSession() logic
- Ensure sessions assigned to correct categories
- Validate category group construction

**If visibility state bug**:
- Fix initial state setup in tests
- Ensure no filters active by default
- Validate showPaused flag

### Phase 3: Comprehensive Testing (30 minutes)

1. **Run all UI tests**:
   ```bash
   go test ./ui -v
   ```

2. **Run category-specific tests**:
   ```bash
   go test ./ui -run TestCategory -v
   ```

3. **Manual TUI verification**:
   ```bash
   ./stapler-squad
   # Create sessions
   # Verify rendering
   # Toggle filters
   # Verify categories
   ```

## Expected Fix Outcomes

After investigation and fix:
- Sessions render correctly in categories ✅
- getVisibleItems() returns expected sessions ✅
- Category count matches visible items ✅
- Tests pass without false negatives ✅
- Manual TUI verification confirms fix ✅

## Impact Assessment

**Severity**: **Critical**
- **User-Facing**: Indirect (tests only, but blocks development)
- **Data Loss**: No
- **Workaround**: None (can't verify rendering changes)
- **Frequency**: Every test run
- **Scope**: Entire test suite, blocks all rendering verification

**Priority**: P1 - Blocks test stabilization and further development

**Timeline**:
- Investigation: 1-2 hours (isolate root cause)
- Fix implementation: 1-2 hours (depends on complexity)
- Verification: 30 minutes
- **Total**: 2.5-4.5 hours

## Prevention Strategy

**Testing improvements**:
1. Add more granular unit tests for filtering logic
2. Test each filter stage independently
3. Add integration tests for filter combinations
4. Mock fewer dependencies to catch bugs earlier

**Code quality**:
1. Simplify getVisibleItems() filtering logic
2. Break down into smaller, testable functions
3. Add validation for filter state consistency
4. Document filter precedence and interaction

**Debug tooling**:
1. Add DEBUG build flag for verbose logging
2. Create test utilities for state inspection
3. Add assertions for intermediate filter stages
4. Build visualization tools for filter pipeline

## Related Issues

- **BUG-004**: QueueView nil pointer ✅ FIXED
- **BUG-005**: Category expansion wrong boolean ✅ FIXED
- **BUG-006**: Category name transformation mismatch ✅ FIXED
- **BUG-007**: Default category expansion not forced ✅ FIXED
- **Test Stabilization Epic**: See `docs/tasks/test-stabilization-and-teatest-integration.md`

## Additional Notes

**Why this is critical**:

Despite fixing 4 related bugs (BUG-004 through BUG-007), the test still fails. This indicates:

1. **Deeper issue**: More fundamental problem with rendering or filtering logic
2. **Test-specific bug**: Issue only manifests in test environment (e.g., missing initialization)
3. **Complex interaction**: Multiple bugs interacting in unexpected ways
4. **False test**: Test expectations might be wrong (verify manually in TUI)

**Recommended approach**:

1. **Start with manual TUI verification** (5 minutes)
   - If rendering works in TUI, this is a test setup issue
   - If rendering broken in TUI, this is a production bug

2. **Add comprehensive debug logging** (30 minutes)
   - Trace execution from session creation to rendering
   - Log every filter stage and state transition
   - Identify exact point where sessions disappear

3. **Compare test vs production paths** (30 minutes)
   - Find differences in initialization
   - Check for missing setup steps
   - Validate test environment matches production

4. **Fix root cause** (1-2 hours)
   - Apply targeted fix based on findings
   - Add regression tests
   - Verify no side effects

**Don't proceed with more blind fixes** - Investigation required to avoid wasting time on wrong hypotheses.

---

**Bug Tracking ID**: BUG-008
**Related Feature**: Category Rendering (ui/list.go, ui/list_test.go)
**Fix Complexity**: Unknown (requires investigation)
**Fix Risk**: Medium-High (core rendering logic)
**Blocked By**: Investigation needed
**Blocks**: All UI test development, rendering verification
**Related To**: BUG-004, BUG-005, BUG-006, BUG-007 (fixed precursors)
