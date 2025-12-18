/**
 * ANSI Escape Code Test Harness
 *
 * Comprehensive testing for all 77 escape codes observed in production.
 * Based on escape-codes.json (26,974 total occurrences).
 *
 * Usage:
 * ```typescript
 * import {
 *   ESCAPE_CODE_LIBRARY,
 *   EscapeCodeFrameGenerator,
 *   ESCAPE_CODE_TEST_PRESETS
 * } from '@/lib/test-generators/escape-codes';
 *
 * // Create generator with preset
 * const generator = new EscapeCodeFrameGenerator(ESCAPE_CODE_TEST_PRESETS.CRITICAL_ONLY);
 *
 * // Generate frames
 * const frame = generator.nextFrame();
 * console.log(frame.content);
 * ```
 */

// Export types
export * from './types';

// Export library
export {
  ESCAPE_CODE_LIBRARY,
  getCodesByCategory,
  getCodesByPriority,
  getCodesAtOrAbovePriority,
  getCategoryStats,
  getLibraryStats,
  findCodeByHex,
  findCodeBySequence,
} from './library';

// Export generators
export {
  EscapeCodeFrameGenerator,
  generateEscapeCodeTestFrames,
  generateSingleCodeFrame,
} from './generators';
