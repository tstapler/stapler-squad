# Requirements: touch-selection-magnifier

**Date**: 2026-10-17
**Type**: feature
**Item ID**: 73339148-ff0f-4d0f-86b9-99b89a842b32

## Problem Statement

On mobile, users drag selection handles to set the start and end of a text selection in
the terminal. Because the finger covers the selection endpoint, they cannot see which
character their handle is pointing at — they're flying blind during precise selection
adjustments. This is the same usability gap Android's built-in magnifier (API 28+) solves
for native text fields.

The fix: show a small pop-up **above** the active handle while the finger is held down.
The pop-up shows the terminal row content at the drag point, zoomed or highlighted so the
user can read the exact character their handle sits on. It disappears when the finger lifts.
It must not alter how the selection is computed — pure display aid.

## Context: existing code

The selection handle infrastructure already exists in `XtermTerminal.tsx`:

- `startHandleRef` / `endHandleRef` — DOM elements (`div.selectionHandle`) rendered into
  `document.body` via React portal (`createPortal`). Shown/hidden and repositioned on
  every `terminal.onSelectionChange` via direct DOM mutation (no React re-render).
- `attachHandleDrag(el, 'start'|'end')` — called inside the `isTouchPrimary` branch;
  registers `touchstart` / `touchmove` / `touchend` on the handle element and updates
  `terminal.select(...)` via `rafThrottlePoint`.
- `touchDrag.ts` — `pointToCell(clientX, clientY, geometry)` converts a touch point to
  a clamped `{ col, row }` cell coordinate. `CellGeometry` is captured once per drag.
- `rafThrottlePoint` — ensures handle drag updates run at most once per animation frame.
- `isTouchPrimary` — `window.matchMedia('(pointer: coarse)').matches`, computed once on
  terminal mount.

## Users / Consumers

Mobile users (Android Chrome, iOS Safari) using the terminal to read and copy AI agent
output, command output, and log streams. The magnifier is visible only on `pointer: coarse`
(touch-primary) devices — desktop users are unaffected.

## Success Metrics

1. **Finger-occluded cell is visible**: While a selection handle is held and dragged, a
   pop-up appears above the touch point showing the content of the terminal row at the
   current cell column.
2. **Correct placement**: The pop-up sits entirely above the touch point (not below, not
   over the finger), following the handle as it moves, with no off-screen clipping at the
   top edge.
3. **No selection regression**: The selection coordinates produced by handle drag are
   unchanged (same `terminal.select(...)` calls, same `pointToCell` math).
4. **Performance ceiling**: The pop-up updates at the same cadence as the existing
   `rafThrottlePoint` handle drag (once per animation frame); it does not add synchronous
   DOM layout reads that weren't already present.
5. **Lifecycle correctness**: Pop-up appears on `touchstart`, updates on `touchmove`,
   disappears on `touchend` / `touchcancel`. No pop-up lingers after the finger lifts.
6. **Desktop unaffected**: On `pointer: fine` (mouse) devices the magnifier DOM elements
   are never created; the existing code path is unchanged.
7. **Accessibility**: The magnifier element carries `aria-hidden="true"` and
   `role="presentation"`; it adds nothing to the accessibility tree.
8. **Theme consistency**: The pop-up uses `vars.color.cardBackground` / `vars.color.text*`
   from the existing vanilla-extract theme contract so it renders correctly in both light
   and dark terminal themes.

## Constraints

- **No selection logic changes**: `terminal.select(...)`, `pointToCell(...)`, and
  `rafThrottlePoint(...)` are not modified.
- **No React re-renders**: The magnifier must be positioned and shown/hidden via direct DOM
  mutation (same pattern as the existing copy button / handles), not via React `setState`.
- **Portal, not local DOM**: The magnifier element must be portalled to `document.body`
  (same as the existing handles) so `fixed` positioning works correctly inside any
  CSS-transformed ancestor.
- **No new external dependencies**: Implement with vanilla DOM APIs and vanilla-extract CSS
  (already in use). No new npm packages.
- **iOS Safari safe**: No synchronous layout reads in the hot `touchmove` path beyond what
  `rafThrottlePoint` already coalesces. `getBoundingClientRect()` calls used by the magnifier
  must be captured at `touchstart`, not on every frame.
- **Single shared magnifier**: One magnifier element (not one per handle) is sufficient —
  only one handle is dragged at a time.

## Scope

### In Scope

- A `magnifierRef` DOM element rendered alongside the existing handles in the portal.
- CSS class `selectionMagnifier` in `XtermTerminal.css.ts` (vanilla-extract) for the
  pop-up visual treatment.
- Logic inside `attachHandleDrag` to:
  - Show magnifier on `touchstart`.
  - Update magnifier text (row content at current col) and position on each
    `rafThrottlePoint` frame.
  - Hide magnifier on `touchend` / `touchcancel`.
- Helper `getTerminalRowText(terminal, row, col)` to extract a short context window
  (e.g. ~20 chars centered on `col`) from `terminal.buffer.active.getLine(row)`.
- Unit tests for `getTerminalRowText` covering: col clamping, short-row (fewer columns),
  empty buffer line.

### Out of Scope

- Actual CSS zoom / canvas-based pixel magnification (e.g. drawing a scaled bitmap of
  the terminal cell). The requirement is to show the **text content**, not a magnified
  pixel rendering of the canvas.
- Magnifier during an initial long-press selection (only handle drag — the existing long-press
  selection in `useTerminalGestures` is a separate code path).
- Changes to the existing selection handle visual design (size, color, shape).
- Any backend (Go) changes.

## Open Questions

1. **How many characters to show in the context window?** The pop-up needs to be wide
   enough to show context around the active column but narrow enough not to overflow on a
   320-px-wide phone. ~20 chars is a reasonable starting point; confirm in visual review.
2. **Should the current-column character be visually highlighted** (e.g. underlined or
   bold) in the pop-up? This would make it easier to see exactly which character the
   handle points to. Recommended yes, but no explicit requirement stated in the backlog item.
3. **Should the magnifier show the active column indicator** (e.g. a cursor or caret
   above the highlighted character)? Adds visual clarity. Deferred to implementation phase.
