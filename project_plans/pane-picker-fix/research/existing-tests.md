# Existing Tests — Pane System Coverage

## Test Files Found

All pane-related test files in `web-app/src/`:

| File | What it tests |
|---|---|
| `web-app/src/lib/pane/__tests__/paneReducer.test.ts` | Pure reducer logic (all action types) |
| `web-app/src/lib/pane/__tests__/usePaneLayout.test.ts` | localStorage persistence + layout validation/repair |
| `web-app/src/components/pane/__tests__/PaneSplitRenderer.focus.test.tsx` | Focus outline gating (single vs multi-pane) |
| `web-app/src/components/pane/__tests__/PaneSplitRenderer.mobile.test.tsx` | Mobile layout collapse, tab strip, reset button |
| `web-app/src/components/sessions/__tests__/OmnibarCreationPanel.attach.test.tsx` | Attach mode in OmnibarCreationPanel (adjacent concern) |

**No test file exists for `PaneTilingContainer.tsx`** — the file specified in the
requirements (`web-app/src/components/pane/__tests__/PaneTilingContainer.test.tsx`)
must be created.

## Testing Framework and Helpers

- **Framework**: Jest + React Testing Library (RTL)
- **Import pattern**: `render`, `screen` from `@testing-library/react`
- **TypeScript**: all test files use `.test.ts` or `.test.tsx`
- **Mock pattern**: `jest.mock(...)` at module level for all component dependencies
  (viewport hook, child components, context providers)

## paneReducer.test.ts — Coverage and Patterns

### Fixtures

```ts
function leaf(id, sessionId = null): LeafPane
function split(id, direction, first, second, ratio = 0.5): SplitPane
function stateOf(root, focusedPaneId): PaneState  // always zoomedPaneId: null
```

### Action coverage

| Action | Tests | Notes |
|---|---|---|
| `SPLIT_PANE` | 4 | split direction, ratio, max depth guard, new leaf is empty |
| `CLOSE_PANE` | 4 | collapse, focus sibling, reset root, not found |
| `RESIZE_PANE` | 4 | update, clamp min, clamp max, not found |
| `FOCUS_PANE` | 2 | update, not found |
| `ASSIGN_SESSION` | 4 | assign, reset tab to terminal, **duplicate guard returns unchanged state**, null guard |
| `RESET_LAYOUT` | 2 | returns initial split, focus lands on detail pane |
| `ZOOM_PANE` | 3 | set, clear with null, toggle off |
| `getAdjacentLeaf` | 4 | right, only-one-pane, no-adjacent, bottom |
| State invariants | 7 + 1 | focusedPaneId always in tree, ratio always within bounds |

**Critical gap**: The test at line 185–199 asserts the old (buggy) behavior:
```ts
it("paneReducer_should_returnUnchangedState_When_sessionAlreadyPresentInAnotherPane", () => {
  // ... expects state unchanged when assigning a session already in another pane
  expect(next).toEqual(state);
});
```
This test will need to be **updated** to assert the new move-and-clear behavior.

**Missing SPLIT_AND_ASSIGN_SESSION tests**: There are no tests for the
`SPLIT_AND_ASSIGN_SESSION` action in the current suite. Tests need to be added for:
- Normal split-and-assign (session not in any pane)
- Move-and-clear (session already in another pane)
- Max depth fallback (assign in place)

## usePaneLayout.test.ts — Coverage

Tests `loadPaneLayout`, `savePaneLayout`, `clearPaneLayout`, and `validateAndRepair`.
Uses `localStorage.clear()` in `beforeEach`. Tests the stale-session nullification
logic in `validateAndRepair`. No gaps relevant to the picker fix.

## PaneSplitRenderer.focus.test.tsx — Coverage

Tests that `leafContainer` is called with `{ focused: false }` for single pane
and `{ focused: true }` for the focused pane in a split. Uses a `jest.fn()` spy
on the CSS module export. Mocks `PaneContext` with `triggerPicker: jest.fn()` and
`cancelPicker: jest.fn()` — no assertions on when they are called.

## PaneSplitRenderer.mobile.test.tsx — Coverage

Tests mobile layout collapse (vertical splits show only focused pane), horizontal
splits show both panes, tab strip presence, reset layout button presence. Uses
`mockIsMobile` flag mutated in `beforeEach`. Mocks `PaneContext` with stub functions.

## What Needs to Be Added (per requirements)

### 1. `paneReducer.test.ts` additions (R2 + R3)

For `ASSIGN_SESSION`:
- `paneReducer_should_moveToPaneAndClearSource_When_sessionAlreadyInAnotherPane`
  - Setup: two leaves, leaf1 has session-A, leaf2 is empty
  - Action: ASSIGN_SESSION to leaf2 with session-A
  - Assert: leaf2.sessionId === "session-A", leaf1.sessionId === null
- Update existing `_should_returnUnchangedState_When_sessionAlreadyPresentInAnotherPane`
  to assert move behavior instead of no-op

For `SPLIT_AND_ASSIGN_SESSION`:
- `paneReducer_should_splitAndAssign_When_sessionNotInAnyPane` (normal case)
- `paneReducer_should_clearSourceAndSplitAssign_When_sessionAlreadyInAnotherPane`
  - Assert source pane cleared, new split pane has the session
- `paneReducer_should_assignInPlace_When_maxDepthReached`

### 2. `PaneTilingContainer.test.tsx` (new file, R1 + R4)

The PaneTilingContainer contains the `triggerPicker` function with Bug 1. Testing
it requires mounting the component with a mocked `usePaneReducer` and asserting
that `pickerPendingSession` is set vs. `ASSIGN_SESSION` is dispatched.

Key test scenarios:
- `triggerPicker_should_showPickerOverlay_When_twoDetailPanesExist` (Bug 1 fix)
- `triggerPicker_should_assignDirectly_When_oneDetailPane`
- `triggerPicker_should_autoSplit_When_noDetailPanes`
- `triggerPicker_should_showPickerOverlay_When_detailPaneFocusedButTwoPanesExist` (the bypass scenario)

### Mock patterns required for PaneTilingContainer tests

The component uses:
- `usePaneReducer` (from `@/lib/pane/usePaneReducer`) — needs to be mockable with
  a preset `[state, dispatch]` tuple
- `usePaneShortcuts` (from `@/lib/pane/usePaneShortcuts`) — stub to no-op
- `PaneSplitRenderer` — mock to a simple `<div>` to avoid deep rendering
- `PaneContext.Provider` — used internally, no mock needed (component creates it)

Because `triggerPicker` is an internal `useCallback` and not directly exported,
tests must exercise it indirectly:
- Option A: Pass `externalSessionAssign` prop (with a version counter) and observe
  whether `pickerPendingSession` appears in the rendered tree or `dispatch` was called.
- Option B: Render with a session-list pane in focus and simulate the `onSessionClick`
  callback through a mocked `PaneSplitRenderer`.

The simplest approach is Option A — it exercises the exact same code path as the
omnibar, URL nav, and keyboard shortcuts.
