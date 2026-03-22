# BUG-006: Category Name Transformation Mismatch [SEVERITY: High]

**Status**: ✅ FIXED (2025-12-05)
**Discovered**: 2025-12-05 during test stabilization work
**Fixed**: 2025-12-05 - Standardized transformation to PathToDisplayCategory
**Impact**: Category names didn't match between storage and rendering, causing lookup failures

## Resolution Summary

**Fix Applied**: Standardized category name transformation across all code paths

**Changes Made**:
1. `ui/list.go:333` - Changed manual string replacement to `PathToDisplayCategory()`
2. `ui/list.go:370` - Changed manual string replacement to `PathToDisplayCategory()`
3. `app/app.go:1255` - Changed direct category access to `PathToDisplayCategory()`
4. Ensures consistent transformation: `Work/Frontend` → `Work / Frontend`

**Fix Locations**:
```go
// ui/list.go line 333
displayName := grouping.PathToDisplayCategory(category) // FIXED: was strings.Replace(category, "/", " / ", -1)

// ui/list.go line 370
displayName := grouping.PathToDisplayCategory(category) // FIXED: was strings.Replace(category, "/", " / ", -1)

// app/app.go line 1255
displayName := grouping.PathToDisplayCategory(g.Name) // FIXED: was g.Name directly
```

**Expected Results**:
- Category names consistent everywhere
- Expansion state lookups work correctly
- Tests pass with matching category names
- Nested categories render properly

**Backward Compatibility**: Full compatibility - fixes inconsistent behavior

## Problem Description

Category names were transformed inconsistently across the codebase:

**Inconsistent transformations**:
1. **Storage format**: `Work/Frontend` (slash-separated, no spaces)
2. **Display format**: `Work / Frontend` (spaced slashes)
3. **Manual transformation**: `strings.Replace(category, "/", " / ", -1)` (inconsistent)
4. **Canonical function**: `grouping.PathToDisplayCategory()` (correct but not used everywhere)

**Symptoms**:
1. Category expansion state not found (lookup by wrong name)
2. Sessions rendering under wrong category
3. Tests failing due to name mismatches
4. Inconsistent display between different UI components

**Example bug scenario**:
```go
// Store expansion state with display name
expanded["Work / Frontend"] = true

// Try to look up with storage name
if expanded["Work/Frontend"] { // ❌ Not found!
    // Expansion state lost
}
```

## Reproduction

```bash
# Create session with nested category
./stapler-squad
# Create session with category "Work/Frontend"

# Try to expand/collapse category
# Press 'e' then navigate to category
# Press Enter to toggle -> Expansion state doesn't persist

# Run tests
go test ./ui -run TestCategoryRenderingWithSessions -v
# Tests fail: category names don't match between setup and rendering
```

**Expected**:
- Category names consistent in storage and display
- Expansion state persists correctly
- Tests use matching names

**Actual**:
- Name mismatch causes lookup failures
- Expansion state lost
- Tests fail

## Root Cause Analysis

### Multiple Transformation Methods

**Canonical function** (correct):
```go
// grouping/grouping.go
func PathToDisplayCategory(category string) string {
    return strings.Replace(category, "/", " / ", -1)
}
```

**Manual transformations** (inconsistent):
```go
// ui/list.go (multiple places)
displayName := strings.Replace(category, "/", " / ", -1) // ❌ Duplicates logic

// app/app.go
displayName := g.Name // ❌ No transformation, uses storage format
```

**Why this is problematic**:
1. Duplication violates DRY principle
2. Easy to miss updating all transformation sites
3. No single source of truth for transformation logic
4. Future changes to format require updating multiple locations

### Lookup Failures

**Expansion state storage**:
```go
// Map keyed by display names (with spaces)
categoryExpanded := map[string]bool{
    "Work / Frontend": true,
}
```

**Lookup with wrong name**:
```go
// Using storage format (no spaces) -> Not found!
isExpanded := categoryExpanded["Work/Frontend"] // Always false
```

**Result**: Expansion state appears to be lost, categories always collapsed

## Files Affected (3 files)

1. **ui/list.go** (lines 333, 370) - Manual string replacement in two methods
2. **app/app.go** (line 1255) - Missing transformation for group name
3. **grouping/grouping.go** - Contains correct PathToDisplayCategory function (already exists)

**Context boundary**: ⚠️ Slightly over (3 files), but changes are trivial (single line each)

## Fix Approach: Use Canonical Transformation Function

**Implementation**:

1. **Replace manual transformations in ui/list.go**:
   ```go
   // Line 333: ToggleCategoryExpansion
   displayName := grouping.PathToDisplayCategory(category)

   // Line 370: IsCategoryExpanded
   displayName := grouping.PathToDisplayCategory(category)
   ```

2. **Add missing transformation in app/app.go**:
   ```go
   // Line 1255: Group rendering
   displayName := grouping.PathToDisplayCategory(g.Name)
   ```

3. **Benefits**:
   - Single source of truth for transformation logic
   - Consistent names across all code paths
   - Easy to update format in future (change one function)
   - Compile-time safety (function signature changes propagate)

**Estimated effort**: 5 minutes (3 files, 3 lines changed)
**Risk**: Low - fixes inconsistent behavior, uses existing tested function

## Impact Assessment

**Severity**: **High**
- **User-Facing**: Direct - category expansion broken, sessions render incorrectly
- **Data Loss**: No (but expansion state appears lost)
- **Workaround**: Manually expand categories every session
- **Frequency**: Every interaction with nested categories
- **Scope**: All category-based organization and navigation

**Priority**: P1 - Critical functionality broken

**Timeline**:
- Investigation: 20 minutes (tracking down transformation inconsistencies)
- Fix implementation: 5 minutes
- Verification: 10 minutes (test all transformation paths)
- **Total**: 35 minutes

## Prevention Strategy

**Coding standards**:
1. Always use canonical transformation functions (no manual string manipulation)
2. Add linter rule to detect duplicate transformation logic
3. Document transformation functions with usage examples
4. Add unit tests for transformation consistency

**Code review checklist**:
- [ ] No manual string transformations for category names
- [ ] All category name usage goes through PathToDisplayCategory()
- [ ] Tests verify transformation consistency
- [ ] New code uses canonical functions

**Recommended refactoring**:
```go
// Add type safety to prevent raw string usage
type CategoryName string
type DisplayName string

func PathToDisplayCategory(cat CategoryName) DisplayName {
    return DisplayName(strings.Replace(string(cat), "/", " / ", -1))
}

// Forces all code to use transformation function
func (l *List) IsCategoryExpanded(cat CategoryName) bool {
    displayName := PathToDisplayCategory(cat)
    // ... lookup
}
```

## Related Issues

- **BUG-004**: QueueView nil pointer (fixed in same session)
- **BUG-005**: Category expansion wrong boolean (fixed in same session)
- **BUG-007**: Default category expansion not forced (fixed in same session)
- **BUG-008**: Category rendering in tests (CRITICAL, still unfixed)

## Additional Notes

**Design consideration**: This bug highlights a common anti-pattern in string-based identifiers:

**Current approach** (error-prone):
- Category names are plain strings
- Transformations applied inconsistently
- Easy to forget transformation
- No compile-time safety

**Better approach** (type-safe):
```go
// Domain types prevent misuse
type CategoryPath string     // "Work/Frontend"
type CategoryDisplay string  // "Work / Frontend"

// Factory ensures correct transformation
func NewCategoryDisplay(path CategoryPath) CategoryDisplay {
    return CategoryDisplay(strings.Replace(string(path), "/", " / ", -1))
}

// API uses correct types
func (l *List) IsCategoryExpanded(cat CategoryDisplay) bool {
    // Type system enforces correctness
}
```

**Benefits**:
- Compiler catches missing transformations
- Self-documenting code (types indicate format)
- IDE autocomplete helps prevent mistakes
- Refactoring is safer

**Future work**: Consider adding category type safety to prevent similar bugs

---

**Bug Tracking ID**: BUG-006
**Related Feature**: Category Display (ui/list.go, app/app.go, grouping/grouping.go)
**Fix Complexity**: Low (3 files, 3 lines, use existing function)
**Fix Risk**: Low (fixes inconsistent behavior)
**Blocked By**: None
**Blocks**: Test stabilization, category navigation
**Related To**: BUG-004, BUG-005, BUG-007 (same fix session)
