/**
 * Integration test for AC7 (terminal-visibility-resync): proves that writing
 * the server's clear+home-prefixed full-resync payload through a REAL
 * TerminalStreamManager into a REAL @xterm/xterm Terminal produces a genuine
 * full repaint — stale/corrupted prior content is fully gone from the
 * rendered buffer, not merely "the write() mock was called."
 *
 * Deliberately does NOT mock '@xterm/xterm' or TerminalStreamManager: the
 * whole point of this test is to exercise actual xterm.js ANSI parsing
 * (DECSTR + ED2 + CUP) rather than assume it behaves a particular way.
 *
 * See project_plans/terminal-visibility-resync/implementation/plan.md,
 * Phase 3 / Epic 3.1 / Story 3.1.1 for the Given-When-Then this test encodes.
 */

import { Terminal } from '@xterm/xterm';
import { TerminalStreamManager, type ITerminal } from '../TerminalStreamManager';

// Mirrors ansiSnapshotPrefix in server/services/connectrpc_websocket.go
// (ansiDECSTR + ansiEraseScreen + ansiCursorHome): DECSTR resets terminal
// modes, ED2 erases the screen, CUP homes the cursor.
// keep in sync with ansiSnapshotPrefix (server/services/connectrpc_websocket.go)
const clearAndHome = '\x1b[!p\x1b[2J\x1b[H';

describe('TerminalStreamManager (AC7 full-resync integration)', () => {
  let terminal: Terminal;
  let manager: TerminalStreamManager;

  beforeEach(() => {
    terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    // Note: intentionally NOT calling terminal.open(...) — xterm.js's core
    // write()/buffer/parser machinery works headlessly without a DOM
    // container; only renderer addons (canvas/webgl/dom) require open(),
    // and none are loaded here.
    manager = new TerminalStreamManager(terminal as unknown as ITerminal, jest.fn());
  });

  afterEach(() => {
    terminal.dispose();
  });

  it('write_should_produceCleanFullRepaint_When_givenClearAndHomePrefixedFreshContent', async () => {
    // Given: the terminal has stale/corrupted content already rendered,
    // simulating what a backgrounded tab's coalesced/dropped deltas would
    // leave on screen.
    await new Promise<void>((resolve) => {
      terminal.write('STALE LINE ONE\r\nSTALE LINE TWO — CORRUPTED\r\n', () => resolve());
    });

    // Sanity check the seed actually landed before proceeding — otherwise a
    // false pass could result from the seed silently no-oping.
    expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('STALE LINE ONE');

    // When: a real resync payload (clearAndHome + fresh content) is written
    // through TerminalStreamManager.write() — the same entry point the
    // client uses for a plain `output` message carrying a CurrentPaneRequest
    // resync response.
    await new Promise<void>((resolve) => {
      const done = () => resolve();
      // TerminalStreamManager.write() does not expose a completion
      // callback, so wrap the underlying terminal write completion instead
      // by writing directly after manager.write() has synchronously handed
      // off to terminal.write() (RedrawThrottler passes both the seed and
      // this payload through synchronously — neither matches the throttler's
      // cursor-up+erase pattern, since clearAndHome starts with \x1b[!p, not
      // \x1b[<N>A).
      manager.write(clearAndHome + 'FRESH LINE ONE\r\nFRESH LINE TWO\r\n');
      // Yield until xterm has finished parsing the write synchronously
      // queued above by polling the parser via a follow-up no-op write with
      // a callback (guarantees ordering without fake timers).
      terminal.write('', done);
    });

    // Then: the buffer shows ONLY the fresh content...
    expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('FRESH LINE ONE');
    expect(terminal.buffer.active.getLine(1)?.translateToString(true)).toBe('FRESH LINE TWO');

    // ...and no leftover glyphs from the stale content survive anywhere in
    // the populated buffer (a true full repaint, not just a two-line
    // coincidental match).
    let sawStale = false;
    let sawCorrupted = false;
    for (let i = 0; i < terminal.buffer.active.length; i++) {
      const lineText = terminal.buffer.active.getLine(i)?.translateToString(true) ?? '';
      if (lineText.includes('STALE')) sawStale = true;
      if (lineText.includes('CORRUPTED')) sawCorrupted = true;
    }
    expect(sawStale).toBe(false);
    expect(sawCorrupted).toBe(false);
  });
});
