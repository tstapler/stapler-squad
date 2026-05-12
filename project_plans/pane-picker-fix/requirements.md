# Pane Session Picker — Bug Fix & Edge Case Hardening

## Problem Statement

When the user has 2+ session-detail panes open and clicks a session in the session list,
the pane picker (A/B letter overlay) either:
1. **Never appears** — the session is silently assigned to the currently-focused detail pane
2. **Appears but clicking does nothing** — when the target session is already open in
   another pane, `ASSIGN_SESSION` silently rejects the action due to a duplicate guard

The user expected to be able to choose which pane receives the session every time there are
multiple eligible panes, and to be able to move a session from one pane to another.

## Root Causes (Identified via Code Review)

### Bug 1 — `triggerPicker` bypasses picker for focused detail pane (PaneTilingContainer.tsx:74–79)

```ts
const focusedLeaf = findLeaf(state.root, state.focusedPaneId);
if (focusedLeaf && focusedLeaf.viewKind === "session-detail") {
  dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
  return;  // ← skips picker regardless of how many panes exist
}
```

**Impact:** After the user clicks any detail pane (which sets `focusedPaneId`), every subsequent
session-list click bypasses the picker and goes straight to the focused pane, even with 2+
eligible panes.

**Correct behavior:** This shortcut should only apply when there is exactly 1 eligible
detail pane. With 2+ panes, always show the picker.

### Bug 2 — `ASSIGN_SESSION` duplicate guard silently blocks moves (paneReducer.ts:99–104)

```ts
const isDuplicate = allLeaves.some(
  (l) => l.id !== paneId && l.sessionId === sessionId
);
if (isDuplicate) return state;  // ← no-op, no feedback
```

**Impact:** If a session is already visible in pane B and the user selects pane A in the
picker, ASSIGN_SESSION does nothing, but `cancelPicker()` still fires — the overlay
disappears but the session doesn't move.

**Correct behavior:** Moving a session from one pane to another should be allowed.
The old pane should be left empty (sessionId: null), and the session assigned to the target.

### Bug 3 — `SPLIT_AND_ASSIGN_SESSION` has the same duplicate guard (paneReducer.ts:205–208)

```ts
const isDuplicate = allLeaves.some((l) => l.sessionId === sessionId);
if (isDuplicate) return state;
```

**Impact:** Alt+click / "Open in new pane" silently fails if the session is already open
in any pane.

**Correct behavior:** Opening an already-visible session in a new pane should close it
from its current pane and open it in the new one.

## Requirements

### R1 — Picker always shown with 2+ eligible panes (fixes Bug 1)

When `triggerPicker()` is called:
- If there are 0 eligible detail panes → auto-split (existing behavior)
- If there is exactly 1 eligible detail pane → assign directly to it (existing behavior)
- If there are 2+ eligible detail panes → **always** show the A/B picker overlay,
  regardless of which pane has focus

The "focused detail pane = bypass" shortcut must be removed from the 2+ pane path.

### R2 — ASSIGN_SESSION allows moving sessions between panes (fixes Bug 2)

When `ASSIGN_SESSION` targets pane A with session X, and session X is currently in pane B:
- Clear pane B (set `sessionId: null`)
- Assign session X to pane A
- Return updated state

The duplicate guard must be removed or replaced with this move-and-clear logic.

### R3 — SPLIT_AND_ASSIGN_SESSION allows moving sessions to a new split (fixes Bug 3)

When `SPLIT_AND_ASSIGN_SESSION` targets pane A with session X, and session X is in pane B:
- Clear pane B (set `sessionId: null`)
- Create the split in pane A and assign session X to the new pane
- Return updated state

The duplicate guard must be replaced with the same move-and-clear logic.

### R4 — No regression on single-pane workflows

When there is exactly 1 eligible detail pane:
- Session-list click → assign directly (no picker, no overlay)
- Focus is moved to the detail pane on mobile (existing behavior preserved)

### R5 — Keyboard picker (A/Z letters) continues to work correctly

The keydown handler in `PaneTilingContainer.tsx:104–130` already correctly routes to
`ASSIGN_SESSION`. After fixing R2, pressing A/B to select a pane must also correctly
move sessions that are already open elsewhere.

### R6 — Picker closes reliably after selection

After any successful assignment (click overlay or keyboard), `cancelPicker()` must fire.
It already fires in all code paths — verify this is preserved after refactor.

## Out of Scope

- Drag-and-drop pane reordering (separate feature, already working via SWAP_PANES)
- Mobile layout changes (single-pane mode on narrow screens is intentional)
- Persisting layout across page reloads (separate concern)
- UX polish to the picker overlay visual design

## Affected Files

| File | Change |
|---|---|
| `web-app/src/components/pane/PaneTilingContainer.tsx` | Remove focused-pane bypass in `triggerPicker` for 2+ pane case |
| `web-app/src/lib/pane/paneReducer.ts` | Replace duplicate guard in `ASSIGN_SESSION` and `SPLIT_AND_ASSIGN_SESSION` with move-and-clear |
| `web-app/src/lib/pane/__tests__/paneReducer.test.ts` | Add tests for move-between-panes behavior |
| `web-app/src/components/pane/__tests__/PaneTilingContainer.test.tsx` | Add tests for triggerPicker with 2+ eligible panes |

## Test Cases

| Scenario | Expected |
|---|---|
| 2 detail panes, detail pane focused, click session in list | Picker overlay shows |
| 2 detail panes, session-list pane focused, click session | Picker overlay shows |
| 1 detail pane, click session in list | Assigned directly, no picker |
| 0 detail panes, click session in list | Auto-split + assign |
| Picker shows, click overlay for pane A | Session appears in pane A |
| Picker shows, session already in pane B, click pane A | Session moves to A, pane B becomes empty |
| Picker shows, press keyboard letter A | Session assigned to first eligible pane |
| Alt+click session already in pane B | Closes from B, new split opens it |
| Picker shows, press Escape | Picker closes, no assignment |
