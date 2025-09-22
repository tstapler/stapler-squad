# Claude Squad: Task Tracking

## Current Todo List - CRITICAL PHASE: Test Stabilization

### 🚨 BLOCKING ISSUES (Immediate Priority) 🚨

1. **[URGENT]** Fix compilation error in state manager
   - **File**: `app/state/manager.go:216`
   - **Issue**: Non-constant format string in fmt.Errorf
   - **Impact**: Project won't compile - blocks all development
   - **Estimate**: 1 hour
   - **Task File**: `docs/tasks/build-error-state-manager.md`

2. **[CRITICAL]** Fix nil pointer dereferences in app teatest integration
   - **File**: `app/app.go:1879` (stateManager nil panic)
   - **Issue**: Multiple teatest failures with nil pointer panics
   - **Impact**: Core app functionality broken in tests
   - **Estimate**: 3 hours
   - **Task File**: `docs/tasks/critical-nil-pointer-fixes.md`

### High Priority Test Fixes 🔄

3. **[PENDING]** Fix UI package tmux timeout failures
   - **Files**: `ui/preview_test.go` (TestPreviewScrolling, TestPreviewContentWithoutScrolling)
   - **Issue**: Tmux session creation timeouts in tests
   - **Impact**: UI package test failures
   - **Estimate**: 2.5 hours
   - **Task File**: `docs/tasks/ui-test-tmux-timeouts.md`

4. **[PENDING]** Fix session rendering test failures
   - **File**: `test/ui/session_ui_test.go` (TestSessionCategoriesRendering)
   - **Issue**: Expected sessions not appearing in rendered output
   - **Impact**: Session display functionality questionable
   - **Estimate**: 2 hours
   - **Task File**: `docs/tasks/ui-session-rendering-fix.md`

### Post-Stabilization Tasks 📋 (After Critical Issues Resolved)

5. **[COMPLETED ✅]** Contextual Git repository discovery implementation
   - **Status**: Feature implemented and working
   - **Note**: Documentation shows as "IN PROGRESS" but current-work-plan.md shows completed
   - **Action Needed**: Update documentation status to reflect reality

6. **[COMPLETED ✅]** Unit tests for contextual Git repository discovery
   - **Status**: Tests implemented and passing per current-work-plan.md
   - **Note**: Comprehensive test coverage achieved

7. **[COMPLETED ✅]** TTY testing documentation
   - **Status**: CLAUDE.md updated with testing strategies
   - **Note**: Manual testing protocols documented

8. **[PENDING]** Enhanced contextual discovery features (FUTURE)
   - **Enhancement**: Improve user feedback for invalid paths
   - **Features**: Better error messages, network path support
   - **Priority**: Low - after test stabilization

9. **[PENDING]** Advanced UX improvements (FUTURE)
   - **UI**: Enhanced keyboard shortcuts, auto-completion
   - **Feature**: Path auto-completion for partially typed paths
   - **Priority**: Low - after core stability

### Backlog Tasks 📚

8. **[PENDING]** Evaluate session health check and recovery system integration
   - **Investigation**: Assess existing health check system
   - **Decision**: Integration value vs complexity

9. **[PENDING]** Evaluate tag filtering system vs current category-based organization
   - **Analysis**: Compare tag-based vs category-based session organization
   - **Outcome**: Recommendation for future direction

10. **[PENDING]** Compare current help system vs unused help generator system
    - **Files**: Current help vs `help/generator.go`
    - **Decision**: Which system to keep/improve

11. **[PENDING]** Remove truly unnecessary dead code (unused constructors, test mocks)
    - **Scope**: Only truly unused code, not code that might be useful
    - **Goal**: Reduce maintenance burden

### Completed Tasks ✅

12. **[COMPLETED]** Investigate remaining input lag from tmux pane loading
13. **[COMPLETED]** Integrate ResponsiveNavigationManager to replace current flow
14. **[COMPLETED]** Remove dead code: debouncedInstanceChanged, fastInstanceChanged, instanceExpensiveUpdate
15. **[COMPLETED]** Convert UpdateDiff to async operation like UpdatePreview
16. **[COMPLETED]** Send async results via tea.Msg for UI updates
17. **[COMPLETED]** Test async improvements with 50+ sessions
18. **[COMPLETED]** Fix BubbleTea UI resizing in IntelliJ terminal during session navigation
19. **[COMPLETED]** CRITICAL: Fix blocking time.Sleep patterns in BubbleTea event loop
20. **[COMPLETED]** Create ADR documenting async event loop patterns
21. **[COMPLETED]** Evaluate advanced session creation system (repo/branch/worktree selectors + advanced setup)
22. **[COMPLETED]** Test Git integration implementation for SessionSetupOverlay

## Task Dependencies

```
Write unit tests (1)
├── Test contextual scanning (3)
└── Update CLAUDE.md (2)

Enhanced path validation (4)
├── Handle edge cases (6)
└── Improve UX (5)

Integration testing (7)
├── All previous tasks completed
└── Comprehensive validation

Evaluation tasks (8,9,10,11)
├── No dependencies
└── Can be done in parallel
```

## Priority Matrix - UPDATED FOR CRITICAL STATE

### 🚨 URGENT (Must Fix Immediately)
1. Build compilation error (1) - **1 hour**
2. Nil pointer dereferences (2) - **3 hours**

### High Priority (Current Sprint)
3. UI tmux timeout fixes (3) - **2.5 hours**
4. Session rendering fixes (4) - **2 hours**

### Medium Priority (After Stabilization)
- Documentation status updates
- Enhanced contextual features
- UX improvements

### Low Priority (Future Sprints)
- System evaluations (health check, tag filtering)
- Dead code cleanup
- Advanced testing features

## Completion Criteria

### For Each Task
- [ ] Implementation complete
- [ ] Tests written and passing
- [ ] Documentation updated if needed
- [ ] No regressions introduced
- [ ] User experience validated

### For Current Phase (CRITICAL STABILIZATION)
- [ ] Project compiles successfully (build error fixed)
- [ ] No nil pointer panics in app tests
- [ ] UI package tests pass consistently
- [ ] Session rendering works correctly
- [ ] Test suite runs reliably end-to-end

### For Completed Features (Already Done)
- [x] Contextual Git discovery fully implemented
- [x] TTY testing approach documented
- [x] All path input scenarios working
- [x] Performance acceptable
- [x] Error handling robust

---

*Last Updated: 2025-09-22*
*CRITICAL STATE: 2 blocking issues, 2 high-priority test fixes*
*Recently Completed: Contextual discovery feature (major milestone)*
*Current Focus: Test suite stabilization and deployment readiness*