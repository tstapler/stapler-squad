# Critical Nil Pointer Fixes

## Objective
Fix critical nil pointer dereferences causing test failures and application panics, focusing on `stateManager` initialization and teatest integration issues.

## Prerequisites
- Understanding of BubbleTea model initialization patterns
- Knowledge of Go nil safety best practices
- Familiarity with teatest API limitations

## Context Boundary
**Files (3-4 max)**:
- `app/app.go` (primary - nil pointer at line 1879)
- `app/state/manager.go` (secondary - state manager initialization)
- `app/app_teatest_test.go` (supporting - failing teatest cases)
- `app/app_test.go` (supporting - test model setup)

**Lines**: ~500-600 total context
**Time Estimate**: 3 hours
**Concepts**: State manager lifecycle, BubbleTea model initialization, teatest setup patterns

## Atomic Steps

### Step 1: Identify Nil Pointer Root Cause (30 min)
- Examine `app.go:1879` where `m.stateManager.GetViewDirective()` panics
- Trace stateManager initialization in model constructor
- Identify conditions where stateManager remains nil

### Step 2: Fix State Manager Initialization (90 min)
- Ensure stateManager is properly initialized in all constructors
- Add nil checks with safe defaults before accessing stateManager
- Validate initialization order in model setup

### Step 3: Fix Teatest Model Setup (60 min)
- Investigate why teatest models don't initialize properly
- Ensure all required fields are set in test model creation
- Add proper initialization guards for test scenarios

### Step 4: Validate Fixes (30 min)
- Run failing teatest cases to verify panic resolution
- Ensure no regression in normal application startup
- Confirm state manager behaves correctly in both test and production

## Validation Criteria
- [ ] No nil pointer panics in `app.go:1879`
- [ ] All teatest integration tests pass
- [ ] State manager properly initialized in all scenarios
- [ ] No regression in application startup

## Success Metrics
- Zero nil pointer dereferences in app package tests
- Teatest integration tests execute without panics
- State manager reliably available throughout model lifecycle

## Dependencies
None - this is a blocking issue requiring immediate resolution

## Links
- Related: `docs/tasks/test-stabilization-and-teatest-integration.md` (Story 1, Task 1.1)
- Test failures: Multiple teatest integration tests failing with nil pointer panics