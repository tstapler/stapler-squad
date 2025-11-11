/**
 * High-throughput stress tests for flow control implementation
 *
 * Tests watermark-based backpressure management under extreme loads
 * to verify browser stability and terminal responsiveness.
 *
 * Reference: https://xtermjs.org/docs/guides/flowcontrol/
 */

import { EscapeSequenceParser } from '../EscapeSequenceParser';

describe('Flow Control Stress Tests', () => {
  // Mock watermark tracker for testing
  class WatermarkTracker {
    private watermark = 0;
    private paused = false;
    private readonly HIGH_WATERMARK = 100000; // 100KB
    private readonly LOW_WATERMARK = 10000;   // 10KB
    private pauseCount = 0;
    private resumeCount = 0;

    write(data: string, callback: () => void): void {
      this.watermark += data.length;

      // Check if HIGH_WATERMARK exceeded (check BEFORE async parse)
      if (this.watermark > this.HIGH_WATERMARK && !this.paused) {
        this.paused = true;
        this.pauseCount++;
      }

      // Simulate async parse completion
      setTimeout(() => {
        this.watermark = Math.max(0, this.watermark - data.length);

        // Check if LOW_WATERMARK reached (check AFTER parse reduces watermark)
        if (this.watermark < this.LOW_WATERMARK && this.paused) {
          this.paused = false;
          this.resumeCount++;
        }

        callback();
      }, 1);
    }

    getMetrics() {
      return {
        watermark: this.watermark,
        paused: this.paused,
        pauseCount: this.pauseCount,
        resumeCount: this.resumeCount,
      };
    }

    reset() {
      this.watermark = 0;
      this.paused = false;
      this.pauseCount = 0;
      this.resumeCount = 0;
    }
  }

  describe('Large Volume Tests', () => {
    test('handles 500KB plain text without crash', async () => {
      const tracker = new WatermarkTracker();
      const chunkSize = 1024; // 1KB chunks
      const totalSize = 500 * 1024; // 500KB
      const chunks = Math.ceil(totalSize / chunkSize);

      let completed = 0;
      let maxWatermark = 0;

      // Write all chunks rapidly to build up watermark
      for (let i = 0; i < chunks; i++) {
        const chunk = 'x'.repeat(chunkSize);
        tracker.write(chunk, () => {
          completed++;
        });

        // Track maximum watermark reached
        const currentMetrics = tracker.getMetrics();
        maxWatermark = Math.max(maxWatermark, currentMetrics.watermark);
      }

      // Wait for all writes to complete
      await new Promise(resolve => {
        const checkInterval = setInterval(() => {
          if (completed === chunks) {
            clearInterval(checkInterval);
            resolve(undefined);
          }
        }, 10);
      });

      expect(completed).toBe(chunks);
      expect(maxWatermark).toBeGreaterThan(tracker['HIGH_WATERMARK']); // Watermark should have exceeded threshold
      const metrics = tracker.getMetrics();
      expect(metrics.pauseCount).toBeGreaterThan(0);  // Should have paused at least once
      expect(metrics.resumeCount).toBeGreaterThan(0); // Should have resumed
      expect(metrics.watermark).toBeLessThan(tracker['HIGH_WATERMARK']); // Final watermark should be low
    });

    test('handles 1MB with ANSI color codes', async () => {
      const tracker = new WatermarkTracker();
      const parser = new EscapeSequenceParser();

      const colorCodes = ['\x1b[31m', '\x1b[32m', '\x1b[33m', '\x1b[34m', '\x1b[0m'];
      const chunkSize = 1024;
      const totalSize = 1024 * 1024; // 1MB
      const chunks = Math.ceil(totalSize / chunkSize);

      let completed = 0;
      let maxWatermark = 0;

      // Write all chunks rapidly to build up watermark
      for (let i = 0; i < chunks; i++) {
        const color = colorCodes[i % colorCodes.length];
        const text = 'x'.repeat(chunkSize - color.length - 4);
        const chunk = `${color}${text}\x1b[0m`;

        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          tracker.write(safeChunk, () => {
            completed++;
          });

          // Track maximum watermark reached
          const currentMetrics = tracker.getMetrics();
          maxWatermark = Math.max(maxWatermark, currentMetrics.watermark);
        }
      }

      // Wait for all writes to complete
      await new Promise(resolve => {
        const checkInterval = setInterval(() => {
          if (completed >= chunks - 10) { // Allow some buffering
            clearInterval(checkInterval);
            resolve(undefined);
          }
        }, 10);
      });

      expect(completed).toBeGreaterThan(0);
      expect(maxWatermark).toBeGreaterThan(tracker['HIGH_WATERMARK']); // Watermark should have exceeded threshold
      const metrics = tracker.getMetrics();
      expect(metrics.pauseCount).toBeGreaterThan(0); // Should have paused at least once
      expect(metrics.resumeCount).toBeGreaterThan(0); // Should have resumed
    }, 10000); // 10s timeout for large test
  });

  describe('Rapid Small Writes', () => {
    test('handles 10000 small writes with batching', async () => {
      const tracker = new WatermarkTracker();
      const writes = 10000;
      const chunkSize = 100; // 100 bytes per write

      let completed = 0;
      let maxWatermark = 0;

      // Write rapidly to build up watermark
      for (let i = 0; i < writes; i++) {
        const chunk = `Line ${i}: ${'x'.repeat(80)}\n`;
        tracker.write(chunk, () => {
          completed++;
        });

        // Track maximum watermark reached
        const currentMetrics = tracker.getMetrics();
        maxWatermark = Math.max(maxWatermark, currentMetrics.watermark);
      }

      // Wait for all writes to complete
      await new Promise(resolve => {
        const checkInterval = setInterval(() => {
          if (completed === writes) {
            clearInterval(checkInterval);
            resolve(undefined);
          }
        }, 10);
      });

      expect(completed).toBe(writes);
      expect(maxWatermark).toBeGreaterThan(tracker['HIGH_WATERMARK']); // Should have exceeded threshold
      const metrics = tracker.getMetrics();
      // Should have cycled through pause/resume states
      expect(metrics.pauseCount + metrics.resumeCount).toBeGreaterThan(0);
    }, 15000); // 15s timeout
  });

  describe('Control Code Heavy Output', () => {
    test('handles Claude Code style animations', async () => {
      const tracker = new WatermarkTracker();
      const parser = new EscapeSequenceParser();

      // Simulate Claude Code rewriting progress
      const frames = 100;
      let completed = 0;

      for (let i = 0; i < frames; i++) {
        const percent = Math.floor((i / frames) * 100);
        const bar = '='.repeat(Math.floor(percent / 2)) + ' '.repeat(50 - Math.floor(percent / 2));

        // Line clear + cursor home + colored text
        const chunk = `\x1b[2K\r\x1b[34m[${percent}%]\x1b[0m ${bar}`;

        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
        }

        // Simulate 60fps animation
        await new Promise(resolve => setTimeout(resolve, 16));
      }

      expect(completed).toBeGreaterThan(0);
      expect(parser.getBuffered()).toBe(''); // No partial sequences left
    }, 10000);

    test('handles cursor positioning sequences', async () => {
      const tracker = new WatermarkTracker();
      const parser = new EscapeSequenceParser();

      const iterations = 1000;
      let completed = 0;

      for (let i = 0; i < iterations; i++) {
        const row = (i % 24) + 1;
        const col = (i % 80) + 1;
        const chunk = `\x1b[${row};${col}H*`;

        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
        }
      }

      expect(completed).toBeGreaterThan(0);
    }, 10000);
  });

  describe('Mixed Content Stress', () => {
    test('handles alternating text and control codes', async () => {
      const tracker = new WatermarkTracker();
      const parser = new EscapeSequenceParser();

      const iterations = 5000;
      let completed = 0;

      for (let i = 0; i < iterations; i++) {
        let chunk: string;

        if (i % 3 === 0) {
          // Color code
          chunk = `\x1b[3${i % 8}mColored text ${i}\x1b[0m\n`;
        } else if (i % 3 === 1) {
          // Cursor movement
          chunk = `\x1b[${(i % 10) + 1}AMove up\n`;
        } else {
          // Plain text
          chunk = `Plain text line ${i}: ${'x'.repeat(50)}\n`;
        }

        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
        }

        // Yield occasionally
        if (i % 100 === 0) {
          await new Promise(resolve => setTimeout(resolve, 0));
        }
      }

      expect(completed).toBeGreaterThan(0);
      const metrics = tracker.getMetrics();
      expect(metrics.watermark).toBeLessThan(50000); // Should drain well
    }, 15000);
  });

  describe('Watermark Behavior', () => {
    test('watermark correctly tracks pending vs completed bytes', async () => {
      const tracker = new WatermarkTracker();

      // Write 150KB quickly (should exceed HIGH_WATERMARK)
      const largeChunk = 'x'.repeat(150000);
      let writeCompleted = false;

      tracker.write(largeChunk, () => {
        writeCompleted = true;
      });

      // Check immediately - watermark should be high
      let metrics = tracker.getMetrics();
      expect(metrics.watermark).toBe(150000);
      expect(metrics.paused).toBe(true);

      // Wait for completion
      await new Promise<void>((resolve) => {
        const checkInterval = setInterval(() => {
          if (writeCompleted) {
            clearInterval(checkInterval);
            resolve();
          }
        }, 10);
      });

      // After completion - watermark should be 0
      metrics = tracker.getMetrics();
      expect(metrics.watermark).toBe(0);
      expect(metrics.paused).toBe(false);
    });

    test('multiple small writes below HIGH_WATERMARK never pause', async () => {
      const tracker = new WatermarkTracker();

      const writes = 50;
      const chunkSize = 1000; // 1KB each = 50KB total (below HIGH_WATERMARK)

      for (let i = 0; i < writes; i++) {
        const chunk = 'x'.repeat(chunkSize);
        await new Promise<void>((resolve) => {
          tracker.write(chunk, resolve);
        });
      }

      const metrics = tracker.getMetrics();
      expect(metrics.pauseCount).toBe(0);  // Should never pause
      expect(metrics.resumeCount).toBe(0);
    });

    test('pause/resume cycles work correctly', async () => {
      const tracker = new WatermarkTracker();

      // Pattern: large write, wait, repeat
      const cycles = 10;

      for (let i = 0; i < cycles; i++) {
        // Large write to trigger pause
        const largeChunk = 'x'.repeat(120000); // 120KB
        await new Promise<void>((resolve) => {
          tracker.write(largeChunk, resolve);
        });

        // Small delay for watermark to drain
        await new Promise(resolve => setTimeout(resolve, 50));
      }

      const metrics = tracker.getMetrics();
      expect(metrics.pauseCount).toBeGreaterThanOrEqual(cycles);
      expect(metrics.resumeCount).toBeGreaterThanOrEqual(cycles - 1);
    }, 10000);
  });

  describe('Error Recovery', () => {
    test('handles partial sequences at chunk boundaries gracefully', async () => {
      const parser = new EscapeSequenceParser();
      const tracker = new WatermarkTracker();

      // Intentionally split escape sequences
      const chunks = [
        'Text \x1b',
        '[31',
        'mRed',
        '\x1b[',
        '0m Normal'
      ];

      let completed = 0;

      for (const chunk of chunks) {
        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
        }
      }

      // All sequences should eventually be output
      expect(parser.getBuffered()).toBe(''); // Nothing left buffered
      expect(completed).toBeGreaterThan(0);
    });

    test('recovers from parser reset during high load', async () => {
      const parser = new EscapeSequenceParser();
      const tracker = new WatermarkTracker();

      let completed = 0;

      for (let i = 0; i < 100; i++) {
        const chunk = `\x1b[3${i % 8}mLine ${i}\x1b[0m\n`;

        const safeChunk = parser.processChunk(chunk);
        if (safeChunk.length > 0) {
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
        }

        // Reset parser halfway through
        if (i === 50) {
          parser.reset();
        }
      }

      // Should continue processing after reset
      expect(completed).toBeGreaterThan(50);
    });
  });
});
