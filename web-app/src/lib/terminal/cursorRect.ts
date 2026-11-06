/**
 * Where the xterm cursor cell is on screen, for UI that must not cover the
 * line being typed on (the desktop toast deck). Terminals register a getter while
 * mounted; the reader picks the first one that reports a visible rectangle.
 */
export interface CellRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

type CursorSource = () => CellRect | null;

const sources = new Set<CursorSource>();

export function registerTerminalCursorSource(source: CursorSource): () => void {
  sources.add(source);
  return () => sources.delete(source);
}

export type CursorRead =
  | { kind: "no-terminal" }
  | { kind: "unreadable" }
  | { kind: "rect"; rect: CellRect };

/** "no-terminal": nothing to cover. "unreadable": a terminal is mounted but gave no rectangle. */
export function readTerminalCursor(): CursorRead {
  if (sources.size === 0) return { kind: "no-terminal" };
  for (const source of sources) {
    const rect = source();
    if (rect && rect.width > 0 && rect.height > 0) return { kind: "rect", rect };
  }
  return { kind: "unreadable" };
}

interface TerminalLike {
  cols: number;
  rows: number;
  buffer: { active: { cursorX: number; cursorY: number } };
}

/** Cursor cell in viewport coordinates, from the `.xterm-screen` box and the cell grid. */
export function cursorCellRect(terminal: TerminalLike, screen: Element | null): CellRect | null {
  if (!screen || terminal.cols <= 0 || terminal.rows <= 0) return null;
  // The pool keeps off-screen terminals mounted with visibility: hidden.
  if (screen instanceof Element && getComputedStyle(screen).visibility === "hidden") return null;
  const box = screen.getBoundingClientRect();
  if (box.width === 0 || box.height === 0) return null; // hidden (pooled terminal not on screen)
  const cellW = box.width / terminal.cols;
  const cellH = box.height / terminal.rows;
  const { cursorX, cursorY } = terminal.buffer.active;
  return { left: box.left + cursorX * cellW, top: box.top + cursorY * cellH, width: cellW, height: cellH };
}
