/**
 * Tests for getCachedDimensions functionality.
 * Verifies that dimensions are correctly read from localStorage with proper error handling.
 */

import { getCachedDimensions, setupStorageMock, cleanupMocks } from './TerminalDimensionCacheTestUtils';

describe('getCachedDimensions', () => {
  beforeEach(() => {
    setupStorageMock();
  });

  afterEach(() => {
    cleanupMocks();
  });

  it('should read from localStorage and return dimensions', () => {
    const mockStorage = setupStorageMock();
    mockStorage['terminal-dimensions-session-abc'] = JSON.stringify({ cols: 100, rows: 30 });

    const result = getCachedDimensions('session-abc');

    expect(result).toEqual({ cols: 100, rows: 30 });
  });

  it('should return null when no cached value exists', () => {
    getCachedDimensions('nonexistent');

    expect(getCachedDimensions('nonexistent')).toBeNull();
  });

  it('should return null when localStorage contains invalid JSON', () => {
    const mockStorage = setupStorageMock();
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});
    mockStorage['terminal-dimensions-bad'] = 'not-json';

    const result = getCachedDimensions('bad');

    expect(result).toBeNull();
    expect(warnSpy).toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});