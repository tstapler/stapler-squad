# Validation Plan — Pane Picker Fix

**Project**: pane-picker-fix
**Requirements**: `../requirements.md`
**Implementation plan**: `plan.md`
**Date**: 2026-05-08

---

## Overview

This plan maps each requirement (R1–R6) to concrete test cases. All tests are written against
the implementation plan's described code changes (Steps 1–5). Tests are grouped by file/describe
block. Every test case includes a concrete state shape or dispatch call assertion and explicit
pass criteria.

---

## Test Cases

### File: `web-app/src/lib/pane/__tests__/paneReducer.test.ts`

#### describe: `ASSIGN_SESSION`

---

**T-001**
- **Name**: `paneReducer_should_moveSessionAndClearSource_When_sessionAlreadyPresentInAnotherPane`
- **Type**: Unit
- **Requirement(s)**: R2
- **Replaces**: Existing test `paneReducer_should_returnUnchangedState_When_sessionAlreadyPresentInAnotherPane` (line 185)
- **Setup**: Two-pane vertical split. `pane-1` holds `session-A`. `pane-2` is empty. `focusedPaneId = "pane-2"`.
- **Action**: `{ type: "ASSIGN_SESSION", paneId: "pane-2", sessionId: "session-A" }`
- **Assert**:
  - `leaves.find(l => l.id === "pane-2").sessionId === "session-A"`
  - `leaves.find(l => l.id === "pane-1").sessionId === null`
  - `next !== state` (state is not reference-equal to previous state)
- **Pass criteria**: The reducer returns a new state where `pane-2` has `session-A` and `pane-1` is cleared. The old no-op return is gone.

---

**T-002**
- **Name**: `paneReducer_should_clearSourceAndAssignTarget_When_sessionMovedAcrossPanes`
- **Type**: Unit
- **Requirement(s)**: R2
- **Setup**: Three-pane tree. `pane-1` holds `session-A`, `pane-2` holds `session-B`, `pane-3` is empty. Nested: `split(split(pane1, pane2), pane3)`. `focusedPaneId = "pane-3"`.
- **Action**: `{ type: "ASSIGN_SESSION", paneId: "pane-3", sessionId: "session-A" }`
- **Assert**:
  - `leaves.find(l => l.id === "pane-3").sessionId === "session-A"`
  - `leaves.find(l => l.id === "pane-1").sessionId === null`
  - `leaves.find(l => l.id === "pane-2").sessionId === "session-B"` (unaffected third pane)
- **Pass criteria**: Only the source pane (`pane-1`) is cleared; the uninvolved pane (`pane-2`) retains its session unchanged.

---

**T-003**
- **Name**: `paneReducer_should_setSessionId_When_assignSessionDispatched` *(existing — must continue to pass)*
- **Type**: Unit (regression)
- **Requirement(s)**: R4
- **Setup**: Single leaf `pane-1` with `sessionId: null`.
- **Action**: `{ type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A" }`
- **Assert**: `(next.root as LeafPane).sessionId === "session-A"`
- **Pass criteria**: Baseline assign (no move) is unaffected by the guard removal.

---

**T-004**
- **Name**: `paneReducer_should_resetActiveTabToTerminal_When_sessionAssigned` *(existing — must continue to pass)*
- **Type**: Unit (regression)
- **Requirement(s)**: R4
- **Setup**: Single leaf `pane-1` with `activeTab: "diff"`.
- **Action**: `{ type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A" }`
- **Assert**: `(next.root as LeafPane).activeTab === "terminal"`
- **Pass criteria**: Tab reset behavior is preserved after refactor.

---

**T-005**
- **Name**: `paneReducer_should_neverProduceStateWithMissingFocusedPaneId_When_ASSIGN_SESSION` *(existing invariant — must continue to pass)*
- **Type**: Unit (regression)
- **Requirement(s)**: R4, R6
- **Setup**: Single leaf `pane-1`. `focusedPaneId = "pane-1"`.
- **Action**: `{ type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-X" }`
- **Assert**: `getAllLeaves(next.root).map(l => l.id)` contains `next.focusedPaneId`
- **Pass criteria**: `focusedPaneId` always references an existing leaf after the action. Move-and-clear must never orphan `focusedPaneId`.

---

#### describe: `SPLIT_AND_ASSIGN_SESSION` *(new describe block)*

---

**T-006**
- **Name**: `paneReducer_should_splitAndAssignNewLeaf_When_sessionNotInAnyPane`
- **Type**: Unit
- **Requirement(s)**: R3, R4
- **Setup**: Single leaf `pane-1` with `sessionId: null`.
- **Action**: `{ type: "SPLIT_AND_ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A", tab: "terminal", direction: "vertical" }`
- **Assert**:
  - `next.root.type === "split"`
  - `(next.root as SplitPane).direction === "vertical"`
  - `(next.root as SplitPane).first` has `id === "pane-1"` (original pane unchanged)
  - `(next.root as SplitPane).second` (new leaf) has `sessionId === "session-A"` and `activeTab === "terminal"`
  - `next.focusedPaneId === newLeaf.id`
- **Pass criteria**: Fresh split path is fully preserved after the duplicate-guard removal.

---

**T-007**
- **Name**: `paneReducer_should_clearSourceAndSplitAssign_When_sessionAlreadyInAnotherPane`
- **Type**: Unit
- **Requirement(s)**: R3
- **Setup**: Two-pane vertical split. `pane-1` holds `session-A`. `pane-2` is empty. `focusedPaneId = "pane-2"`.
- **Action**: `{ type: "SPLIT_AND_ASSIGN_SESSION", paneId: "pane-2", sessionId: "session-A", tab: "terminal", direction: "vertical" }`
- **Assert**:
  - `getAllLeaves(next.root)` has exactly 3 leaves (pane-1, pane-2, new leaf)
  - `leaves.find(l => l.id === "pane-1").sessionId === null` (source cleared)
  - `leaves.find(l => l.sessionId === "session-A")` is not `pane-1` and not `pane-2`
  - `next.focusedPaneId === newLeaf.id`
- **Pass criteria**: Alt+click on a session already open in another pane closes it from its source and opens it in a new split.

---

**T-008**
- **Name**: `paneReducer_should_assignInPlace_When_maxDepthReached`
- **Type**: Unit
- **Requirement(s)**: R3, R4
- **Setup**: Leaf `deep-leaf` nested at depth 8 (constructed with 8 wrapping splits).
- **Action**: `{ type: "SPLIT_AND_ASSIGN_SESSION", paneId: "deep-leaf", sessionId: "session-X", tab: "terminal", direction: "vertical" }`
- **Assert**:
  - `getAllLeaves(next.root).length === getAllLeaves(state.root).length` (no new leaves added)
  - `getAllLeaves(next.root).find(l => l.id === "deep-leaf").sessionId === "session-X"`
- **Pass criteria**: Max-depth fallback (assign in place) is preserved; no new leaf is created.

---

**T-009**
- **Name**: `paneReducer_should_notClearSource_When_sourcePaneIsSameAsSplitTarget`
- **Type**: Unit
- **Requirement(s)**: R3
- **Setup**: Single leaf `pane-1` with `sessionId: "session-A"`. Splitting `pane-1` and assigning `session-A` to the new pane (session already in the pane being split).
- **Action**: `{ type: "SPLIT_AND_ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-A", tab: "terminal", direction: "vertical" }`
- **Assert**:
  - `next.root.type === "split"`
  - `(next.root as SplitPane).first` (which is the original `pane-1`) retains `sessionId === "session-A"` (not cleared)
  - `(next.root as SplitPane).second` (new leaf) also has `sessionId === "session-A"`
- **Pass criteria**: The `sourcePaneId !== paneId` guard correctly skips the clear when the source and target are the same pane. Both child panes end up showing the same session (a transient state resolved by the UX; not blocked by the reducer).

---

#### describe: `state invariants after any action`

---

**T-010**
- **Name**: `paneReducer_should_neverProduceStateWithMissingFocusedPaneId_When_SPLIT_AND_ASSIGN_SESSION` *(extend existing parametrized invariant test)*
- **Type**: Unit (regression)
- **Requirement(s)**: R3, R4, R6
- **Setup**: Add `{ type: "SPLIT_AND_ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-X", tab: "terminal", direction: "vertical" }` to the `ALL_ACTIONS` array in the existing invariant `it.each` block.
- **Assert**: `getAllLeaves(next.root).map(l => l.id)` contains `next.focusedPaneId`
- **Pass criteria**: `focusedPaneId` is always a valid leaf after `SPLIT_AND_ASSIGN_SESSION` — including the source-clear path and the max-depth fallback.

---

### File: `web-app/src/components/pane/__tests__/PaneTilingContainer.test.tsx` *(new file)*

#### describe: `triggerPicker`

---

**T-011**
- **Name**: `triggerPicker_should_showPickerOverlay_When_twoDetailPanesExist`
- **Type**: Integration
- **Requirement(s)**: R1
- **Setup**: Mock `usePaneReducer` to return a state with two `session-detail` leaves (`pane-1`, `pane-2`). `focusedPaneId = "pane-1"` (detail pane focused — the pre-fix bypass scenario). Mock `dispatch`. Render `<PaneTilingContainer>` with `externalSessionAssign={{ sessionId: "session-X", version: 2 }}`.
- **Assert**: `dispatch` is NOT called with `{ type: "ASSIGN_SESSION" }` at any point during the render/effect.
- **Pass criteria**: The component does not auto-assign; `pickerPendingSession` is set instead (picker overlay would render). The removed bypass block is confirmed absent.

---

**T-012**
- **Name**: `triggerPicker_should_assignDirectly_When_oneDetailPane`
- **Type**: Integration
- **Requirement(s)**: R4
- **Setup**: Mock `usePaneReducer` to return a state with one `session-list` leaf (`pane-list`) and one `session-detail` leaf (`pane-detail`). `focusedPaneId = "pane-list"`. Mock `dispatch`. Render `<PaneTilingContainer>` with `externalSessionAssign={{ sessionId: "session-X", version: 1 }}`.
- **Assert**:
  - `dispatch` is called with `{ type: "ASSIGN_SESSION", paneId: "pane-detail", sessionId: "session-X" }`
  - `dispatch` is called with `{ type: "ASSIGN_TAB", paneId: "pane-detail" }`
  - `dispatch` is called with `{ type: "FOCUS_PANE", paneId: "pane-detail" }`
- **Pass criteria**: All three dispatches fire in the single-eligible-pane branch; picker is never shown.

---

**T-013**
- **Name**: `triggerPicker_should_autoSplit_When_noDetailPanes`
- **Type**: Integration
- **Requirement(s)**: R4
- **Setup**: Mock `usePaneReducer` to return a state with only a `session-list` leaf (`pane-list`). `focusedPaneId = "pane-list"`. Mock `dispatch`. Render with `externalSessionAssign={{ sessionId: "session-X", version: 1 }}`.
- **Assert**: `dispatch` is called with `{ type: "ASSIGN_SESSION", paneId: "pane-list", sessionId: "session-X" }` (the reducer handles creating a new detail pane via auto-split).
- **Pass criteria**: Zero-eligible-pane path dispatches to `focusedPaneId` (the session-list pane); the reducer auto-splits to create a detail pane.

---

**T-014**
- **Name**: `triggerPicker_should_showPicker_When_detailPaneFocusedAndTwoPanesExist`
- **Type**: Integration (regression guard)
- **Requirement(s)**: R1
- **Setup**: Mock `usePaneReducer` to return a state with two `session-detail` leaves (`pane-1` holds `session-A`, `pane-2` is empty). `focusedPaneId = "pane-1"` — the exact Bug 1 scenario. Mock `dispatch`. Render with `externalSessionAssign={{ sessionId: "session-B", version: 1 }}`.
- **Assert**: `dispatch` is NOT called with `{ type: "ASSIGN_SESSION" }`.
- **Pass criteria**: Even with a detail pane focused, the 2+ pane branch fires and shows the picker. This test fails against the unfixed code and passes against the fixed code — making it a definitive regression guard for Bug 1.

---

**T-015**
- **Name**: `triggerPicker_should_notDispatchCancelPicker_When_twoDetailPanes`
- **Type**: Integration
- **Requirement(s)**: R6
- **Setup**: Same as T-011 (two detail panes, detail pane focused). Mock `dispatch`.
- **Assert**: `dispatch` is NOT called with `{ type: "CANCEL_PICKER" }` or any picker-cancel action during the trigger path.
- **Pass criteria**: `cancelPicker` is not called before the user makes a selection; the picker overlay stays open after `triggerPicker` fires.

---

**T-016**
- **Name**: `triggerPicker_should_allowKeyboardAssignToMoveSession` *(keyboard path — R5 + R2 interaction)*
- **Type**: Integration
- **Requirement(s)**: R5, R2
- **Setup**: Mock `usePaneReducer` to return a state with two `session-detail` leaves (`pane-1` holds `session-A`, `pane-2` is empty). Mock `dispatch` to capture calls. Simulate the keyboard handler firing `{ type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-B" }` directly (the key handler routes to `ASSIGN_SESSION` — we verify the mock reducer would be called correctly, not re-testing keyboard event routing which is covered by existing shortcut tests).
- **Assert**: After `paneReducer` processes `{ type: "ASSIGN_SESSION", paneId: "pane-1", sessionId: "session-B" }` on a state where `session-B` is in `pane-2`:
  - `leaves.find(l => l.id === "pane-1").sessionId === "session-B"`
  - `leaves.find(l => l.id === "pane-2").sessionId === null`
- **Note**: This test runs at the reducer layer (same setup as T-001 but with keyboard-initiated action) to confirm that the keyboard path inherits move-and-clear automatically with no component code changes.
- **Pass criteria**: Keyboard-initiated `ASSIGN_SESSION` gets move-and-clear semantics from the reducer. No separate keyboard-handler change was needed.

---

## Coverage Matrix

| Requirement | Description | Test IDs |
|---|---|---|
| R1 | Picker always shown with 2+ eligible panes | T-011, T-014 |
| R2 | ASSIGN_SESSION allows moving sessions between panes | T-001, T-002, T-016 |
| R3 | SPLIT_AND_ASSIGN_SESSION allows moving sessions to a new split | T-007, T-008, T-009, T-010 |
| R4 | No regression on single-pane workflows | T-003, T-004, T-005, T-006, T-008, T-012, T-013 |
| R5 | Keyboard picker continues to work correctly | T-016 |
| R6 | Picker closes reliably after selection | T-005, T-010, T-015 |

All 6 requirements are covered (6/6).

---

## Test Count Summary

| Type | Count | Test IDs |
|---|---|---|
| Unit (new) | 6 | T-001, T-002, T-006, T-007, T-008, T-009 |
| Unit (regression — existing tests that must continue to pass) | 4 | T-003, T-004, T-005, T-010 |
| Integration (new component tests) | 6 | T-011, T-012, T-013, T-014, T-015, T-016 |
| **Total** | **16** | |

Unit tests (new + regression): 10
Integration tests: 6
Regression tests (subset of unit): 4

---

## File Mapping

| File | Describe Block | Test IDs |
|---|---|---|
| `web-app/src/lib/pane/__tests__/paneReducer.test.ts` | `ASSIGN_SESSION` | T-001, T-002, T-003, T-004, T-005 |
| `web-app/src/lib/pane/__tests__/paneReducer.test.ts` | `SPLIT_AND_ASSIGN_SESSION` | T-006, T-007, T-008, T-009 |
| `web-app/src/lib/pane/__tests__/paneReducer.test.ts` | `state invariants after any action` | T-010 |
| `web-app/src/components/pane/__tests__/PaneTilingContainer.test.tsx` | `triggerPicker` | T-011, T-012, T-013, T-014, T-015, T-016 |

---

## Verification Commands

Run in this order to validate all tests pass:

```bash
# Reducer unit tests (pure TS, no DOM)
cd web-app && npx jest --no-coverage --testPathPatterns="paneReducer.test"

# PaneTilingContainer integration tests
cd web-app && npx jest --no-coverage --testPathPatterns="PaneTilingContainer.test"

# Full pane suite (catches any regressions in adjacent pane tests)
cd web-app && npx jest --no-coverage --testPathPatterns="pane"

# Full frontend suite
cd web-app && npx jest --no-coverage
```

---

## Pre-Condition: Test for Bug (Regression Guards)

T-014 and T-001 are the canonical "test fails against unfixed code, passes against fixed code" guards:

- **T-014**: Pass a state with 2 detail panes + focused detail pane to `triggerPicker`. Pre-fix: `dispatch(ASSIGN_SESSION)` fires. Post-fix: it does not.
- **T-001**: Pass `ASSIGN_SESSION` targeting a pane where the session is already in another pane. Pre-fix: `next === state` (no-op). Post-fix: source pane is cleared, target pane has the session.

These two tests should be written and run against the unfixed code first to confirm they fail, then re-run against the fixed code to confirm they pass.
