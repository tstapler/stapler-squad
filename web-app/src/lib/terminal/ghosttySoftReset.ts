/**
 * ghostty-web 0.4.0's VT parser logs `ignoring unimplemented CSI p with intermediates: !` and does
 * nothing for DECSTR (soft terminal reset, `CSI ! p`), which Claude Code and many TUIs emit on exit
 * or startup. Left unhandled, cursor visibility, origin mode, scroll region, SGR and charsets stay
 * stale and later redraws land on the wrong rows. This rewriter splits the stream at each DECSTR and
 * writes the equivalent explicit sequences instead (xterm.js's `softReset` is the reference).
 */

const DECSTR = "\x1b[!p";

export interface CursorPos {
  x: number;
  y: number;
}

/**
 * Explicit equivalent of DECSTR that leaves the cursor at `cursor` (0-based, absolute). Resetting
 * DECOM and DECSTBM both home the cursor, so it is put back with an absolute CUP at the end.
 */
export function softResetSequence(cursor: CursorPos): string {
  return (
    "\x1b[?6l" + // DECOM: absolute origin (homes the cursor)
    "\x1b[r" + // DECSTBM: full-screen scroll region
    "\x1b[?25h" + // DECTCEM: cursor visible
    "\x1b[4l" + // IRM: replace mode
    "\x1b[?7h" + // DECAWM: autowrap on (matches xterm.js)
    "\x1b[?1l\x1b>" + // DECCKM off, DECKPNM numeric keypad
    "\x1b(B\x1b)B\x0f" + // G0/G1 = ASCII, shift-in
    "\x1b[0m" + // SGR reset
    // A saved cursor (DECSC) restores to home with defaults after DECSTR; re-save at home, then move back.
    "\x1b[H\x1b7" +
    `\x1b[${cursor.y + 1};${cursor.x + 1}H`
  );
}

export interface SoftResetRewriter {
  /**
   * Passes `chunk` to `write`, replacing each DECSTR with its explicit equivalent. `cursor()` is read
   * after everything before the DECSTR has been written. A DECSTR split across chunks is carried
   * over; `write` is never called with an empty string (ghostty-web's `write("")` throws).
   */
  feed(chunk: string, write: (data: string) => void, cursor: () => CursorPos): void;
}

/** Length of the longest proper prefix of DECSTR that `text` ends with, so a split sequence is held back. */
function partialSuffixLength(text: string): number {
  const max = Math.min(DECSTR.length - 1, text.length);
  for (let n = max; n > 0; n--) {
    if (text.endsWith(DECSTR.slice(0, n))) return n;
  }
  return 0;
}

export function createSoftResetRewriter(): SoftResetRewriter {
  let carry = "";
  return {
    feed(chunk, write, cursor) {
      const text = carry + chunk;
      const keep = partialSuffixLength(text);
      carry = text.slice(text.length - keep);
      const parts = text.slice(0, text.length - keep).split(DECSTR);
      parts.forEach((part, i) => {
        if (part) write(part);
        if (i < parts.length - 1) write(softResetSequence(cursor()));
      });
    },
  };
}
