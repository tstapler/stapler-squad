/**
 * touchDrag — shared helpers for touch-driven terminal drag gestures
 * (long-press text selection in useTerminalGestures, and the draggable
 * selection handles in XtermTerminal). Kept here instead of duplicated in
 * both call sites so the cell-coordinate math and per-frame throttling stay
 * consistent between the two gestures.
 */

/**
 * Converts a viewport point to clamped terminal cell coordinates.
 * `rect` should be captured once per drag (at touchstart), not
 * re-measured on every touchmove — the DOM read is what makes raw,
 * unthrottled touchmove handlers janky on Android.
 */
export interface CellGeometry {
  rect: { left: number; top: number };
  cellW: number;
  cellH: number;
  maxCol: number;
  maxRow: number;
}

export function pointToCell(
  clientX: number,
  clientY: number,
  geometry: CellGeometry
): { col: number; row: number } {
  const { rect, cellW, cellH, maxCol, maxRow } = geometry;
  return {
    col: Math.max(0, Math.min(maxCol, Math.floor((clientX - rect.left) / cellW))),
    row: Math.max(0, Math.min(maxRow, Math.floor((clientY - rect.top) / cellH))),
  };
}

/**
 * Wraps a per-point handler so it runs at most once per animation frame,
 * always with the most recent point — intermediate touchmove events are
 * coalesced instead of each one forcing an immediate xterm redraw.
 * Returns [throttled handler, cancel] — call cancel() on gesture end/cleanup
 * to drop any frame still pending from the last point before teardown.
 */
export function rafThrottlePoint(
  handler: (clientX: number, clientY: number) => void
): [(clientX: number, clientY: number) => void, () => void] {
  let rafId: number | null = null;
  let latestX = 0;
  let latestY = 0;

  const throttled = (clientX: number, clientY: number) => {
    latestX = clientX;
    latestY = clientY;
    if (rafId !== null) return;
    rafId = requestAnimationFrame(() => {
      rafId = null;
      handler(latestX, latestY);
    });
  };

  const cancel = () => {
    if (rafId !== null) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
  };

  return [throttled, cancel];
}
