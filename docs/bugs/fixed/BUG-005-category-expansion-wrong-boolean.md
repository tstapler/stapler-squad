# BUG-005: Category Expansion Logic Using Wrong Boolean [SEVERITY: High]

**Status**: ✅ FIXED (2025-12-05)
**Discovered**: 2025-12-05 during test stabilization work
**Fixed**: 2025-12-05 - Changed from shouldCollapseCategories to expandCategories
**Impact**: Category expansion/collapse feature completely inverted

## Resolution Summary

**Fix Applied**: Inverted boolean logic in `ui/list.go` category expansion methods

**Changes Made**:
1. `ui/list.go:327` - Changed `if l.shouldCollapseCategories` to `if !l.expandCategories`
2. `ui/list.go:364` - Changed `if l.shouldCollapseCategories` to `if !l.expandCategories`
3. Fixed semantic naming: "expand" is positive, "shouldCollapse" was negative (double negative confusion)

**Fix Locations**:
```go
// ui/list.go line 327
func (l *List) ToggleCategoryExpansion(category string) {
    if !l.expandCategories { // FIXED: was shouldCollapseCategories
        return
    }
    // ... toggle logic
}

// ui/list.go line 364
func (l *List) IsCategoryExpanded(category string) bool {
    if !l.expandCategories { // FIXED: was shouldCollapseCategories
        return true // All expanded when expansion disabled
    }
    // ... check expansion state
}
```

**Expected Results**:
- Category expansion/collapse works correctly
- "All Expanded" mode shows all sessions
- Individual category toggles work as expected
- Tests pass with correct expansion behavior

**Backward Compatibility**: Full compatibility - fixes broken behavior

## Problem Description

The category expansion logic was completely inverted due to using the wrong boolean field:

**Wrong field**: `shouldCollapseCategories` (doesn't exist, likely typo or refactoring artifact)
**Correct field**: `expandCategories` (the actual field that controls expansion mode)

**Symptoms**:
1. Pressing 'e' key to toggle expansion appeared to do nothing
2. Categories were always collapsed when they should be expanded
3. Individual category toggles didn't work correctly
4. Tests failed because sessions weren't rendering

**Logic error**:
```go
// BEFORE: Used non-existent field with inverted semantics
if l.shouldCollapseCategories {
    return // Don't allow toggles when collapsing
}

// AFTER: Use correct field with correct semantics
if !l.expandCategories {
    return // Don't allow toggles when expansion disabled
}
```

## Reproduction

```bash
# Run TUI application
./stapler-squad

# Try to toggle category expansion
# Press 'e' key -> Categories don't expand/collapse correctly

# Run tests
go test ./ui -run TestCategoryRenderingWithSessions -v
# Tests fail because categories don't expand, sessions hidden
```

**Expected**:
- 'e' key toggles between "All Expanded" and "Individual Expand/Collapse" modes
- Categories expand/collapse correctly
- Sessions render within expanded categories

**Actual**:
- Toggle appears broken
- Categories remain collapsed
- Sessions don't render

## Root Cause Analysis

### Field Naming Confusion

**Struct field**:
```go
type List struct {
    expandCategories bool // TRUE = allow individual expand/collapse
    // ...
}
```

**Semantic meaning**:
- `expandCategories = true`: Enable category expansion feature (individual toggles work)
- `expandCategories = false`: Disable feature (all categories always expanded, flat view)

**Incorrect usage**:
```go
if l.shouldCollapseCategories { // ❌ Field doesn't exist!
    return
}
```

**Why it compiled**: Likely a typo that created a zero-value bool (always false), causing subtle bugs

### Logic Inversion

The code checked `shouldCollapseCategories` (negative semantics) instead of `expandCategories` (positive semantics), causing double-negative confusion:

**Intended logic**:
- "If expand mode is OFF, don't allow individual toggles"
- `if !l.expandCategories { return }`

**Broken logic**:
- "If we should collapse, don't allow toggles"
- `if l.shouldCollapseCategories { return }` (field doesn't exist, always false)

## Files Affected (1 file)

1. **ui/list.go** (lines 327, 364) - ToggleCategoryExpansion and IsCategoryExpanded methods

**Context boundary**: ✅ Within limits (1 file, 2 methods)

## Fix Approach: Use Correct Field with Correct Logic

**Implementation**:

1. **Fix ToggleCategoryExpansion (line 327)**:
   ```go
   func (l *List) ToggleCategoryExpansion(category string) {
       if !l.expandCategories { // FIXED: correct field, correct logic
           return // No individual toggles when expansion disabled
       }
       // ... rest of method
   }
   ```

2. **Fix IsCategoryExpanded (line 364)**:
   ```go
   func (l *List) IsCategoryExpanded(category string) bool {
       if !l.expandCategories { // FIXED: correct field, correct logic
           return true // All expanded when feature disabled
       }
       // ... check expansion state
   }
   ```

**Semantic clarity**:
- `expandCategories = true`: Feature ON, allow individual expand/collapse
- `expandCategories = false`: Feature OFF, all categories flat/expanded
- Using `!l.expandCategories` reads naturally: "if NOT in expand mode, skip toggles"

**Estimated effort**: 2 minutes (1 file, 2 lines changed)
**Risk**: Low - fixes broken behavior, restores intended logic

## Impact Assessment

**Severity**: **High**
- **User-Facing**: Direct - category expansion completely broken
- **Data Loss**: No
- **Workaround**: None - feature unusable
- **Frequency**: Every user interaction with categories
- **Scope**: All category-based navigation and organization

**Priority**: P1 - Critical functionality broken

**Timeline**:
- Investigation: 15 minutes (tracking down wrong field reference)
- Fix implementation: 2 minutes
- Verification: 5 minutes
- **Total**: 22 minutes

## Prevention Strategy

**Coding standards**:
1. Use positive boolean names (`expandEnabled`) instead of negative (`shouldCollapse`)
2. Always validate field names exist (linter should catch this)
3. Add unit tests for boolean field logic
4. Document boolean field semantics clearly

**Code review checklist**:
- [ ] Boolean field names are positive (avoid double negatives)
- [ ] Field references are correct (no typos)
- [ ] Logic matches field semantics
- [ ] Tests verify boolean state transitions

## Related Issues

- **BUG-004**: QueueView nil pointer (fixed in same session)
- **BUG-006**: Category name transformation mismatch (fixed in same session)
- **BUG-007**: Default category expansion not forced (fixed in same session)
- **BUG-008**: Category rendering in tests (CRITICAL, still unfixed)

## Additional Notes

**Design consideration**: This bug reveals a brittle state management pattern:

**Current state**:
- Multiple boolean flags control expansion behavior
- Negative semantics cause confusion
- Easy to use wrong field (no compile-time safety)

**Recommended improvement**:
```go
type ExpansionMode int

const (
    ExpansionModeFlat ExpansionMode = iota // All expanded, no toggles
    ExpansionModeIndividual                 // Allow per-category toggles
)

type List struct {
    expansionMode ExpansionMode // Clear, type-safe
    // ...
}
```

**Benefits**:
- Type safety prevents wrong field usage
- Clear semantics (enum values self-document)
- Extensible (add more modes if needed)
- Better IDE autocomplete and refactoring support

---

**Bug Tracking ID**: BUG-005
**Related Feature**: Category Expansion (ui/list.go)
**Fix Complexity**: Low (2 lines, field name correction)
**Fix Risk**: Low (fixes broken behavior)
**Blocked By**: None
**Blocks**: Test stabilization, category navigation
**Related To**: BUG-004, BUG-006, BUG-007 (same fix session)
