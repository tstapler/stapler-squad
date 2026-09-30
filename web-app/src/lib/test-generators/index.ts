/**
 * Terminal Stress Test Generators
 *
 * This module provides data generators for stress testing terminal streaming:
 * - ASCII video playback at various frame rates
 * - High-volume log output simulation
 * - Color stress testing (16/256/true color)
 * - Large payload testing
 */

export * from './types';
export * from './ascii-frames';
export * from './log-lines';
export * from './color-stress';
export * from './large-payload';

import { TestConfig, GeneratorFrame } from './types';
import { AsciiFrameGenerator } from './ascii-frames';
import { LogLineGenerator } from './log-lines';
import { ColorStressGenerator } from './color-stress';
import { LargePayloadGenerator } from './large-payload';

/**
 * Factory function to create appropriate generator based on config
 */
export function createGenerator(config: TestConfig): {
  nextFrame: (batchSize?: number) => GeneratorFrame;
  reset: () => void;
  getSequence: () => number;
} {
  switch (config.type) {
    case 'ascii-video':
      return new AsciiFrameGenerator(config);
    case 'log-flood':
      return new LogLineGenerator(config);
    case 'color-stress':
      return new ColorStressGenerator(config);
    case 'large-payload':
      return new LargePayloadGenerator(config);
    default:
      throw new Error(`Unknown generator type: ${(config as any).type}`);
  }
}
