# ADR-001: Text content display instead of pixel-level magnification

**Date**: 2026-10-17
**Status**: Accepted

## Context

The magnifier feature must show "the content under the finger" while a selection handle is
being dragged. Two interpretations exist:

1. **Pixel zoom** — scale up a rectangular region of the terminal canvas, as a native OS
   magnifier would do.
2. **Text content** — read the terminal buffer at the target row/column and display the
   text in a floating overlay.

## Decision

Use **text content** (option 2).

## Rationale

### Why pixel zoom is not viable here

- The xterm.js terminal renders into a `<canvas>` element using the WebGL or canvas
  renderer. Reading pixels from a hardware-accelerated WebGL canvas requires
  `preserveDrawingBuffer: true` in the WebGL context options. The xterm.js WebGL addon
  does not expose this option and does not set it — the default is `false`, which means
  `drawImage(canvas, ...)` onto another canvas produces a blank image after the first
  composed frame (WebGL contexts swap buffers and the "source" buffer is not readable).
- Even with `preserveDrawingBuffer: true`, cloning a canvas region requires:
  - A second `<canvas>` element
  - A `getContext('2d')` call
  - A `drawImage(sourceCanvas, sx, sy, sw, sh, dx, dy, dw, dh)` call per frame
  
  At 60fps on a mobile device this is a measurable rendering cost, potentially 5–15ms per
  frame depending on GPU/driver readback speed — far above the budget for a pure display aid.
- There is no public xterm.js API to request `preserveDrawingBuffer`, and patching the
  WebglAddon would be a fork-maintenance burden.

### Why text content is sufficient and appropriate

- The user's goal is to see **which character the handle is pointing at**, not a pixel-faithful
  reproduction of the terminal display. Text content answers that question directly.
- `terminal.buffer.active.getLine(row).translateToString(false, startCol, endCol)` is an
  O(N) string copy with no layout side effects — safe to call once per animation frame.
- The monospace font in the overlay makes each character the same width as in the terminal,
  so the underlined cursor character visually "lands" at the correct column.
- No new npm packages, no WebGL API surface, no canvas.

## Consequences

- The magnifier does not show ANSI color attributes (bold, italic, color) — only the
  plain text content.
- If a future ticket requires a "true" magnified pixel view, it would need either:
  a. Upstream xterm.js support for `preserveDrawingBuffer` (unlikely given their
     explicit choice not to expose it), or
  b. A custom off-screen canvas renderer that duplicates xterm's drawing pipeline.
  Neither is in scope for this ticket.
