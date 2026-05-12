/**
 * getCellDimensions — compute terminal cell dimensions from public xterm.js API.
 *
 * Uses element.clientHeight / rows (public) instead of the private
 * _core._renderService.dimensions.css.cell.height, which may be undefined until
 * after the first render frame in xterm.js 6.x (R3.4 / R4.4).
 *
 * Shared between useTerminalGestures and XtermTerminal to avoid duplication.
 */

import type { Terminal } from "@xterm/xterm";

export function getCellDimensions(terminal: Terminal): { cellH: number; cellW: number } {
  const el = terminal.element;
  if (el && terminal.rows > 0 && terminal.cols > 0) {
    return {
      cellH: el.clientHeight / terminal.rows,
      cellW: el.clientWidth / terminal.cols,
    };
  }
  // Fallback: use font metrics (less accurate but never undefined)
  const fontSize = terminal.options.fontSize ?? 14;
  const lineHeight = (terminal.options.lineHeight as number | undefined) ?? 1.0;
  return {
    cellH: fontSize * lineHeight,
    cellW: fontSize * 0.6,
  };
}
