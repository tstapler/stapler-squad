# Research Findings: touch-selection-magnifier

**Date**: 2026-10-17

## 1. Existing code inventory

### XtermTerminal.tsx — mobile handle infrastructure

`web-app/src/components/sessions/XtermTerminal.tsx` already implements draggable selection
handles for `pointer: coarse` (touch-primary) devices:

- **`startHandleRef` / `endHandleRef`** (`useRef<HTMLDivElement>`) — React refs to DOM
  divs rendered into `document.body` via `createPortal`. Positioned via `style.left` /
  `style.top` direct DOM mutation inside `terminal.onSelectionChange`.
- **`attachHandleDrag(el, 'start'|'end')`** — registers native `touchstart`,
  `touchmove`, `touchend` on `el`. On `touchstart` captures the selection anchor and a
  `CellGeometry` snapshot. On `touchmove` calls `selectThrottled(touch.clientX,
  touch.clientY)` (RAF-throttled). On `touchend` / `touchcancel` tears down and resets.
- **`rafThrottlePoint`** — coalesces touch points to one per animation frame (from
  `touchDrag.ts`).
- **`handleCleanupFns`** — array of cleanup functions run on terminal dispose /
  component unmount. `attachHandleDrag` pushes its own cleanup here.
- **Portal pattern** — the handles and copy button already live in a single
  `createPortal(..., document.body)` block. The magnifier should join this same portal.

### XtermTerminal.css.ts — existing styles

- `selectionHandle` — `position: fixed`, `zIndex: 1090`, renders as a circle with a stem
  above it.
- `floatingCopyButton` — `position: fixed`, `zIndex: zIndex.floatingTerminalUI`.
- Uses vanilla-extract `style()` and references `vars.*` from the theme contract.

### touchDrag.ts

```ts
export interface CellGeometry { rect, cellW, cellH, maxCol, maxRow }
export function pointToCell(clientX, clientY, geometry): { col, row }
export function rafThrottlePoint(handler): [throttled, cancel, flush]
```

`pointToCell` is called once per frame with the latest touch point and the
`CellGeometry` captured at `touchstart`. No extra layout reads inside the hot path.

### xterm.js buffer API

```ts
terminal.buffer.active.getLine(row): IBufferLine | undefined
line.translateToString(trimRight?: boolean, startColumn?: number, endColumn?: number): string
```

`translateToString(false, startCol, endCol)` strips ANSI escape sequences and returns
plain text for a column range. This is the correct API to read a text window around the
active cell — it does not trigger layout or canvas operations.

`terminal.buffer.active.length` gives total line count; `terminal.rows` gives the visible
viewport height. The `row` from `pointToCell` is a viewport-relative row; add
`terminal.buffer.active.viewportY` to get the absolute buffer row.

### getCellDimensions (lib/terminal/cellDimensions.ts)

```ts
export function getCellDimensions(terminal: Terminal): { cellW: number; cellH: number }
```

Already used at `touchstart` in `attachHandleDrag` via `CellGeometry`; no new import
needed.

## 2. Android Magnifier API — reference model

Android's `android.widget.Magnifier` (API level 28, 2018) is the UX reference:

- Appears above the drag point while the handle is held.
- Shows a zoomed view of the content around the touch point.
- Follows the handle as it moves.
- Dismisses on `ACTION_UP` / `ACTION_CANCEL`.

The iOS cursor loupe has the same behavior for cursor placement.

The web equivalent cannot do pixel-level zoom of a `<canvas>` (the canvas is
cross-origin from the browser's perspective once hardware-accelerated). The practical
equivalent is showing the **text content** of the row at the current column, which is
the approach taken here.

## 3. CSS approach for the pop-up

```
position: fixed         — avoids ancestor transform issues (same reason handles use fixed)
z-index: 1091+          — above the handles (1090)
bottom: auto / top: ... — positioned to sit above the touch point
pointer-events: none    — never intercepts touches
user-select: none       — no accidental text selection
font-family: monospace  — mirrors terminal font
font-size: ~13-14px     — slightly smaller than terminal to fit on narrow screens
background: ...         — semi-opaque dark or light bg from theme vars
border-radius: 6px      — rounded bubble appearance
box-shadow: 0 2px 8px rgba(0,0,0,0.5)  — elevation
```

Positioning formula (computed at each frame, no layout reads):
```
top = (handle.top px) - MAGNIFIER_HEIGHT_PX - STEM_GAP_PX
left = (handle.left px) - MAGNIFIER_WIDTH_PX / 2
```
`handle.top` / `handle.left` are the same values already computed when repositioning
the handle itself (`rect.top + (row + 1) * cellH`, etc.), so no extra reads.

Clamp `left` to `[EDGE_MARGIN, window.innerWidth - MAGNIFIER_WIDTH_PX - EDGE_MARGIN]`
to prevent off-screen overflow.

## 4. Text extraction approach

```ts
function getTerminalRowText(
  terminal: Terminal,
  viewportRow: number,
  activeCol: number,
  windowSize = 20
): { text: string; cursorOffset: number } {
  const bufRow = viewportRow + terminal.buffer.active.viewportY;
  const line = terminal.buffer.active.getLine(bufRow);
  if (!line) return { text: ' '.repeat(windowSize), cursorOffset: Math.floor(windowSize / 2) };
  const half = Math.floor(windowSize / 2);
  const startCol = Math.max(0, activeCol - half);
  const endCol = Math.min(terminal.cols, startCol + windowSize);
  const text = line.translateToString(false, startCol, endCol);
  return { text: text || ' '.repeat(endCol - startCol), cursorOffset: activeCol - startCol };
}
```

`cursorOffset` tells the renderer which character within `text` is the active one, so
it can be highlighted (e.g. wrapped in a `<span>` with `text-decoration: underline`).

Because we use `translateToString` with explicit `startColumn`/`endColumn`, and
`getLine` returns a reference to an already-decoded buffer object, this call is O(windowSize)
character copies with no layout side effects.

## 5. Performance analysis

The magnifier's DOM update happens inside the existing `rafThrottlePoint` callback —
the same frame budget as the `terminal.select(...)` call. The additional work per frame:

- One `getTerminalRowText(...)` call: O(20) string chars, one buffer row lookup.
- One `style.left` / `style.top` assignment.
- One `textContent` assignment on a `<span>`.

All three are "cheap DOM property writes with no layout flush". The existing handle drag
already assigns `style.left` / `style.top` to two handles per frame; adding a third
fixed-position element is not measurably different.

## 6. Alternative approaches considered

| Approach | Pros | Cons | Decision |
|---|---|---|---|
| Canvas-based pixel zoom (draw scaled region from xterm canvas) | True visual magnifier | Canvas taints (CORS/GPU), `drawImage` from WebGL canvas requires explicit `preserveDrawingBuffer`, perf cost, complex | **Rejected** |
| New xterm.js addon | Clean API | No such addon exists; out of scope for this ticket | **Rejected** |
| Text content bubble (this plan) | Simple, fast, uses existing APIs, no new deps | Not a "true" pixel zoom | **Adopted** |
| CSS `transform: scale()` clone | Possible via `element.innerHTML` copy | Snapshot timing issues with canvas renderer; no API to extract HTML from xterm buffer | **Rejected** |

## 7. Risk assessment

| Risk | Likelihood | Mitigation |
|---|---|---|
| `getLine(row)` returns null for viewport rows beyond buffer length | Low (clamps in `pointToCell`) | Null-guard returns blank string |
| Magnifier positioned off-screen top (very short viewport) | Low | `Math.max(EDGE_MARGIN, ...)` clamp on `top` |
| z-index conflict with other portal overlays | Low | Use `zIndex.floatingTerminalUI + 1` or a new named token |
| Magnifier visible on desktop via pointer-type change | Very low | `isTouchPrimary` flag gates creation; not rendered at all on non-touch |
| iOS Safari `translateToString` performance | Very low | API is synchronous string copy; no layout |
