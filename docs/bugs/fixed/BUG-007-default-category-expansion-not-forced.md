# BUG-007: Default Category Expansion Not Forcing True [SEVERITY: High]

**Status**: ✅ FIXED (2025-12-05)
**Discovered**: 2025-12-05 during test stabilization work
**Fixed**: 2025-12-05 - Forced "All" category to be always expanded
**Impact**: Default "All" category appeared collapsed, hiding all sessions

## Resolution Summary

**Fix Applied**: Added forced expansion for "All" category in `ui/list.go`

**Changes Made**:
1. `ui/list.go:364` - Added check to force "All" category to return `true`
2. Ensures default uncategorized sessions always visible

**Fix Locations**:
```go
// ui/list.go lines 364-377
func (l *List) IsCategoryExpanded(category string) bool {
    if !l.expandCategories {
        return true // All expanded when expansion disabled
    }

    // FIXED: Force "All" category to be always expanded
    if category == "All" {
        return true
    }

    displayName := grouping.PathToDisplayCategory(category)
    expanded, exists := l.categoryExpanded[displayName]
    if !exists {
        return true // New categories default to expanded
    }
    return expanded
}
```

**Expected Results**:
- "All" category always expanded (can't be collapsed)
- Default sessions always visible
- Tests pass with sessions rendering
- Other categories toggle normally

**Backward Compatibility**: Full compatibility - fixes broken default behavior

## Problem Description

The "All" category (default category for uncategorized sessions) was not being forced to expand, causing all sessions to be hidden by default:

**Symptoms**:
1. New users see empty UI (all sessions hidden under collapsed "All")
2. Tests fail because sessions don't render
3. Confusing UX: appears like no sessions exist
4. Users must manually expand "All" category to see any sessions

**Logic flaw**:
```go
// BEFORE: "All" category treated like any other
func (l *List) IsCategoryExpanded(category string) bool {
    displayName := grouping.PathToDisplayCategory(category)
    expanded, exists := l.categoryExpanded[displayName]
    return expanded // Could be false!
}

// AFTER: "All" category always expanded
func (l *List) IsCategoryExpanded(category string) bool {
    if category == "All" {
        return true // Force expanded
    }
    // ... rest of logic
}
```

## Reproduction

```bash
# Start fresh TUI with no expansion state
rm ~/.stapler-squad/state.json
./stapler-squad

# Create sessions without categories
# Press 'n' -> leave category empty -> create session

# Observe: Sessions don't appear in list!
# "All" category is collapsed by default

# Run tests
go test ./ui -run TestCategoryRenderingWithSessions -v
# Tests fail: expected sessions to render, but "All" collapsed
```

**Expected**:
- "All" category always expanded (uncollapsible)
- Sessions visible immediately after creation
- Tests pass with sessions rendering

**Actual**:
- "All" category can be collapsed
- Sessions hidden by default
- Tests fail

## Root Cause Analysis

### Special Category Semantics

The "All" category serves as the **default bucket** for sessions without explicit categories:

**Category types**:
1. **User-defined categories**: "Work", "Personal", "Work/Frontend" (can toggle)
2. **Default category**: "All" (should always be expanded)

**Why "All" is special**:
- Contains uncategorized sessions (most common use case)
- Serves as the main view for simple workflows
- New users create sessions without categories first
- Collapsing "All" hides the entire session list (bad UX)

**Design intent**:
- "All" should behave like "no categorization" (flat view)
- Should not be collapsible like user-defined categories
- Always expanded = always visible

### Missing Special Case Handling

**IsCategoryExpanded logic**:
```go
func (l *List) IsCategoryExpanded(category string) bool {
    // Check expansion state map
    expanded, exists := l.categoryExpanded[displayName]

    // BUG: Returns false if "All" not in map!
    return expanded // ❌ Can be false for "All"
}
```

**What should happen**:
```go
func (l *List) IsCategoryExpanded(category string) bool {
    // Special case: "All" always expanded
    if category == "All" {
        return true // ✅ Force expanded
    }

    // Normal categories can toggle
    expanded, exists := l.categoryExpanded[displayName]
    return expanded
}
```

## Files Affected (1 file)

1. **ui/list.go** (line 364) - IsCategoryExpanded method

**Context boundary**: ✅ Within limits (1 file, single method, 3 lines added)

## Fix Approach: Force "All" Category Expansion

**Implementation**:

1. **Add special case check before normal logic**:
   ```go
   func (l *List) IsCategoryExpanded(category string) bool {
       if !l.expandCategories {
           return true // All expanded when expansion disabled
       }

       // ADDED: Force "All" category to be always expanded
       if category == "All" {
           return true
       }

       // Normal category expansion logic
       displayName := grouping.PathToDisplayCategory(category)
       expanded, exists := l.categoryExpanded[displayName]
       if !exists {
           return true // New categories default to expanded
       }
       return expanded
   }
   ```

2. **Semantic meaning**:
   - "All" category = no categorization = always expanded
   - User-defined categories = can toggle expand/collapse
   - Clear separation between default and custom categories

**Estimated effort**: 2 minutes (1 file, 3 lines added)
**Risk**: Low - fixes broken default behavior, matches user expectations

## Impact Assessment

**Severity**: **High**
- **User-Facing**: Direct - sessions appear hidden to new users
- **Data Loss**: No (but appears like sessions are missing)
- **Workaround**: Manually expand "All" category (non-obvious)
- **Frequency**: Every new user, every fresh state file
- **Scope**: Default session visibility, first-run experience

**Priority**: P1 - Critical first-run UX issue

**Timeline**:
- Investigation: 10 minutes (identifying special category semantics)
- Fix implementation: 2 minutes
- Verification: 5 minutes
- **Total**: 17 minutes

## Prevention Strategy

**Coding standards**:
1. Document special-case categories in code comments
2. Add unit tests for special category behavior
3. Test first-run experience (empty state files)
4. Consider enum for category types (Default vs Custom)

**Code review checklist**:
- [ ] Special categories documented
- [ ] Default behavior tested
- [ ] First-run UX verified
- [ ] State initialization includes default categories

**Recommended enhancement**:
```go
// Define category types explicitly
type CategoryType int

const (
    CategoryTypeDefault CategoryType = iota // "All" - always expanded
    CategoryTypeUser                        // User-defined - can toggle
)

type Category struct {
    Name string
    Type CategoryType
}

// Helper to determine if category can be collapsed
func (c Category) IsCollapsible() bool {
    return c.Type == CategoryTypeUser
}
```

## Related Issues

- **BUG-004**: QueueView nil pointer (fixed in same session)
- **BUG-005**: Category expansion wrong boolean (fixed in same session)
- **BUG-006**: Category name transformation mismatch (fixed in same session)
- **BUG-008**: Category rendering in tests (CRITICAL, still unfixed)

## Additional Notes

**UX consideration**: This bug reveals a fundamental UX issue with category expansion:

**Current behavior**:
- All categories (including "All") can be collapsed
- New users see collapsed "All" by default
- Appears like no sessions exist (confusing)

**Better UX**:
1. **Option A**: "All" is not a real category (don't show it when there are no other categories)
2. **Option B**: "All" is always expanded and can't be collapsed (current fix)
3. **Option C**: Flat view by default, categories only appear when user creates them

**Current fix implements Option B** (simplest, safest)

**Future consideration**:
- Remove "All" category when no user-defined categories exist (cleaner UX)
- Show flat list by default, enable categories on first user-created category
- Add onboarding tooltip explaining category expansion

**Testing improvement**:
```go
// Test that "All" category is always expanded
func TestAllCategoryAlwaysExpanded(t *testing.T) {
    list := NewList(100, 50)

    // Even if explicitly collapsed in state
    list.categoryExpanded["All"] = false

    // Should still return true
    assert.True(t, list.IsCategoryExpanded("All"))
}
```

---

**Bug Tracking ID**: BUG-007
**Related Feature**: Category Expansion (ui/list.go)
**Fix Complexity**: Low (1 file, 3 lines)
**Fix Risk**: Low (fixes broken default behavior)
**Blocked By**: None
**Blocks**: Test stabilization, first-run UX
**Related To**: BUG-004, BUG-005, BUG-006 (same fix session)
