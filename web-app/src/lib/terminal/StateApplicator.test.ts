/**
 * Tests for StateApplicator - MOSH-style terminal state synchronization
 */

import { StateApplicator } from './StateApplicator';
import { TerminalState, TerminalLine, CursorPosition, TerminalDimensions, LineAttributes } from '@/gen/session/v1/events_pb';

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
    mockTerminal = new MockTerminal();
    stateApplicator = new StateApplicator(mockTerminal as any);
  });

  describe('sequence handling', () => {
    it('should apply state with sequence 1', () => {
      const state = createTestState(BigInt(1), ['Hello World']);

      const result = stateApplicator.applyState(state);

      expect(result).toBe(true);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(1));
    });

    it('should ignore old sequence numbers', () => {
      // Apply sequence 5
      const state1 = createTestState(BigInt(5), ['First state']);
      stateApplicator.applyState(state1);

      // Try to apply sequence 3 (older)
      const state2 = createTestState(BigInt(3), ['Old state']);
      const result = stateApplicator.applyState(state2);

      expect(result).toBe(false);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(5));
    });

    it('should ignore duplicate sequence numbers', () => {
      // Apply sequence 3
      const state1 = createTestState(BigInt(3), ['First']);
      stateApplicator.applyState(state1);

      // Try to apply sequence 3 again (duplicate)
      const state2 = createTestState(BigInt(3), ['Duplicate']);
      const result = stateApplicator.applyState(state2);

      expect(result).toBe(false);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(3));
    });

    it('should accept future sequences', () => {
      // Apply sequence 1
      const state1 = createTestState(BigInt(1), ['First']);
      stateApplicator.applyState(state1);

      // Apply sequence 10 (big jump)
      const state2 = createTestState(BigInt(10), ['Future']);
      const result = stateApplicator.applyState(state2);

      expect(result).toBe(true);
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(10));
    });
  });

  describe('terminal content application', () => {
    it('should clear terminal and write lines', () => {
      const lines = ['Line 1', 'Line 2 with ANSI \\x1b[31mred\\x1b[0m'];
      const state = createTestState(BigInt(1), lines);

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('CLEAR');
      expect(written).toContain('\x1b[1;1HLine 1');
      expect(written).toContain('\x1b[2;1HLine 2 with ANSI \\x1b[31mred\\x1b[0m');
    });

    it('should handle empty lines', () => {
      const state = createTestState(BigInt(1), ['First line', '', 'Third line']);

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('\x1b[1;1HFirst line');
      expect(written).toContain('\x1b[2;1H'); // Empty line
      expect(written).toContain('\x1b[3;1HThird line');
    });

    it('should stop writing when terminal rows exceeded', () => {
      mockTerminal.rows = 2; // Small terminal
      const lines = ['Line 1', 'Line 2', 'Line 3', 'Line 4']; // More lines than terminal
      const state = createTestState(BigInt(1), lines);

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('\x1b[1;1HLine 1');
      expect(written).toContain('\x1b[2;1HLine 2');
      expect(written).not.toContain('\x1b[3;1HLine 3'); // Should be dropped
    });
  });

  describe('dimension handling', () => {
    it('should resize terminal when dimensions change', () => {
      const state = createTestState(BigInt(1), ['Test']);
      state.dimensions = new TerminalDimensions({ rows: 30, cols: 100 });

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('RESIZE:100x30');
      expect(mockTerminal.cols).toBe(100);
      expect(mockTerminal.rows).toBe(30);
    });

    it('should not resize when dimensions match', () => {
      const state = createTestState(BigInt(1), ['Test']);
      state.dimensions = new TerminalDimensions({ rows: 24, cols: 80 }); // Same as mock default

      mockTerminal.clearWrittenData();
      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).not.toContain('RESIZE:80x24');
    });
  });

  describe('cursor handling', () => {
    it('should position cursor correctly', () => {
      const state = createTestState(BigInt(1), ['Test line']);
      state.cursor = new CursorPosition({ row: 5, col: 10, visible: true });

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('\x1b[6;11H'); // 1-indexed in ANSI codes
      expect(written).toContain('\x1b[?25h'); // Show cursor
    });

    it('should hide cursor when not visible', () => {
      const state = createTestState(BigInt(1), ['Test line']);
      state.cursor = new CursorPosition({ row: 0, col: 0, visible: false });

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('\x1b[?25l'); // Hide cursor
    });

    it('should clamp out-of-bounds cursor position', () => {
      mockTerminal.rows = 5;
      mockTerminal.cols = 10;
      const state = createTestState(BigInt(1), ['Test']);
      state.cursor = new CursorPosition({ row: 100, col: 200, visible: true }); // Way out of bounds

      stateApplicator.applyState(state);

      const written = mockTerminal.getWrittenData();
      expect(written).toContain('\x1b[5;10H'); // Clamped to max (1-indexed: 5x10)
    });
  });

  describe('reset functionality', () => {
    it('should reset sequence to 0', () => {
      // Apply some states
      stateApplicator.applyState(createTestState(BigInt(5), ['Test']));
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(5));

      // Reset
      stateApplicator.resetSequence();
      expect(stateApplicator.getCurrentSequence()).toBe(BigInt(0));
    });

    it('should clear last state info on reset', () => {
      stateApplicator.applyState(createTestState(BigInt(3), ['Test']));
      expect(stateApplicator.getLastStateInfo()).not.toBeNull();

      stateApplicator.resetSequence();
      expect(stateApplicator.getLastStateInfo()).toBeNull();
    });
  });

  describe('state info tracking', () => {
    it('should track last applied state info', () => {
      const state = createTestState(BigInt(7), ['Line 1', 'Line 2']);
      stateApplicator.applyState(state);

      const info = stateApplicator.getLastStateInfo();
      expect(info).toEqual({
        sequence: BigInt(7),
        lines: 2,
        compression: undefined
      });
    });

    it('should check if sequence has been applied', () => {
      stateApplicator.applyState(createTestState(BigInt(5), ['Test']));

      expect(stateApplicator.hasAppliedSequence(BigInt(3))).toBe(true); // Older
      expect(stateApplicator.hasAppliedSequence(BigInt(5))).toBe(true); // Current
      expect(stateApplicator.hasAppliedSequence(BigInt(7))).toBe(false); // Future
    });
  });
});

// Helper function to create test states
function createTestState(sequence: bigint, lines: string[]): TerminalState {
  const terminalLines = lines.map(content =>
    new TerminalLine({
      content: new TextEncoder().encode(content),
      attributes: new LineAttributes({
        isEmpty: content.length === 0,
        asciiOnly: true
      })
    })
  );

  return new TerminalState({
    sequence,
    dimensions: new TerminalDimensions({ rows: 24, cols: 80 }),
    lines: terminalLines,
    cursor: new CursorPosition({ row: 0, col: 0, visible: true })
  });
}