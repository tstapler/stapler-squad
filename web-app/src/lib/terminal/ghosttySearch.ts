/** Case-insensitive buffer search replacing `@xterm/addon-search`, which ghostty-web has no equivalent of. */

export interface SearchState {
  term: string;
  /** Absolute buffer row and column of the previous match; row -1 means no previous match. */
  row: number;
  col: number;
}

export interface SearchHit {
  row: number;
  col: number;
}

interface SearchableBuffer {
  buffer: {
    active: {
      length: number;
      getLine(y: number): { translateToString(trimRight?: boolean): string } | undefined;
    };
  };
}

/** Next (or previous) match after `state`, wrapping around the buffer; null when `term` is absent. */
export function findInBuffer(
  terminal: SearchableBuffer,
  term: string,
  state: SearchState,
  backwards: boolean,
): SearchHit | null {
  const { length, getLine } = terminal.buffer.active;
  const needle = term.toLowerCase();
  const rowText = (y: number) => getLine.call(terminal.buffer.active, y)?.translateToString(true).toLowerCase() ?? "";
  const continuing = state.term === term && state.row >= 0;

  for (let step = 0; step <= length; step++) {
    const offset = backwards ? -step : step;
    const row = continuing ? ((state.row + offset) % length + length) % length : backwards ? length - 1 - step : step;
    if (row < 0 || row >= length) continue;
    const text = rowText(row);
    // On the starting row only a match strictly past (or before) the previous one counts, until the wrap returns.
    const onStartRow = continuing && step === 0;
    let col: number;
    if (backwards) {
      col = onStartRow ? (state.col > 0 ? text.lastIndexOf(needle, state.col - 1) : -1) : text.lastIndexOf(needle);
    } else {
      col = text.indexOf(needle, onStartRow ? state.col + 1 : 0);
    }
    if (col >= 0) return { row, col };
  }
  return null;
}
