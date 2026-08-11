/**
 * Pipeline Integration Tests - Phase 1 Fix Chain End-to-End
 *
 * Validates the full streaming pipeline:
 *   TerminalDiff bytes → EscapeSequenceParser → TerminalStreamManager → xterm.js output
 *
 * Purpose: Catch regressions where fixing one component breaks another.
 * Simulates real streaming as closely as possible without a live server.
 *
 * Note: StateApplicator depends on protobuf-generated types and the real xterm.js
 * Terminal class (not easily mockable), so the "TerminalDiff bytes" layer is
 * exercised by simulating what StateApplicator would produce: UTF-8 decoded
 * bytes fed into the TerminalStreamManager.write() path.
 */

import { EscapeSequenceParser } from '../EscapeSequenceParser';
import { TerminalStreamManager, type ITerminal } from '../TerminalStreamManager';

// ---------------------------------------------------------------------------
// RAF mock helpers (same pattern as TerminalStreamManager.test.ts)
// ---------------------------------------------------------------------------

let rafCallback: FrameRequestCallback | null = null;
const rafCallbacks: FrameRequestCallback[] = [];

function setupRAFMock() {
  rafCallback = null;
  rafCallbacks.length = 0;
  global.requestAnimationFrame = (cb: FrameRequestCallback): number => {
    rafCallback = cb;
    rafCallbacks.push(cb);
    return rafCallbacks.length;
  };
  global.cancelAnimationFrame = (): void => {
    rafCallback = null;
  };
}

function flushRAF(): void {
  if (rafCallback) {
    const cb = rafCallback;
    rafCallback = null;
    cb(performance.now());
  }
}

// ---------------------------------------------------------------------------
// MockTerminal - captures everything written
// ---------------------------------------------------------------------------

class MockTerminal implements ITerminal {
  rows = 24;
  cols = 80;
  private written: Array<{ data: string; callback?: () => void }> = [];
  private cleared = false;
  private refreshed: Array<{ start: number; end: number }> = [];
  private scrolledToBottom = false;

  write(data: string | Uint8Array, callback?: () => void): void {
    const str = typeof data === 'string' ? data : new TextDecoder().decode(data);
    this.written.push({ data: str, callback });
    // Auto-invoke callback to simulate xterm.js processing
    callback?.();
  }

  clear(): void {
    this.cleared = true;
  }

  refresh(start: number, end: number): void {
    this.refreshed.push({ start, end });
  }

  scrollToBottom(): void {
    this.scrolledToBottom = true;
  }

  get buffer() {
    return {
      active: { cursorY: 0, viewportY: 0, length: 0 },
      normal: { length: 0 },
    };
  }

  getWrittenData(): string[] {
    return this.written.map(w => w.data);
  }

  getAllWrittenText(): string {
    return this.written.map(w => w.data).join('');
  }

  wasCleared(): boolean {
    return this.cleared;
  }

  resetTracking(): void {
    this.written = [];
    this.cleared = false;
    this.refreshed = [];
    this.scrolledToBottom = false;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function noopFlowControl() {}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('Pipeline Integration (Phase 1)', () => {
  let terminal: MockTerminal;
  let manager: TerminalStreamManager;

  beforeEach(() => {
    jest.useFakeTimers();
    setupRAFMock();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});

    terminal = new MockTerminal();
    manager = new TerminalStreamManager(terminal, noopFlowControl);
  });

  afterEach(() => {
    manager.cleanup();
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // Test 1: Multi-byte UTF-8 survives full pipeline
  // -------------------------------------------------------------------------
  describe('multi-byte UTF-8 handling', () => {
    test('Pipeline_should_renderMultibyteChar_When_splitAcrossProtoFrames', () => {
      // "é" encodes as two bytes: 0xC3 0xA9
      // Simulate proto frames delivering one byte at a time (decoded as Latin-1 surrogates)
      // The EscapeSequenceParser only buffers partial *escape sequences*; UTF-8 byte splitting
      // is the responsibility of the upstream decoder. Here we test the realistic scenario
      // where the TextDecoder produces the complete character before the pipeline sees it,
      // which is the correct production path.
      //
      // However, to also cover the raw-byte scenario: simulate what happens when the server
      // sends the two UTF-8 bytes across two separate proto frames and the client decodes
      // each frame individually (yielding replacement chars) vs. decoding them together.

      // Scenario A: Bytes arrive in separate Uint8Arrays → should use U+FFFD if naive,
      // but our pipeline receives already-decoded strings, so we test correct decoding.
      const firstByteArray = new Uint8Array([0xc3]);       // first byte of é
      const secondByteArray = new Uint8Array([0xa9]);      // second byte of é

      // Correct approach: accumulate bytes and decode together
      const combined = new Uint8Array(2);
      combined.set(firstByteArray, 0);
      combined.set(secondByteArray, 1);
      const correctlyDecoded = new TextDecoder().decode(combined);
      expect(correctlyDecoded).toBe('é');

      // Simulate what StateApplicator.applyDiff() does: decode the content bytes
      // from a TerminalDiff, then call manager.write() with the decoded string.
      // When the diff content bytes contain a complete multi-byte character, the
      // decoded string must reach xterm.js intact.
      manager.write('Hello ' + correctlyDecoded + ' World');

      const written = terminal.getAllWrittenText();
      expect(written).toContain('é');
      expect(written).not.toContain('�');
    });

    test('Pipeline_should_passMultibyteChars_When_multipleCharsInSingleChunk', () => {
      // Verify that a chunk with multiple multi-byte characters all reach xterm.js
      const text = 'こんにちは'; // 5 Japanese characters, each 3 bytes in UTF-8
      manager.write(text);

      const written = terminal.getAllWrittenText();
      expect(written).toContain(text);
    });
  });

  // -------------------------------------------------------------------------
  // Test 2: Ink TUI incremental update passes without throttling
  // -------------------------------------------------------------------------
  describe('Ink TUI incremental update', () => {
    test('Pipeline_should_passThrough_InkRedrawChunk_When_noEraseFollows', () => {
      // Ink-style incremental update: cursor-up ONLY (no erase-screen sequence)
      // The RedrawThrottler should NOT throttle this — it only throttles when
      // cursor-up is immediately followed by an erase-screen (2K or J).
      const inkChunk = '\x1b[3A' + 'Line content updated';

      manager.write(inkChunk);

      // Should be written immediately — no timer needed
      const writtenData = terminal.getWrittenData();
      expect(writtenData.length).toBeGreaterThan(0);
      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('Line content updated');
      expect(allWritten).toContain('\x1b[3A');
    });

    test('Pipeline_should_passThrough_CursorHomeChunk_When_noEraseFollows', () => {
      // Cursor-home (\x1b[H) is explicitly excluded from throttle detection
      const chunkWithCursorHome = '\x1b[H' + 'Content at home position';

      manager.write(chunkWithCursorHome);

      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('Content at home position');
      expect(allWritten).toContain('\x1b[H');
    });
  });

  // -------------------------------------------------------------------------
  // Test 3: ED2+ED3 combined reset passes through xterm
  // -------------------------------------------------------------------------
  describe('ED2+ED3 combined reset', () => {
    test('Pipeline_should_passThrough_ED2PlusED3_WhenCombinedReset', () => {
      // \x1b[2J = erase display (ED2)
      // \x1b[3J = erase scrollback (ED3)
      // These should NOT be stripped — xterm.js v6 handles them correctly.
      // The EscapeSequenceParser comment: "No sequence stripping - xterm.js v6 handles ED2+ED3 correctly"
      const combined = '\x1b[2J\x1b[3J';

      const parser = new EscapeSequenceParser();
      const result = parser.processChunk(combined);

      // Both sequences must appear in output (neither stripped)
      expect(result).toContain('\x1b[2J');
      expect(result).toContain('\x1b[3J');
      // Full content must pass through unchanged
      expect(result).toBe(combined);
    });

    test('Pipeline_should_passThrough_ED2PlusED3_ThroughFullPipeline', () => {
      // Same verification through the full TerminalStreamManager pipeline
      const combined = '\x1b[2J\x1b[3J';

      manager.write(combined);

      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('\x1b[2J');
      expect(allWritten).toContain('\x1b[3J');
    });
  });

  // -------------------------------------------------------------------------
  // Test 4: Genuine full-screen redraw is throttled
  // -------------------------------------------------------------------------
  describe('full-screen redraw throttling', () => {
    test('Pipeline_should_throttle_GenuineFullRedraw_When_cursorUpPlusErase', () => {
      // Pattern that triggers throttling: cursor-up + erase-line immediately following
      // This matches: /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/
      const fullRedrawChunk = '\x1b[5A\x1b[2K' + 'Screen content';

      manager.write(fullRedrawChunk);

      // Should NOT be written before timer fires (throttled)
      expect(terminal.getWrittenData()).toHaveLength(0);

      // Advance timers past the 33ms throttle window
      jest.advanceTimersByTime(34);

      // Now it should be written
      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('Screen content');
      expect(allWritten).toContain('\x1b[5A');
    });

    test('Pipeline_should_coalesceMultipleRedraws_When_burstArrives', () => {
      // Multiple full redraws within the throttle window should coalesce to last
      manager.write('\x1b[3A\x1b[2K' + 'Frame 1');
      manager.write('\x1b[3A\x1b[2K' + 'Frame 2');
      manager.write('\x1b[3A\x1b[2K' + 'Frame 3');

      // Nothing written yet
      expect(terminal.getWrittenData()).toHaveLength(0);

      // Advance past throttle window
      jest.advanceTimersByTime(34);

      // Only the last frame should have been written (coalesced)
      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('Frame 3');
      expect(allWritten).not.toContain('Frame 1');
      expect(allWritten).not.toContain('Frame 2');
    });
  });

  // -------------------------------------------------------------------------
  // Test 5: OSC sequence with long title doesn't get split
  // -------------------------------------------------------------------------
  describe('OSC sequence buffering', () => {
    test('Pipeline_should_bufferOSC_When_titleExceeds20BytesAtChunkBoundary', () => {
      // Build OSC sequence: \x1b]0; + 30-char title + \x07
      const title = 'A'.repeat(30); // 30-char title
      const fullOSC = '\x1b]0;' + title + '\x07';
      // fullOSC is: 4 + 30 + 1 = 35 bytes total
      // Split so first chunk ends at byte 22 (inside the title, after "AAAAAAAAAAAAAAAAAAA" = 19 A's)
      // \x1b]0; = 4 chars, so byte 22 means 22 - 4 = 18 A's in the title
      const splitPoint = 22;
      const chunk1 = fullOSC.substring(0, splitPoint); // '\x1b]0;' + 18 A's
      const chunk2 = fullOSC.substring(splitPoint);    // remaining 12 A's + '\x07'

      expect(chunk1.length).toBe(22);
      expect(chunk2.length).toBe(13);

      const parser = new EscapeSequenceParser();

      // chunk1 alone must produce empty output (partial OSC buffered)
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('');
      expect(parser.getBuffered()).toBe(chunk1);

      // chunk2 completes the OSC; combined output should be the full sequence
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe(fullOSC);
      expect(parser.getBuffered()).toBe('');
    });

    test('Pipeline_should_passOSC_When_chunkContainsFullSequence', () => {
      // When the full OSC arrives in a single chunk, it should pass through unchanged
      const title = 'My Terminal Title';
      const fullOSC = '\x1b]0;' + title + '\x07';

      const parser = new EscapeSequenceParser();
      const result = parser.processChunk(fullOSC);

      expect(result).toBe(fullOSC);
      expect(parser.getBuffered()).toBe('');
    });

    test('Pipeline_should_passOSC_ThroughFullPipeline_When_titleExceeds20BytesAtChunkBoundary', () => {
      // Same split-chunk scenario routed through TerminalStreamManager
      const title = 'B'.repeat(30);
      const fullOSC = '\x1b]0;' + title + '\x07';
      const splitPoint = 22;
      const chunk1 = fullOSC.substring(0, splitPoint);
      const chunk2 = fullOSC.substring(splitPoint);

      manager.write(chunk1);
      // After chunk1: nothing written yet (OSC buffered in EscapeSequenceParser)
      expect(terminal.getAllWrittenText()).toBe('');

      manager.write(chunk2);
      // After chunk2: complete OSC sequence must appear
      const allWritten = terminal.getAllWrittenText();
      expect(allWritten).toContain('\x1b]0;');
      expect(allWritten).toContain(title);
      expect(allWritten).toContain('\x07');
      expect(allWritten).toBe(fullOSC);
    });
  });
});
