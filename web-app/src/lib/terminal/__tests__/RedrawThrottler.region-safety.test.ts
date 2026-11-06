/**
 * RedrawThrottler content-loss regression (real @xterm/xterm, no mocks).
 *
 * \x1b[NA\x1b[2K (cursor-up N, erase ONE line) is the same leading byte shape
 * for two different things an Ink-style multi-line TUI emits: a genuine
 * full-viewport redraw (many such pairs bundled in one write(), safe to
 * coalesce) and a single targeted line update — a spinner tick, a token
 * counter — where N picks out just that line. The throttler used to treat
 * any two "redraw-shaped" chunks arriving within its 33ms window as
 * interchangeable and kept only the latest, so two independent single-line
 * updates to DIFFERENT lines (different N) silently lost the earlier one:
 * nothing ever resends it, so that line goes stale on screen. See
 * docs/bugs/open/BUG-113-redrawthrottler-drops-unrelated-single-line-updates.md.
 */
import { Terminal } from '@xterm/xterm';
import { TerminalStreamManager, type ITerminal } from '../TerminalStreamManager';

function wrap(term: Terminal): ITerminal {
  return term as unknown as ITerminal;
}

describe('RedrawThrottler region safety', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  it('preserves both updates when two different-region redraws arrive within the throttle window', () => {
    const term = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    const manager = new TerminalStreamManager(wrap(term), () => {});

    term.write('line1\r\nline2\r\nline3\r\nline4\r\nline5');

    // Frame A: Ink-style single-line redraw 2 lines up from the cursor.
    manager.write('\x1b[2A\x1b[2KSPINNER-TICK-1\x1b[2B\r');

    // Frame B, 10ms later (inside the 33ms window), targets a DIFFERENT line
    // (4 lines up) — a different cursor-up count, so a different region.
    jest.advanceTimersByTime(10);
    manager.write('\x1b[4A\x1b[2KTOKENS: 42\x1b[4B\r');

    jest.advanceTimersByTime(50);

    const rendered = Array.from({ length: 5 }, (_, y) => term.buffer.active.getLine(y)!.translateToString(true)).join('\n');
    expect(rendered).toContain('SPINNER-TICK-1');
    expect(rendered).toContain('TOKENS: 42');
  });

  it('still coalesces repeated same-region redraws to only the latest (the original flicker fix)', () => {
    const term = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    const manager = new TerminalStreamManager(wrap(term), () => {});
    term.write('line1\r\nline2\r\nline3');

    manager.write('\x1b[2A\x1b[2KFRAME-1\x1b[2B\r');
    jest.advanceTimersByTime(10);
    manager.write('\x1b[2A\x1b[2KFRAME-2\x1b[2B\r');
    jest.advanceTimersByTime(50);

    const rendered = Array.from({ length: 3 }, (_, y) => term.buffer.active.getLine(y)!.translateToString(true)).join('\n');
    expect(rendered).toContain('FRAME-2');
    expect(rendered).not.toContain('FRAME-1');
  });
});
