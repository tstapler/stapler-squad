# Stack Research: bulk-select-ux

**Date**: 2026-06-23
**Branch**: stapler-squad-session-list

---

## 1. Dependency Versions

| Library | Version |
|---|---|
| React | `^19.0.0` |
| `@tanstack/react-virtual` | `^3.13.25` |
| `@vanilla-extract/css` | `^1.20.1` |
| `@vanilla-extract/recipes` | `^0.5.7` |
| `@connectrpc/connect` | `^2.1.1` |
| `@connectrpc/connect-web` | `^2.1.1` |

Source: `web-app/package.json`

---

## 2. Existing Checkbox / Multi-select Pattern (SessionCard)

### Props interface (`SessionCard.tsx` lines 104–106)
```ts
selectMode?: boolean;
isSelected?: boolean;
onToggleSelect?: () => void;
```

### How the card uses them
- When `selectMode=true`, the whole card click calls `onToggleSelect()` instead of opening the session.
- A visible checkbox div (`<div className={checkbox}>`) is conditionally rendered **only when `selectMode` is true** (lines 396–406). It is hidden otherwise — no hover-reveal affordance exists today.
- The card root `aria-label` switches to include "Selected / Not selected" prefix when in select mode.
- Interactive sub-elements (tag editor, overflow actions) get `inert` / `aria-hidden` / `tabIndex={-1}` when `selectMode=true` so they don't steal keyboard focus.
- CSS classes `cardSelectMode` and `cardSelected` are applied from `SessionCard.css.ts` for visual state.

### Wiring in SessionList
- `selectMode`, `isSelected`, and `onToggleSelect` are passed to `<SessionCard>` at lines 1144–1146.
- **`<SessionRow>` at lines 979–1000 receives none of these props.** `SessionRowProps` has no `selectMode`, `isSelected`, or `onToggleSelect` fields.

---

## 3. `buildRowGridTemplate` — How It Works and Checkbox Column Support

Source: `web-app/src/components/sessions/session-columns.ts`

```ts
export function buildRowGridTemplate(visible: ColumnKey[]): string {
  const cols = ["8px", "1fr"]; // dot + name always present
  for (const def of COLUMN_DEFS) {
    if (visible.includes(def.key)) cols.push(def.gridWidth);
  }
  cols.push("auto"); // actions always present
  return cols.join(" ");
}
```

The function builds a `gridTemplateColumns` CSS string. The first two slots (`8px` = status dot, `1fr` = name) and the last slot (`auto` = actions) are hardcoded. Optional columns (`ColumnKey` values) are injected in the middle.

**Does it support a prepended non-`ColumnKey` checkbox column?**
No — not currently. A checkbox column would need to be prepended *before* the `8px` status dot. Two approaches:
1. Add a fixed first column (e.g. `"24px"`) when `selectMode=true`, passed to `buildRowGridTemplate` as an optional `selectMode: boolean` argument.
2. Change the first element from `["8px", "1fr"]` to `["24px", "8px", "1fr"]` when select mode is active, and add a corresponding checkbox `<span>` as the first child in `SessionRow`.

Neither approach breaks the existing `ColumnKey` union — a checkbox column would be a hardcoded slot outside the `COLUMN_DEFS` array. The inline `style={{ gridTemplateColumns: buildRowGridTemplate(visibleColumns) }}` at `SessionRow.tsx` line 183 is the single override point.

The `SessionRow.css.ts` fallback `gridTemplateColumns` at line 15 (`"8px 1fr 20px auto 32px auto"`) would also need updating to include the checkbox slot, but that fallback only applies when JS is unavailable.

---

## 4. `RestoreSession` / `UndeleteSession` RPC — Does It Exist?

Source: `proto/session/v1/session.proto`

RPCs found via grep:
- `DeleteSession` — exists (line 25)
- `RestartSession` — exists (line 106), but restarts a stopped session, does not restore a deleted one
- `ArchiveSession` / `UnarchiveSession` — exists (lines 392/395), but this archives, not deletes
- `BatchCreateSessions` — exists (line 272)

**No `RestoreSession`, `UndeleteSession`, or equivalent RPC exists.**

The modal at `SessionList.tsx` lines 1162–1186 warns "This cannot be undone." This is accurate — deletion is hard-delete with no server-side recovery.

**Implication for undo toast**: The undo feature must use a **client-side pending-delete pattern**:
1. Optimistically remove the session from the UI and store the IDs + session data in pending state.
2. Show a 5-second undo toast.
3. If the user clicks Undo, cancel the deletion (do not call `DeleteSession`).
4. If the toast expires without Undo, call `DeleteSession` for each pending ID.
5. If `WatchSessions` pushes a server-driven deletion event during the pending window, cancel the undo opportunity.

This pattern avoids any new proto RPC.

---

## 5. Keyboard Shortcut Integration Point

### `useKeyboard` hook (`web-app/src/lib/hooks/useKeyboard.ts`)
A `window.addEventListener("keydown", ...)` wrapper. Ignores events from `INPUT`, `TEXTAREA`, `SELECT`. Takes a `Record<string, () => void>` map. Modifier keys (Ctrl, Meta) are not built-in key identifier — they are included in `event.ctrlKey` / `event.metaKey` booleans but **not** in the `event.key` string (so `"Control+A"` is not a valid key identifier in the current hook).

Currently used in `app/page.tsx` lines 345–405 for global shortcuts: `Escape`, `R`, `j`, `k`, `Enter`, `p`, `r`, `d`, `t`.

### `G` shortcut for grouping
`handleCycleGrouping` is defined inside `SessionList.tsx` at line 471 but is **not wired to any keyboard event handler inside SessionList**. The `G`-to-cycle-grouping shortcut referenced in comments and UI hints is only active on the `/history` page (`app/history/page.tsx` line 265), where it fires via a local `keydown` listener. On the main sessions page, pressing `G` does nothing — it is an aspirational shortcut not yet wired.

### How to add Cmd+A and Escape to SessionList
Since `useKeyboard` does not support modifier-key combinations natively, new shortcuts should be added via a `useEffect` + `window.addEventListener("keydown", ...)` inside `SessionList`, checking `(e.metaKey || e.ctrlKey) && e.key === "a"` manually. Alternatively, extend `useKeyboard` to support modifier keys.

For `Escape` (exit select mode), the global `Escape` handler in `page.tsx` (line 347) currently handles closing modals and the open session. `SessionList` would need its own `Escape` handler that fires first when `selectMode` is active — this is safe because both handlers are independent `window.addEventListener` registrations; the inner handler (SessionList) can call `e.stopImmediatePropagation()` to prevent the outer one from closing the session detail pane.

---

## Summary

1. **`SessionRow` has no select props at all** — `selectMode`, `isSelected`, and `onToggleSelect` exist only on `SessionCard`. Row mode's `<SessionRow>` call at `SessionList.tsx` line 979–1000 omits all three, so row-mode users have zero selection capability today.

2. **No `RestoreSession` RPC exists** — undo for bulk delete must be implemented client-side as a pending-delete pattern (store IDs, delay actual `DeleteSession` RPC until 5-second toast expires).

3. **`buildRowGridTemplate` does not support a checkbox column but is easily extended** — it is a pure function that prepends fixed columns (`8px` dot, `1fr` name) and appends `auto` (actions). Adding a `selectMode?: boolean` parameter that prepends `"24px"` before the dot is a minimal change with no impact on `ColumnKey` or existing columns. The `G`-to-cycle-grouping shortcut is **not wired on the main sessions page** (only on `/history`), so no existing shortcut handler in `SessionList` competes with new Cmd+A / Escape registrations.
