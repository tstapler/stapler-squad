# Implementation Plan: touch-selection-magnifier

**Date**: 2026-10-17
**Based on**: requirements.md, research/findings.md

## Overview

Add a floating magnifier pop-up to the mobile terminal selection handle drag that shows
the terminal row content at the current drag position, positioned above the touch point.
All changes are confined to:

- `web-app/src/components/sessions/XtermTerminal.tsx` — DOM ref, show/hide/position logic
- `web-app/src/components/sessions/XtermTerminal.css.ts` — CSS for the magnifier bubble
- `web-app/src/lib/terminal/touchDrag.ts` — new `getTerminalRowText` helper
- `web-app/src/lib/terminal/__tests__/touchDrag.test.ts` — unit tests for the helper

No backend changes. No new npm packages.

---

## Task 1 — `getTerminalRowText` helper in `touchDrag.ts`

**File**: `web-app/src/lib/terminal/touchDrag.ts`

Add and export:

```ts
/**
 * Returns a short context string centered on `activeCol` from the buffer row
 * at `viewportRow` (0-based viewport coordinates). Also returns `cursorOffset`,
 * the index within `text` that corresponds to `activeCol` — used by the
 * magnifier to underline the active character.
 *
 * Safe to call on every animation frame: O(windowSize) string copy, no layout.
 */
export function getTerminalRowText(
  terminal: { buffer: { active: { viewportY: number; getLine(row: number): { translateToString(trim: boolean, start?: number, end?: number): string } | undefined } }; cols: number },
  viewportRow: number,
  activeCol: number,
  windowSize = 20,
): { text: string; cursorOffset: number } {
  const bufRow = viewportRow + terminal.buffer.active.viewportY;
  const line = terminal.buffer.active.getLine(bufRow);
  const half = Math.floor(windowSize / 2);
  const startCol = Math.max(0, activeCol - half);
  const endCol = Math.min(terminal.cols, startCol + windowSize);
  const rawText = line?.translateToString(false, startCol, endCol) ?? '';
  const text = rawText.padEnd(endCol - startCol, ' ');
  return { text, cursorOffset: activeCol - startCol };
}
```

**Note on the type signature**: Use a structural interface type rather than importing
`Terminal` from `@xterm/xterm` directly — this keeps the helper testable without a real
xterm instance.

---

## Task 2 — Unit tests for `getTerminalRowText`

**File**: `web-app/src/lib/terminal/__tests__/touchDrag.getTerminalRowText.test.ts`

Test cases (all pure, no DOM):

1. **Normal case** — 80-col line, `activeCol=10`, `windowSize=20`: returns 20 chars,
   `cursorOffset` = 10.
2. **Near left edge** — `activeCol=2`: `startCol` clamps to 0; `cursorOffset` = 2.
3. **Near right edge** — `activeCol=78`, `terminal.cols=80`: `endCol` clamps to 80.
4. **Empty buffer line** — `getLine()` returns `undefined`: returns spaces, no crash.
5. **Short row** — `translateToString` returns fewer chars than `windowSize`: result is
   padded to full window size.
6. **`viewportY` offset** — verify `bufRow = viewportRow + viewportY` (buffer row
   calculation is correct when the user has scrolled).

---

## Task 3 — Magnifier CSS class in `XtermTerminal.css.ts`

**File**: `web-app/src/components/sessions/XtermTerminal.css.ts`

Add:

```ts
// ---- Mobile selection magnifier ----
// Floating bubble that appears above the active handle during drag.
// Shows the terminal row content at the current cell column.
// Hidden by default; shown/positioned via direct DOM mutation (no React state).
export const selectionMagnifier = style({
  position: "fixed",
  zIndex: 1091,                      // above handles (1090)
  pointerEvents: "none",
  userSelect: "none",
  WebkitUserSelect: "none",
  background: "rgba(24, 24, 28, 0.94)",
  color: "rgba(255, 255, 255, 0.95)",
  border: "1px solid rgba(255, 255, 255, 0.18)",
  borderRadius: vars.radii.md,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  fontSize: vars.fontSize.xs,
  lineHeight: 1.4,
  whiteSpace: "pre",
  boxShadow: "0 4px 16px rgba(0,0,0,0.55)",
  // Stem pointing downward toward the touch point
  "::after": {
    content: '""',
    position: "absolute",
    bottom: -6,
    left: "50%",
    transform: "translateX(-50%)",
    width: 0,
    height: 0,
    borderLeft: "6px solid transparent",
    borderRight: "6px solid transparent",
    borderTop: "6px solid rgba(24, 24, 28, 0.94)",
  },
});

// Highlighted character inside the magnifier (the active column)
export const selectionMagnifierCursor = style({
  textDecoration: "underline",
  textDecorationColor: vars.color.primary,
  textDecorationThickness: "2px",
  textUnderlineOffset: "2px",
  color: vars.color.primary,
});
```

**Light-theme note**: The hardcoded rgba values match the existing `floatingCopyButton`
dark treatment. A future ticket can wire in `@media (prefers-color-scheme: light)` variants;
the terminal is dark by default and the magnifier will always be readable against it.

---

## Task 4 — `magnifierRef` DOM element in the portal

**File**: `web-app/src/components/sessions/XtermTerminal.tsx`

### 4a. Add ref

```ts
const magnifierRef = useRef<HTMLDivElement>(null);
```

Add alongside `startHandleRef` / `endHandleRef` (around line 317).

### 4b. Add to portal

Inside the existing `createPortal(...)` block, add the magnifier alongside the handles:

```tsx
<div
  ref={magnifierRef}
  className={styles.selectionMagnifier}
  aria-hidden="true"
  role="presentation"
  style={{ display: 'none' }}
/>
```

---

## Task 5 — Show / update / hide magnifier inside `attachHandleDrag`

**File**: `web-app/src/components/sessions/XtermTerminal.tsx`

Modify `attachHandleDrag` (lines ~989–1061):

### 5a. Import `getTerminalRowText`

Add to the import from `@/lib/terminal/touchDrag`:

```ts
import { pointToCell, rafThrottlePoint, type CellGeometry, getTerminalRowText } from "@/lib/terminal/touchDrag";
```

### 5b. Add magnifier update inside `applyHandleDrag`

`applyHandleDrag` is called once per RAF frame. After the `terminal.select(...)` call:

```ts
// Update magnifier position and content
const mag = magnifierRef.current;
if (mag) {
  const { text, cursorOffset } = getTerminalRowText(terminal, row, col);
  // Render: text split around cursorOffset, cursor char highlighted
  const before = text.slice(0, cursorOffset);
  const cursor = text.slice(cursorOffset, cursorOffset + 1);
  const after = text.slice(cursorOffset + 1);
  mag.innerHTML = `${escapeHtml(before)}<span class="${styles.selectionMagnifierCursor}">${escapeHtml(cursor)}</span>${escapeHtml(after)}`;

  // Position above the handle touch point
  // handle.left is: rect.left + (col + (handle==='end' ? 1 : 0)) * cellW
  // Use the same rect/cellW from geometry
  const MAGNIFIER_HEIGHT = 36; // px — approximate, no layout read
  const MAGNIFIER_WIDTH = 200; // px — approximate
  const GAP = 12; // px above the handle stem
  const handleX = geometry.rect.left + col * geometry.cellW;
  const handleY = geometry.rect.top + (row + 1) * geometry.cellH;
  const rawLeft = handleX - MAGNIFIER_WIDTH / 2;
  const rawTop = handleY - MAGNIFIER_HEIGHT - GAP;
  const EDGE = 8;
  mag.style.left = `${Math.max(EDGE, Math.min(window.innerWidth - MAGNIFIER_WIDTH - EDGE, rawLeft))}px`;
  mag.style.top = `${Math.max(EDGE, rawTop)}px`;
  mag.style.width = `${MAGNIFIER_WIDTH}px`;
}
```

**`escapeHtml`** — add a tiny private function (4 lines) to escape `<`, `>`, `&` in
text snippets before setting `innerHTML`:

```ts
function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
```

### 5c. Show magnifier on `touchstart`

Inside `onTouchStart`, after setting `anchor` / `geometry`:

```ts
if (magnifierRef.current) {
  magnifierRef.current.style.display = 'block';
}
```

### 5d. Hide magnifier on `touchend` / `touchcancel`

Inside `onTouchEnd`:

```ts
if (magnifierRef.current) {
  magnifierRef.current.style.display = 'none';
}
```

---

## Task 6 — Cleanup

`magnifierRef.current` is a React-managed ref; no explicit cleanup beyond what the portal
and `handleCleanupFns` already do. The element is always hidden (`display: none`) when not
in a drag, so no visible state leaks on unmount.

---

## Task 7 — Manual verification checklist (not automated)

These require a real touch device or Android/iOS emulator; jsdom cannot exercise them:

1. Start a session; open the terminal on Android Chrome or iOS Safari.
2. Long-press to select a word; handles appear at selection boundaries.
3. Drag the end handle right → magnifier appears above the handle showing terminal text
   with the active character underlined.
4. Drag to a different row → magnifier updates to show the new row's content.
5. Release the handle → magnifier disappears.
6. Drag the start handle left → same behavior.
7. Drag to the top row of the terminal (y near 0) → magnifier stays on-screen (clamped,
   not cut off at top).
8. Drag to the right edge of a narrow phone (320px) → magnifier stays on-screen (clamped,
   not cut off at right).
9. On desktop (mouse): no magnifier element appears; no visual regressions.

---

## Estimated effort

| Task | Estimate |
|---|---|
| Task 1 — `getTerminalRowText` helper | 1h |
| Task 2 — unit tests | 1h |
| Task 3 — CSS class | 30m |
| Task 4 — DOM ref + portal element | 30m |
| Task 5 — show/update/hide logic | 2h |
| Task 6 — cleanup review | 30m |
| Task 7 — manual verification | 1h |
| **Total** | **~6.5h** |
