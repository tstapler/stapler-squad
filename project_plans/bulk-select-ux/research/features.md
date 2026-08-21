# Feature Research: Bulk Select UX

**Date**: 2026-06-23
**Feature**: bulk-select-ux

---

## 1. Existing Codebase Patterns: How SessionCard Implements Selection

### Props interface (SessionCard.tsx lines 104–106)
```ts
selectMode?: boolean;
isSelected?: boolean;
onToggleSelect?: () => void;
```

### Rendering
- The checkbox renders **only when `selectMode === true`** — it is not hover-revealed or always-visible:
  ```tsx
  {selectMode && (
    <div className={checkbox} aria-hidden="true" onClick={handleCheckboxClick}>
      <input type="checkbox" checked={isSelected} tabIndex={-1} ... />
    </div>
  )}
  ```
- `checkbox` CSS class (SessionCard.css.ts line 84):
  - `position: absolute`, `left: vars.space["4"]`, `top: 50%`, `transform: translateY(-50%)`
  - Input: `width: 20px; height: 20px; cursor: pointer`
- When `selectMode` is true, `cardSelectMode` adds `paddingLeft: "52px"` to the card to leave room for the absolute-positioned checkbox.
- When `isSelected` is true, `cardSelected` applies `borderColor: vars.color.primary` and `background: vars.color.accentBg`.
- Card click in select mode: calls `onToggleSelect()` and stops propagation.
- Keyboard: `Enter`/`Space` on the card calls `onToggleSelect()` when in select mode.
- `aria-label` in select mode: `"Selected: title"` or `"Not selected: title"`.
- The edit-tags button gets `tabIndex={-1}` and `inert` when in select mode (correctly removes from tab flow).

### How card view wires it in SessionList.tsx (lines 1144–1146)
```tsx
selectMode={selectMode}
isSelected={selectedSessions.has(session.id)}
onToggleSelect={() => handleToggleSession(session.id)}
```

### How row view wires it (SessionList.tsx lines 979–999)
**SessionRow receives NO `selectMode`, `isSelected`, or `onToggleSelect` props.** These props do not exist in `SessionRowProps` (SessionRow.tsx lines 33–56). The row view's `<SessionRow>` call at line 979–999 passes none of these. The gap is total — row mode has zero selection support.

### State management
- `selectedSessions: Set<string>` lives in `SessionList` component state (line 217).
- `selectMode: boolean` also in `SessionList` state (line 216).
- `handleToggleSession` (line 490): calls `setSelectMode(true)` (auto-enters select mode on first toggle), then toggles the id in the Set.
- `handleSelectAll` (line 503): sets all `filteredSessions` IDs.
- There is also a `useSelection` hook at `web-app/src/lib/hooks/useSelection.ts` and a `bulkSelectionSlice` in Redux, but **neither is used by SessionList** — SessionList uses its own local state.
- BulkActions shows "Select All" only when `selectedCount < totalCount`; shows "Clear Selection" always.

### BulkActions bar (BulkActions.tsx)
- When `selectedCount === 0`: shows "Click sessions to select them" hint + Cancel button.
- When `selectedCount > 0`: shows count, Select All (if not all selected), Clear Selection, Pause/Resume/Add Tag/Group As/Delete buttons.
- No undo button — delete goes through a confirmation modal (`showBulkDeleteConfirm` state in SessionList).
- `RestoreSession` RPC does **not exist** in the proto — undo for bulk delete would require soft-delete infrastructure that is not currently present.

---

## 2. Edge Cases to Handle

### Stale selection (sessions deleted while selected)
**Current behavior**: `selectedSessions` is a `Set<string>`. When `onDeleteSession` is called for individual deletions, the app refetches session list from the server and `sessions` prop updates. `selectedSessions` is never auto-pruned — stale IDs silently remain in the set.
- **Risk**: BulkActions shows "3 selected" but one session was deleted between selection and action. The bulk delete handler uses `Promise.allSettled` and keeps failed IDs selected — this will catch server-side "not found" errors but won't proactively prune.
- **Recommendation**: derive effective selected count as `intersection(selectedSessions, new Set(filteredSessions.map(s => s.id)))` for BulkActions display. Purge stale IDs on next action or on sessions prop change.

### Filter changes removing selected sessions from view
**Current behavior**: `selectedSessions` is not cleared when filters change. `handleSelectAll` selects `filteredSessions` at call time; if the filter changes later, the selection set still contains IDs of sessions no longer visible.
- `BulkActions.totalCount` is `filteredSessions.length`, so the count display becomes inconsistent ("5 of 3 selected" is possible).
- **Recommendation**: on filter/sort/grouping change, either (a) clear selection with a toast, or (b) auto-prune to `selectedSessions ∩ filteredSessions`. Option (b) is less disruptive.

### Shift+click with no prior anchor
- No anchor/lastClicked state exists anywhere in the codebase today.
- **Recommendation**: treat the first click in a shift+click sequence as a normal toggle and set the anchor. On the second shift+click, select the range between anchor and current item using `flatItems` (row mode) or `groupedSessions`-flattened array (card mode).
- If anchor is undefined when shift is held: treat as a normal click and set anchor.

### Shift+click across group boundaries in flatItems
- `flatItems` (line 445) interleaves `{ kind: "header" }` and `{ kind: "session" }` items. A range select must filter to only `kind === "session"` items when computing the range.
- The anchor index in `flatItems` must be tracked by `flatItems` position, not by session list position, so that cross-group ranges work correctly (e.g., selecting from last item in group A to second item in group B).
- Group header items must be skipped during range iteration.

### Cmd+A when 0 sessions visible
- `handleSelectAll` does `new Set(filteredSessions.map(s => s.id))`. If `filteredSessions.length === 0`, this produces an empty Set — safe, no issue.
- However, Cmd+A should be a no-op or show feedback "No sessions to select" when count is 0.

### Sorting/grouping changes mid-selection
- `flatItems` is recomputed from `groupedSessions` on every sort/group change (via `useMemo`). The shift+click anchor stored as a `flatItems` index would become stale.
- **Recommendation**: store the anchor as a **session ID**, not a list index. When needed for range select, recompute the index of the anchor ID in the current `flatItems`.

---

## 3. Comparable UX Patterns

### Gmail
- **Checkbox visibility**: Always visible in the sender column (no hover reveal, no special mode needed). Left-click the sender avatar to toggle selection.
- **Shift+click**: Standard range select from last-clicked to current. First selection sets anchor; second shift+click extends range.
- **Cmd+A**: Not available; instead, a "Select All" button appears in the toolbar that first selects the current page, then offers "Select all N conversations."
- **Undo on destructive actions**: 5-second undo toast at the bottom of the screen after delete/archive. The toast stacks; each action shows its own undo. Implemented via server-side soft-delete — messages go to Trash and are fully recoverable.
- **Deselect all**: "Deselect all" appears in the top checkbox dropdown when any are selected.

### Linear (issue tracker)
- **Checkbox visibility**: Hover-reveal only — checkboxes appear on row hover. No special "select mode" required; selecting any item implicitly enters multi-select.
- **Shift+click**: Supported for range select. Anchor is set on first selection.
- **No select mode toggle**: Linear has no explicit "Select" button — hover + click is the entry point. The selection toolbar appears at the bottom when ≥1 item is selected.
- **Undo**: No undo for bulk delete (delete is permanent in Linear for issues moved to "deleted" state). Bulk archive is undoable.
- **Deselect all**: Clicking the selection count in the bottom toolbar clears selection.

### GitHub Issues
- **Checkbox visibility**: Always visible on the left side of each row. No hover required.
- **Shift+click**: Supported.
- **Select mode**: No explicit mode — checkboxes are always present.
- **Undo**: No undo for bulk close/open. No soft-delete for issues.
- **Deselect all**: Checkbox in the header row clears all.
- **Indeterminate state**: Header checkbox shows indeterminate state when some (not all) rows are selected.

### Notion (database table view)
- **Checkbox visibility**: Hover-reveal on the leftmost cell of each row.
- **Shift+click**: Supported for range select.
- **Undo**: Cmd+Z undoes the last bulk action. Notion has a full undo/redo stack backed by operational transforms.
- **Deselect all**: Click outside the selection or press Escape.

### Airtable
- **Checkbox visibility**: Always visible (dedicated checkbox column).
- **Select all in group**: Clicking the group header checkbox selects all rows in that group. This is a strong user expectation for grouped list views.
- **Undo**: Cmd+Z undoes most actions including bulk.

### Summary: Dominant UX patterns
| Concern | Dominant Pattern |
|---|---|
| Checkbox visibility | Hover-reveal (Linear, Notion) or always-visible (GitHub, Airtable, Gmail) |
| Enter select mode | No special mode needed in most apps — hover+click is sufficient |
| Shift+click | Universal expectation; anchor = last single-click |
| Cmd/Ctrl+A | Select all visible (not all pages); noop if 0 visible |
| Undo for destructive bulk | Toast with 5s window (Gmail); requires soft-delete infrastructure |
| Header checkbox | Selects all in group (Airtable) or selects all visible (GitHub) |
| Escape key | Deselects all / exits select mode |

---

## 4. Unstated User Needs

Beyond the explicit requirements, users of bulk-select in a grouped/filtered list typically want:

### "Select all in this group" per group header
- Airtable pattern: clicking a group header checkbox selects all sessions in that group only.
- In stapler-squad's row mode, group headers (`kind: "header"` items) could show a checkbox that selects `item.groupSessions`.
- This is especially useful in Project grouping mode where a user wants to pause all sessions in one project.

### Indeterminate header checkbox state
- When some (not all) sessions in a group are selected, the group header checkbox should show the indeterminate (`-`) state.
- Standard `<input type="checkbox" ref={(el) => { if (el) el.indeterminate = partiallySelected; }}` pattern.

### Visual count of selected vs. total per group
- "3/5 selected" in the group header row when some sessions in that group are selected.
- Makes it easier to audit what's being operated on without scrolling through the whole list.

### Selection persistence across filter/sort changes (or explicit clear)
- Users expect their selection to survive a re-sort. Clearing selection silently on sort change is frustrating.
- Best approach: keep selection, prune IDs no longer in `filteredSessions`, show toast "X sessions no longer visible were deselected" if any were pruned.

### Keyboard-driven bulk select without a mouse
- Tab to a row, Space to toggle selection, Shift+Down/Up to extend selection range (native list keyboard pattern).
- Escape to exit select mode and clear selection.
- Not in the current requirements but essential for accessibility.

### Right-click context menu entry point for select
- Right-click on a row should offer "Select" as an option (Linear pattern), entering select mode for that item without having to click a "Select" button in the header.
- SessionRow already has `handleContextMenu` that opens `SessionActionsOverflow` — a "Select" option could be added there.

### "Select by status" quick action
- "Select all Paused" or "Select all Needs Approval" from the BulkActions toolbar — avoids manual filtering + select-all + operate.
- Could be a dropdown in BulkActions: "Select by status: Active / Paused / Needs Approval".

### Undo requires soft-delete infrastructure
- No `RestoreSession` RPC exists in the proto. Implementing undo for bulk delete requires either:
  1. A new `RestoreSession` RPC that un-deletes a session (requires backend changes to keep deleted sessions recoverable for N seconds).
  2. Client-side optimistic approach: delay the actual RPC by 5 seconds, show undo toast, cancel the RPC if undo is clicked. Risk: session state may have changed if server processing is immediate.
  3. Defer undo entirely — show a non-undoable confirmation modal (current behavior) with a clear warning.
- The requirements spec notes this as a rabbit hole. Approach 2 (client-side delay) is the pragmatic path without backend changes.

---

## Key Files

| File | Role |
|---|---|
| `web-app/src/components/sessions/SessionCard.tsx` | Card with select props wired |
| `web-app/src/components/sessions/SessionCard.css.ts` | `checkbox`, `cardSelectMode`, `cardSelected` styles |
| `web-app/src/components/sessions/SessionRow.tsx` | Row — **no select props** (gap to fill) |
| `web-app/src/components/sessions/SessionRow.css.ts` | Row CSS — needs checkbox column |
| `web-app/src/components/sessions/SessionList.tsx` | Orchestrator — `selectedSessions` state, `flatItems`, virtualizer |
| `web-app/src/components/sessions/BulkActions.tsx` | Toolbar — needs undo slot |
| `web-app/src/lib/hooks/useSelection.ts` | Existing hook — not used by SessionList (potential DRY refactor) |
| `web-app/src/lib/store/bulkSelectionSlice.ts` | Redux slice — not used by SessionList |
| `proto/session/v1/session.proto` | No `RestoreSession` RPC — undo requires new infra |
