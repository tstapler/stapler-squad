# Cockpit Tiling Layout — Implementation Plan

## 1. Architecture Overview

### Pane Tree State Location

The pane tree state lives in a custom `usePaneReducer` hook, colocated with `HomeContent` in `web-app/src/app/page.tsx`. This replaces the existing `selectedSession` / `activeTab` pair with a `PaneState` object that encapsulates all pane-related state. The hook is **not** a Context or Redux slice — it returns `[state, dispatch]` and the values are threaded downward as props.

The hook is extracted to `web-app/src/lib/pane/usePaneReducer.ts` so it can be unit-tested independently of React components.

### Integration with Existing `selectedSession` State

`page.tsx` currently holds:

```
const [selectedSession, setSelectedSession] = useState<Session | null>(null);
const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
```

These two lines are replaced by:

```
const [paneState, dispatchPane] = usePaneReducer(sessions);
```

Derived values that the rest of `HomeContent` needs are computed from `paneState`:

- `selectedSession` → `getFocusedLeaf(paneState.root, paneState.focusedPaneId)?.sessionId` looked up in `sessions`
- `activeTab` → `getFocusedLeaf(...)?.activeTab`
- `sessionSelected` (for cockpitGrid variant) → `hasFocusedSession(paneState)`

`SessionList`'s `onSessionClick` callback becomes:

```
dispatchPane({ type: "ASSIGN_SESSION", paneId: paneState.focusedPaneId, sessionId: session.id })
```

The session-list resize handle (US-1) is separate from pane tree state. It uses its own isolated `useListColumnWidth` hook that reads/writes `cockpit.listColumnWidth` in localStorage.

### File Structure

**New files to create:**

```
web-app/src/lib/pane/
  types.ts                     — PaneTree, PaneState, PaneAction types
  paneReducer.ts               — Pure reducer function (no React dependency)
  paneUtils.ts                 — Tree traversal helpers (findLeaf, findParentSplit, etc.)
  usePaneReducer.ts            — useReducer wrapper + localStorage restore/save
  usePaneLayout.ts             — localStorage read/write + validateAndRepair

web-app/src/components/pane/
  PaneSplitRenderer.tsx        — Recursive tree renderer (PaneNode + PaneLeaf)
  PaneHeader.tsx               — 32px pane header bar (title, tab switcher, close button)
  ResizeHandle.tsx             — Pointer-events drag handle
  MobilePaneTabStrip.tsx       — Bottom tab strip for stacked panes on <768px

web-app/src/styles/pane/
  paneSplit.css.ts             — splitContainer recipe (CSS grid + --split-ratio bridge)
  paneLeaf.css.ts              — pane wrapper, focused border, zoom overlay
  resizeHandle.css.ts          — handle 6px visual / 20px hit target
  paneHeader.css.ts            — 32px header bar, title truncation, close button
  mobilePaneTabStrip.css.ts    — mobile bottom tab strip

web-app/src/lib/hooks/
  useListColumnWidth.ts        — localStorage persistence for session list column width
  useSplitContainerSize.ts     — ResizeObserver wrapper for nudge-resize container size
```

**Existing files to modify:**

```
web-app/src/app/page.tsx
  — Replace selectedSession/activeTab state with usePaneReducer
  — Add ListColumnResizeHandle between sessionListColumn and detailColumn
  — Thread dispatchPane + focusedPaneId to SessionList onSessionClick
  — Replace SessionDetail render with <PaneSplitRenderer>

web-app/src/styles/sessionCockpit.css.ts
  — cockpitGrid: change gridTemplateColumns hardcoded "280px" to "var(--list-col-width, 280px)"
  — Add listResizeHandle recipe (the column boundary handle, US-1)

web-app/src/lib/shortcuts/shortcutRegistry.ts
  — Add "cockpit" to ShortcutContext union
  — Add "cockpit" key to getAll() result object

web-app/src/components/sessions/SessionDetail.tsx
  — Accept optional paneId prop; use key={paneId + "-" + sessionId} on pool root

docs/registry/features/frontend/cockpit-tiling.json
  — New feature registry entry
```

---

## 2. Data Model

### TypeScript Types (`web-app/src/lib/pane/types.ts`)

```typescript
// nanoid(8) from the existing nanoid dep, or crypto.randomUUID()
export type PaneId = string;

// Mirrors SessionDetail's tab union — imported from SessionDetail.tsx
export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

export interface LeafPane {
  type: "leaf";
  id: PaneId;
  sessionId: string | null;  // null = empty slot ("click a session to load")
  activeTab: SessionDetailTab;
}

export interface SplitPane {
  type: "split";
  id: PaneId;
  // "vertical"   = children sit left | right (column split)
  // "horizontal" = children sit top | bottom (row split)
  direction: "horizontal" | "vertical";
  ratio: number;           // [0.0, 1.0], fraction of space given to `first`
  first: PaneTree;
  second: PaneTree;
}

export type PaneTree = LeafPane | SplitPane;

export interface PaneState {
  root: PaneTree;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;  // Ctrl+Z: null = normal view
}

// Persisted to localStorage as-is (no DOM refs, no functions)
export interface PersistedPaneLayout {
  version: 1;
  root: PaneTree;
  focusedPaneId: PaneId;
  zoomedPaneId: PaneId | null;
}
```

### PaneAction Union (`web-app/src/lib/pane/types.ts`, continued)

```typescript
export type PaneAction =
  // Epic A
  | { type: "SPLIT_PANE";    paneId: PaneId; direction: "horizontal" | "vertical" }
  | { type: "CLOSE_PANE";    paneId: PaneId }
  | { type: "RESIZE_PANE";   splitId: PaneId; ratio: number }        // drag handle → splitId is the SplitPane node id
  | { type: "FOCUS_PANE";    paneId: PaneId }
  | { type: "ASSIGN_SESSION"; paneId: PaneId; sessionId: string }    // session list click
  | { type: "ASSIGN_TAB";    paneId: PaneId; tab: SessionDetailTab } // tab click inside pane header
  | { type: "ZOOM_PANE";     paneId: PaneId | null }                 // null = unzoom
  | { type: "NUDGE_RESIZE";  paneId: PaneId; direction: "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown"; amountPx: number; containerSizePx: number }
  | { type: "RESET_LAYOUT" }
  | { type: "RESTORE_LAYOUT"; state: PaneState };                    // from localStorage
```

Note: `RESIZE_PANE` uses `splitId` (the `SplitPane` node's id) not a leaf id, because the resize ratio lives on the split node. `NUDGE_RESIZE` uses the leaf `paneId` of the focused pane — the reducer walks up to the nearest ancestor `SplitPane` to find what to resize.

### Reducer Action Logic Summary

| Action | Reducer behavior |
|--------|-----------------|
| `SPLIT_PANE` | Find leaf by `paneId`; replace with `SplitPane { first: originalLeaf, second: newEmptyLeaf }`. Set `focusedPaneId` to the new leaf. Reject if session is already open in another leaf (see Known Issues #2). |
| `CLOSE_PANE` | Find parent `SplitPane` of `paneId`. Replace parent with the other child. If closing root leaf, reset to initial state. Set focus to remaining sibling leaf (BFS to nearest leaf). |
| `RESIZE_PANE` | Find `SplitPane` where `id === splitId`. Clamp `ratio` to `[MIN_PX/totalPx, 1 - MIN_PX/totalPx]`. Update in-place. |
| `FOCUS_PANE` | Set `focusedPaneId`. |
| `ASSIGN_SESSION` | Find leaf by `paneId`. Reject if `sessionId` already appears in any other leaf. Set `sessionId` and reset `activeTab` to `"terminal"`. |
| `ASSIGN_TAB` | Find leaf by `paneId`. Set `activeTab`. |
| `ZOOM_PANE` | Toggle `zoomedPaneId`. Null = show all panes. |
| `NUDGE_RESIZE` | Walk tree to find nearest ancestor `SplitPane` whose direction aligns with the arrow key. Compute `delta = amountPx / containerSizePx`. Add/subtract based on whether `paneId` is in `first` subtree. Clamp. |
| `RESET_LAYOUT` | Return `initialPaneState()` — single empty leaf, no session. Clear localStorage key. |
| `RESTORE_LAYOUT` | Replace state entirely. Run `validateAndRepair` before dispatch (in the hook, not the reducer). |

### localStorage Serialization Schema

Key: `cockpit.paneLayout`

```json
{
  "version": 1,
  "root": { "type": "leaf", "id": "ab12cd34", "sessionId": null, "activeTab": "terminal" },
  "focusedPaneId": "ab12cd34",
  "zoomedPaneId": null
}
```

`PaneTree` is fully JSON-serializable. `version: 1` enables future migrations. Deserialization:

1. `JSON.parse(stored)`
2. Validate `version === 1` (discard if unknown version)
3. Call `validateAndRepair(parsed.root, new Set(sessions.map(s => s.id)))` to null out stale session IDs
4. Dispatch `RESTORE_LAYOUT`

---

## 3. Implementation Tasks

### Epic A: Data Model + Reducer

**A-001** — Create `web-app/src/lib/pane/types.ts`
- Full TypeScript type definitions: `PaneId`, `LeafPane`, `SplitPane`, `PaneTree`, `PaneState`, `PaneAction`, `PersistedPaneLayout`
- No runtime code; types only
- Est. 60 lines
- Dependencies: none

**A-002** — Create `web-app/src/lib/pane/paneUtils.ts`
- Tree traversal helpers:
  - `findLeaf(root, paneId): LeafPane | null`
  - `findParentSplit(root, paneId): SplitPane | null`
  - `findNearestAncestorSplit(root, paneId, direction): SplitPane | null`
  - `getAllLeaves(root): LeafPane[]`
  - `replaceNode(root, targetId, replacement): PaneTree`
  - `getAdjacentLeaf(root, paneId, direction): LeafPane | null` (for focus navigation)
  - `initialPaneState(): PaneState`
- All pure functions, no side effects
- Est. 120 lines
- Dependencies: A-001

**A-003** — Create `web-app/src/lib/pane/paneReducer.ts`
- Pure `paneReducer(state: PaneState, action: PaneAction): PaneState`
- Implements all 10 action branches using helpers from A-002
- `SPLIT_PANE`: reject (return unchanged state) if resulting session would duplicate — callers should check `getAllLeaves` first, but reducer guards defensively
- `CLOSE_PANE`: special case when `root` is a leaf (reset to initial rather than error)
- `NUDGE_RESIZE`: uses `findNearestAncestorSplit` to locate the target split; handles case where no split found (single pane, no-op)
- Est. 150 lines
- Dependencies: A-001, A-002

**A-004** — Create `web-app/src/lib/pane/usePaneLayout.ts`
- `validateAndRepair(tree: PaneTree, validIds: Set<string>): PaneTree` — recursive, nullifies stale sessionIds
- `savePaneLayout(state: PaneState): void` — `localStorage.setItem("cockpit.paneLayout", JSON.stringify(...))`
- `loadPaneLayout(): PersistedPaneLayout | null` — parse + version check; returns null on error
- Est. 50 lines
- Dependencies: A-001

**A-005** — Create `web-app/src/lib/pane/usePaneReducer.ts`
- `usePaneReducer(sessions: Session[] | null): [PaneState, Dispatch<PaneAction>]`
- Wraps `useReducer(paneReducer, undefined, initialPaneState)`
- `useEffect` on `sessions` change: call `loadPaneLayout()`, run `validateAndRepair`, dispatch `RESTORE_LAYOUT` on first sessions load (ref guard to run once)
- `useEffect` on state change: call `savePaneLayout(state)` (debounced 100ms to avoid rapid localStorage writes during drag)
- Export derived selector: `getFocusedLeaf(state: PaneState): LeafPane | null`
- Est. 70 lines
- Dependencies: A-001, A-003, A-004

**A-006** — Unit tests for paneReducer
- File: `web-app/src/lib/pane/paneReducer.test.ts`
- Test coverage:
  - `SPLIT_PANE`: produces correct tree shape, new leaf is focused, duplicate session rejected
  - `CLOSE_PANE`: collapses tree correctly, closing root resets, focus moves to sibling
  - `RESIZE_PANE`: ratio updated, clamped to min bounds
  - `NUDGE_RESIZE`: correct ancestor found, delta applied, direction handling
  - `ASSIGN_SESSION`: sets sessionId, rejects duplicate
  - `ZOOM_PANE`: toggles correctly
  - `RESET_LAYOUT`: returns initial state
- Est. 180 lines
- Dependencies: A-003

---

### Epic B: Pane Tree Renderer

**B-001** — Create `web-app/src/styles/pane/paneSplit.css.ts`
- `splitContainer` recipe with variants `direction: "vertical" | "horizontal"`
- Vertical: `gridTemplateColumns: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr"`
- Horizontal: `gridTemplateRows: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr"`
- Base: `display: grid; width: 100%; height: 100%; overflow: hidden`
- Est. 35 lines
- Dependencies: none (vanilla-extract only)

**B-002** — Create `web-app/src/styles/pane/paneLeaf.css.ts`
- `paneLeaf` recipe with variant `focused: true | false`
  - Base: `display: flex; flex-direction: column; overflow: hidden; minWidth: 0; minHeight: 0`
  - `focused: true`: `outline: 2px solid ${vars.color.primary}; outline-offset: -2px`
  - `focused: false`: `outline: none`
- `paneLeafZoomed`: absolute overlay filling parent, `zIndex: zIndex.raised`, used when `zoomedPaneId` matches
- `emptyPaneSlot`: centered placeholder text style for null sessionId
- Est. 40 lines
- Dependencies: none

**B-003** — Create `web-app/src/styles/pane/paneHeader.css.ts`
- `paneHeader`: `height: 32px; display: flex; align-items: center; gap: vars.space["2"]; padding: 0 vars.space["2"]; background: vars.color.cardBackground; borderBottom: 1px solid vars.color.borderColor; flexShrink: 0`
- `paneTitle`: `flex: 1; overflow: hidden; textOverflow: ellipsis; whiteSpace: nowrap; fontSize: vars.fontSize.sm`
- `paneCloseButton`: small 20px × 20px button, hover shows `vars.color.error`
- `paneTabButton` recipe with `active: boolean` — compact tab switcher buttons in the header
- Est. 50 lines
- Dependencies: none

**B-004** — Create `web-app/src/components/pane/PaneHeader.tsx`
- Props: `pane: LeafPane`, `sessions: Session[]`, `onClose: () => void`, `onFocus: () => void`, `onTabChange: (tab: SessionDetailTab) => void`
- Renders: session name (or "Empty" if null) | tab switcher buttons | close (✕) button
- `onClick` on the header div dispatches `FOCUS_PANE`
- Tab buttons use compact labels: T, D, V, L, I, F (Terminal, Diff, VCS, Logs, Info, Files)
- Est. 60 lines
- Dependencies: B-003, A-001

**B-005** — Create `web-app/src/components/pane/PaneSplitRenderer.tsx`
- `PaneNode` component: `{ node: PaneTree, state: PaneState, dispatch: Dispatch<PaneAction>, sessions: Session[] }`
  - If `node.type === "leaf"`: render `<PaneLeaf>`
  - If `node.type === "split"`: render `<div className={splitContainer({ direction })} style={{ "--split-ratio": String(node.ratio) }}>`  with `PaneNode first`, `ResizeHandle`, `PaneNode second`
- `PaneLeaf` component:
  - Renders `<PaneHeader>` + `<SessionDetail key={pane.id + "-" + (pane.sessionId ?? "empty")} session={...} />`
  - When `pane.sessionId === null`: renders `<EmptyPaneSlot>` instead of SessionDetail
  - Applies `focused` variant from pane leaf style
  - When `state.zoomedPaneId === pane.id`: applies zoom overlay style
- `EmptyPaneSlot`: centered message "Click a session to open it here"
- Est. 100 lines
- Dependencies: B-001, B-002, B-004, A-001, Epic C (ResizeHandle)

**B-006** — Create `web-app/src/styles/pane/mobilePaneTabStrip.css.ts`
- `mobileTabStrip`: `display: flex; borderTop: 1px solid vars.color.borderColor; background: vars.color.cardBackground; overflowX: auto; flexShrink: 0; height: 40px`
- `mobileTabButton` recipe with `active: boolean`
- Est. 25 lines
- Dependencies: none

**B-007** — Create `web-app/src/components/pane/MobilePaneTabStrip.tsx`
- Props: `leaves: LeafPane[]`, `focusedPaneId: PaneId`, `sessions: Session[]`, `onFocus: (paneId: PaneId) => void`
- Renders a scrollable row of tab buttons, one per leaf; tapping focuses that pane
- Shown only on `<768px` when there are vertical splits (detected by `isVerticalSplit` on root)
- Est. 40 lines
- Dependencies: B-006, A-001

---

### Epic C: Drag Resize

**C-001** — Create `web-app/src/lib/hooks/useSplitContainerSize.ts`
- `useSplitContainerSize(ref: RefObject<HTMLElement | null>): { width: number; height: number }`
- `ResizeObserver` on the element, updates `useState` on every change
- Cleanup: `ro.disconnect()` on unmount
- Used by the `SplitPane` renderer to supply `containerSizePx` for `NUDGE_RESIZE`
- Est. 30 lines
- Dependencies: none

**C-002** — Create `web-app/src/styles/pane/resizeHandle.css.ts`
- `resizeHandle` recipe with variants `direction: "vertical" | "horizontal"`
- Vertical: `width: 6px; cursor: col-resize; marginLeft: -7px; marginRight: -7px; paddingLeft: 7px; paddingRight: 7px`
- Horizontal: `height: 6px; cursor: row-resize; marginTop: -7px; marginBottom: -7px; paddingTop: 7px; paddingBottom: 7px`
- Base: `position: relative; flexShrink: 0; touchAction: none; userSelect: none; display: flex; alignItems: center; justifyContent: center; zIndex: zIndex.raised`
- `::after` pseudo-element: indicator bar (2px × 24px for vertical, 24px × 2px for horizontal), colored `vars.color.borderColor`, brightens to `vars.color.primary` on `:hover`
- Est. 55 lines
- Dependencies: none

**C-003** — Create `web-app/src/components/pane/ResizeHandle.tsx`
- Props: `splitId: PaneId`, `direction: "horizontal" | "vertical"`, `onResize: (splitId: PaneId, ratio: number) => void`
- Pointer events: `onPointerDown` → `setPointerCapture` + set `draggingRef.current = true`
- `onPointerMove`: compute `newRatio` from `getBoundingClientRect()` of `parentElement`; store in `pendingRatioRef`; schedule single rAF (skip if already pending); rAF callback calls `onResize` then clears itself
- `onPointerUp` / `onPointerCancel`: `releasePointerCapture`, `draggingRef.current = false`, cancel pending rAF
- No `useEffect` — pointer capture handles event routing automatically
- Min clamp: `clampRatio(raw, 200, totalPx)` inline helper — no import needed
- Est. 70 lines
- Dependencies: C-002

---

### Epic D: Keyboard Shortcuts

**D-001** — Extend `ShortcutContext` in `web-app/src/lib/shortcuts/shortcutRegistry.ts`
- Change `export type ShortcutContext = "global" | "session-list" | "approval" | "terminal"` to add `| "cockpit"`
- Add `cockpit: []` to `getAll()` result object initializer
- Update `dispatch()` terminal context guard: the existing rule `if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global") continue` must allow `"cockpit"` — change to `&& shortcut.context !== "cockpit"`
- Est. 8 lines changed
- Dependencies: none

**D-002** — Create `web-app/src/lib/pane/usePaneShortcuts.ts`
- Single hook that registers all 12 pane shortcuts via `useShortcut`
- Receives `state: PaneState`, `dispatch: Dispatch<PaneAction>`, `containerRef: RefObject<HTMLElement | null>` (for nudge container size)
- Shortcuts table:

| ID | Key | Modifiers | Label | Action |
|----|-----|-----------|-------|--------|
| `cockpit.split-vertical` | `\` | ctrl | Split pane vertically | `SPLIT_PANE direction: "vertical"` |
| `cockpit.split-horizontal` | `-` | ctrl | Split pane horizontally | `SPLIT_PANE direction: "horizontal"` |
| `cockpit.close-pane` | `w` | ctrl | Close focused pane | `CLOSE_PANE` |
| `cockpit.focus-right` | `ArrowRight` | ctrl | Focus pane right | `FOCUS_PANE` via `getAdjacentLeaf` |
| `cockpit.focus-left` | `ArrowLeft` | ctrl | Focus pane left | `FOCUS_PANE` via `getAdjacentLeaf` |
| `cockpit.focus-up` | `ArrowUp` | ctrl | Focus pane up | `FOCUS_PANE` via `getAdjacentLeaf` |
| `cockpit.focus-down` | `ArrowDown` | ctrl | Focus pane down | `FOCUS_PANE` via `getAdjacentLeaf` |
| `cockpit.resize-right` | `ArrowRight` | ctrl+alt | Resize right | `NUDGE_RESIZE` |
| `cockpit.resize-left` | `ArrowLeft` | ctrl+alt | Resize left | `NUDGE_RESIZE` |
| `cockpit.resize-up` | `ArrowUp` | ctrl+alt | Resize up | `NUDGE_RESIZE` |
| `cockpit.resize-down` | `ArrowDown` | ctrl+alt | Resize down | `NUDGE_RESIZE` |
| `cockpit.zoom-pane` | `z` | ctrl | Zoom/unzoom focused pane | `ZOOM_PANE` |

- All use `context: "cockpit"`
- Each action wrapped in `useCallback` to stabilize identity
- `containerRef` passed in from `PaneSplitRenderer`'s outer div ref; `useSplitContainerSize` not needed here — use `containerRef.current?.getBoundingClientRect()` at call time
- Est. 90 lines
- Dependencies: D-001, A-001, A-002, A-005

**D-003** — Update the `?` shortcut overlay component to include the "cockpit" section
- Find the file rendering the shortcut overlay (search for `getAll()` usage)
- Add a `"Cockpit / Panes"` section grouping `cockpit` context shortcuts
- Est. 15 lines changed
- Dependencies: D-001

---

### Epic E: Session Assignment Routing

**E-001** — Modify `web-app/src/app/page.tsx`
- Remove `const [selectedSession, setSelectedSession]` and `const [activeTab, setActiveTab]`
- Add `const [paneState, dispatchPane] = usePaneReducer(sessions)`
- Compute derived: `const focusedLeaf = getFocusedLeaf(paneState)` and `const selectedSession = focusedLeaf?.sessionId ? sessions?.find(s => s.id === focusedLeaf.sessionId) ?? null : null`
- `lastVisibleSessionRef` logic: keep for the detail column's `sessionSelected` variant prop (drives mobile layout)
- Update `sessionListColumn` `sessionSelected` prop: `!!paneState.focusedPaneId && getFocusedLeaf(paneState)?.sessionId != null`
- Replace the inner wrapper + `<SessionDetail>` with `<PaneSplitRenderer state={paneState} dispatch={dispatchPane} sessions={sessions ?? []} />`
- Pass `dispatchPane` and `paneState.focusedPaneId` to `SessionList` via updated prop (or a thin adapter callback `onSessionClick`)
- Call `usePaneShortcuts(paneState, dispatchPane, detailColumnRef)` at `HomeContent` level
- Add list column resize handle between `sessionListColumn` and `detailColumn` (see E-002)
- Est. 60 lines changed
- Dependencies: A-005, B-005, D-002, E-002

**E-002** — Create `web-app/src/lib/hooks/useListColumnWidth.ts`
- `useListColumnWidth(): [number, (w: number) => void]`
- Reads `localStorage.getItem("cockpit.listColumnWidth")` on init; defaults to 280
- Returns `[width, setWidth]`; `setWidth` clamps to `[160, viewportWidth * 0.5]` and saves to localStorage
- Applied to the cockpit grid via `style={{ "--list-col-width": width + "px" }}` on the grid div in `page.tsx`
- Est. 35 lines
- Dependencies: none

**E-003** — Create list column `ResizeHandle` at cockpit grid level
- The `ResizeHandle` component (C-003) is reused here with a different `onResize` callback that calls `setListColumnWidth` instead of dispatching `RESIZE_PANE`
- Add it as a direct child between `sessionListColumn` and `detailColumn` in `page.tsx`
- Requires `sessionCockpit.css.ts` change: cockpitGrid `gridTemplateColumns` to `"var(--list-col-width, 280px) 6px 1fr"` (adds handle column)
- Est. 20 lines changed in `page.tsx` + 10 lines in `sessionCockpit.css.ts`
- Dependencies: C-003, E-002

**E-004** — Modify `SessionList` to accept `onSessionClick(session: Session)` callback
- Verify existing `onSessionClick` prop signature — if already a prop, confirm it passes the full `Session` object
- The handler in `HomeContent` changes from `setSelectedSession` to `dispatchPane({ type: "ASSIGN_SESSION", ... })`
- No changes to `SessionList` itself if the prop contract is already correct
- Est. 5–15 lines changed
- Dependencies: E-001

---

### Epic F: localStorage Persistence

**F-001** — `usePaneLayout.ts` implementation (see A-004)
Already captured in A-004. Epic F tasks cover edge cases and the list column width.

**F-002** — Handle async sessions on restore
- In `usePaneReducer.ts`: use a `restoredRef = useRef(false)` to ensure `RESTORE_LAYOUT` is dispatched only once after sessions first become non-null
- While `sessions === null` (loading): `paneState` holds the initial single-leaf state; layout is not restored yet
- When sessions first load: run `loadPaneLayout()` + `validateAndRepair` + dispatch `RESTORE_LAYOUT`
- On subsequent sessions changes (session deleted externally): re-run `validateAndRepair` inline and dispatch `RESTORE_LAYOUT` again (idempotent)
- Est. 30 lines in `usePaneReducer.ts`
- Dependencies: A-004, A-005

**F-003** — Validate `PersistedPaneLayout` structure on load
- Guard against malformed JSON: `try/catch` around `JSON.parse`
- Guard against wrong `version` field: return null if `version !== 1`
- Guard against missing `root`, `focusedPaneId`: return null if any required field absent
- Guard against non-existent `focusedPaneId` in the restored tree: fall back to the first leaf's id
- Est. 25 lines in `usePaneLayout.ts`
- Dependencies: A-004

---

### Epic G: Mobile Adaptations

**G-001** — Conditional render for vertical splits on `<768px`
- In `PaneNode` (component from B-005): read `isMobile` from `ViewportProvider` via `useViewport()` hook
- When `isMobile && node.type === "split" && node.direction === "vertical"`: render only the child containing the `focusedPaneId` (not the handle, not the sibling)
- Helper: `containsPaneId(subtree: PaneTree, paneId: PaneId): boolean` — add to `paneUtils.ts`
- Est. 20 lines in `PaneSplitRenderer.tsx` + 10 lines in `paneUtils.ts`
- Dependencies: B-005, A-002

**G-002** — Mobile tab strip for stacked vertical panes
- In `PaneSplitRenderer.tsx` root level: when `isMobile` and the root tree contains any vertical splits, collect all leaves and render `<MobilePaneTabStrip>` below the pane area
- `MobilePaneTabStrip` dispatches `FOCUS_PANE` when a tab is tapped
- Est. 25 lines in `PaneSplitRenderer.tsx` + full B-007 implementation
- Dependencies: B-007, G-001

**G-003** — Touch hit targets on `ResizeHandle`
- Confirmed in research: the `marginLeft/Right: -7px; paddingLeft/Right: 7px` pattern already expands hit target to 20px
- The `touchAction: "none"` on the handle prevents scroll hijacking
- Verify: add `data-testid="resize-handle"` for any future tests
- Est. 0 lines new (already designed into C-002)
- Dependencies: C-002

---

### Epic H: Reset Layout + Feature Registry

**H-001** — "Reset layout" button in `SessionDetailBar`
- Locate `web-app/src/components/sessions/SessionDetailBar.tsx`
- Add a small "Reset layout" button (or icon button) in the bar — only visible when `paneCount > 1` (derived from `getAllLeaves(paneState.root).length > 1`)
- On click: `dispatchPane({ type: "RESET_LAYOUT" })`
- Thread `dispatchPane` + `paneCount` as props to `SessionDetailBar`
- Est. 25 lines changed
- Dependencies: A-001, A-005

**H-002** — Feature registry entry
- Create `docs/registry/features/frontend/cockpit-tiling.json`
- Schema-compliant entry with `id: "cockpit-tiling"`, `type: "frontend"`, list of component paths, `tested: false` initially
- Update once tests are written (A-006 covers reducer; add Jest/RTL tests for PaneSplitRenderer in B-005 as a follow-up)
- Est. 20 lines JSON
- Dependencies: none

---

## 4. Key Risks and Mitigations

### Risk 1: `Ctrl+W` Browser Tab Close Conflict

**Severity:** High

The `ShortcutRegistry` calls `event.preventDefault()` before the action (line 106 of `shortcutRegistry.ts`). This should block the browser tab close. However, when xterm.js has focus, xterm intercepts `keydown` events on its canvas before they bubble to `document`. The `ShortcutRegistry` listens at `document` level; xterm's canvas listener fires first.

**Mitigation:**
- Register `cockpit.close-pane` with `context: "cockpit"` rather than `"global"`. The terminal context guard in `shortcutRegistry.ts` dispatch must be updated (D-001) so `"cockpit"` shortcuts fire even when terminal is active.
- The pane header close button (✕) is the primary close path and requires no keyboard at all.
- Add a fallback `Ctrl+Shift+W` binding registered with the same action, in case `Ctrl+W` proves unreliable in Chrome+xterm.
- Manual test checklist: verify in Chrome/Firefox on Windows and macOS that `Ctrl+W` closes the pane, not the browser tab, when clicking inside a terminal pane.

### Risk 2: Duplicate Session in Two Panes (WebSocket Conflict)

**Severity:** High

Assigning the same `sessionId` to two leaves creates two `SessionDetail` instances, each opening its own terminal pool and WebSocket connection to the same session stream. The backend may not support two simultaneous readers; even if it does, resize signals from two xterm instances will conflict.

**Mitigation (MVP):**
- Defensive check in `SPLIT_PANE` and `ASSIGN_SESSION` reducers: call `getAllLeaves(state.root)` and return unchanged state if `sessionId` is already present in any leaf.
- `PaneLeaf` component: when `ASSIGN_SESSION` is rejected (state unchanged), show a transient warning toast or header message: "Session already open in another pane."
- This is enforced in both the reducer (pure check) and the UI (feedback).

### Risk 3: `ShortcutContext` Union Extension — Exhaustive Record

**Severity:** Medium

`getAll()` returns `Record<ShortcutContext, Shortcut[]>`. After adding `"cockpit"` to the union, TypeScript will emit a compile error if `cockpit: []` is not added to the result object initializer. This is the desired safety net — it will catch the omission at build time.

**Mitigation:**
- D-001 adds `cockpit: []` to the initializer as part of the same edit that expands the union type.
- The `dispatch()` terminal context guard must also be updated in the same commit (as noted in D-001) or the cockpit shortcuts will be silently swallowed when terminal has focus.
- Run `make lint` and `make build` immediately after D-001 to confirm no type errors.

### Risk 4: `Ctrl+-` Browser Zoom Conflict

**Severity:** Medium

`Ctrl+-` is browser zoom-out on Windows/Linux. `event.preventDefault()` in `shortcutRegistry.ts` dispatch reliably blocks it on Chrome and Firefox (Windows). On macOS, zoom is `Cmd+-`, so `Ctrl+-` is safe.

**Mitigation:**
- Register as `{ key: "-", modifiers: { ctrl: true } }` — confirm key string with `console.log(event.key)` in devtools on target platforms.
- If Safari macOS proves unreliable, provide `Ctrl+Shift+H` as a secondary binding for horizontal split.
- Add a comment in `usePaneShortcuts.ts` documenting the platform behavior and the fallback.

### Risk 5: xterm.js `fit()` on Drag Resize

**Severity:** Low (already handled)

Research confirms `XtermTerminal.tsx` already has a `ResizeObserver` that calls `fitAddon.fit()` on container size changes. When the tiling engine changes pane sizes via the CSS grid `--split-ratio` custom property, the flex/grid children change their rendered dimensions, which the existing `ResizeObserver` detects automatically. No additional `fit()` calls needed from the tiling engine.

The only risk is that hidden panes (via `visibility: hidden` in the pool) report zero dimensions. This is already guarded in the existing code. No new risk.

### Risk 6: `validateAndRepair` Timing (Flash of Wrong Content)

**Severity:** Low

Sessions are loaded asynchronously. On first render, `sessions` is null. The pane layout is restored only after sessions load. During the load window, the UI shows the initial empty pane state rather than the persisted layout. This is acceptable (no wrong session shown, just a blank pane briefly).

**Mitigation:**
- Show a skeleton/loading state in `PaneLeaf` when `sessions === null` (same as the existing `SessionListSkeleton` pattern).
- `RESTORE_LAYOUT` fires as soon as sessions first resolve — no user-visible delay on subsequent reloads after initial page hydration.

---

## 5. Integration Points

### `page.tsx` — Replace `selectedSession` Flow

**Before:**
```typescript
const [selectedSession, setSelectedSession] = useState<Session | null>(null);
const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
// ...
<SessionDetail session={detailSession} activeTab={activeTab} onTabChange={setActiveTab} />
```

**After:**
```typescript
const [paneState, dispatchPane] = usePaneReducer(sessions);
// ...
<PaneSplitRenderer state={paneState} dispatch={dispatchPane} sessions={sessions ?? []} />
```

`PaneSplitRenderer` internally renders `<SessionDetail>` per leaf. The `selectedSession` variable is kept only as a derived value for logic that still needs it (e.g., `sessionSelected` variant for mobile layout, `deleteConfirmTarget` checks).

The `cockpitGrid` div gains a style prop: `style={{ "--list-col-width": listColumnWidth + "px" } as React.CSSProperties}`. The grid template changes from hardcoded `"280px 1fr"` to `"var(--list-col-width, 280px) 6px 1fr"` to accommodate the list resize handle column.

### `SessionDetail` — `paneId` Key Prop

`SessionDetail` is rendered inside `PaneLeaf` as:

```tsx
<SessionDetail
  key={pane.id + "-" + (pane.sessionId ?? "empty")}
  session={sessionObj}
  // existing props...
/>
```

The `key` ensures each pane gets an independent React subtree with its own terminal pool. If `pane.id` changes (new pane created), the terminal pool is torn down and recreated. If `pane.sessionId` changes (different session assigned to same pane), pool is also recreated.

No changes are required inside `SessionDetail.tsx` itself for MVP — the `key` pattern at the call site is sufficient.

### `ShortcutRegistry` — "cockpit" Context

The dispatch loop in `shortcutRegistry.ts` has this terminal guard:

```typescript
if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global") continue;
```

This must be updated to:

```typescript
if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global" && shortcut.context !== "cockpit") continue;
```

Without this change, all `"cockpit"` shortcuts are swallowed when a terminal pane is focused — which is the primary use case for keyboard splits.

The pane renderer must set `data-context="cockpit"` on the outer pane area div so `getActiveContext()` returns `"cockpit"` when a pane (but not a terminal within it) has focus.

---

## Summary

**Epic count:** 8 epics (A–H)

**Task count:** 26 tasks (A-001 through H-002)

| Epic | Tasks | Est. Lines |
|------|-------|-----------|
| A: Data model + reducer | 6 | ~630 |
| B: Pane tree renderer | 7 | ~350 |
| C: Drag resize | 3 | ~155 |
| D: Keyboard shortcuts | 3 | ~115 |
| E: Session assignment routing | 4 | ~130 |
| F: localStorage persistence | 3 | ~85 |
| G: Mobile adaptations | 3 | ~55 |
| H: Reset layout + registry | 2 | ~45 |
| **Total** | **31** | **~1565** |

**Flagged technical risks (priority order):**

1. `Ctrl+W` xterm focus interception — cockpit context guard in `shortcutRegistry.ts` dispatch loop is the required fix; close button is the safe primary path
2. Duplicate session in two panes — reducer-level guard + UI warning; same-session multi-pane deferred to post-MVP
3. `ShortcutContext` union exhaustiveness — TypeScript compile error will catch missing `cockpit: []` in `getAll()`; fix in same commit as union extension
4. `Ctrl+-` zoom conflict — reliable on Chrome/Firefox Windows; Safari macOS fallback binding `Ctrl+Shift+H` if needed
