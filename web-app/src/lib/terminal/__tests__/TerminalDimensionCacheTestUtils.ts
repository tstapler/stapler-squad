/**
 * Shared test utilities for TerminalDimensionCache tests.
 * Extracted to avoid duplication across test files.
 */

import { getCachedDimensions, saveDimensions } from '../TerminalDimensionCache';

let mockStorage: Record<string, string>;

/**
 * Setup localStorage mock for TerminalDimensionCache tests.
 * Call in beforeEach; call cleanupMockStorage() in afterEach.
 */
export function setupStorageMock(): Record<string, string> {
  mockStorage = {};

  jest.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => {
    return mockStorage[key] ?? null;
  });
  jest.spyOn(Storage.prototype, 'setItem').mockImplementation((key: string, value: string) => {
    mockStorage[key] = value;
  });

  return mockStorage;
}

export function cleanupMocks(): void {
  jest.restoreAllMocks();
}

export { getCachedDimensions, saveDimensions, mockStorage };