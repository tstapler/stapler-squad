/**
 * Reproduces the "duplicate lines while typing" bug reported for pi sessions
 * (docs/how-to/enable-pi-support.md's approval-extension flow triggers pi's
 * own line-editor redraws). pi's TUI is built on @earendil-works/pi-tui,
 * whose dist/tui-main-screen.js redraws a single input line via:
 *   ESC[?2026h (begin synchronized output) + \r + ESC[2K (erase line) +
 *   <new text> + ESC[?2026l (end synchronized output)
 * -- verified by extracting and reading pi-tui@0.84.4's compiled JS directly
 * (`npm pack @earendil-works/pi-tui`), not guessed.
 *
 * This feeds that exact byte sequence through the real write pipeline
 * (TerminalStreamManager -> RedrawThrottler -> EscapeSequenceParser) into a
 * REAL @xterm/xterm Terminal (not MockTerminal, which only records raw
 * strings and can't reveal a rendering bug) and asserts on the terminal's
 * actual buffer content: a redrawn line must overwrite in place, never stack
 * as separate rows.
 */
import { Terminal } from '@xterm/xterm';
import { TerminalStreamManager, type ITerminal } from '../TerminalStreamManager';

// xterm.js's write() is asynchronous internally (queued, flushed on a later
// microtask/animation frame) even though TerminalStreamManager's write()
// call itself returns immediately. Issuing one more write with a real
// completion callback flushes the queue in FIFO order, so awaiting it is a
// reliable "all prior writes are now reflected in the buffer" signal.
function flush(term: Terminal): Promise<void> {
  return new Promise((resolve) => term.write('', resolve));
}

function bufferLines(term: Terminal): string[] {
  const lines: string[] = [];
  for (let i = 0; i < term.buffer.active.length; i++) {
    const line = term.buffer.active.getLine(i);
    if (line) lines.push(line.translateToString(true));
  }
  // Drop trailing blank rows so the assertion isn't sensitive to viewport height.
  while (lines.length > 0 && lines[lines.length - 1] === '') lines.pop();
  return lines;
}

describe('pi-tui line-redraw sequence (real xterm.js, no mocks)', () => {
  let term: Terminal;
  let manager: TerminalStreamManager;

  beforeEach(() => {
    term = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    manager = new TerminalStreamManager(term as unknown as ITerminal, () => {});
  });

  afterEach(() => {
    term.dispose();
  });

  it('single-line synchronized-output redraw overwrites in place, not appended as new rows', async () => {
    const SYNC_BEGIN = '\x1b[?2026h';
    const SYNC_END = '\x1b[?2026l';
    const words = ["w", "wh", "whe", "when", "wheni", "wheni'", "wheni'm"];

    for (const w of words) {
      manager.write(SYNC_BEGIN + '\r\x1b[2K' + w + SYNC_END);
    }
    await flush(term);

    const lines = bufferLines(term);
    expect(lines).toEqual(["wheni'm"]);
  });

  it('multi-keystroke burst without synchronized-output brackets also overwrites in place', async () => {
    // Some pi-tui code paths (per tui-main-screen.js) emit the CR+erase pair
    // without the 2026 brackets at all when no vertical movement is needed.
    // This isolates whether the brackets themselves are load-bearing for
    // correct behavior in our pipeline (they should be irrelevant to a
    // spec-compliant CR+EL2 interpretation).
    const words = ["w", "wh", "whe", "when", "wheni", "wheni'", "wheni'm"];
    for (const w of words) {
      manager.write('\r\x1b[2K' + w);
    }
    await flush(term);

    const lines = bufferLines(term);
    expect(lines).toEqual(["wheni'm"]);
  });

  it('several keystroke redraws coalesced into ONE write() call (matches production batching) still overwrite in place', async () => {
    // session/streamhub's BatchWindow coalesces rapid PTY output within a short
    // window before broadcasting one frame (see hub.go's onBatchFlush) -- so in
    // production, several of these per-keystroke redraws typically arrive
    // concatenated in a SINGLE write() call, not one write() per keystroke as
    // the tests above assume. This isolates whether that concatenation itself
    // (as opposed to each redraw individually) is what breaks.
    const SYNC_BEGIN = '\x1b[?2026h';
    const SYNC_END = '\x1b[?2026l';
    const words = ["w", "wh", "whe", "when", "wheni", "wheni'", "wheni'm"];

    const batched = words.map((w) => SYNC_BEGIN + '\r\x1b[2K' + w + SYNC_END).join('');
    manager.write(batched);
    await flush(term);

    const lines = bufferLines(term);
    expect(lines).toEqual(["wheni'm"]);
  });
});
