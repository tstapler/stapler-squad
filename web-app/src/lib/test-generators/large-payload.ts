/**
 * Large Payload Generator
 * Generates large terminal payloads for buffer stress testing
 */

import { LargePayloadConfig, GeneratorFrame } from './types';

// Simple seeded random number generator
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

// Simple checksum
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
 * Generate random printable content
 */
function generateRandomContent(size: number, rng: SeededRandom): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()';
  let content = '';
  let lineLength = 0;

  for (let i = 0; i < size; i++) {
    if (lineLength >= 80) {
      content += '\n';
      lineLength = 0;
    } else {
      content += chars[rng.nextInt(chars.length)];
      lineLength++;
    }
  }

  return content;
}

/**
 * Generate repeated pattern content
 */
function generateRepeatedContent(size: number, sequence: number): string {
  const pattern = `[PAYLOAD-${sequence.toString().padStart(6, '0')}] `;
  const linePattern = pattern + '='.repeat(80 - pattern.length) + '\n';

  let content = '';
  while (content.length < size) {
    content += linePattern;
  }

  return content.slice(0, size);
}

/**
 * Generate structured log-like content
 */
function generateStructuredContent(size: number, sequence: number, rng: SeededRandom): string {
  const lines: string[] = [];
  const colors = ['\x1b[32m', '\x1b[33m', '\x1b[34m', '\x1b[35m', '\x1b[36m'];

  // Header
  lines.push(`\x1b[1m\x1b[7m Large Payload Test - Sequence ${sequence} - Size: ${(size / 1024).toFixed(1)}KB \x1b[0m`);
  lines.push(`${'='.repeat(80)}`);

  let currentSize = lines.join('\n').length + 2;

  // Generate structured lines until we reach target size
  let lineNum = 1;
  while (currentSize < size - 100) {
    const color = colors[lineNum % colors.length];
    const timestamp = new Date().toISOString();

    // Vary line length for realistic output
    const dataLength = 20 + rng.nextInt(40);
    const data = 'x'.repeat(dataLength);

    const line = `${color}[${timestamp}]\x1b[0m Line ${lineNum.toString().padStart(6, '0')}: ${data}`;
    lines.push(line);

    currentSize += line.length + 1;
    lineNum++;
  }

  // Footer with checksum info
  lines.push(`${'='.repeat(80)}`);
  lines.push(`\x1b[1mEnd of payload - ${lineNum} lines, ${currentSize} bytes\x1b[0m`);

  return lines.join('\n') + '\n';
}

/**
 * Large Payload Generator class
 */
export class LargePayloadGenerator {
  private config: LargePayloadConfig;
  private rng: SeededRandom;
  private sequence: number = 0;

  constructor(config: LargePayloadConfig) {
    this.config = config;
    this.rng = new SeededRandom(config.seed || Date.now());
  }

  /**
   * Generate next payload
   */
  nextFrame(): GeneratorFrame {
    const startTime = performance.now();

    let content: string;
    switch (this.config.pattern) {
      case 'random':
        content = generateRandomContent(this.config.size, this.rng);
        break;
      case 'repeated':
        content = generateRepeatedContent(this.config.size, this.sequence);
        break;
      case 'structured':
        content = generateStructuredContent(this.config.size, this.sequence, this.rng);
        break;
      default:
        content = generateStructuredContent(this.config.size, this.sequence, this.rng);
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
   * Generate payload and split into chunks
   */
  nextFrameChunked(chunkSize: number = 4096): GeneratorFrame[] {
    const frame = this.nextFrame();
    const chunks: GeneratorFrame[] = [];

    for (let i = 0; i < frame.content.length; i += chunkSize) {
      const chunk = frame.content.slice(i, i + chunkSize);
      chunks.push({
        sequence: frame.sequence * 1000 + Math.floor(i / chunkSize),
        content: chunk,
        timestamp: frame.timestamp,
        checksum: checksum(chunk),
      });
    }

    return chunks;
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

  /**
   * Get configured payload size
   */
  getPayloadSize(): number {
    return this.config.size;
  }
}
