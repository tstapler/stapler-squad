# Feature Plan: Fix Test Failures for Commit Readiness

## Overview

This plan addresses the test failures preventing the current changeset (145 files, +30,599/-2,203 lines) from being commit-ready. The build passes, but multiple test suites fail.

## Current Status

### Build Status
- **Go Build**: PASS
- **Web App Build**: PASS
- **Go Format**: Fixed (20 files formatted)

### Test Failure Summary

| Category | Package | Failures | Root Cause |
|----------|---------|----------|------------|
| TypeScript | CircularBuffer.test.ts | 1 | Type narrowing issue |
| Go - Status Detector | session | ~30 | Disabled TestsFailing patterns |
| Go - Filtering Service | app/services | 5 | List filtering behavior mismatch |
| Go - Tmux | session/tmux | 5 | Missing method + pattern mismatch |
| Go - UI Tests | test/ui | ~15 | Snapshot/behavior drift |
| Go - UI Component | ui | 1 | Filtering edge case |
| Go - Session | session | 5 | Migration + Review Queue |
| Go - TUI Modal | app | 11 | Teatest timing/modal issues |
| Go - Testutil | testutil | 1 | Timeout (600s) |

---

## Phase 1: Quick Wins (Estimated: 30 minutes)

### Task 1.1: Fix TypeScript Type Error

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/web-app/src/lib/terminal/CircularBuffer.test.ts`

**Line 283**: Type 'boolean | undefined' is not assignable to type 'boolean'

**Root Cause**: The `filter` callback expects explicit boolean return, but the expression `line.tags?.includes("deploy")` returns `boolean | undefined` due to optional chaining.

**Fix**:
```typescript
// Before (line 283)
const deployLines = buffer.filter(line => line.tags?.includes("deploy"));

// After
const deployLines = buffer.filter(line => line.tags?.includes("deploy") ?? false);
// OR
const deployLines = buffer.filter(line => Boolean(line.tags?.includes("deploy")));
```

**Verification**: `npm run type-check` or `npx tsc --noEmit`

---

## Phase 2: Root Cause Fix - Status Detector (Estimated: 1 hour)

### Problem Analysis

The `StatusTestsFailing` patterns were intentionally disabled in `status_detector.go` (line 421-425):

```go
// TestsFailing: DISABLED - These patterns cause too many false positives.
// Test output varies wildly across languages/frameworks, and matching "FAIL"
// anywhere in output catches non-test-related content.
TestsFailing: []StatusPattern{},
```

However, a test file `status_detector_tests_failing_test.go` still expects these patterns to work, causing **30+ test failures**.

### Decision Point

**Option A: Re-enable patterns with refined regex**
- Pro: Feature works as designed
- Con: Risk of false positives in production

**Option B: Delete or skip the test file**
- Pro: Tests pass immediately
- Con: Loses test coverage for future pattern implementation

**Option C: Add build tag to conditionally include tests**
- Pro: Tests can be run when patterns are re-enabled
- Con: Additional complexity

### Recommended Approach: Option B or C

Since the patterns are intentionally disabled with documented rationale, the tests should be either:
1. **Deleted** if the feature is permanently disabled
2. **Skipped** with a skip message referencing the disabled patterns
3. **Build-tagged** for conditional execution

### Task 2.1: Disable Status Detector TestsFailing Tests

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/session/status_detector_tests_failing_test.go`

**Action**: Add `t.Skip()` to each test function with explanation:

```go
func TestStatusDetector_TestsFailingDetection(t *testing.T) {
    t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
    // ... existing test code
}
```

**Affected Tests**:
- `TestStatusDetector_TestsFailingDetection` (19 sub-tests)
- `TestStatusDetector_TestsFailingWithContext` (5 sub-tests)
- `TestStatusDetector_TestsFailingPriority` (3 sub-tests)
- `TestStatusDetector_TestsFailingMultiline`
- `TestStatusDetector_TestsFailingRealWorldExamples` (3 sub-tests)
- `TestStatusDetector_TestsFailingPatternNames`

---

## Phase 3: Filtering Service Tests (Estimated: 1-2 hours)

### Problem Analysis

The filtering service tests expect the `List.GetVisibleItems()` method to reflect search/filter state immediately after calling `FilteringService` methods. However, the actual `List` implementation may:
1. Use caching that isn't immediately invalidated
2. Require UI event processing before state updates
3. Have async debouncing for search operations

### Failing Tests

| Test | Expected | Issue |
|------|----------|-------|
| `TestFilteringService_UpdateSearchQuery` | 1 item | GetVisibleItems returns wrong count |
| `TestFilteringService_ClearSearch` | 3 items | Search state not cleared properly |
| `TestFilteringService_TogglePausedFilter` | 2 items | Filter not applied |
| `TestFilteringService_CombinedFilters` | 0 items | Combined state mismatch |
| `TestFilteringService_SearchWithNoMatches` | 0 items | Empty result handling |

### Task 3.1: Investigate List Cache Invalidation

**Files to check**:
- `/Users/tylerstapler/IdeaProjects/claude-squad/ui/list.go` - `invalidateVisibleCache()` calls
- `/Users/tylerstapler/IdeaProjects/claude-squad/app/services/filtering.go` - service implementation

**Hypothesis**: The `List.SearchByTitle()` method invalidates cache, but `GetVisibleItems()` may recalculate incorrectly.

### Task 3.2: Fix or Refactor FilteringService Tests

**Option A**: Add explicit cache invalidation after filter operations
```go
func (f *filteringService) UpdateSearchQuery(query string) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.searchQuery = query
    f.list.SearchByTitle(query)
    f.list.invalidateVisibleCache() // Ensure cache is fresh
    return nil
}
```

**Option B**: Use `getVisibleItems()` internal method for testing instead of `GetVisibleItems()`

**Option C**: Restructure tests to use BubbleTea's test harness for proper event processing

---

## Phase 4: Tmux Package Fixes (Estimated: 1 hour)

### Task 4.1: Fix TestBannerFilter_HasMeaningfulContent

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/session/tmux/banner_filter_test.go`

**Failing Test Cases**:
- "only banners" - expects `false`, getting `true`
- "mixed content with banners" - expects `true`, getting `false` (or vice versa)

**Root Cause Analysis**:
The `HasMeaningfulContent` method excludes the last line (tmux status bar), but test expectations may not account for this.

**Test Input Analysis**:
```go
{
    name:       "only banners",
    text:       "14:23 5-Jan-24\n[session] | window:0 | 14:23",
    hasMeaning: false,
}
```

With 2 lines total and last line excluded, only line 0 is checked. If "14:23 5-Jan-24" isn't matching banner patterns, it returns `true` instead of `false`.

**Fix Options**:
1. Add timestamp-only pattern to banner detection
2. Update test expectations to match actual behavior

### Task 4.2: Fix TestPromptDetection

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/session/tmux/prompt_detection_test.go`

**Issue**: Tests call `tmux.detectPromptInContent(tt.content)` but this method may not exist or have different behavior.

**Verification**:
```bash
grep -n "detectPromptInContent" /Users/tylerstapler/IdeaProjects/claude-squad/session/tmux/tmux.go
```

Confirmed the method exists at line 547. Check if:
1. Method signature matches test expectations
2. Program-specific detection logic works correctly
3. Pattern matching is case-sensitive as expected

**Failing Sub-tests** (3):
- Likely program type mismatches or pattern changes

---

## Phase 5: UI Test Fixes (Estimated: 2-3 hours)

### Overview

UI tests in `/Users/tylerstapler/IdeaProjects/claude-squad/test/ui/` are failing, likely due to:
1. Snapshot drift (UI rendering changed)
2. Height calculation changes
3. Overlay behavior modifications

### Task 5.1: Update Snapshots

**Affected Tests**:
- `TestSessionSetupOverlay_AllSteps_Snapshots`
- `TestSessionSetupOverlay_Snapshots` (6 sub-tests)

**Action**: Regenerate golden snapshots after verifying UI looks correct:
```bash
go test ./test/ui -run TestSessionSetupOverlay -update
```

### Task 5.2: Fix Height Responsiveness Tests

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/test/ui/responsive_height_test.go`

**Failing Tests**:
- `TestSessionSetupHeightResponsiveness` (4 sub-tests)
- `TestOverlayResponsiveness` (3 sub-tests)

**Root Cause**: The `overlay.GetResponsiveHeight()`, `overlay.ShouldShowDetailedContent()`, or `overlay.ShouldShowHelpText()` functions have changed behavior.

**Fix**: Update test expectations to match new responsive breakpoints, OR update overlay functions to match documented behavior.

### Task 5.3: Fix Category Rendering Test

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/test/ui/session_ui_test.go`

**Test**: `TestSessionCategoriesRendering`

**Likely Issue**: Category grouping or rendering output changed.

### Task 5.4: Fix Navigation Test

**Test**: `TestSessionSetupOverlay_Navigation` (2 sub-tests)

**Likely Issue**: Key handling or step navigation logic changed.

---

## Phase 6: UI Component Fix (Estimated: 30 minutes)

### Task 6.1: Fix TestFilteringWithScrolling

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/ui/list_test.go` (or similar)

**Issue**: Edge case in filtering + scrolling interaction.

**Root Cause Hypothesis**: When filtering reduces visible items below scroll offset, the offset isn't properly reset.

---

## Phase 7: Session Package Fixes (Estimated: 1 hour)

### Task 7.1: Fix TestMigration_EndToEnd

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/session/migration_test.go` (estimated)

**Likely Issue**: Schema migration or data transformation test failing due to schema changes in the changeset.

### Task 7.2: Fix TestReviewQueue_GetStatistics (4 tests)

**File**: `/Users/tylerstapler/IdeaProjects/claude-squad/session/review_queue_test.go` (estimated)

**Likely Issue**: Statistics calculation or queue state assertions.

---

## Phase 8: TUI Modal Tests (Estimated: 1-2 hours)

### Overview

The `app` package has 11 failing teatest/modal tests. These are often timing-sensitive in BubbleTea applications.

### Common Issues

1. **Race conditions**: Modal state changes happening faster than test assertions
2. **Missing Update cycles**: Tests not pumping enough events through the model
3. **Timing assumptions**: Fixed delays that don't match actual behavior

### Task 8.1: Review Teatest Configuration

Check for:
- Adequate timeout settings
- Proper event loop flushing
- Stable assertions (waiting for state rather than fixed delays)

### Task 8.2: Add Retry/Wait Logic

For flaky modal tests, consider:
```go
// Instead of immediate assertion
require.Eventually(t, func() bool {
    return model.modalVisible
}, 1*time.Second, 10*time.Millisecond)
```

---

## Phase 9: Testutil Timeout (Estimated: Investigation)

### Task 9.1: Investigate 600s Timeout

The `testutil` package test timed out after 10 minutes. This suggests:
1. Infinite loop
2. Deadlock
3. Waiting on unavailable resource

**Action**: Run with verbose logging and race detector:
```bash
go test -v -race -timeout 30s ./testutil/...
```

---

## Implementation Order (Priority)

1. **Phase 1**: TypeScript fix (blocks CI type checking)
2. **Phase 2**: Status detector tests (30+ failures with single fix)
3. **Phase 3**: Filtering service (5 failures, service layer)
4. **Phase 4**: Tmux tests (5 failures, core functionality)
5. **Phase 6**: UI component (1 failure, quick)
6. **Phase 5**: UI tests (15 failures, may need snapshot regeneration)
7. **Phase 7**: Session tests (5 failures)
8. **Phase 8**: Modal tests (11 failures, may be flaky)
9. **Phase 9**: Testutil timeout (investigation)

---

## Verification Checklist

After each phase, run:

```bash
# TypeScript
cd web-app && npm run type-check

# Go tests by package
go test ./session/... -v
go test ./app/services/... -v
go test ./session/tmux/... -v
go test ./test/ui/... -v
go test ./ui/... -v
go test ./app/... -v

# Full test suite
go test ./...
```

---

## Risk Assessment

### High Risk
- **Status detector changes**: Ensure disabling tests doesn't hide real issues
- **UI snapshot updates**: Verify visual changes are intentional

### Medium Risk
- **Filtering service**: Service layer changes may affect app behavior
- **Modal tests**: Flaky tests may pass locally but fail in CI

### Low Risk
- **TypeScript fix**: Isolated type correction
- **Banner filter**: Edge case pattern matching

---

## Rollback Plan

If fixes introduce regressions:
1. Revert individual fix commits
2. Add skip markers to problematic tests with issue tracking
3. Create follow-up issues for deferred fixes

---

## Success Criteria

- [ ] `npm run type-check` passes in web-app
- [ ] `go test ./...` passes with no failures
- [ ] `go fmt ./...` reports no changes needed
- [ ] CI pipeline passes on all test stages
