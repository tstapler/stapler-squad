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

/**
 * Run a stress test with the given configuration
 * Returns a promise that resolves when the test completes or is aborted
 */
export async function runStressTest(
  config: TestConfig,
  onFrame: (frame: GeneratorFrame) => void,
  onProgress?: (progress: { elapsed: number; framesGenerated: number; bytesGenerated: number }) => void,
  abortSignal?: AbortSignal
): Promise<{
  framesGenerated: number;
  bytesGenerated: number;
  totalTime: number;
  avgFrameTime: number;
}> {
  const generator = createGenerator(config);
  const startTime = performance.now();
  const durationMs = config.duration * 1000;
  const targetIntervalMs = 1000 / config.rate;

  let framesGenerated = 0;
  let bytesGenerated = 0;
  let lastProgressUpdate = startTime;

  // For log flood, adjust batch size based on rate
  const batchSize = config.type === 'log-flood' ? Math.ceil(config.rate / 100) : 1;

  return new Promise((resolve) => {
    const generateFrame = () => {
      if (abortSignal?.aborted) {
        resolve({
          framesGenerated,
          bytesGenerated,
          totalTime: performance.now() - startTime,
          avgFrameTime: (performance.now() - startTime) / framesGenerated,
        });
        return;
      }

      const elapsed = performance.now() - startTime;
      if (elapsed >= durationMs) {
        resolve({
          framesGenerated,
          bytesGenerated,
          totalTime: elapsed,
          avgFrameTime: elapsed / framesGenerated,
        });
        return;
      }

      // Generate and emit frame
      const frame = generator.nextFrame(batchSize);
      onFrame(frame);
      framesGenerated++;
      bytesGenerated += frame.content.length;

      // Progress callback (throttled to once per 100ms)
      if (onProgress && performance.now() - lastProgressUpdate > 100) {
        onProgress({ elapsed, framesGenerated, bytesGenerated });
        lastProgressUpdate = performance.now();
      }

      // Schedule next frame
      const nextFrameDelay = Math.max(0, targetIntervalMs - (performance.now() - startTime - elapsed));
      setTimeout(generateFrame, nextFrameDelay);
    };

    // Start generation
    generateFrame();
  });
}
