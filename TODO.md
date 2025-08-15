# Testing Plan for Status Indicators Feature

This document outlines a comprehensive testing strategy for the new status indicators feature, focusing on the "NeedsApproval" state addition and UI changes.

## 1. Unit Tests

### Status Enum Tests
- [ ] Test that the new `NeedsApproval` status value is correctly defined in the Status enum
- [ ] Test string representation and marshaling of the new status value (if applicable)

### UI Rendering Tests
- [ ] Test `InstanceRenderer.Render` function with instance status set to `NeedsApproval`
- [ ] Verify that the correct icon and style are applied when status is `NeedsApproval`
- [ ] Test that the `needsApprovalStyle` renders with the correct color

### Status Determination Logic Tests
- [ ] Test status determination in app.go when a prompt is detected with AutoYes disabled
- [ ] Test status determination in app.go when a prompt is detected with AutoYes enabled
- [ ] Test that `TapEnter` is called only when AutoYes is enabled

## 2. Integration Tests

### Status Transition Tests
- [ ] Test that status correctly transitions from `Running` to `NeedsApproval` when a prompt is detected and AutoYes is disabled
- [ ] Test that status correctly transitions from `NeedsApproval` to `Running` when the user attaches and responds to a prompt
- [ ] Test that status transitions directly from `Running` to `Ready` when a process completes without requiring approval

### UI Rendering Integration Tests
- [ ] Test that the UI correctly renders all status types in the instance list
- [ ] Verify that status indicators are properly updated when status changes

## 3. End-to-End Tests

### User Interaction Tests
- [ ] Test end-to-end scenario: Claude asks for approval, status changes to `NeedsApproval`, user attaches and responds
- [ ] Test end-to-end scenario with multiple instances with different statuses
- [ ] Test that clicking on an instance with `NeedsApproval` status properly attaches to the session

### Visual Verification
- [ ] Manually verify that the `NeedsApproval` icon is visually distinct and attention-grabbing
- [ ] Test with different terminal color schemes to ensure visibility

## 4. Property-Based Tests

### Status Transition Properties
- [ ] Property: An instance with a prompt and AutoYes disabled should always have `NeedsApproval` status
- [ ] Property: An instance with a prompt and AutoYes enabled should never have `NeedsApproval` status
- [ ] Property: Status transitions should be deterministic based on instance state

## 5. Mutation Testing

- [ ] Apply mutation testing to status determination logic to verify test robustness
- [ ] Possible mutations:
  - Change condition from `if instance.AutoYes` to `if !instance.AutoYes`
  - Remove setting status to `NeedsApproval`
  - Change status assignment to another status type

## Test Implementation Approach

### For Unit Tests
1. Add tests to `ui/list_test.go` for UI rendering with the new status
2. Add tests to `session/instance_test.go` for the status enum
3. Add tests to `app/app_test.go` for status determination logic

### For Integration Tests
1. Create new integration test cases in `session/integration_test.go` (if exists)
2. Focus on status transitions based on prompt detection and AutoYes setting

### For End-to-End Tests
1. Manual testing scenarios documented in a test plan
2. Consider adding automated UI tests if the framework supports it

## Test Prioritization

1. **High Priority**: Unit tests for status determination logic (ensures core functionality)
2. **High Priority**: Integration tests for status transitions
3. **Medium Priority**: UI rendering unit tests
4. **Medium Priority**: End-to-end tests
5. **Low Priority**: Property-based and mutation tests

## Coverage Goals

- Aim for 90%+ branch coverage of the status determination logic
- Ensure all status transition paths are covered
- Cover edge cases like rapid status transitions
