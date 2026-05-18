import { truncateMiddle } from './truncateMiddle';

describe('truncateMiddle', () => {
  describe('when name is shorter than or equal to maxLen', () => {
    it('returns unchanged when shorter than maxLen', () => {
      const result = truncateMiddle('short.txt', 20);
      expect(result).toBe('short.txt');
    });

    it('returns unchanged when exactly at maxLen', () => {
      const result = truncateMiddle('medium-name.tsx', 15);
      expect(result).toBe('medium-name.tsx');
    });

    it('returns unchanged when name is exactly maxLen length', () => {
      const result = truncateMiddle('filename.ts', 11);
      expect(result).toBe('filename.ts');
    });
  });

  describe('when name is longer than maxLen', () => {
    it('truncates normal filename preserving extension', () => {
      const result = truncateMiddle('very-long-component-name.tsx', 20);
      expect(result).toMatch(/very.+\.tsx$/);
      expect(result.length).toBe(20);
    });

    it('shows head + ellipsis + tail + extension for typical case', () => {
      const result = truncateMiddle('very-long-component-name.tsx', 18);
      // With maxLen=18 and ".tsx" (4 chars), keep = 18 - 4 - 1 = 13
      // head = ceil(13 * 0.6) = 8, tail = 5
      // "very-lon" + "…" + "name" + ".tsx" (but tail is 5 chars from base.length - 5)
      expect(result).toMatch(/^very-lon….+\.tsx$/);
      expect(result.length).toBe(18);
    });
  });

  describe('when name has no extension', () => {
    it('truncates middle for name without extension', () => {
      const result = truncateMiddle('very-long-component-name', 15);
      expect(result.length).toBe(15);
      expect(result).toContain('…');
      expect(result).toMatch(/^very.+name$/);
    });

    it('preserves meaningful parts when no extension', () => {
      const result = truncateMiddle('abcdefghijklmnop', 10);
      expect(result.length).toBe(10);
      expect(result).toContain('…');
      // Should have head + ellipsis + tail
      const parts = result.split('…');
      expect(parts).toHaveLength(2);
      expect(parts[0]).toBeTruthy();
      expect(parts[1]).toBeTruthy();
    });
  });

  describe('when maxLen is very small', () => {
    it('falls back to right-truncation when maxLen < 5', () => {
      const result = truncateMiddle('verylongname.tsx', 4);
      expect(result.length).toBe(4);
      expect(result).toMatch(/…$/);
    });

    it('falls back to right-truncation for maxLen=4', () => {
      const result = truncateMiddle('filename.txt', 4);
      expect(result).toBe('fil…');
      expect(result.length).toBe(4);
    });

    it('falls back to right-truncation for maxLen=3', () => {
      const result = truncateMiddle('abc.txt', 3);
      expect(result).toBe('ab…');
      expect(result.length).toBe(3);
    });

    it('returns minimal output for maxLen=2', () => {
      const result = truncateMiddle('filename', 2);
      expect(result.length).toBe(2);
      expect(result).toBe('f…');
    });

    it('handles maxLen=1 by showing only ellipsis', () => {
      const result = truncateMiddle('verylongname', 1);
      expect(result).toBe('…');
    });
  });

  describe('when name is empty or whitespace', () => {
    it('returns empty string unchanged', () => {
      const result = truncateMiddle('', 20);
      expect(result).toBe('');
    });

    it('returns whitespace-only string unchanged', () => {
      const result = truncateMiddle('   ', 20);
      expect(result).toBe('   ');
    });
  });

  describe('when extension is very long', () => {
    it('preserves long extension and truncates base', () => {
      // maxLen=15, ext=".verylongext" (12 chars), keep = 15 - 12 - 1 = 2
      // Should have at least 1 head + 1 tail
      const result = truncateMiddle('abcdefghij.verylongext', 15);
      expect(result).toBe('a…j.verylongext');
      expect(result.length).toBe(15);
    });

    it('falls back to right-truncation when suffix too long', () => {
      // maxLen=10, ext=".verylongextension" (18 chars) - extension alone is too long
      // keep = 10 - 18 - 1 = negative, so falls back to right truncation
      const result = truncateMiddle('short.verylongextension', 10);
      expect(result.length).toBe(10);
      expect(result).toContain('…');
    });
  });

  describe('when name has multiple dots', () => {
    it('preserves only last extension (.ts)', () => {
      const result = truncateMiddle('file.test.ts', 9);
      expect(result).toMatch(/\.ts$/);
      expect(result).toContain('…');
    });

    it('treats only final dot-extension as suffix', () => {
      const result = truncateMiddle('my.component.file.tsx', 18);
      expect(result).toMatch(/\.tsx$/);
      expect(result.length).toBe(18);
    });

    it('handles filename like .gitignore', () => {
      const result = truncateMiddle('.gitignore', 30);
      expect(result).toBe('.gitignore');
    });

    it('truncates hidden file if needed', () => {
      const result = truncateMiddle('.verylonghiddenfilename', 10);
      expect(result.length).toBe(10);
      expect(result).toContain('…');
    });
  });

  describe('edge cases', () => {
    it('handles single character filename', () => {
      const result = truncateMiddle('a', 10);
      expect(result).toBe('a');
    });

    it('handles single character with extension', () => {
      const result = truncateMiddle('a.txt', 10);
      expect(result).toBe('a.txt');
    });

    it('handles very long maxLen', () => {
      const result = truncateMiddle('filename.txt', 1000);
      expect(result).toBe('filename.txt');
    });

    it('preserves ellipsis character exactly once', () => {
      const result = truncateMiddle('verylongcomponentname.tsx', 20);
      const ellipsisCount = (result.match(/…/g) || []).length;
      expect(ellipsisCount).toBe(1);
    });

    it('ensures output length equals maxLen when truncation needed', () => {
      const longName = 'this-is-a-very-long-component-name.tsx';
      for (const maxLen of [5, 10, 15, 20, 25]) {
        const result = truncateMiddle(longName, maxLen);
        if (longName.length > maxLen) {
          expect(result.length).toBe(maxLen);
        }
      }
    });

    it('handles numeric extension', () => {
      const result = truncateMiddle('verylongfilename.123', 15);
      expect(result).toMatch(/\.123$/);
      expect(result.length).toBe(15);
    });

    it('handles special characters in filename', () => {
      const result = truncateMiddle('very-long_component.name.tsx', 20);
      expect(result).toMatch(/\.tsx$/);
      expect(result.length).toBe(20);
    });
  });

  describe('consistency checks', () => {
    it('produces same output for same input', () => {
      const input = 'very-long-component-name.tsx';
      const result1 = truncateMiddle(input, 20);
      const result2 = truncateMiddle(input, 20);
      expect(result1).toBe(result2);
    });

    it('progressively truncates as maxLen decreases', () => {
      const input = 'verylongcomponentname.tsx';
      const result20 = truncateMiddle(input, 20);
      const result15 = truncateMiddle(input, 15);
      const result10 = truncateMiddle(input, 10);

      expect(result20.length).toBeLessThanOrEqual(20);
      expect(result15.length).toBeLessThanOrEqual(15);
      expect(result10.length).toBeLessThanOrEqual(10);

      // Each should have ellipsis (since original is longer)
      expect(result20).toContain('…');
      expect(result15).toContain('…');
      expect(result10).toContain('…');
    });
  });
});
