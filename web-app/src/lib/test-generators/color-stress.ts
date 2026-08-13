/**
 * Color Stress Generator
 * Generates terminal output with maximum ANSI color complexity for stress testing
 */

import { ColorStressConfig, GeneratorFrame } from './types';

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
 * Generate 16-color ANSI output
 */
function generate16Color(sequence: number, width: number, height: number): string {
  const lines: string[] = [];
  const fgColors = [30, 31, 32, 33, 34, 35, 36, 37, 90, 91, 92, 93, 94, 95, 96, 97];
  const bgColors = [40, 41, 42, 43, 44, 45, 46, 47];

  // Header
  lines.push(`\x1b[1m\x1b[7m 16-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));

  // Color grid
  for (let row = 0; row < Math.min(height - 2, 16); row++) {
    let line = '';
    for (let col = 0; col < Math.min(width, 64); col++) {
      const fgIndex = (row + col + sequence) % fgColors.length;
      const bgIndex = (row + sequence) % bgColors.length;
      const char = String.fromCharCode(65 + ((row + col) % 26));
      line += `\x1b[${fgColors[fgIndex]}m\x1b[${bgColors[bgIndex]}m${char}\x1b[0m`;
    }
    lines.push(line);
  }

  // Fill remaining lines
  while (lines.length < height) {
    lines.push(' '.repeat(width));
  }

  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate 256-color ANSI output
 */
function generate256Color(sequence: number, width: number, height: number): string {
  const lines: string[] = [];

  // Header
  lines.push(`\x1b[1m\x1b[7m 256-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));

  // Color cube (6x6x6 = 216 colors, starting at 16)
  for (let row = 0; row < Math.min(height - 2, 36); row++) {
    let line = '';
    for (let col = 0; col < Math.min(width, 72); col++) {
      // Cycle through 256 colors with animation
      const colorIndex = (16 + (row * 6 + col + sequence * 3) % 216);
      const block = '█';
      line += `\x1b[38;5;${colorIndex}m${block}\x1b[0m`;
    }
    lines.push(line);
  }

  // Grayscale ramp
  if (lines.length < height - 1) {
    let grayLine = '';
    for (let i = 0; i < Math.min(width, 24); i++) {
      const grayIndex = 232 + (i + sequence) % 24;
      grayLine += `\x1b[38;5;${grayIndex}m█\x1b[0m`;
    }
    lines.push(grayLine);
  }

  // Fill remaining lines
  while (lines.length < height) {
    lines.push(' '.repeat(width));
  }

  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate true-color (24-bit) output
 */
function generateTrueColor(sequence: number, width: number, height: number): string {
  const lines: string[] = [];

  // Header
  lines.push(`\x1b[1m\x1b[7m True-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));

  // HSV to RGB conversion
  const hsvToRgb = (h: number, s: number, v: number): [number, number, number] => {
    const c = v * s;
    const x = c * (1 - Math.abs((h / 60) % 2 - 1));
    const m = v - c;

    let r = 0, g = 0, b = 0;
    if (h < 60) { r = c; g = x; }
    else if (h < 120) { r = x; g = c; }
    else if (h < 180) { g = c; b = x; }
    else if (h < 240) { g = x; b = c; }
    else if (h < 300) { r = x; b = c; }
    else { r = c; b = x; }

    return [
      Math.round((r + m) * 255),
      Math.round((g + m) * 255),
      Math.round((b + m) * 255),
    ];
  };

  // Generate colorful grid with animation
  for (let row = 0; row < Math.min(height - 2, 20); row++) {
    let line = '';
    for (let col = 0; col < Math.min(width, 80); col++) {
      const hue = ((col * 5 + row * 15 + sequence * 10) % 360);
      const saturation = 0.7 + (row / height) * 0.3;
      const value = 0.8 + Math.sin((col + sequence) * 0.1) * 0.2;

      const [r, g, b] = hsvToRgb(hue, saturation, value);
      line += `\x1b[38;2;${r};${g};${b}m█\x1b[0m`;
    }
    lines.push(line);
  }

  // Fill remaining lines
  while (lines.length < height) {
    lines.push(' '.repeat(width));
  }

  return '\x1b[H' + lines.join('\n');
}

/**
 * Generate smooth gradient animation
 */
function generateGradient(sequence: number, width: number, height: number): string {
  const lines: string[] = [];

  // Header
  lines.push(`\x1b[1m\x1b[7m Gradient Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));

  // Generate rainbow gradient with wave animation
  for (let row = 0; row < height - 1; row++) {
    let line = '';
    for (let col = 0; col < width; col++) {
      // Create wave effect
      const wave = Math.sin((col + row + sequence) * 0.1) * 0.5 + 0.5;
      const hue = ((col + sequence * 5) % 360);

      // HSV to RGB (simplified)
      const h = hue / 60;
      const i = Math.floor(h);
      const f = h - i;
      const p = 0;
      const q = Math.floor(255 * (1 - f));
      const t = Math.floor(255 * f);
      const v = Math.floor(200 + wave * 55);

      let r = 0, g = 0, b = 0;
      switch (i % 6) {
        case 0: r = v; g = t; b = p; break;
        case 1: r = q; g = v; b = p; break;
        case 2: r = p; g = v; b = t; break;
        case 3: r = p; g = q; b = v; break;
        case 4: r = t; g = p; b = v; break;
        case 5: r = v; g = p; b = q; break;
      }

      // Vertical gradient blend
      const brightness = 1 - (row / height) * 0.5;
      r = Math.floor(r * brightness);
      g = Math.floor(g * brightness);
      b = Math.floor(b * brightness);

      line += `\x1b[38;2;${r};${g};${b}m█\x1b[0m`;
    }
    lines.push(line);
  }

  return '\x1b[H' + lines.join('\n');
}

/**
 * Color Stress Generator class
 */
export class ColorStressGenerator {
  private config: ColorStressConfig;
  private sequence: number = 0;
  private width: number = 80;
  private height: number = 24;

  constructor(config: ColorStressConfig, width: number = 80, height: number = 24) {
    this.config = config;
    this.width = width;
    this.height = height;
  }

  /**
   * Generate next frame
   */
  nextFrame(): GeneratorFrame {
    const startTime = performance.now();

    let content: string;
    switch (this.config.mode) {
      case '16-color':
        content = generate16Color(this.sequence, this.width, this.height);
        break;
      case '256-color':
        content = generate256Color(this.sequence, this.width, this.height);
        break;
      case 'true-color':
        content = generateTrueColor(this.sequence, this.width, this.height);
        break;
      case 'gradient':
        content = generateGradient(this.sequence, this.width, this.height);
        break;
      default:
        content = generateGradient(this.sequence, this.width, this.height);
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
  }

  /**
   * Get current sequence number
   */
  getSequence(): number {
    return this.sequence;
  }

  /**
   * Set terminal dimensions
   */
  setDimensions(width: number, height: number): void {
    this.width = width;
    this.height = height;
  }
}
