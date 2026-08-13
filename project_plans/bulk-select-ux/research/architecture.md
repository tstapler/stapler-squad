# Architecture Research: bulk-select-ux

**Date**: 2026-06-23

---

## 1. State Management for Range Select

### Current state shape

`SessionList.tsx` currently tracks:
```ts
const [selectMode, setSelectMode] = useState(false);
const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
```

### What needs to be added for Shift+click range select

An **anchor** state is required — the index of the last item clicked without Shift held. On a Shift+click, all items between the anchor index and the clicked index become selected (or deselected, matching the anchor's final state).

**Tradeoff: session ID vs. flat index as anchor**

| | Session ID anchor | Flat index anchor |
|---|---|---|
| Survives re-render | Yes (IDs are stable) | No — `flatItems` is recomputed on filter/group change |
| Survives filter change | Yes (can remap via `flatItems.findIndex`) | No |
| Works with virtualization | Yes (ID is always meaningful) | Fragile: index shifts when headers are inserted |
| Implementation complexity | Slightly more (must findIndex on Shift+click) | Simple array slice |

**Recommendation: store anchor as session ID.** `flatItems` contains both `header` and `session` kinds; storing an index means the anchor silently shifts whenever grouping or filtering changes. With session ID, the anchor can be re-resolved via `flatItems.findIndex(i => i.kind === "session" && i.session.id === anchorId)` at Shift+click time, giving the correct range even after the list reorders.

```ts
const [anchorSessionId, setAnchorSessionId] = useState<string | null>(null);
```

On a plain click (no Shift): select that session, set `anchorSessionId` to its ID.
On a Shift+click: resolve anchor index + clicked index in `flatItems`, collect all session IDs in that range, toggle their selection.

---

## 2. Virtualization Compatibility

### How the row virtualizer is configured

```ts
const rowVirtualizer = useVirtualizer({
  count: viewMode === "row" ? flatItems.length : 0,
  getScrollElement: () => containerRef.current,
  estimateSize: (i) => (flatItems[i]?.kind === "header" ? 40 : 50),
  overscan: 8,
  measureElement: (el) => el.getBoundingClientRect().height,
});
```

Each virtual item wrapper has `ref={rowVirtualizer.measureElement}` and `data-index={virtualItem.index}`, so TanStack Virtual v3 auto-measures row heights via `ResizeObserver`.

### Adding a checkbox column

`buildRowGridTemplate` in `session-columns.ts` prepends the fixed columns `["8px", "1fr"]` (dot + name) before optional columns. Adding a checkbox column means inserting a new fixed width (e.g., `"24px"`) at position 0:

```ts
const cols = ["24px", "8px", "1fr"]; // checkbox + dot + name
```

This is a **pure CSS change** — vanilla-extract generates build-time class names; the grid template is set via an inline style (`gridTemplateColumns` on the `row` div), so changing the template only requires updating `buildRowGridTemplate` and the `SessionRow.css.ts` fallback default. No `measureElement` call is needed: the checkbox column adds fixed width, not variable height, so existing `ResizeObserver`-based height measurement is unaffected.

**Key constraint**: the checkbox must be `position: relative` or `position: static` inside the grid; `position: absolute` or `position: fixed` would escape the grid flow and break height measurement. The current `row` style already uses `display: grid` with `position: relative` set on the row itself — safe.

### Hover-reveal in virtualized context

The current `row` style has `:hover { background: vars.color.hoverBackground }`. A hover-reveal checkbox requires showing the checkbox only on hover. In vanilla-extract this is done with a selector on the checkbox element:

```ts
export const checkbox = style({
  opacity: 0,
  selectors: {
    [`${row}:hover &`]: { opacity: 1 },
    [`${row}[data-selected="true"] &`]: { opacity: 1 }, // always show when selected
  },
});
```

This is pure CSS and works with virtualization — no JS hover tracking needed.

---

## 3. Keyboard Event Integration

### Current G-key pattern (history page reference)

`web-app/src/app/history/page.tsx` wires global keyboard shortcuts via a `useEffect` with `window.addEventListener("keydown", handler)`. `SessionList.tsx` exposes `handleCycleGrouping` as an internal function but does not register a `document` listener itself — the G-key shortcut text in `SessionList.tsx` (line 470–473) is defined but the actual keyboard wiring appears to be done through the select element's `title` attribute as a hint, with no programmatic listener inside `SessionList`.

### Recommended approach for Cmd+A and Escape

Use a `useEffect` inside `SessionList` that registers on `document` (or `window`), with the handler guarded by `selectMode`:

```ts
useEffect(() => {
  if (!selectMode) return;
  const handler = (e: KeyboardEvent) => {
    const inInput = (e.target as HTMLElement).closest("input, textarea, [contenteditable]");
    if (inInput) return;
    if ((e.metaKey || e.ctrlKey) && e.key === "a") {
      e.preventDefault();
      handleSelectAll();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      handleClearSelection();
    }
  };
  document.addEventListener("keydown", handler);
  return () => document.removeEventListener("keydown", handler);
}, [selectMode, handleSelectAll, handleClearSelection]);
```

**Why `document` rather than container `onKeyDown`**: the virtualized container `div[role="list"]` is not focusable by default; relying on its `onKeyDown` would require `tabIndex` and focus management, which has accessibility side effects. A `document`-level listener with `!inInput` guard is the established pattern in this codebase (`Omnibar.tsx` uses `document.addEventListener("keydown", ...)` at line 838).

**Focus management**: Cmd+A should not steal focus. Escape should return focus to the "Select" toggle button (the existing `handleClearSelection` already calls `setTimeout(() => selectButtonRef.current?.focus(), 0)`).

---

## 4. Undo Pattern Options

### Option A: Client-side "pending delete"

Hold deletions locally for 5 seconds. The session appears removed from the list immediately (optimistic removal from local state), but the `DeleteSession` RPC is not called until the timer fires. A cancel button stops the timer and re-adds the session to visible state.

**Failure modes**:
- Page refresh/close during the 5s window loses the pending state — items are never deleted AND the user doesn't know this.
- Race: if the WatchSessions stream sends an update for a "pending delete" session during the window, the item may reappear.
- Multiple concurrent pending deletes complicate state management.
- No server-side soft-delete infrastructure exists.

### Option B: Fire immediately, "Undo" toast calls RestoreSession RPC

Delete fires immediately. A toast shows "N sessions deleted — Undo". Clicking Undo calls a `RestoreSession` RPC.

**Checking the proto**: `session.proto` has no `RestoreSession` RPC. The only session deletion method is `DeleteSession` (line 25). The `WatchSessions` stream uses `SessionEvent` for create/update/delete events. A restore would require either: (a) adding a new `RestoreSession` RPC + backend soft-delete infrastructure, or (b) treating undo as "re-create" from stored metadata.

**Failure modes**:
- Without a soft-delete store in the backend, Option B is infeasible without new proto RPCs.
- If the `DeleteSession` implementation destroys tmux sessions and git worktrees immediately, sessions cannot be restored.

### Verdict

**Option A (pending delete) is safer for UX** given the current architecture. The 5-second delay is a well-established pattern (Gmail) and avoids the need for a new RPC. The main risk (page close losing pending state) can be mitigated with `beforeunload` to fire pending deletes immediately. The existing `showBulkDeleteConfirm` confirmation modal (`handleDeleteSelected → setShowBulkDeleteConfirm(true)`) already provides a guard; the undo toast replaces or supplements this modal.

However, the simplest implementation that matches the current codebase architecture is to **keep the confirmation modal** (already implemented in `handleConfirmBulkDelete`) and add a simple success toast with no undo — the modal already covers the "accidental delete" case. A pending-delete undo should only be added if the confirmation modal is removed.

---

## 5. Integration Points: SessionRow Props

### Current props passed to `SessionRow` in row mode (lines 979–1000)

```tsx
<SessionRow
  session={item.session}
  onClick={() => onSessionClick?.(item.session)}
  onPause={...}
  onResume={...}
  onDelete={...}
  onClone={...}
  onOpenInNewPane={...}
  onNewWorkspace={...}
  onRestart={onRestartSession}
  onCreateCheckpoint={onCreateCheckpoint}
  onRunOneShot={onRunOneShot}
  onSetRateLimitEnabled={onSetRateLimitEnabled}
  onToggleAutonomousMode={onToggleAutonomousMode}
  onSteerAutonomousSession={onSteerAutonomousSession}
  onClearConversationState={onClearConversationState}
  onHibernate={...}
  onResumeFromHibernation={...}
  onUpdateTags={onUpdateTags}
  suppressApprovalSubStatus={clearedSessions.has(item.session.id)}
  visibleColumns={visibleColumns}
/>
```

**Missing from `SessionRow` interface** (currently only in `SessionCard`):
- `selectMode?: boolean`
- `isSelected?: boolean`
- `onToggleSelect?: () => void`

These three props are already defined in `SessionCard` (confirmed at lines 1144–1146 for card mode) but absent from `SessionRowProps` (lines 33–56 of `SessionRow.tsx`).

### New props to add to `SessionRowProps`

```ts
interface SessionRowProps {
  // ... existing ...
  selectMode?: boolean;
  isSelected?: boolean;
  /** Called when the checkbox is toggled; also handles Shift+click via the parent. */
  onToggleSelect?: (e: React.MouseEvent) => void;
}
```

Note: `onToggleSelect` should receive the `MouseEvent` so the caller (`SessionList`) can inspect `e.shiftKey` for range-select logic.

### Thread-through in SessionList row mode

At the `SessionRow` call site, add:
```tsx
selectMode={selectMode}
isSelected={selectedSessions.has(item.session.id)}
onToggleSelect={(e) => handleToggleSession(item.session.id, e)}
```

`handleToggleSession` needs to be updated to accept `(sessionId: string, e?: React.MouseEvent)` and implement the anchor+range logic when `e.shiftKey` is true.

---

## Summary

1. **Anchor as session ID** is the correct choice for range select — flat indices are unstable across filter/group changes, which `flatItems` is subject to. Resolve the anchor to an index only at Shift+click time via `findIndex`.

2. **No `measureElement` intervention needed** for adding a checkbox column — it's a fixed-width CSS grid column change, and TanStack Virtual v3's `ResizeObserver`-based height measurement is unaffected by column count. Hover-reveal is pure CSS via vanilla-extract `selectors`.

3. **`RestoreSession` RPC does not exist** — Option B undo is not feasible without new backend work. The existing confirmation modal already prevents accidental deletes; keep it and add a post-delete success toast rather than a pre-delete pending-delete timer for the initial implementation. If the confirmation modal is removed in the future, a pending-delete with `beforeunload` flush is the right undo mechanism.
