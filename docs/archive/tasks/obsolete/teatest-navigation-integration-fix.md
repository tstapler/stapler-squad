# Fix Teatest Navigation Integration Issues

## Objective
Fix remaining teatest integration test failures in the app package where navigation tests are producing empty output instead of expected content.

## Context & Prerequisites
- **Build Status**: ✅ Project compiles successfully
- **Core Tests**: ✅ Basic teatest integration works (TestTeatestBasicIntegration passes)
- **Issue Scope**: Limited to specific navigation model tests in app package
- **Files Involved**: 2-3 files max (teatest helpers, navigation models, app integration)

## Problem Analysis
```bash
# Failing tests:
go test ./app -run TestAppNavigationTeatest -v
# Output: Empty string instead of navigation content

# Passing tests:
go test ./app -run TestTeatestBasicIntegration -v
# Output: Works correctly with basic model
```

**Root Cause**: Navigation test models aren't rendering content properly in teatest environment, suggesting either:
1. Model initialization issues
2. View method not returning expected content
3. Size/render context problems

## Implementation Plan

### Step 1: Diagnose Navigation Model Behavior (1 hour)
**Files**: `app/app_teatest_test.go:283-342` (navigationTestModel)
- [ ] Add debug logging to navigationTestModel.View() method
- [ ] Verify model initialization state (items, selected index)
- [ ] Test model behavior in isolation before teatest integration
- [ ] Compare working vs failing model implementations

### Step 2: Fix Model Content Generation (1 hour)
**Files**: `app/app_teatest_test.go` (navigationTestModel.View)
- [ ] Ensure View() method returns non-empty content
- [ ] Fix any string formatting or content building issues
- [ ] Validate model state consistency (selected index bounds)
- [ ] Add fallback content for edge cases

### Step 3: Integration Testing & Validation (1 hour)
**Files**: `app/app_teatest_test.go`, `testutil/teatest_helpers.go`
- [ ] Run navigation tests individually to isolate failures
- [ ] Test with different initial states and selections
- [ ] Ensure teatest timing and output consumption works correctly
- [ ] Validate all navigation scenarios (up/down/page movement)

## Acceptance Criteria
- [ ] `go test ./app -run TestAppNavigationTeatest -v` passes
- [ ] `go test ./app -run TestAppNavigationPageMovementTeatest -v` passes
- [ ] `go test ./app -run TestAppNavigationMultipleSelectionsTeatest -v` passes
- [ ] `go test ./app -run TestAppConfirmationModalTeatest -v` passes
- [ ] No regressions in existing passing teatest integration tests
- [ ] Navigation model produces expected output format

## Context Boundaries
- **Files**: 3 max (app_teatest_test.go, teatest_helpers.go, minimal app integration)
- **Lines**: ~400-500 lines total context
- **Time**: 3 hours maximum
- **Scope**: Navigation model teatest integration only

## Risk Assessment
- **Low Risk**: Isolated to test code, no production functionality impact
- **High Value**: Enables reliable teatest-based TUI testing for future development
- **Dependencies**: None - can be completed independently

## Implementation Notes
- Focus on navigation model content generation first
- Use working basic integration test as reference implementation
- Maintain existing teatest helper patterns
- Add debug output temporarily for diagnosis, remove when fixed

## Validation Commands
```bash
# Individual test validation
go test ./app -run TestAppNavigationTeatest -v
go test ./app -run TestAppNavigationPageMovementTeatest -v
go test ./app -run TestAppNavigationMultipleSelectionsTeatest -v
go test ./app -run TestAppConfirmationModalTeatest -v

# Full app package validation
go test ./app -v

# Regression check
go test ./testutil -v
```

---
**AIC Compliance**: ✅ Atomic, ✅ Independent, ✅ Negotiable, ✅ Valuable, ✅ Estimable, ✅ Small, ✅ Testable
**Context Boundary**: ✅ 3 files, ✅ ~500 lines, ✅ 3 hours, ✅ Single concern