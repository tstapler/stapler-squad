/**
 * Tests for Bug 1 cache corruption prevention - part 1.
 *
 * XtermTerminal fires onResize(terminal.cols, terminal.rows) = onResize(80, 24)
 * synchronously at mount (XtermTerminal.tsx lines 287-290), BEFORE
 * fitAddon.fit() runs via double requestAnimationFrame.
 *
 * The implementation prevents corruption by:
 * 1. isValidToCache() rejecting xterm.js default dimensions (80×24)
 * 2. shouldOverwriteCache() rejecting smaller dimensions that would overwrite larger cache
 */

import { saveDimensions, getCachedDimensions, setupStorageMock, cleanupMocks } from './TerminalDimensionCacheTestUtils';

describe('Bug 1: cache protection - 80×24 should NOT corrupt valid 200×50 cache', () => {
  beforeEach(() => {
    setupStorageMock();
  });

  afterEach(() => {
    cleanupMocks();
  });

  it('should reject smaller dimensions to prevent cache corruption (80x24 vs 200x50)', () => {
    const mockStorage = setupStorageMock();
    mockStorage['terminal-dimensions-session-1'] = JSON.stringify({ cols: 200, rows: 50 });
    expect(getCachedDimensions('session-1')).toEqual({ cols: 200, rows: 50 });

    saveDimensions('session-1', 80, 24);

    expect(getCachedDimensions('session-1')).toEqual({ cols: 200, rows: 50 });
  });
});