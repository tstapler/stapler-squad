
import { generateSingleCodeFrame } from './generators';
import { ESCAPE_CODE_LIBRARY } from './library';

describe('EscapeCodeFrameGenerator Erase Logic', () => {
  const width = 80;
  const height = 24;

  it('should use dynamic validation for Erase to End of Line', () => {
    // Find "Erase to End of Line" code
    const eraseLineCode = ESCAPE_CODE_LIBRARY.find(c => c.humanReadable === 'Erase to End of Line');
    if (!eraseLineCode) throw new Error('Erase to End of Line code not found');

    const frame = generateSingleCodeFrame(eraseLineCode.code, { width, height });

    expect(frame).not.toBeNull();
    if (frame) {
      // Should have 3 lines filled with X
      const expectedFill = 'X'.repeat(width - 1);
      expect(frame.content).toContain(`\x1b[6;1H${expectedFill}\n`);
      expect(frame.content).toContain(`\x1b[7;1H${expectedFill}\n`);
      expect(frame.content).toContain(`\x1b[8;1H${expectedFill}\n`);

      // Should position at 7;20 and erase
      expect(frame.content).toContain('\x1b[7;20H');
      expect(frame.content).toContain(eraseLineCode.sequence);

      // Validation textAbsent should be dynamic based on width
      const startCol = 20;
      const expectedErasedLength = width - startCol;
      const expectedAbsent = 'X'.repeat(expectedErasedLength);

      expect(frame.validation.textAbsent).toContain(expectedAbsent);
    }
  });

  it('should handle different widths correctly for Erase to End of Line', () => {
    const customWidth = 40;
    const eraseLineCode = ESCAPE_CODE_LIBRARY.find(c => c.humanReadable === 'Erase to End of Line');
    if (!eraseLineCode) throw new Error('Erase to End of Line code not found');

    const frame = generateSingleCodeFrame(eraseLineCode.code, { width: customWidth, height });

    if (frame) {
      const startCol = 20;
      const expectedErasedLength = customWidth - startCol;
      const expectedAbsent = 'X'.repeat(expectedErasedLength);
      expect(frame.validation.textAbsent).toContain(expectedAbsent);
    }
  });

  it('should use dynamic validation for Full Screen erasure', () => {
    const eraseAllCode = ESCAPE_CODE_LIBRARY.find(c => c.humanReadable === 'Erase All (Full Screen)');
    if (!eraseAllCode) throw new Error('Erase All (Full Screen) code not found');

    const frame = generateSingleCodeFrame(eraseAllCode.code, { width, height });

    if (frame) {
      expect(frame.content).toContain(eraseAllCode.sequence);
      // For full screen, we check for a subset of Xs being gone
      const expectedAbsent = 'X'.repeat(10);
      expect(frame.validation.textAbsent).toContain(expectedAbsent);
    }
  });
});
