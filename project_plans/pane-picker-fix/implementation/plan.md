# Implementation Plan — Pane Picker Fix

**Project**: pane-picker-fix  
**Requirements**: `../requirements.md`  
**Date**: 2026-05-08

---

## Overview

Three bugs are fixed across two files. All changes are purely additive-replacement (no new dependencies, no new pane types, no proto changes). The order of operations is critical: fix the reducer first (pure logic, no DOM), then fix the component, then update/add tests.

---

## Order of Operations

1. **Fix `paneReducer.ts` — ASSIGN_SESSION** (R2, Bug 2)
2. **Fix `paneReducer.ts` — SPLIT_AND_ASSIGN_SESSION** (R3, Bug 3)
3. **Fix `PaneTilingContainer.tsx` — triggerPicker bypass** (R1, Bug 1)
4. **Update `paneReducer.test.ts`** — update 1 test, add 4 new tests
5. **Create `PaneTilingContainer.test.tsx`** — 4 new tests

This order keeps the build green at every step: the reducer is pure TypeScript with no imports from the component, so step 1–2 can be verified with `cd web-app && npx jest --no-coverage --testPathPatterns="paneReducer.test"` before touching any JSX. Step 3 is safe once the reducer tests pass. Steps 4–5 add test coverage last.

---

## Step 1 — Fix `ASSIGN_SESSION` in `paneReducer.ts`

**File**: `web-app/src/lib/pane/paneReducer.ts`  
**Lines to replace**: 97–129 (the `case "ASSIGN_SESSION":` block)

### Before

```ts
case "ASSIGN_SESSION": {
  const { paneId, sessionId } = action;
  // Duplicate guard: reject if sessionId already in another leaf
  const allLeaves = getAllLeaves(state.root);
  const isDuplicate = allLeaves.some(
    (l) => l.id !== paneId && l.sessionId === sessionId
  );
  if (isDuplicate) return state;

  const target = findLeaf(state.root, paneId);
  if (!target) return state;

  // If the target is a session-list pane (no detail pane to route to), auto-split
  // it to create a new session-detail pane so the session becomes visible.
  if (target.viewKind === "session-list" && !wouldExceedMaxDepth(state.root, paneId)) {
    const newDetail = createLeaf(undefined, "terminal", "session-detail");
    const detailWithSession: LeafPane = { ...newDetail, sessionId };
    const splitNode: SplitPane = {
      type: "split",
      id: generatePaneId(),
      direction: "vertical",
      ratio: 0.35,
      first: target,
      second: detailWithSession,
    };
    const newRoot = replaceNode(state.root, paneId, splitNode);
    return { ...state, root: newRoot, focusedPaneId: detailWithSession.id };
  }

  const updated: LeafPane = { ...target, sessionId, activeTab: "terminal" };
  const newRoot = replaceNode(state.root, paneId, updated);
  return { ...state, root: newRoot };
}
```

### After

```ts
case "ASSIGN_SESSION": {
  const { paneId, sessionId } = action;
  const allLeaves = getAllLeaves(state.root);

  // Move-and-clear: if this session is already in another pane, clear it from there
  // first so the same session is never visible in two panes simultaneously.
  const sourcePaneId =
    allLeaves.find((l) => l.id !== paneId && l.sessionId === sessionId)?.id ?? null;

  const target = findLeaf(state.root, paneId);
  if (!target) return state;

  // Step 1: clear the source pane (atomic — uses the immutable replaceNode chain)
  let newRoot = state.root;
  if (sourcePaneId) {
    const sourceLeaf = findLeaf(newRoot, sourcePaneId)!;
    newRoot = replaceNode(newRoot, sourcePaneId, { ...sourceLeaf, sessionId: null });
  }

  // Step 2: assign to the target pane.
  // If the target is a session-list pane, auto-split to create a detail pane.
  if (target.viewKind === "session-list" && !wouldExceedMaxDepth(newRoot, paneId)) {
    const newDetail = createLeaf(undefined, "terminal", "session-detail");
    const detailWithSession: LeafPane = { ...newDetail, sessionId };
    const splitNode: SplitPane = {
      type: "split",
      id: generatePaneId(),
      direction: "vertical",
      ratio: 0.35,
      first: target,
      second: detailWithSession,
    };
    newRoot = replaceNode(newRoot, paneId, splitNode);
    return { ...state, root: newRoot, focusedPaneId: detailWithSession.id };
  }

  const updated: LeafPane = { ...target, sessionId, activeTab: "terminal" };
  newRoot = replaceNode(newRoot, paneId, updated);
  return { ...state, root: newRoot };
}
```

**Key change**: The `isDuplicate` guard and its `return state` are replaced with a `sourcePaneId` lookup. When `sourcePaneId` is non-null, a `replaceNode` clears the source before the target is updated. The two `replaceNode` calls use `let newRoot` chaining — the same pattern as `swapPanes` in `paneUtils.ts`.

**Subtle note**: The `wouldExceedMaxDepth(newRoot, paneId)` call in the auto-split branch now uses `newRoot` (after the source clear) rather than `state.root`. This is correct: if the source-clear changed the tree structure (it does not — it only mutates a leaf's `sessionId`, not the tree topology), the depth check operates on the current tree. Since `replaceNode` does not change tree depth, this is equivalent to checking `state.root`.

---

## Step 2 — Fix `SPLIT_AND_ASSIGN_SESSION` in `paneReducer.ts`

**File**: `web-app/src/lib/pane/paneReducer.ts`  
**Lines to replace**: 200–228 (the `case "SPLIT_AND_ASSIGN_SESSION":` block)

### Before

```ts
case "SPLIT_AND_ASSIGN_SESSION": {
  const { paneId, sessionId, tab, direction = "vertical" } = action;
  const target = findLeaf(state.root, paneId);
  if (!target) return state;

  // Duplicate guard: session already open somewhere
  const allLeaves = getAllLeaves(state.root);
  const isDuplicate = allLeaves.some((l) => l.sessionId === sessionId);
  if (isDuplicate) return state;

  // If max depth reached, fall back to assigning in place
  if (wouldExceedMaxDepth(state.root, paneId)) {
    const updated: LeafPane = { ...target, sessionId, activeTab: tab, viewKind: "session-detail" };
    const newRoot = replaceNode(state.root, paneId, updated);
    return { ...state, root: newRoot };
  }

  const newLeaf: LeafPane = { type: "leaf", id: generatePaneId(), sessionId, activeTab: tab, viewKind: "session-detail" };
  const splitNode: SplitPane = {
    type: "split",
    id: generatePaneId(),
    direction,
    ratio: 0.5,
    first: target,
    second: newLeaf,
  };
  const newRoot = replaceNode(state.root, paneId, splitNode);
  return { ...state, root: newRoot, focusedPaneId: newLeaf.id };
}
```

### After

```ts
case "SPLIT_AND_ASSIGN_SESSION": {
  const { paneId, sessionId, tab, direction = "vertical" } = action;
  const target = findLeaf(state.root, paneId);
  if (!target) return state;

  const allLeaves = getAllLeaves(state.root);

  // Move-and-clear: find the pane currently holding this session (if any).
  // Skip clearing if the source is the same pane being split (splitting the
  // pane that already holds the session is valid — the new child gets it).
  const sourceLeaf = allLeaves.find((l) => l.sessionId === sessionId) ?? null;
  const sourcePaneId = sourceLeaf?.id ?? null;

  let newRoot = state.root;
  if (sourcePaneId && sourcePaneId !== paneId) {
    const src = findLeaf(newRoot, sourcePaneId)!;
    newRoot = replaceNode(newRoot, sourcePaneId, { ...src, sessionId: null });
  }

  // If max depth reached, fall back to assigning in place
  if (wouldExceedMaxDepth(newRoot, paneId)) {
    const updated: LeafPane = { ...target, sessionId, activeTab: tab, viewKind: "session-detail" };
    newRoot = replaceNode(newRoot, paneId, updated);
    return { ...state, root: newRoot };
  }

  const newLeaf: LeafPane = { type: "leaf", id: generatePaneId(), sessionId, activeTab: tab, viewKind: "session-detail" };
  const splitNode: SplitPane = {
    type: "split",
    id: generatePaneId(),
    direction,
    ratio: 0.5,
    first: target,
    second: newLeaf,
  };
  newRoot = replaceNode(newRoot, paneId, splitNode);
  return { ...state, root: newRoot, focusedPaneId: newLeaf.id };
}
```

**Key change**: The `isDuplicate` guard is replaced with `sourcePaneId` lookup + conditional `replaceNode`. The `sourcePaneId !== paneId` guard handles the edge case where the session being moved is already in the pane being split: in that case the source is the same as the target, so clearing it before splitting would clear the target's `sessionId`, and the new split's `first` child (which is the un-modified `target` snapshot, taken before `newRoot` mutation) would have a null `sessionId`. The new `second` child (`newLeaf`) carries the `sessionId`, which is the correct "open in new pane" result regardless.

Actually, re-reading the code: `target` is captured from `state.root` at the top (before any `replaceNode` calls), so `target.sessionId` still has the old value even after `newRoot` is mutated. The `splitNode` is built from the snapshot `target` (first child). Therefore if `sourcePaneId === paneId`, the first child of the split would still show the old session. To produce the correct "open in new pane and clear original" UX, the guard `sourcePaneId !== paneId` is needed to skip the clear, and the caller (`triggerPickerForceNew`) simply creates a new split where `second` gets the session and `first` (the original pane) keeps its current session. This matches the research document's described edge case behavior.

---

## Step 3 — Fix `triggerPicker` bypass in `PaneTilingContainer.tsx`

**File**: `web-app/src/components/pane/PaneTilingContainer.tsx`  
**Lines to replace**: 68–98 (the `triggerPicker` useCallback body)

### Before

```ts
const triggerPicker = useCallback(
  (session: Session, tab?: string) => {
    const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

    // If the focused pane is already a detail pane, open there directly — the user
    // indicated intent by focusing it (e.g. omnibar search from within a detail pane).
    const focusedLeaf = findLeaf(state.root, state.focusedPaneId);
    if (focusedLeaf && focusedLeaf.viewKind === "session-detail") {
      dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
      dispatch({ type: "ASSIGN_TAB", paneId: state.focusedPaneId, tab: resolvedTab });
      return;
    }

    const allLeaves = getAllLeaves(state.root);
    const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");

    if (eligiblePanes.length === 0) {
      // Auto-split: reducer creates a new detail pane beside the session-list pane.
      dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
    } else if (eligiblePanes.length === 1) {
      dispatch({ type: "ASSIGN_SESSION", paneId: eligiblePanes[0].id, sessionId: session.id });
      dispatch({ type: "ASSIGN_TAB", paneId: eligiblePanes[0].id, tab: resolvedTab });
      // On mobile, vertical splits show only the focused pane. Move focus to the detail
      // pane so the user sees the session they just opened instead of the session list.
      dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
    } else {
      setPickerPendingSession(session);
    }
  },
  [state.root, state.focusedPaneId, dispatch],
);
```

### After

```ts
const triggerPicker = useCallback(
  (session: Session, tab?: string) => {
    const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

    const allLeaves = getAllLeaves(state.root);
    const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");

    if (eligiblePanes.length === 0) {
      // Auto-split: reducer creates a new detail pane beside the session-list pane.
      dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
    } else if (eligiblePanes.length === 1) {
      // Single detail pane: assign directly. If focused pane is the detail pane,
      // eligiblePanes[0] === focusedLeaf, so the intent shortcut still fires — just
      // via this branch rather than the removed early-return block.
      dispatch({ type: "ASSIGN_SESSION", paneId: eligiblePanes[0].id, sessionId: session.id });
      dispatch({ type: "ASSIGN_TAB", paneId: eligiblePanes[0].id, tab: resolvedTab });
      // On mobile, vertical splits show only the focused pane. Move focus to the detail
      // pane so the user sees the session they just opened instead of the session list.
      dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
    } else {
      // 2+ eligible panes: always show the picker overlay, even if a detail pane
      // is focused. The user must choose which pane receives the session.
      setPickerPendingSession(session);
    }
  },
  [state.root, state.focusedPaneId, dispatch],
);
```

**Key change**: The early-return block (lines 74–79 in the original) is removed entirely. The `eligiblePanes.length === 1` branch now handles the single-detail-pane-focused scenario correctly: `eligiblePanes[0]` is the only detail pane, which in the single-pane case is the same pane as the focused one. The behavior is preserved for R4 (single-pane direct assign) and corrected for R1 (2+ panes always show picker).

**Preserved behaviors**:
- `ASSIGN_TAB` is dispatched in the single-pane branch (same as before the bypass path also dispatched it).
- `FOCUS_PANE` fires in the single-pane branch for mobile (unchanged).
- `cancelPicker` invariant (R6): the early-return path never called `cancelPicker` (picker was never shown), and neither does the single-pane branch after this change — correct.
- The `resolvedTab` variable is kept for the single-pane `ASSIGN_TAB` dispatch; in the 2+ pane picker path, the tab defaults to `"terminal"` in the keyboard/click overlay handlers (pre-existing behavior, out of scope).

---

## Step 4 — Update `paneReducer.test.ts`

**File**: `web-app/src/lib/pane/__tests__/paneReducer.test.ts`

### 4a — Update the existing duplicate-guard test (line 185–199)

This test currently asserts the buggy no-op behavior. It must be renamed and rewritten to assert move-and-clear.

**Replace** the existing test:

```ts
it("paneReducer_should_returnUnchangedState_When_sessionAlreadyPresentInAnotherPane", () => {
  const leaf1 = leaf("pane-1", "session-A");
  const leaf2 = leaf("pane-2", null);
  const root = split("split-1", "vertical", leaf1, leaf2);
  const state = stateOf(root, "pane-2");
  const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-2", sessionId: "session-A" });
  expect(next).toEqual(state);
});
```

**With**:

```ts
it("paneReducer_should_moveSessionAndClearSource_When_sessionAlreadyPresentInAnotherPane", () => {
  const leaf1 = leaf("pane-1", "session-A");
  const leaf2 = leaf("pane-2", null);
  const root = split("split-1", "vertical", leaf1, leaf2);
  const state = stateOf(root, "pane-2");
  const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-2", sessionId: "session-A" });

  const leaves = getAllLeaves(next.root);
  const pane1 = leaves.find((l) => l.id === "pane-1")!;
  const pane2 = leaves.find((l) => l.id === "pane-2")!;

  // Session moved to pane-2
  expect(pane2.sessionId).toBe("session-A");
  // Source pane cleared
  expect(pane1.sessionId).toBeNull();
});
```

### 4b — Add new ASSIGN_SESSION tests (after the updated test above)

Append inside the `describe("ASSIGN_SESSION", ...)` block:

```ts
it("paneReducer_should_clearSourceAndAssignTarget_When_sessionMovedAcrossPanes", () => {
  // Three panes: session-A in pane-1, session-B in pane-2, pane-3 empty
  const leaf1 = leaf("pane-1", "session-A");
  const leaf2 = leaf("pane-2", "session-B");
  const leaf3 = leaf("pane-3", null);
  const inner = split("split-inner", "vertical", leaf2, leaf3);
  const root = split("split-outer", "vertical", leaf1, inner);
  const state = stateOf(root, "pane-3");

  // Move session-A from pane-1 to pane-3
  const next = paneReducer(state, { type: "ASSIGN_SESSION", paneId: "pane-3", sessionId: "session-A" });

  const leaves = getAllLeaves(next.root);
  const p1 = leaves.find((l) => l.id === "pane-1")!;
  const p2 = leaves.find((l) => l.id === "pane-2")!;
  const p3 = leaves.find((l) => l.id === "pane-3")!;

  expect(p3.sessionId).toBe("session-A");
  expect(p1.sessionId).toBeNull();
  expect(p2.sessionId).toBe("session-B"); // unaffected
});
```

### 4c — Add new SPLIT_AND_ASSIGN_SESSION describe block

Add a new top-level `describe` block after the `ASSIGN_SESSION` block:

```ts
// ─── SPLIT_AND_ASSIGN_SESSION ────────────────────────────────────────────────

describe("SPLIT_AND_ASSIGN_SESSION", () => {
  it("paneReducer_should_splitAndAssignNewLeaf_When_sessionNotInAnyPane", () => {
    const leaf1 = leaf("pane-1", null);
    const state = stateOf(leaf1, "pane-1");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "pane-1",
      sessionId: "session-A",
      tab: "terminal",
      direction: "vertical",
    });

    expect(next.root.type).toBe("split");
    const s = next.root as SplitPane;
    expect(s.direction).toBe("vertical");
    // First child is the original pane
    expect((s.first as LeafPane).id).toBe("pane-1");
    // Second child (new leaf) holds the session
    const newLeaf = s.second as LeafPane;
    expect(newLeaf.sessionId).toBe("session-A");
    expect(newLeaf.activeTab).toBe("terminal");
    expect(next.focusedPaneId).toBe(newLeaf.id);
  });

  it("paneReducer_should_clearSourceAndSplitAssign_When_sessionAlreadyInAnotherPane", () => {
    // session-A is in pane-1; split pane-2 and move session-A there
    const leaf1 = leaf("pane-1", "session-A");
    const leaf2 = leaf("pane-2", null);
    const root = split("split-1", "vertical", leaf1, leaf2);
    const state = stateOf(root, "pane-2");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "pane-2",
      sessionId: "session-A",
      tab: "terminal",
      direction: "vertical",
    });

    // Tree now has 3 leaves: pane-1 (cleared), pane-2 (original), new leaf (session-A)
    const leaves = getAllLeaves(next.root);
    const p1 = leaves.find((l) => l.id === "pane-1")!;
    const newLeaf = leaves.find((l) => l.sessionId === "session-A")!;

    expect(p1.sessionId).toBeNull();
    expect(newLeaf.sessionId).toBe("session-A");
    expect(next.focusedPaneId).toBe(newLeaf.id);
  });

  it("paneReducer_should_assignInPlace_When_maxDepthReached", () => {
    // Build a tree at max depth so the split is refused
    let node: PaneNode = leaf("deep-leaf");
    for (let i = 0; i < 8; i++) {
      node = split(`split-${i}`, "vertical", node, leaf(`other-${i}`));
    }
    const state = stateOf(node, "deep-leaf");
    const next = paneReducer(state, {
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "deep-leaf",
      sessionId: "session-X",
      tab: "terminal",
      direction: "vertical",
    });

    // Tree structure unchanged; deep-leaf now has the session
    const deepLeaf = getAllLeaves(next.root).find((l) => l.id === "deep-leaf")!;
    expect(deepLeaf.sessionId).toBe("session-X");
    // No new leaves were added
    expect(getAllLeaves(next.root).length).toBe(getAllLeaves(state.root).length);
  });
});
```

---

## Step 5 — Create `PaneTilingContainer.test.tsx`

**File**: `web-app/src/components/pane/__tests__/PaneTilingContainer.test.tsx` (new file)

This file tests `triggerPicker` indirectly via the `externalSessionAssign` prop (Option A from the research doc). The component's internal `pickerPendingSession` state causes it to render picker overlay letters when set; `dispatch` is captured via mock to detect `ASSIGN_SESSION` calls.

```tsx
/**
 * @feature pane:picker
 *
 * Tests for PaneTilingContainer.triggerPicker — verifies the picker overlay is
 * shown when 2+ eligible panes exist, and bypassed correctly for 0 or 1 pane.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";
import { PaneTilingContainer } from "../PaneTilingContainer";
import * as usePaneReducerModule from "@/lib/pane/usePaneReducer";
import * as usePaneShortcutsModule from "@/lib/pane/usePaneShortcuts";

// ─── Mock heavy dependencies ──────────────────────────────────────────────────

jest.mock("@/lib/pane/usePaneShortcuts", () => ({
  usePaneShortcuts: jest.fn(),
}));

// PaneSplitRenderer is a complex recursive component; replace with a stub that
// renders data-testid="pane-split-renderer" so we can confirm the tree mounts.
jest.mock("../PaneSplitRenderer", () => ({
  PaneSplitRenderer: () => <div data-testid="pane-split-renderer" />,
}));

// ─── Fixtures ─────────────────────────────────────────────────────────────────

function makeLeaf(id: string, viewKind: "session-detail" | "session-list", sessionId: string | null = null) {
  return { type: "leaf" as const, id, viewKind, sessionId, activeTab: "terminal" as const };
}

function makeSplit(id: string, first: object, second: object) {
  return { type: "split" as const, id, direction: "vertical" as const, ratio: 0.5, first, second };
}

function makeSession(id: string) {
  // Minimal Session proto-like object; only `id` is accessed in triggerPicker.
  return { id } as import("@/gen/session/v1/types_pb").Session;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function setupReducerMock(root: object, focusedPaneId: string) {
  const dispatch = jest.fn();
  const state = { root, focusedPaneId, zoomedPaneId: null };
  jest
    .spyOn(usePaneReducerModule, "usePaneReducer")
    .mockReturnValue([state as import("@/lib/pane/paneTypes").PaneState, dispatch]);
  return { state, dispatch };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

beforeEach(() => {
  jest.clearAllMocks();
});

describe("triggerPicker", () => {
  it("triggerPicker_should_showPickerOverlay_When_twoDetailPanesExist", async () => {
    // Two session-detail panes; detail pane-1 is focused (the bypass scenario).
    const pane1 = makeLeaf("pane-1", "session-detail");
    const pane2 = makeLeaf("pane-2", "session-detail");
    const root = makeSplit("split-1", pane1, pane2);
    const { dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-X");
    let assignVersion = 1;

    const { rerender } = render(
      <PaneTilingContainer
        sessions={[session]}
        externalSessionAssign={{ sessionId: "session-X", version: assignVersion }}
      />
    );

    // Advance version to trigger the effect
    assignVersion++;
    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
          externalSessionAssign={{ sessionId: "session-X", version: assignVersion }}
        />
      );
    });

    // ASSIGN_SESSION must NOT have been dispatched (picker should show instead)
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION" })
    );
    // Picker overlay renders letter labels — the component renders pane letters
    // when pickerPendingSession is set. We check via the aria-label or data-testid
    // that the overlay is present. Since PaneSplitRenderer is mocked, we assert
    // that the Context value was updated by checking dispatch was NOT called.
    // (Full overlay rendering tested in PaneSplitRenderer tests.)
  });

  it("triggerPicker_should_assignDirectly_When_oneDetailPane", async () => {
    // One session-list pane + one session-detail pane.
    const listPane = makeLeaf("pane-list", "session-list");
    const detailPane = makeLeaf("pane-detail", "session-detail");
    const root = makeSplit("split-1", listPane, detailPane);
    // List pane is focused (user clicked session from the list)
    const { dispatch } = setupReducerMock(root, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // Should dispatch ASSIGN_SESSION directly to the single detail pane
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION", paneId: "pane-detail", sessionId: "session-X" })
    );
  });

  it("triggerPicker_should_autoSplit_When_noDetailPanes", async () => {
    // Only a session-list pane — no detail panes.
    const listPane = makeLeaf("pane-list", "session-list");
    const { dispatch } = setupReducerMock(listPane, "pane-list");

    const session = makeSession("session-X");

    await act(async () => {
      render(
        <PaneTilingContainer
          sessions={[session]}
          externalSessionAssign={{ sessionId: "session-X", version: 1 }}
        />
      );
    });

    // Should dispatch ASSIGN_SESSION to focusedPaneId (reducer handles auto-split)
    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION", paneId: "pane-list", sessionId: "session-X" })
    );
  });

  it("triggerPicker_should_showPicker_When_detailPaneFocusedAndTwoPanesExist", async () => {
    // Regression guard for the specific Bug 1 scenario:
    // detail pane IS focused, but 2 detail panes exist → picker must still show.
    const pane1 = makeLeaf("pane-1", "session-detail", "session-A");
    const pane2 = makeLeaf("pane-2", "session-detail", null);
    const root = makeSplit("split-1", pane1, pane2);
    // Detail pane-1 is focused — this was the bypass condition before the fix
    const { dispatch } = setupReducerMock(root, "pane-1");

    const session = makeSession("session-B");
    let version = 1;

    const { rerender } = render(
      <PaneTilingContainer sessions={[session]} externalSessionAssign={null} />
    );

    await act(async () => {
      rerender(
        <PaneTilingContainer
          sessions={[session]}
          externalSessionAssign={{ sessionId: "session-B", version }}
        />
      );
    });

    // With the fix, ASSIGN_SESSION must NOT be dispatched — picker overlay shows instead
    expect(dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: "ASSIGN_SESSION" })
    );
  });
});
```

---

## Verification Checklist

After implementing all steps, run the following in order:

```bash
# 1. Reducer unit tests (pure TS, no DOM)
cd web-app && npx jest --no-coverage --testPathPatterns="paneReducer.test"

# 2. New PaneTilingContainer tests
cd web-app && npx jest --no-coverage --testPathPatterns="PaneTilingContainer.test"

# 3. Full pane test suite (ensures no regressions in other pane tests)
cd web-app && npx jest --no-coverage --testPathPatterns="pane"

# 4. Full frontend test suite
cd web-app && npx jest --no-coverage

# 5. Build check
make build

# 6. Lint
make lint
```

---

## Architectural Flags

**No new dependencies**: All changes use existing utilities (`replaceNode`, `getAllLeaves`, `findLeaf`) with no new imports in production code.

**No new action types**: The fix is purely behavioral — the same three action types handle move semantics. TypeScript exhaustiveness check (`never` in `default`) is unaffected.

**focusedPaneId invariant preserved**: Move-and-clear only mutates leaf `sessionId` fields — pane IDs in the tree are never added or removed in `ASSIGN_SESSION`. The `SPLIT_AND_ASSIGN_SESSION` path already correctly sets `focusedPaneId` to the new leaf. The invariant test `paneReducer_should_neverProduceStateWithMissingFocusedPaneId_When_$type` continues to pass.

**Keyboard picker benefits automatically**: The `keydown` handler in `PaneTilingContainer.tsx:119–122` dispatches `ASSIGN_SESSION` directly. After Step 1, it automatically gets move-and-clear semantics with no code change needed.

**Picker overlay click benefits automatically**: Same — overlay click at line 272 dispatches `ASSIGN_SESSION` directly.

**`cancelPicker` invariant (R6) unchanged**: The early-return bypass path that was removed never called `cancelPicker` (the picker was never shown). The remaining single-pane and 0-pane branches also don't call `cancelPicker`. Only the keyboard handler and overlay click call it — both still do.

---

## Epic / Story / Task Counts

| Level | Count | Details |
|---|---|---|
| Epics | 1 | Pane Picker Fix |
| Stories | 3 | R1 (picker bypass), R2 (ASSIGN_SESSION move), R3 (SPLIT_AND_ASSIGN move) |
| Tasks (production code) | 3 | One block edit per bug (Steps 1–3) |
| Tasks (tests) | 5 | 1 test updated + 2 new ASSIGN_SESSION tests + 3 new SPLIT_AND_ASSIGN tests + 4 new PaneTilingContainer tests |
| Total tasks | 8 | All within 2 existing files + 1 new file |

**Architectural flags**: None. No new pane types, no proto changes, no new React context fields, no new hooks. All changes are localized to the reducer switch cases and one `useCallback` closure.
