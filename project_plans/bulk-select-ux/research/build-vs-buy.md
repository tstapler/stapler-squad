# Bulk Select — Build vs. Buy Research

_Date: 2026-06-23_

---

## Existing Codebase Foundation

Before evaluating libraries, the relevant existing pieces are:

| File | What it does |
|---|---|
| `web-app/src/lib/store/bulkSelectionSlice.ts` | Redux slice: `selectedIds: string[]`, reducers `toggleSelection`, `selectAll`, `clearSelection`. No shift-range, no lastAnchor. |
| `web-app/src/components/sessions/BulkActions.tsx` | Toolbar: select-all, clear, pause, resume, add-tag, delete, group-as. Fully wired. |
| `web-app/src/components/sessions/SessionList.tsx` | Uses `@tanstack/react-virtual` (`useVirtualizer`). Supports `viewMode: "card" | "row"`. Orchestrates both `SessionCard` and `SessionRow`. |
| `web-app/src/components/ui/NotificationToast.tsx` | Custom bespoke toast: CSS class-based entrance/exit animation, auto-close timer, auto-minimize, approve/deny callbacks. No external toast lib. |

Key missing pieces (not implemented anywhere):
- Shift+click range select (no `lastAnchorId` in the slice)
- Cmd+A global shortcut
- Escape to exit select mode
- Checkbox rendering inside virtual rows
- Undo toast for destructive bulk ops

---

## 1. OSS Libraries for Multi-Select in React

### @tanstack/react-table (TanStack Table v8)

**What it offers**: `useReactTable` with a `rowSelection` model, built-in shift-click range select, Cmd+A (`toggleAllRowsSelected`), and first-class `@tanstack/react-virtual` integration (both are from the same author/org, designed to compose).

**Fit assessment**:
- `SessionList` already uses `@tanstack/react-virtual` directly, _not_ `react-table`. Adopting `react-table` would require migrating the existing column/sort/filter logic that is currently bespoke (search, status filter, tag filter, grouping strategies) into the TanStack Table column model. That is a significant rewrite.
- The existing `SessionRow` and `SessionCard` are hand-crafted with complex sub-components (inline editing, review badges, approval suppression, tag editor). Mapping these into table cell renderers is non-trivial.
- The row selection model from `react-table` is genuinely good: it handles `rowSelection` state, shift-range logic, and Cmd+A cleanly. The `selectAll` action in the existing Redux slice already does what `react-table` would manage internally.

**Verdict**: The shift-click range logic in `react-table` is ~50 lines of internal code (`rowsById`, anchor tracking, index-range fill). It is not complex enough to justify migrating the entire list to `react-table`. **Do not adopt. Extract the range-select pattern instead.**

**License**: MIT. Maintained actively (v8.21+ as of 2025). Bundle: ~14 KB gzipped.

---

### react-aria (Adobe)

**What it offers**: `useListBox`, `useGridList`, `useCheckbox`, `useTable` — all with full WAI-ARIA keyboard semantics. `useGridList` specifically implements: Space to select, Shift+Arrow for range, Ctrl/Cmd+A for all, Escape to exit selection mode.

**Fit assessment**:
- `react-aria` is "bring your own styles" and works with vanilla-extract. Composable, not an all-or-nothing replacement.
- The `useCheckboxGroupItem` + `useGridList` combination would give correct keyboard semantics (arrow-key navigation inside the virtual list) with no custom implementation.
- However, wiring `react-aria` `useGridList` into an existing `useVirtualizer` list requires passing `react-aria`'s `ref` callbacks to the virtualizer's `measureElement`, which is documented but non-trivial. The two libraries have different assumptions about DOM measurement.
- The real value of `react-aria` here is keyboard accessibility (arrow-key navigation + roving tabindex). For simple checkbox-based bulk select (no arrow-key roving needed), it is overengineering.

**Verdict**: Recommended _only_ if full WAI-ARIA grid keyboard nav (arrow keys, roving tabindex) is a requirement. For the stated scope (checkbox + Shift+click + Cmd+A + Escape), the complexity cost is not worth it. **Do not adopt for MVP. Revisit if a11y audit requires arrow-key navigation.**

**License**: Apache-2.0. Maintained by Adobe (stable, used in Adobe Spectrum).

---

### downshift

**What it offers**: Combobox/dropdown primitives. No list multi-select model.

**Verdict**: Not relevant to this use case.

---

### Headless UI (Tailwind Labs)

**What it offers**: Listbox (single-select), Dialog, Disclosure, Menu, Transition. No multi-select list or row-selection primitives.

**Verdict**: Not relevant.

---

## 2. SaaS / Managed Solution

There is no hosted service that manages bulk session operations or provides a "table component as a service." All commercial table products (AG Grid, Handsontable, Bryntum) are client-side libraries with licensing costs, not SaaS. AG Grid Community Edition is MIT and does have virtualized row selection with shift-click — but adoption cost is identical to `react-table` and bundle is 2× larger.

**Verdict**: No viable SaaS option exists. Not applicable.

---

## 3. LLM-Generated vs. Battle-Tested Library — Algorithm Analysis

### Shift+click range select

**Complexity**: Low. The algorithm is:
1. Store `lastAnchorId` in the Redux slice alongside `selectedIds`.
2. On a Shift+click: find the indices of `lastAnchorId` and the clicked item in the _current ordered flat list_, fill the range, merge with existing selection.
3. On a plain click: set `lastAnchorId = clickedId`, toggle.

This is approximately 30–40 lines of reducer logic. It is deterministic and has no edge cases that benefit from a library (no async, no DOM measurement, no focus management). **Build bespoke. Add `lastAnchorId: string | null` to `bulkSelectionSlice`.**

### Keyboard grid navigation (Arrow keys + Space)

**Complexity**: Medium–High. WAI-ARIA grid pattern requires roving tabindex, arrow key interception at the container level, and coordinating with the virtualizer's scroll-into-view. This is non-trivial to implement correctly and tends to be brittle with virtual lists (items not in the DOM cannot receive focus).

However, the feature requirement as stated is: checkbox click + Shift+click + Cmd+A + Escape. Arrow-key navigation is **not listed as a requirement**. Skipping it is the correct MVP decision. If required later, `react-aria`'s `useGridList` is the right tool.

**Recommendation**: Skip arrow-key navigation for MVP. Implement Space-to-toggle only (when a row already has focus from checkbox tab-stop).

### Cmd+A (Select All)

Already implemented as a Redux action (`selectAll`). Wire a `useEffect` / `keydown` listener on the list container for `(e.metaKey || e.ctrlKey) && e.key === 'a'` → dispatch `selectAll(allVisibleIds)`. **Build bespoke. ~5 lines.**

### Escape to Exit Select Mode

Wire a `keydown` listener for `e.key === 'Escape'` → dispatch `clearSelection()` + toggle off select mode. **Build bespoke. ~3 lines.**

### Undo Toast

**Current state**: The project has a fully custom `NotificationToast` system (`/components/ui/NotificationToast.tsx` + `NotificationContext`). It uses CSS class-based animation, auto-close timers, and an audit log hook. There is **no external toast library** (`sonner`, `react-hot-toast`, `@radix-ui/react-toast` are all absent from `package.json`).

**Options**:
1. **Extend the existing `NotificationContext`** with an "undo" toast variant. The existing system already supports `onDismiss`/`onAcknowledge` callbacks and time-based auto-close. A new `notificationType: "undo"` with an `onUndo` callback would fit naturally. Zero new dependencies. This is the correct approach given the existing infrastructure.
2. **Add `sonner`**: Sonner is 4.6 KB gzipped, has a built-in undo/promise pattern, and zero runtime dependencies. If the existing notification system is considered too complex for a simple undo toast, sonner is the right addition. However, adding a second toast system alongside the existing one creates inconsistency.
3. **Add `@radix-ui/react-toast`**: Already partly in the dependency tree (`@radix-ui/react-dialog`, `@radix-ui/react-tabs`, `@radix-ui/react-tooltip` are all present). Using the Radix toast primitive would be consistent. However it's lower-level than sonner and requires more wiring.

**Verdict**: Extend the existing `NotificationContext` with an `undo` toast variant. This avoids a new dependency, stays consistent with the existing toast UI, and reuses the auto-close timer infrastructure. If the undo interaction proves to need richer promise-chaining patterns, add `sonner` at that point.

---

## 4. Fork or Adapt — Existing Implementation Assessment

The existing implementation is a **solid foundation to extend**, not a candidate for rewrite. Key evidence:

**Already done (do not rebuild)**:
- `BulkActions` toolbar is complete: pause, resume, delete, add-tag, group-as, select-all, clear-selection. All ARIA-labeled.
- Redux slice has `selectedIds`, `toggleSelection`, `selectAll`, `clearSelection`. The shape is correct — only needs `lastAnchorId` added.
- `SessionList` already imports and renders `BulkActions`. Select-mode toggle already exists in the component (the `selectModeButton` CSS class is imported).
- Both `SessionCard` and `SessionRow` are the rendering targets — checkboxes need to be added to each.

**Missing pieces (the actual work)**:
1. Add `lastAnchorId: string | null` and a `setRangeSelection(anchor, target, allIds)` reducer to `bulkSelectionSlice`.
2. Add a `selected` prop + checkbox to `SessionCard` and `SessionRow`. In `SessionCard`, this is partially present (the `SessionCard.click.test.tsx` suggests click handling exists).
3. Wire Cmd+A and Escape keyboard shortcuts to the list container in `SessionList`.
4. Add an undo toast (extend `NotificationContext`) for destructive bulk operations (delete).
5. Verify select-mode rendering in both `card` and `row` `viewMode` branches of `SessionList`.

The existing approach (Redux for selection state, BulkActions as a separate toolbar, SessionList as orchestrator) is architecturally sound. Do not change the architecture.

---

## Summary Decision Matrix

| Sub-feature | Decision | Rationale |
|---|---|---|
| @tanstack/react-table | **Do not adopt** | Migration cost > benefit; range-select is ~40 lines bespoke |
| react-aria | **Skip for MVP** | Overengineered for checkbox-only select; revisit for a11y arrow-key nav |
| downshift / Headless UI | **Not applicable** | Wrong domain |
| Shift+click range select | **Build bespoke** | Simple reducer; add `lastAnchorId` to existing slice |
| Cmd+A | **Build bespoke** | 5 lines; calls existing `selectAll` action |
| Escape | **Build bespoke** | 3 lines; calls existing `clearSelection` action |
| Arrow-key navigation | **Defer** | Not in requirements; add `react-aria` later if needed |
| Undo toast | **Extend existing NotificationContext** | No new dependency; existing system already supports callbacks/timers |
| Overall architecture | **Extend, do not rewrite** | Existing slice + toolbar + list orchestration is correct; add missing pieces only |
