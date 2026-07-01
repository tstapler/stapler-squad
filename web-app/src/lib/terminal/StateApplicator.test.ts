/**
 * Tests for StateApplicator - MOSH-style terminal state synchronization
 */

import { StateApplicator } from './StateApplicator';
import { TerminalState, TerminalStateSchema, TerminalLine, TerminalLineSchema, CursorPosition, CursorPositionSchema, TerminalDimensions, TerminalDimensionsSchema, LineAttributes, LineAttributesSchema, TerminalDiff, TerminalDiffSchema } from '@/gen/session/v1/events_pb';
import { create } from "@bufbuild/protobuf";

// Mock requestAnimationFrame to queue callbacks and flush them on demand in tests.
// Synchronous inline execution breaks the rafId assignment (id is set AFTER callback returns),
// so we queue and flush explicitly after each applyState call.
let rafCallbacks: Array<FrameRequestCallback> = [];
let rafIdCounter = 0;

global.requestAnimationFrame = (callback: FrameRequestCallback): number => {
  rafIdCounter++;
  rafCallbacks.push(callback);
  return rafIdCounter;
};
global.cancelAnimationFrame = (id: number): void => {
  rafCallbacks = rafCallbacks.filter((_cb, _idx) => false); // clear all on cancel
};

function flushAnimationFrames(): void {
  while (rafCallbacks.length > 0) {
    const cbs = [...rafCallbacks];
    rafCallbacks = [];
    cbs.forEach(cb => cb(0));
  }
}

// Mock xterm Terminal
class MockTerminal {
  public rows = 24;
  public cols = 80;
  private writtenData: string[] = [];

  write(data: string): void {
    this.writtenData.push(data);
  }

  clear(): void {
    this.writtenData.push('CLEAR');
  }

  resize(cols: number, rows: number): void {
    this.cols = cols;
    this.rows = rows;
    this.writtenData.push(`RESIZE:${cols}x${rows}`);
  }

  getWrittenData(): string[] {
    return [...this.writtenData];
  }

  clearWrittenData(): void {
    this.writtenData = [];
  }
}

describe('StateApplicator', () => {
  let mockTerminal: MockTerminal;
  let stateApplicator: StateApplicator;

  beforeEach(() => {
    rafCallbacks = [];
    rafIdCounter = 0;
    mockTerminal = new MockTerminal();
    stateApplicator = new StateApplicator(mockTerminal as any);
  });

  // Helper to flush pending RAF callbacks after each applyState call
  const applyAndFlush = (state: TerminalState): boolean => {
    const result = stateApplicator.applyState(state);
    flushAnimationFrames();
    return result;
  };

  describe('sequence handling', () => {
    it('should apply state with sequence 1', () => {
      const state = createTestState(BigInt(1), ['Hello World']);

      const result = applyAndFlush(state);

      expect(result).toBe(true);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(1));
    });

    it('should ignore old sequence numbers', () => {
      // Apply sequence 5
      const state1 = createTestState(BigInt(5), ['First state']);
      applyAndFlush(state1);

      // Try to apply sequence 3 (older)
      const state2 = createTestState(BigInt(3), ['Old state']);
      const result = applyAndFlush(state2);

      expect(result).toBe(false);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(5));
    });

    it('should ignore duplicate sequence numbers', () => {
      // Apply sequence 3
      const state1 = createTestState(BigInt(3), ['First']);
      applyAndFlush(state1);

      // Try to apply sequence 3 again (duplicate)
      const state2 = createTestState(BigInt(3), ['Duplicate']);
      const result = applyAndFlush(state2);

      expect(result).toBe(false);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(3));
    });

    it('should accept future sequences', () => {
      // Apply sequence 1
      const state1 = createTestState(BigInt(1), ['First']);
      applyAndFlush(state1);

      // Apply sequence 10 (big jump)
      const state2 = createTestState(BigInt(10), ['Future']);
      const result = applyAndFlush(state2);

      expect(result).toBe(true);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(10));
    });
  });

  describe('terminal content application', () => {
    // Helper: join all written data into one string for substring checking
    const allWritten = () => mockTerminal.getWrittenData().join('');

    it('should clear terminal and write lines', () => {
      const lines = ['Line 1', 'Line 2 with ANSI \\x1b[31mred\\x1b[0m'];
      const state = createTestState(BigInt(1), lines);

      applyAndFlush(state);

      expect(mockTerminal.getWrittenData()).toContain('CLEAR');
      expect(allWritten()).toContain('\x1b[1;1H');
      expect(allWritten()).toContain('Line 1');
      expect(allWritten()).toContain('\x1b[2;1H');
      expect(allWritten()).toContain('Line 2 with ANSI \\x1b[31mred\\x1b[0m');
    });

    it('should handle empty lines', () => {
      const state = createTestState(BigInt(1), ['First line', '', 'Third line']);

      applyAndFlush(state);

      expect(allWritten()).toContain('\x1b[1;1H');
      expect(allWritten()).toContain('First line');
      expect(allWritten()).toContain('\x1b[2;1H'); // Empty line position
      expect(allWritten()).toContain('\x1b[3;1H');
      expect(allWritten()).toContain('Third line');
    });

    it('should stop writing when terminal rows exceeded', () => {
      // Create state with 2-row dimensions so terminal won't be resized back to 24
      const lines = ['Line 1', 'Line 2', 'Line 3', 'Line 4']; // More lines than terminal
      const state = createTestState(BigInt(1), lines);
      state.dimensions = create(TerminalDimensionsSchema, { rows: 2, cols: 80 });

      applyAndFlush(state);

      expect(allWritten()).toContain('Line 1');
      expect(allWritten()).toContain('Line 2');
      expect(allWritten()).not.toContain('Line 3'); // Should be dropped
    });
  });

  describe('dimension handling', () => {
    it('should resize terminal when dimensions change', () => {
      const state = createTestState(BigInt(1), ['Test']);
      state.dimensions = create(TerminalDimensionsSchema, { rows: 30, cols: 100 });

      applyAndFlush(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('RESIZE:100x30');
      expect(mockTerminal.cols).toBe(100);
      expect(mockTerminal.rows).toBe(30);
    });

    it('should not resize when dimensions match', () => {
      const state = createTestState(BigInt(1), ['Test']);
      state.dimensions = create(TerminalDimensionsSchema, { rows: 24, cols: 80 }); // Same as mock default

      mockTerminal.clearWrittenData();
      applyAndFlush(state);

      const written = mockTerminal.getWrittenData();
      expect(written).not.toContain('RESIZE:80x24');
    });
  });

  describe('cursor handling', () => {
    // Helper: join all written data into one string for substring checking
    const allWritten = () => mockTerminal.getWrittenData().join('');

    it('should position cursor correctly', () => {
      const state = createTestState(BigInt(1), ['Test line']);
      state.cursor = create(CursorPositionSchema, { row: 5, col: 10, visible: true });

      applyAndFlush(state);

      expect(allWritten()).toContain('\x1b[6;11H'); // 1-indexed in ANSI codes
      expect(allWritten()).toContain('\x1b[?25h'); // Show cursor
    });

    it('should hide cursor when not visible', () => {
      const state = createTestState(BigInt(1), ['Test line']);
      state.cursor = create(CursorPositionSchema, { row: 0, col: 0, visible: false });

      applyAndFlush(state);

      expect(allWritten()).toContain('\x1b[?25l'); // Hide cursor
    });

    it('should clamp out-of-bounds cursor position', () => {
      // Use state dimensions matching the small terminal to prevent auto-resize to 80x24
      const state = createTestState(BigInt(1), ['Test']);
      state.dimensions = create(TerminalDimensionsSchema, { rows: 5, cols: 10 });
      state.cursor = create(CursorPositionSchema, { row: 100, col: 200, visible: true }); // Way out of bounds

      applyAndFlush(state);

      expect(allWritten()).toContain('\x1b[5;10H'); // Clamped to max (1-indexed: 5x10)
    });
  });

  describe('reset functionality', () => {
    it('should reset sequence to 0', () => {
      // Apply some states
      applyAndFlush(createTestState(BigInt(5), ['Test']));
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(5));

      // Reset
      stateApplicator.resetSequence();
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(0));
    });

    it('should clear last state info on reset', () => {
      applyAndFlush(createTestState(BigInt(3), ['Test']));
      expect(stateApplicator.getLastStateInfo()).not.toBeNull();

      stateApplicator.resetSequence();
      expect(stateApplicator.getLastStateInfo()).toBeNull();
    });
  });

  describe('state info tracking', () => {
    it('should track last applied state info', () => {
      const state = createTestState(BigInt(7), ['Line 1', 'Line 2']);
      applyAndFlush(state);

      const info = stateApplicator.getLastStateInfo();
      expect(info).toEqual({
        sequence: BigInt(7),
        lines: 2,
        compression: undefined
      });
    });

    it('should check if sequence has been applied', () => {
      applyAndFlush(createTestState(BigInt(5), ['Test']));

      expect(stateApplicator.hasAppliedSequence(BigInt(3))).toBe(true); // Older
      expect(stateApplicator.hasAppliedSequence(BigInt(5))).toBe(true); // Current
      expect(stateApplicator.hasAppliedSequence(BigInt(7))).toBe(false); // Future
    });
  });

  describe('multi-byte UTF-8 across diffs', () => {
    it('StateApplicator_should_notEmitReplacementChar_When_multiByteSplitAcrossDiffs', () => {
      // "é" is U+00E9, encoded as 0xC3 0xA9 in UTF-8.
      // Split the two bytes across two consecutive TerminalDiff messages.
      // With { stream: true }, the decoder holds the first byte until the next
      // chunk arrives, emitting the full character instead of U+FFFD.

      const diff1 = create(TerminalDiffSchema, {
        fromSequence: BigInt(0),
        toSequence: BigInt(1),
        diffBytes: new Uint8Array([0xc3]), // first byte of "é"
        changedCells: 1,
      });

      const diff2 = create(TerminalDiffSchema, {
        fromSequence: BigInt(1),
        toSequence: BigInt(2),
        diffBytes: new Uint8Array([0xa9]), // second byte of "é"
        changedCells: 0,
      });

      stateApplicator.applyDiff(diff1);
      flushAnimationFrames(); // advance currentSequence to 1 before applying diff2

      stateApplicator.applyDiff(diff2);
      flushAnimationFrames();

      const written = mockTerminal.getWrittenData().join('');
      expect(written).not.toContain('�');
      expect(written).toContain('é');
    });
  });
});

// Helper function to create test states
function createTestState(sequence: bigint, lines: string[]): TerminalState {
  const terminalLines = lines.map(content =>
    create(TerminalLineSchema, {
      content: new TextEncoder().encode(content),
      attributes: create(LineAttributesSchema, {
        isEmpty: content.length === 0,
        asciiOnly: true
      })
    })
  );

  return create(TerminalStateSchema, {
    sequence,
    dimensions: create(TerminalDimensionsSchema, { rows: 24, cols: 80 }),
    lines: terminalLines,
    cursor: create(CursorPositionSchema, { row: 0, col: 0, visible: true })
  });
}