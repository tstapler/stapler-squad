/**
 * ASCII Frame Generator
 * Generates animated ASCII art frames for terminal stress testing
 */

import { AsciiFrameConfig, GeneratorFrame } from './types';

// Simple seeded random number generator for reproducibility
class SeededRandom {
  private seed: number;

  constructor(seed: number) {
    this.seed = seed;
  }

  next(): number {
    this.seed = (this.seed * 1103515245 + 12345) & 0x7fffffff;
    return this.seed / 0x7fffffff;
  }

  nextInt(max: number): number {
    return Math.floor(this.next() * max);
  }
}

// Simple checksum for frame validation
function checksum(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash);
}

/**
 * Generate bouncing ball animation frame
 */
function generateBouncingBall(
  sequence: number,
  width: number,
  height: number
): string {
  const ball = '\x1b[33m\x1b[1mO\x1b[0m'; // Yellow bold ball
  const period = 60; // frames per cycle

  // Calculate ball position using simple harmonic motion
  const t = (sequence % period) / period * Math.PI * 2;
  const x = Math.floor((Math.sin(t) + 1) / 2 * (width - 2));
  const y = Math.floor((Math.sin(t * 1.5 + 0.5) + 1) / 2 * (height - 3)) + 1;

  const lines: string[] = [];

  // Header
  lines.push(`\x1b[36m${'='.repeat(width)}\x1b[0m`);

  // Body
  for (let row = 1; row < height - 1; row++) {
    let line = '';
    for (let col = 0; col < width; col++) {
      if (row === y && col === x) {
        line += ball;
      } else {
        line += ' ';
      }
    }
    lines.push(line);
  }

  // Footer with frame info
  const footer = `Frame: ${sequence.toString().padStart(6, '0')} | Ball: (${x},${y})`;
  lines.push(`\x1b[36m${footer.padEnd(width)}\x1b[0m`);

  // Move cursor to home and output
  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate scrolling text animation frame
 */
function generateScrollingText(
  sequence: number,
  width: number,
  height: number
): string {
  const messages = [
    '\x1b[32m[INFO]\x1b[0m Processing batch...',
    '\x1b[33m[WARN]\x1b[0m High memory usage detected',
    '\x1b[31m[ERROR]\x1b[0m Connection timeout',
    '\x1b[34m[DEBUG]\x1b[0m Cache hit ratio: 95%',
    '\x1b[35m[TRACE]\x1b[0m Request completed in 45ms',
  ];

  const lines: string[] = [];

  // Header
  lines.push(`\x1b[7m Scrolling Log Test - Frame ${sequence} \x1b[0m`.padEnd(width));

  // Generate scrolling log lines
  for (let row = 1; row < height - 1; row++) {
    const lineIndex = (sequence + row) % messages.length;
    const offset = sequence % (width / 2);
    let line = messages[lineIndex];

    // Add timestamp
    const timestamp = new Date(Date.now() - (height - row) * 100).toISOString().slice(11, 23);
    line = `\x1b[90m${timestamp}\x1b[0m ${line}`;

    // Truncate to width
    lines.push(line.slice(0, width).padEnd(width));
  }

  // Footer
  lines.push(`\x1b[7m ${'Press Ctrl+C to stop'.padEnd(width - 2)} \x1b[0m`);

  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate progress bar animation frame
 */
function generateProgressBar(
  sequence: number,
  width: number,
  height: number
): string {
  const lines: string[] = [];
  const numBars = Math.min(height - 4, 10);

  // Header
  lines.push(`\x1b[1m\x1b[36m${'='.repeat(width)}\x1b[0m`);
  lines.push(`\x1b[1m Multi-Progress Test - Frame ${sequence} \x1b[0m`.padEnd(width));
  lines.push(`\x1b[1m\x1b[36m${'='.repeat(width)}\x1b[0m`);

  // Progress bars with different speeds
  for (let i = 0; i < numBars; i++) {
    const speed = (i + 1) * 0.5;
    const progress = ((sequence * speed) % 100) / 100;
    const barWidth = width - 20;
    const filled = Math.floor(progress * barWidth);
    const empty = barWidth - filled;

    const colors = ['\x1b[32m', '\x1b[33m', '\x1b[34m', '\x1b[35m', '\x1b[36m'];
    const color = colors[i % colors.length];

    const bar = `${color}${'█'.repeat(filled)}\x1b[90m${'░'.repeat(empty)}\x1b[0m`;
    const label = `Task ${(i + 1).toString().padStart(2, '0')}`;
    const percent = `${Math.floor(progress * 100).toString().padStart(3)}%`;

    lines.push(`${label} [${bar}] ${percent}`);
  }

  // Fill remaining space
  while (lines.length < height - 1) {
    lines.push(' '.repeat(width));
  }

  // Footer
  const elapsed = Math.floor(sequence / 60);
  lines.push(`\x1b[90mElapsed: ${elapsed}s | FPS Target: 60\x1b[0m`.padEnd(width));

  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate matrix rain animation frame
 */
function generateMatrixRain(
  sequence: number,
  width: number,
  height: number,
  rng: SeededRandom
): string {
  const chars = 'ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ0123456789';
  const lines: string[] = [];

  // Initialize or update rain columns
  const columns: number[] = [];
  for (let col = 0; col < width; col++) {
    // Each column has a different phase based on seed
    const phase = (rng.nextInt(height) + sequence) % (height + 10);
    columns.push(phase);
  }

  // Generate frame
  for (let row = 0; row < height; row++) {
    let line = '';
    for (let col = 0; col < width; col++) {
      const columnPhase = (columns[col] + col * 3 + sequence) % (height + 10);
      const distFromHead = row - (columnPhase % height);

      if (distFromHead === 0) {
        // Head of rain drop (bright)
        const char = chars[rng.nextInt(chars.length)];
        line += `\x1b[97m\x1b[1m${char}\x1b[0m`;
      } else if (distFromHead > 0 && distFromHead < 8) {
        // Trail (fading green)
        const brightness = Math.max(22, 28 - distFromHead);
        const char = chars[rng.nextInt(chars.length)];
        line += `\x1b[38;5;${brightness}m${char}\x1b[0m`;
      } else if (rng.next() < 0.02) {
        // Random flicker
        const char = chars[rng.nextInt(chars.length)];
        line += `\x1b[38;5;22m${char}\x1b[0m`;
      } else {
        line += ' ';
      }
    }
    lines.push(line);
  }

  return '\x1b[H' + lines.join('\n');
}

/**
 * ASCII Frame Generator class
 */
export class AsciiFrameGenerator {
  private config: AsciiFrameConfig;
  private rng: SeededRandom;
  private sequence: number = 0;

  constructor(config: AsciiFrameConfig) {
    this.config = config;
    this.rng = new SeededRandom(config.seed || Date.now());
  }

  /**
   * Generate next frame
   */
  nextFrame(): GeneratorFrame {
    const startTime = performance.now();

    let content: string;
    switch (this.config.pattern) {
      case 'bouncing-ball':
        content = generateBouncingBall(
          this.sequence,
          this.config.width,
          this.config.height
        );
        break;
      case 'scrolling-text':
        content = generateScrollingText(
          this.sequence,
          this.config.width,
          this.config.height
        );
        break;
      case 'progress-bar':
        content = generateProgressBar(
          this.sequence,
          this.config.width,
          this.config.height
        );
        break;
      case 'matrix-rain':
        content = generateMatrixRain(
          this.sequence,
          this.config.width,
          this.config.height,
          this.rng
        );
        break;
      default:
        content = generateBouncingBall(
          this.sequence,
          this.config.width,
          this.config.height
        );
    }

    const frame: GeneratorFrame = {
      sequence: this.sequence,
      content,
      timestamp: startTime,
      checksum: checksum(content),
    };

    this.sequence++;
    return frame;
  }

  /**
   * Reset generator state
   */
  reset(): void {
    this.sequence = 0;
    this.rng = new SeededRandom(this.config.seed || Date.now());
  }

  /**
   * Get current sequence number
   */
  getSequence(): number {
    return this.sequence;
  }
}
