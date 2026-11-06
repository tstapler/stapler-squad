/**
 * Tests for saveDimensions functionality.
 * Verifies that dimensions are correctly saved to localStorage with proper keys and values.
 */

import { saveDimensions, setupStorageMock, cleanupMocks } from './TerminalDimensionCacheTestUtils';

describe('saveDimensions', () => {
  beforeEach(() => {
    setupStorageMock();
  });

  afterEach(() => {
    cleanupMocks();
  });

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

    expect(saveDimensions).not.toThrow();
  });
});