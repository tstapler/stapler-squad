/**
 * Tests for TerminalDimensionCache - localStorage dimension persistence.
 */

import { getCachedDimensions, saveDimensions } from '../TerminalDimensionCache';

describe('TerminalDimensionCache', () => {
  let mockStorage: Record<string, string>;

  beforeEach(() => {
    mockStorage = {};

    jest.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => {
      return mockStorage[key] ?? null;
    });
    jest.spyOn(Storage.prototype, 'setItem').mockImplementation((key: string, value: string) => {
      mockStorage[key] = value;
    });

    // Suppress console output in tests
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('saveDimensions', () => {
    it('should write to localStorage with correct key and value', () => {
      saveDimensions('session-123', 120, 40);

      expect(localStorage.setItem).toHaveBeenCalledWith(
        'terminal-dimensions-session-123',
        JSON.stringify({ cols: 120, rows: 40 })
      );
    });

    it('should overwrite existing cached dimensions', () => {
      saveDimensions('session-123', 80, 24);
      saveDimensions('session-123', 120, 40);

      const stored = JSON.parse(mockStorage['terminal-dimensions-session-123']);
      expect(stored).toEqual({ cols: 120, rows: 40 });
    });
  });

  describe('getCachedDimensions', () => {
    it('should read from localStorage and return dimensions', () => {
      mockStorage['terminal-dimensions-session-abc'] = JSON.stringify({ cols: 100, rows: 30 });

      const result = getCachedDimensions('session-abc');

      expect(result).toEqual({ cols: 100, rows: 30 });
    });

    it('should return null when no cached value exists', () => {
      const result = getCachedDimensions('nonexistent');

      expect(result).toBeNull();
    });

    it('should return null when localStorage contains invalid JSON', () => {
      mockStorage['terminal-dimensions-bad'] = 'not-json';

      const result = getCachedDimensions('bad');

      expect(result).toBeNull();
      expect(console.warn).toHaveBeenCalled();
    });
  });

  /**
   * Bug 1 cache corruption scenario
   *
   * XtermTerminal fires onResize(terminal.cols, terminal.rows) = onResize(80, 24)
   * synchronously at mount (XtermTerminal.tsx lines 287-290), BEFORE
   * fitAddon.fit() runs via double requestAnimationFrame.
   *
   * TerminalOutput.handleTerminalResize persists whatever dimensions it receives, so
   * saveDimensions(sessionId, 80, 24) corrupts any previously-valid cache entry.
   *
   * On the next load, getCachedDimensions returns the stale 80×24, which is then
   * used as the PTY size in streamViaControlMode → thin, corrupted terminal output.
   */
  describe('Bug 1: premature 80×24 resize corrupts the cache', () => {
    it('saving xterm defaults (80×24) overwrites a previously valid cache entry', () => {
      // GIVEN: a session whose last-known good dimensions were 200×50
      mockStorage['terminal-dimensions-session-1'] = JSON.stringify({ cols: 200, rows: 50 });
      expect(getCachedDimensions('session-1')).toEqual({ cols: 200, rows: 50 });

      // WHEN: Bug 1 fires — XtermTerminal calls onResize(80, 24) before fitAddon.fit()
      // and TerminalOutput faithfully persists those xterm default dimensions
      saveDimensions('session-1', 80, 24);

      // THEN: the cache is corrupted — next load reads 80×24 instead of 200×50
      expect(getCachedDimensions('session-1')).toEqual({ cols: 80, rows: 24 });
    });

    it('stale 80×24 cache can only be cleared if a clean resize saves the real dims', () => {
      // Simulate: Bug 1 corruption has already happened
      mockStorage['terminal-dimensions-session-1'] = JSON.stringify({ cols: 80, rows: 24 });

      // On second load, the stale dims are served immediately
      expect(getCachedDimensions('session-1')).toEqual({ cols: 80, rows: 24 });

      // Eventually fitAddon.fit() fires and saves real dims — if nothing prevents it
      saveDimensions('session-1', 200, 50);
      expect(getCachedDimensions('session-1')).toEqual({ cols: 200, rows: 50 });
    });

    it('illustrates the two-load corruption sequence', () => {
      // Load 1: cache has valid 200×50
      mockStorage['terminal-dimensions-session-1'] = JSON.stringify({ cols: 200, rows: 50 });

      // Bug 1 fires on load 1: saves 80×24 before fitAddon runs
      saveDimensions('session-1', 80, 24);
      // Cache is now corrupted (80×24 overwrote 200×50)

      // Eventually fitAddon fires and saves correct dims
      saveDimensions('session-1', 200, 50);
      // Cache is briefly healed...

      // Load 2: user navigates away and back. Bug 1 fires again.
      saveDimensions('session-1', 80, 24);
      // Cache corrupted AGAIN. This time, if the tab closes before fitAddon runs:
      expect(getCachedDimensions('session-1')).toEqual({ cols: 80, rows: 24 });
      // The NEXT load will use 80×24 as the PTY dimensions → thin terminal.
    });
  });

  describe('error handling', () => {
    it('should handle localStorage.setItem throwing (quota exceeded)', () => {
      jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new DOMException('QuotaExceededError');
      });

      // Should not throw
      expect(() => saveDimensions('session-123', 80, 24)).not.toThrow();
      expect(console.warn).toHaveBeenCalled();
    });

    it('should handle localStorage.getItem throwing', () => {
      jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('SecurityError');
      });

      const result = getCachedDimensions('session-123');

      expect(result).toBeNull();
      expect(console.warn).toHaveBeenCalled();
    });
  });
});
