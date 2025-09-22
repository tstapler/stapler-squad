# UI Test Tmux Timeout Resolution

## Objective
Fix UI package test failures caused by tmux session creation timeouts in `TestPreviewScrolling` and `TestPreviewContentWithoutScrolling`.

## Prerequisites
- Understanding of tmux session lifecycle in tests
- Knowledge of test timeout patterns and cleanup
- Familiarity with preview component tmux integration

## Context Boundary
**Files (3-4 max)**:
- `ui/preview_test.go` (primary - failing timeout tests)
- `ui/preview.go` (secondary - preview tmux integration)
- `session/tmux/tmux.go` (supporting - tmux session management)
- `testutil/tmux_helpers.go` (supporting - test utilities if exists)

**Lines**: ~400-500 total context
**Time Estimate**: 2.5 hours
**Concepts**: Tmux test session management, timeout handling, test resource cleanup

## Atomic Steps

### Step 1: Analyze Timeout Root Cause (45 min)
- Examine failing tests: `TestPreviewScrolling`, `TestPreviewContentWithoutScrolling`
- Identify why tmux session creation times out in test environment
- Check for proper test cleanup and session name conflicts

### Step 2: Fix Test Session Management (90 min)
- Implement proper test session isolation and cleanup
- Add timeout handling for tmux operations in tests
- Ensure unique session names prevent conflicts

### Step 3: Improve Test Reliability (30 min)
- Add retry logic for flaky tmux operations
- Implement proper teardown to prevent resource leaks
- Validate session cleanup between test runs

### Step 4: Validate Test Stability (15 min)
- Run UI tests multiple times to verify consistency
- Confirm no hanging tmux sessions after test completion
- Ensure preview functionality works in test scenarios

## Validation Criteria
- [ ] `TestPreviewScrolling` passes consistently
- [ ] `TestPreviewContentWithoutScrolling` passes consistently
- [ ] No hanging tmux sessions after test completion
- [ ] Test execution time under 5 seconds per test

## Success Metrics
- UI package tests pass with 100% reliability
- No tmux session timeouts in test environment
- Clean resource management throughout test lifecycle

## Dependencies
None - independent UI package test failures

## Links
- Related: `docs/tasks/test-stabilization-and-teatest-integration.md` (Story 1, Task 1.3)
- Test failures: UI package tmux integration timeouts