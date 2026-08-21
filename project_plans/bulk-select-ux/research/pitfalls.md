# Pitfalls: bulk-select-ux

**Date**: 2026-06-23
**Feature**: Bulk session selection — row view, hover checkboxes, Shift+click, Cmd+A, Escape, undo toast

---

## 1. Virtualized list + checkbox pitfalls

### 1a. Adding a checkbox column triggers a remeasure cascade

`@tanstack/react-virtual` with `measureElement` caches each row's height after the first render. When a checkbox column is added (even a hidden one), every row's DOM width changes, which can invalidate cached measurements and trigger a full re-measure pass on the next scroll. **The symptom**: momentary blank strips in the list as items are re-measured on scroll.

**Mitigation**: Reserve the checkbox column width unconditionally (e.g. via a fixed-width `data-column="select"` slot) whether `selectMode` is active or not. Use CSS `visibility: hidden` + `pointer-events: none` when not in select mode rather than conditional rendering. This keeps row height stable across mode transitions.

### 1b. Selection state after scroll-out-of-view

`@tanstack/react-virtual` unmounts rows that leave the overscan window. When they remount on scroll-back, `isSelected` is re-evaluated from the `Set<string>` in `useSelection`. Because `selected` is stored in React state (not in the DOM), the selection is **correctly** restored — this is not a bug if `isSelected` is derived from the Set each render. The risk is if any component caches its own `checked` prop via `React.memo` with a stale equality check. Pass `isSelected(session.id)` as a derived boolean, not the whole `selected` Set, to avoid unnecessary re-renders while keeping correctness.

### 1c. Item count changes mid-selection (sort/filter/re-fetch)

`useSelection` in `/web-app/src/lib/hooks/useSelection.ts` stores selected IDs as a `Set<string>`. When the list is re-sorted or re-filtered, IDs that were selected but are no longer in the visible `items` array remain in the Set — this is correct behavior (selections survive filter changes). However, `selectAll` uses `items.map(i => i.id)`, so it only selects the currently visible items. **The asymmetry**: deselecting during a filter narrows the selection; then clearing the filter re-shows sessions that were never selected. This is expected but can surprise users. Add a visible count like "3 of 5 selected (2 hidden by filter)" if the selected Set contains IDs not in the current `items`.

The more dangerous case: after a **live data re-fetch** (polling or SSE push), a session ID can disappear entirely. The Set retains the stale ID. Before firing bulk delete, filter `selected` against the current `items` ID set.

---

## 2. Range select (Shift+click) pitfalls

### 2a. Anchor session filtered out

If the user selects session A (anchor), then applies a filter that hides session A, then Shift+clicks session B, the anchor index is stale. **The correct behavior**: treat the anchor as "none" when it is not present in `flatItems`. Reset `lastAnchorIndex` to -1 (or to the current click target) when the filter changes, or when the anchor ID is no longer found in `flatItems` at range-select time. Do not use `lastAnchorIndex` as an integer index into a filtered array — store the anchor **session ID** and resolve its index at click time.

### 2b. Group headers in `flatItems`

The current `groupSessions` utility produces a flat list that interleaves group header entries with session entries. A Shift+click range select that naively marks all indices between anchor and target will select group headers. **Fix**: the range-select loop must skip items where `item.type === "header"` (or equivalent discriminant). Only session-type items should be added to the selection Set.

### 2c. Stale closure on `lastAnchorIndex`

If `lastAnchorIndex` is stored in a `useRef` (correct) but read inside a `useCallback` that is not recreated when `flatItems` changes, the handler will see the correct ref value but may use a stale `flatItems` snapshot for index resolution. **Pattern**: store the anchor as a `useRef<string | null>` (the session ID), and at shift-click time call `flatItems.findIndex(item => item.session?.id === anchorId)` — this always resolves against the current list. Never store the integer index across renders.

---

## 3. Keyboard shortcut pitfalls

### 3a. Cmd+A in an input field

A global `keydown` listener for Cmd/Ctrl+A must check `event.target`. If the target is an `<input>`, `<textarea>`, or any `[contenteditable]` element (including the search box in `SessionList`, the rename input in session rows, or the "Group as" input in `BulkActions`), the event should be **ignored** — do not call `event.preventDefault()` and do not trigger `selectAll`. Check:

```ts
const tag = (e.target as HTMLElement).tagName;
const editable = (e.target as HTMLElement).isContentEditable;
if (tag === "INPUT" || tag === "TEXTAREA" || editable) return;
```

### 3b. Escape priority: modal vs. select mode

When a confirmation modal (e.g. bulk delete confirm) is open, pressing Escape should close the modal via the modal's own `keydown` handler — it must **not** also exit select mode. The two listeners must not fire simultaneously. The safest approach is to have the modal's handler call `event.stopPropagation()` so the global Escape handler for select mode never sees it. Alternatively, use `event.defaultPrevented` as a guard: the modal sets it, the select-mode handler bails if it's set.

### 3c. Global listener cleanup

The global `keydown` listener added in a `useEffect` must return a cleanup function that calls `removeEventListener`. Missing cleanup causes the handler to accumulate: each time `SessionList` remounts (e.g. split-pane scenario), another listener is added, leading to double-firing. Verify the effect dependency array is correct so the listener is not replaced on every render.

---

## 4. Hover-reveal checkbox pitfalls

### 4a. React hover state is lost on scroll

In a virtualized list, rows are unmounted when they scroll out of the overscan window. Any `useState(false)` hover state stored in the row component resets to `false` on remount. **Do not use React state for hover.** Use pure CSS `:hover` pseudo-class on the row element to reveal the checkbox — this survives unmount/remount because it's driven by the cursor position at paint time, not component state.

### 4b. Checkboxes must always be visible in select mode

When `selectMode` is active, the checkbox must be unconditionally visible regardless of hover. Implement via a data attribute on the row:

```css
/* always show when select mode is active */
[data-select-mode="true"] .checkbox { opacity: 1; pointer-events: auto; }
/* reveal on hover otherwise */
.row:hover .checkbox { opacity: 1; pointer-events: auto; }
/* hidden by default */
.checkbox { opacity: 0; pointer-events: none; }
```

Pass `data-select-mode={selectMode}` on the list container so the CSS cascade handles both cases without per-row prop drilling.

---

## 5. Undo / pending-delete pitfalls

### 5a. Navigation before undo window closes

If the user navigates away from the session list page (e.g. opens a session) before the 5-second undo window closes, the delete timer fires in a component that may have unmounted. If the delete is fired from a `setTimeout` inside the component, the callback will still execute (closures survive unmount), but any state update it tries to set will warn ("Can't perform a React state update on an unmounted component"). **Fix**: move the pending-delete timer and the actual delete call into a stable context (e.g. a `useRef`-based timer, or the parent page component), not inside `SessionList`. Cancel the timer in a `useEffect` cleanup.

### 5b. Multiple consecutive bulk deletes

Each new bulk delete must cancel the previous pending timer. If the user selects 3 sessions, deletes them, then immediately selects 2 more and deletes again, there should be exactly one active undo toast. Store the pending delete as a single `pendingDelete` ref (not an array). When a new bulk delete is triggered, immediately execute any previously pending delete (no undo for it), then start a new 5-second window for the new batch.

### 5c. Race: delete fires → user clicks undo before response

Timeline: user triggers delete → 5s timer fires → RPC call starts → user clicks undo before RPC response → undefined state. **Fix**: the undo button must be disabled once the delete RPC has started (timer has fired). Use a state machine: `idle → pending (undo available) → deleting (undo disabled) → done`. If `RestoreSession` RPC does not exist, undo must be a client-side cancel-before-fire pattern only — never send a delete RPC and then try to restore.

---

## 6. Accessibility pitfalls

### 6a. ARIA nesting: `role="toolbar"` placement

`BulkActions` uses `role="toolbar"` on its container div (confirmed in `/web-app/src/components/sessions/BulkActions.tsx` line 39). This is rendered **above** the session list, not inside it. As long as the toolbar and the list are siblings in the DOM (not nested), this is valid. Do not inadvertently move the `BulkActions` inside the virtualizer scroll container or inside any `role="list"` element — toolbars inside lists are invalid ARIA.

### 6b. Stable checkbox IDs for `aria-labelledby`

In a virtualized list, row components are mounted and unmounted. If a checkbox uses `id="checkbox-${index}"` (virtual index), the ID changes when the list is scrolled. Use `id="checkbox-${session.id}"` (stable entity ID) so any `aria-labelledby` associations survive re-virtualization.

### 6c. Live region for selection count

Screen readers will not announce changes to "3 of 12 selected" text unless it's in a live region. Add `aria-live="polite"` and `aria-atomic="true"` to the selection count span in `BulkActions`. Without this, VoiceOver and NVDA users have no feedback that their Shift+click or Cmd+A was registered. The existing `BulkActions` count span (line 56) lacks `aria-live`.

---

## Summary of Top 3 Pitfalls

1. **Stale anchor ID for Shift+click range select**: storing `lastAnchorIndex` as an integer is fragile — store the anchor as a session ID and resolve its index at click time via `flatItems.findIndex`. Integer indices break silently after any sort, filter, or data refresh.

2. **Cmd+A in input fields**: the global Cmd/Ctrl+A handler must guard against `event.target` being an input, textarea, or contenteditable element. The search box, rename field, and "Group as" input in `BulkActions` are all in the same component tree and will capture the shortcut incorrectly without this guard.

3. **Pending-delete timer surviving unmount / stacked bulk deletes**: if the 5-second undo timer lives inside `SessionList`, it fires even after navigation away, and a second bulk delete before the first timer expires silently queues two delete RPCs. Use a single stable `pendingDelete` ref with explicit replacement semantics — new delete always flushes the old one immediately before starting its own timer.
