/**
 * Unit tests for EscapeSequenceParser
 *
 * Tests ANSI escape sequence boundary detection to ensure control codes
 * are never split mid-sequence, preventing terminal corruption.
 *
 * Reference: https://xtermjs.org/docs/guides/flowcontrol/
 */

import { EscapeSequenceParser } from '../EscapeSequenceParser';

describe('EscapeSequenceParser', () => {
  let parser: EscapeSequenceParser;

  beforeEach(() => {
    parser = new EscapeSequenceParser();
  });

  describe('CSI Sequences (Control Sequence Introducer)', () => {
    test('buffers partial CSI color sequence', () => {
      const chunk1 = 'Hello \x1b[31';  // Partial: \x1b[31 (red color)
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Hello ');
      expect(parser.getBuffered()).toBe('\x1b[31');

      const chunk2 = 'mWorld';  // Completes: \x1b[31m
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b[31mWorld');
      expect(parser.getBuffered()).toBe('');
    });

    test('buffers partial CSI cursor positioning', () => {
      const chunk1 = 'Text \x1b[10;2';  // Partial: \x1b[10;2 (cursor position)
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Text ');
      expect(parser.getBuffered()).toBe('\x1b[10;2');

      const chunk2 = '0HMore';  // Completes: \x1b[10;20H
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b[10;20HMore');
    });

    test('passes complete CSI sequences through unchanged', () => {
      const chunk = 'Hello \x1b[31mRed\x1b[0m World';
      const result = parser.processChunk(chunk);
      expect(result).toBe(chunk);
      expect(parser.getBuffered()).toBe('');
    });

    test('handles multiple CSI sequences in one chunk', () => {
      const chunk = '\x1b[31mRed\x1b[0m \x1b[32mGreen\x1b[0m';
      const result = parser.processChunk(chunk);
      expect(result).toBe(chunk);
      expect(parser.getBuffered()).toBe('');
    });

    test('buffers CSI with intermediate bytes', () => {
      const chunk1 = 'Text \x1b[!';  // Partial with intermediate byte
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Text ');
      expect(parser.getBuffered()).toBe('\x1b[!');

      const chunk2 = 'pDone';  // Complete: \x1b[!p (soft reset)
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b[!pDone');
    });
  });

  describe('OSC Sequences (Operating System Command)', () => {
    test('buffers partial OSC with BEL terminator', () => {
      const chunk1 = 'Title: \x1b]0;My';  // Partial OSC (set title)
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Title: ');
      expect(parser.getBuffered()).toBe('\x1b]0;My');

      const chunk2 = ' Title\x07Done';  // Complete with BEL (\x07)
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b]0;My Title\x07Done');
    });

    test('buffers partial OSC with ST terminator', () => {
      const chunk1 = 'Color: \x1b]4;1;rgb';  // Partial OSC (set color)
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Color: ');

      const chunk2 = ':ff/00/00\x1b\\Done';  // Complete with ST (\x1b\\)
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b]4;1;rgb:ff/00/00\x1b\\Done');
    });

    test('passes complete OSC sequences through', () => {
      const chunk = 'Before\x1b]0;Terminal Title\x07After';
      const result = parser.processChunk(chunk);
      expect(result).toBe(chunk);
    });
  });

  describe('Simple Escape Sequences', () => {
    test('buffers partial simple escape', () => {
      const chunk1 = 'Text \x1b';  // Just ESC character
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Text ');
      expect(parser.getBuffered()).toBe('\x1b');

      const chunk2 = '7More';  // Complete: \x1b7 (save cursor)
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b7More');
    });

    test('passes complete simple escape through', () => {
      const chunk = 'Save\x1b7Position';
      const result = parser.processChunk(chunk);
      expect(result).toBe(chunk);
    });
  });

  describe('Edge Cases', () => {
    test('handles empty chunks', () => {
      const result = parser.processChunk('');
      expect(result).toBe('');
      expect(parser.getBuffered()).toBe('');
    });

    test('handles chunk with only escape character at end', () => {
      const chunk = 'Hello World\x1b';
      const result = parser.processChunk(chunk);
      expect(result).toBe('Hello World');
      expect(parser.getBuffered()).toBe('\x1b');
    });

    test('handles very long CSI sequence', () => {
      const chunk1 = 'Text \x1b[38;2;255;128;0';  // Partial RGB color
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Text ');

      const chunk2 = 'mOrange';  // Complete
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b[38;2;255;128;0mOrange');
    });

    test('reset clears buffered sequence', () => {
      parser.processChunk('Text \x1b[31');
      expect(parser.getBuffered()).toBe('\x1b[31');

      parser.reset();
      expect(parser.getBuffered()).toBe('');

      // Subsequent chunk should not use old buffer
      const result = parser.processChunk('mRed');
      expect(result).toBe('mRed');  // Not treated as continuation
    });

    test('handles multiple partial sequences across many chunks', () => {
      const result1 = parser.processChunk('A\x1b');
      expect(result1).toBe('A');

      const result2 = parser.processChunk('[');
      expect(result2).toBe('');

      const result3 = parser.processChunk('3');
      expect(result3).toBe('');

      const result4 = parser.processChunk('1');
      expect(result4).toBe('');

      const result5 = parser.processChunk('mB');
      expect(result5).toBe('\x1b[31mB');
    });
  });

  describe('Mixed Content', () => {
    test('handles text with multiple partial sequences', () => {
      const chunk1 = 'Red: \x1b[31mHello\x1b';
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('Red: \x1b[31mHello');
      expect(parser.getBuffered()).toBe('\x1b');

      const chunk2 = '[0m Green: \x1b[32mWorld\x1b[0m';
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b[0m Green: \x1b[32mWorld\x1b[0m');
    });

    test('handles CSI followed by OSC', () => {
      const chunk1 = '\x1b[31mRed\x1b]0;Title';
      const result1 = parser.processChunk(chunk1);
      expect(result1).toBe('\x1b[31mRed');
      expect(parser.getBuffered()).toBe('\x1b]0;Title');

      const chunk2 = '\x07\x1b[0mNormal';
      const result2 = parser.processChunk(chunk2);
      expect(result2).toBe('\x1b]0;Title\x07\x1b[0mNormal');
    });
  });

  describe('Performance and Robustness', () => {
    test('handles large chunks efficiently', () => {
      const largeText = 'x'.repeat(10000);
      const chunk = `Text ${largeText} More`;

      const start = performance.now();
      const result = parser.processChunk(chunk);
      const duration = performance.now() - start;

      expect(result).toBe(chunk);
      expect(duration).toBeLessThan(10); // Should process in < 10ms
    });

    test('handles chunk boundary at escape start', () => {
      const chunk1 = 'Text\x1b';
      const chunk2 = '[31mRed';

      const result1 = parser.processChunk(chunk1);
      const result2 = parser.processChunk(chunk2);

      expect(result1).toBe('Text');
      expect(result2).toBe('\x1b[31mRed');
    });

    test('handles invalid escape sequences gracefully', () => {
      // Invalid sequence should be treated as complete after terminator
      const chunk = 'Text \x1b[999xMore';
      const result = parser.processChunk(chunk);

      // Parser should treat 'x' as terminator even if invalid
      expect(result).toBe(chunk);
    });
  });

  describe('Real-World Scenarios', () => {
    test('Claude Code color animations', () => {
      // Simulates Claude Code rewriting lines with colors
      const chunks = [
        '\x1b[2K\r\x1b[34m[1/10]\x1b[0m Processing',
        '...\x1b[2K\r\x1b[34m[2/10]\x1b',
        '[0m Processing...\x1b[2K\r\x1b[34m[3/',
        '10]\x1b[0m Complete'
      ];

      const results: string[] = [];
      chunks.forEach(chunk => {
        results.push(parser.processChunk(chunk));
      });

      // All escape sequences should be complete in output
      const fullOutput = results.join('');
      expect(fullOutput).toContain('\x1b[34m[1/10]\x1b[0m');
      expect(fullOutput).toContain('\x1b[34m[2/10]\x1b[0m');
      expect(fullOutput).toContain('\x1b[34m[3/10]\x1b[0m');
    });

    test('progress bar with cursor positioning', () => {
      const chunks = [
        'Progress: \x1b[s[',
        '          ]\x1b[u\x1b[2C',
        '==========\x1b[11C'
      ];

      const results: string[] = [];
      chunks.forEach(chunk => {
        results.push(parser.processChunk(chunk));
      });

      // Should preserve all control sequences
      const fullOutput = results.join('');
      expect(fullOutput).toContain('\x1b[s');  // Save cursor
      expect(fullOutput).toContain('\x1b[u');  // Restore cursor
      expect(fullOutput).toContain('\x1b[2C'); // Cursor forward
    });
  });
});
