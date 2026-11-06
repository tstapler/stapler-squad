/**
 * Pure helpers for TerminalOutput's pre-sizing: reading xterm's rendered cell
 * metrics and deriving cols/rows from a container's pixel size, so a connection
 * can start before xterm fires its first onResize.
 */

import type { Terminal } from "@xterm/xterm";
import { MIN_COLS, MIN_ROWS } from "./TerminalDimensionCache";

export interface CellMetrics {
  cellWidth: number;
  cellHeight: number;
}

/**
 * Cell pixel metrics from xterm.js's private render service, or null when
 * unavailable (API changed, not rendered yet). isFinite() guards against
 * NaN/Infinity a corrupted render-service state could return.
 */
export function readRenderedCellMetrics(terminal: Terminal | null | undefined): CellMetrics | null {
  const cell = (terminal as any)?._core?._renderService?.dimensions?.css?.cell;
  if (cell?.width && cell?.height && isFinite(cell.width) && isFinite(cell.height)) {
    return { cellWidth: cell.width, cellHeight: cell.height };
  }
  return null;
}

export type PreSizeResult =
  | { ok: true; cols: number; rows: number }
  | { ok: false; reason: "zero-size" }
  | { ok: false; reason: "below-minimum"; cols: number; rows: number };

/**
 * cols/rows that fit in a container of the given pixel size. Floors, so the
 * result never exceeds what actually fits.
 */
export function computePreSize(
  container: { width: number; height: number },
  cell: CellMetrics,
): PreSizeResult {
  if (!(container.width > 0 && container.height > 0)) {
    return { ok: false, reason: "zero-size" };
  }
  const cols = Math.floor(container.width / cell.cellWidth);
  const rows = Math.floor(container.height / cell.cellHeight);
  if (cols < MIN_COLS || rows < MIN_ROWS) {
    return { ok: false, reason: "below-minimum", cols, rows };
  }
  return { ok: true, cols, rows };
}
