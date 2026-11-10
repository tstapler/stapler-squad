import { encodeWheel } from "./scrollRouting";

/**
 * ghostty-web's own wheel handler scrolls its local scrollback, or sends arrow keys on the alternate
 * screen; it never sends mouse reports. A TUI that enabled mouse tracking (Claude Code) therefore
 * never saw the wheel. xterm.js reports it, so this restores that: while tracking is on, the wheel
 * goes to the app as SGR reports, and Shift+wheel still scrolls the local scrollback (xterm's rule).
 * SGR (1006) is assumed; tracking apps that only speak the legacy X10 encoding are not covered.
 */

const PIXELS_PER_LINE_FALLBACK = 20;

export interface WheelTerminal {
  cols: number;
  rows: number;
  hasMouseTracking(): boolean;
  /** Pixel size of one cell, when the renderer can report it. */
  cellSize(): { width: number; height: number } | undefined;
  /** Bounding box of the canvas the cell grid is drawn on. */
  gridRect(): { left: number; top: number; width: number; height: number } | undefined;
}

export type WheelLike = Pick<WheelEvent, "deltaY" | "deltaMode" | "shiftKey" | "clientX" | "clientY">;

const DOM_DELTA_LINE = 1;
const DOM_DELTA_PAGE = 2;

/** Returns a handler for `attachCustomWheelEventHandler`: true means the wheel was forwarded to the app. */
export function createMouseWheelForwarder(
  term: WheelTerminal,
  send: (data: string) => void,
): (event: WheelLike) => boolean {
  let pendingLines = 0;

  const linesOf = (e: WheelLike): number => {
    if (e.deltaMode === DOM_DELTA_LINE) return e.deltaY;
    if (e.deltaMode === DOM_DELTA_PAGE) return e.deltaY * term.rows;
    return e.deltaY / (term.cellSize()?.height || PIXELS_PER_LINE_FALLBACK);
  };

  const cellAt = (e: WheelLike) => {
    const rect = term.gridRect();
    if (!rect || rect.width <= 0 || rect.height <= 0) return { col: 1, row: 1, cols: term.cols, rows: term.rows };
    return {
      col: Math.floor(((e.clientX - rect.left) / rect.width) * term.cols) + 1,
      row: Math.floor(((e.clientY - rect.top) / rect.height) * term.rows) + 1,
      cols: term.cols,
      rows: term.rows,
    };
  };

  return (event) => {
    if (event.shiftKey || !term.hasMouseTracking()) {
      pendingLines = 0;
      return false;
    }
    pendingLines += linesOf(event);
    const whole = Math.trunc(pendingLines);
    pendingLines -= whole;
    const data = whole === 0 ? "" : encodeWheel(whole, cellAt(event));
    if (data) send(data);
    return true;
  };
}
