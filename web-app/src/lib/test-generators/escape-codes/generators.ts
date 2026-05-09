/**
 * Escape Code Frame Generator
 * Generates test frames for ANSI escape code testing
 */

import {
  EscapeCodeTestConfig,
  EscapeCodeTestFrame,
  EscapeCodeDefinition,
  ValidationRules,
  TerminalState,
  ColorExpectation,
  AttributeExpectation,
} from './types';

import {
  ESCAPE_CODE_LIBRARY,
  getCodesByCategory,
  getCodesByPriority,
  getCodesAtOrAbovePriority,
} from './library';

/**
 * Simple checksum function for frame validation
 * Uses sum of character codes modulo 65536
 */
function calculateChecksum(content: string): number {
  let sum = 0;
  for (let i = 0; i < content.length; i++) {
    sum = (sum + content.charCodeAt(i)) % 65536;
  }
  return sum;
}

/**
 * Generate random text for testing
 */
function generateTestText(length: number): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

/**
 * Frame generator for escape code testing
 */
export class EscapeCodeFrameGenerator {
  private config: EscapeCodeTestConfig;
  private frameCounter: number = 0;
  private random: () => number;

  constructor(config: EscapeCodeTestConfig) {
    this.config = config;
    // Seeded random for reproducibility
    if (config.seed !== undefined) {
      let seed = config.seed;
      this.random = () => {
        seed = (seed * 1664525 + 1013904223) % 2147483648;
        return seed / 2147483648;
      };
    } else {
      this.random = Math.random;
    }
  }

  /**
   * Get codes to test based on config
   */
  private getTestCodes(): EscapeCodeDefinition[] {
    // If specific codes are provided
    if (this.config.codes && this.config.codes.length > 0) {
      return ESCAPE_CODE_LIBRARY.filter(code =>
        this.config.codes!.includes(code.code)
      );
    }

    // If categories are specified
    if (this.config.categories && this.config.categories.length > 0) {
      const codes: EscapeCodeDefinition[] = [];
      for (const category of this.config.categories) {
        codes.push(...getCodesByCategory(category));
      }
      return codes;
    }

    // If minimum priority is specified
    if (this.config.minPriority) {
      return getCodesAtOrAbovePriority(this.config.minPriority);
    }

    // Default: all codes
    return ESCAPE_CODE_LIBRARY;
  }

  /**
   * Generate a single isolated frame testing one escape code
   */
  generateIsolatedFrame(code: EscapeCodeDefinition): EscapeCodeTestFrame {
    const frameNum = ++this.frameCounter;
    let content = '';
    const validation: ValidationRules = {};

    // Clear screen and reset
    content += '\x1b[2J\x1b[H\x1b[0m';

    // Add test header
    content += `\x1b[1;1H\x1b[1mTesting: ${code.humanReadable}\x1b[0m\n`;
    content += `\x1b[2;1HCode: ${code.code} Priority: ${code.priority}\n`;
    content += '\x1b[4;1H' + '─'.repeat(this.config.width) + '\n';

    // Generate test content based on category
    switch (code.category) {
      case 'SGR': // Text styling
        content += '\x1b[6;1HBefore: Normal Text\n';
        content += `\x1b[7;1HStyled: ${code.sequence}Styled Text\x1b[0m\n`;
        content += '\x1b[8;1HAfter: Normal Text\n';

        // Validation for SGR codes
        validation.textContent = ['Styled Text', 'Normal Text'];
        if (code.humanReadable.includes('Bold')) {
          validation.attributes = [{ row: 7, col: 9, bold: true }];
        } else if (code.humanReadable.includes('Underline')) {
          validation.attributes = [{ row: 7, col: 9, underline: true }];
        }
        break;

      case 'Cursor': // Cursor movement
        // Fill screen with markers
        for (let row = 6; row <= 15; row++) {
          content += `\x1b[${row};1H`;
          for (let col = 1; col <= 40; col += 10) {
            content += `[${row},${col}]`.padEnd(10);
          }
        }

        // Apply cursor movement
        content += `\x1b[10;20H${code.sequence}`;
        content += 'CURSOR';

        // Extract position from code if available
        const posMatch = code.humanReadable.match(/\((\d+)[;,](\d+)\)/);
        if (posMatch) {
          validation.cursorPosition = {
            row: parseInt(posMatch[1]),
            col: parseInt(posMatch[2]) + 6 // After "CURSOR"
          };
        }
        validation.textContent = ['CURSOR'];
        break;

      case 'Erase': // Screen/line clearing
        // Fill area with X's
        content += '\x1b[6;1H' + 'X'.repeat(this.config.width - 1) + '\n';
        content += '\x1b[7;1H' + 'X'.repeat(this.config.width - 1) + '\n';
        content += '\x1b[8;1H' + 'X'.repeat(this.config.width - 1) + '\n';

        // Position cursor and erase
        if (code.humanReadable.includes('End of Line')) {
          content += '\x1b[7;20H'; // Position in middle
          content += code.sequence;
          content += '\x1b[9;1HErased from position 20 to end\n';
          validation.textAbsent = ['XXXXXXXXXXXXXXXXXXXX']; // Should be erased
        } else if (code.humanReadable.includes('Full Screen')) {
          content += code.sequence;
          content += '\x1b[1;1HScreen cleared\n';
          validation.textAbsent = ['XXXXXX'];
        }
        break;

      case 'Scroll': // Scroll region
        // Set up content
        for (let i = 1; i <= 20; i++) {
          content += `\x1b[${i + 5};1HLine ${i}\n`;
        }

        // Apply scroll region/command
        content += code.sequence;

        // Add marker after scroll
        content += '\x1b[26;1HScroll applied\n';
        validation.textContent = ['Scroll applied'];
        break;

      case 'DECPriv': // DEC private modes
        content += '\x1b[6;1HApplying: ' + code.humanReadable + '\n';
        content += code.sequence;
        content += '\x1b[8;1HMode changed\n';
        validation.textContent = ['Mode changed'];
        break;

      case 'Charset': // Character set
        content += '\x1b[6;1HBefore charset: ABC 123 !@#\n';
        content += code.sequence;
        content += '\x1b[7;1HAfter charset: ABC 123 !@#\n';
        validation.textContent = ['After charset'];
        break;

      default:
        content += '\x1b[6;1HExecuting: ' + code.humanReadable + '\n';
        content += code.sequence;
        content += '\x1b[8;1HDone\n';
        validation.textContent = ['Done'];
    }

    // Add frame footer
    content += `\x1b[${this.config.height - 1};1H`;
    content += `Frame ${frameNum} | Pattern: isolated | Code: ${code.code}`;

    return {
      sequence: frameNum,
      content,
      codesUsed: [code.code],
      validation,
      timestamp: Date.now(),
      checksum: calculateChecksum(content),
    };
  }

  /**
   * Generate sequential frames testing codes one after another
   */
  generateSequentialFrames(codes: EscapeCodeDefinition[]): EscapeCodeTestFrame[] {
    const frames: EscapeCodeTestFrame[] = [];

    for (const code of codes) {
      // Generate isolated frame for each code
      const frame = this.generateIsolatedFrame(code);

      // Add reset between codes if configured
      if (this.config.resetBetween) {
        frame.content = '\x1b[0m\x1b[2J\x1b[H' + frame.content;
      }

      frames.push(frame);
    }

    return frames;
  }

  /**
   * Generate a mixed frame with realistic combination of codes
   */
  generateMixedFrame(): EscapeCodeTestFrame {
    const frameNum = ++this.frameCounter;
    const codes = this.getTestCodes();
    const usedCodes: string[] = [];
    let content = '';

    // Clear and reset
    content += '\x1b[2J\x1b[H\x1b[0m';

    // Simulate a realistic terminal session
    content += '\x1b[1;1H\x1b[1m=== Mixed Escape Code Test ===\x1b[0m\n\n';

    // Progress bar simulation
    content += '\x1b[3;1HProgress: [';
    for (let i = 0; i < 20; i++) {
      const progress = Math.floor(this.random() * 100);
      if (progress > 50) {
        content += '\x1b[42m \x1b[49m'; // Green background
        usedCodes.push('1b5b34326d', '1b5b34396d');
      } else {
        content += '\x1b[41m \x1b[49m'; // Red background
      }
    }
    content += ']\n\n';

    // Colored log output
    const logLevels = [
      { prefix: '\x1b[32m[INFO]\x1b[39m', code: '1b5b33326d' },
      { prefix: '\x1b[33m[WARN]\x1b[39m', code: '1b5b33336d' },
      { prefix: '\x1b[31m[ERROR]\x1b[39m', code: '1b5b33316d' },
      { prefix: '\x1b[36m[DEBUG]\x1b[39m', code: '1b5b33366d' },
    ];

    for (let i = 0; i < 8; i++) {
      const level = logLevels[Math.floor(this.random() * logLevels.length)];
      content += `\x1b[${i + 5};1H${level.prefix} ${generateTestText(40)}\n`;
      usedCodes.push(level.code, '1b5b33396d');
    }

    // Table with borders (using line drawing)
    content += '\x1b[14;1H\x1b[1mData Table:\x1b[0m\n';
    content += '\x1b[15;1H┌────────┬────────┬────────┐\n';
    content += '\x1b[16;1H│ Col 1  │ Col 2  │ Col 3  │\n';
    content += '\x1b[17;1H├────────┼────────┼────────┤\n';

    for (let row = 0; row < 3; row++) {
      content += `\x1b[${18 + row};1H│ `;
      for (let col = 0; col < 3; col++) {
        const value = Math.floor(this.random() * 1000);
        if (value > 500) {
          content += `\x1b[1m${value.toString().padEnd(6)}\x1b[0m`;
          usedCodes.push('1b5b316d', '1b5b6d');
        } else {
          content += value.toString().padEnd(6);
        }
        content += ' │ ';
      }
      content += '\n';
    }
    content += '\x1b[21;1H└────────┴────────┴────────┘\n';

    // Status line with cursor manipulation
    content += `\x1b[${this.config.height - 2};1H`;
    content += '\x1b[K'; // Clear line
    usedCodes.push('1b5b4b');
    content += '\x1b[7m Status: Running | Memory: 42MB | CPU: 15% \x1b[0m';
    usedCodes.push('1b5b376d', '1b5b306d');

    // Frame info
    content += `\x1b[${this.config.height};1H`;
    content += `Frame ${frameNum} | Pattern: mixed | Codes: ${usedCodes.length}`;

    const validation: ValidationRules = {
      textContent: ['Mixed Escape Code Test', 'Data Table', 'Status: Running'],
      cursorPosition: { row: this.config.height, col: 50 },
    };

    return {
      sequence: frameNum,
      content,
      codesUsed: Array.from(new Set(usedCodes)), // Unique codes
      validation,
      timestamp: Date.now(),
      checksum: calculateChecksum(content),
    };
  }

  /**
   * Generate stress test frame with rapid-fire sequences
   */
  generateStressFrame(): EscapeCodeTestFrame {
    const frameNum = ++this.frameCounter;
    const codes = this.getTestCodes();
    const usedCodes: string[] = [];
    let content = '';

    // Clear screen
    content += '\x1b[2J\x1b[H';

    // Header
    content += '\x1b[1;1H\x1b[31;1mSTRESS TEST - RAPID FIRE\x1b[0m\n';

    // Rapid cursor movements
    for (let i = 0; i < 50; i++) {
      const row = Math.floor(this.random() * (this.config.height - 4)) + 3;
      const col = Math.floor(this.random() * (this.config.width - 10)) + 1;
      content += `\x1b[${row};${col}H`;

      // Random style
      const styleCode = codes[Math.floor(this.random() * codes.length)];
      if (styleCode.category === 'SGR') {
        content += styleCode.sequence;
        usedCodes.push(styleCode.code);
      }

      // Random character
      content += String.fromCharCode(33 + Math.floor(this.random() * 94));

      // Random erase
      if (this.random() > 0.7) {
        content += '\x1b[K';
        usedCodes.push('1b5b4b');
      }
    }

    // Rapid color changes
    for (let i = 0; i < 20; i++) {
      const fgColor = Math.floor(this.random() * 256);
      const bgColor = Math.floor(this.random() * 256);
      content += `\x1b[38;5;${fgColor}m\x1b[48;5;${bgColor}m`;
      content += generateTestText(5);
      content += '\x1b[0m';
    }

    // Rapid attribute toggles
    const attributes = ['\x1b[1m', '\x1b[2m', '\x1b[4m', '\x1b[7m'];
    for (let i = 0; i < 30; i++) {
      content += attributes[Math.floor(this.random() * attributes.length)];
      content += 'X';
      content += '\x1b[0m';
      usedCodes.push('1b5b306d');
    }

    // Scroll region stress
    for (let i = 0; i < 5; i++) {
      const top = Math.floor(this.random() * 10) + 1;
      const bottom = Math.floor(this.random() * 10) + 15;
      content += `\x1b[${top};${bottom}r`;
      content += `\x1b[${bottom}S`; // Scroll up
    }

    // Reset scroll region
    content += '\x1b[r';

    // Status
    content += `\x1b[${this.config.height - 1};1H\x1b[K`;
    content += `\x1b[33mFrame ${frameNum} | STRESS | Codes: ${usedCodes.length} | FPS: ${this.config.frameRate}\x1b[0m`;

    const validation: ValidationRules = {
      textContent: ['STRESS TEST'],
      textAbsent: [], // Too chaotic to validate specific absences
    };

    return {
      sequence: frameNum,
      content,
      codesUsed: Array.from(new Set(usedCodes)),
      validation,
      timestamp: Date.now(),
      checksum: calculateChecksum(content),
    };
  }

  /**
   * Generate frames based on pattern
   */
  generateFrames(): EscapeCodeTestFrame[] {
    const codes = this.getTestCodes();
    const frames: EscapeCodeTestFrame[] = [];
    const totalFrames = Math.floor(this.config.duration * this.config.frameRate);

    switch (this.config.pattern) {
      case 'isolated':
        // Test each code in isolation
        for (let i = 0; i < Math.min(totalFrames, codes.length); i++) {
          frames.push(this.generateIsolatedFrame(codes[i]));
        }
        break;

      case 'sequential':
        // Test codes sequentially
        const framesPerCode = Math.max(1, Math.floor(totalFrames / codes.length));
        for (let i = 0; i < codes.length && frames.length < totalFrames; i++) {
          const frame = this.generateIsolatedFrame(codes[i]);
          for (let j = 0; j < framesPerCode && frames.length < totalFrames; j++) {
            frames.push(frame);
          }
        }
        break;

      case 'mixed':
        // Generate mixed realistic frames
        for (let i = 0; i < totalFrames; i++) {
          frames.push(this.generateMixedFrame());
        }
        break;

      case 'stress':
        // Generate stress test frames
        for (let i = 0; i < totalFrames; i++) {
          frames.push(this.generateStressFrame());
        }
        break;
    }

    return frames;
  }

  /**
   * Reset the frame counter
   */
  reset(): void {
    this.frameCounter = 0;
  }
}

/**
 * Convenience function to create generator and generate frames
 */
export function generateEscapeCodeTestFrames(config: EscapeCodeTestConfig): EscapeCodeTestFrame[] {
  const generator = new EscapeCodeFrameGenerator(config);
  return generator.generateFrames();
}

/**
 * Generate a single test frame for a specific code
 */
export function generateSingleCodeFrame(
  codeHex: string,
  config: Partial<EscapeCodeTestConfig> = {}
): EscapeCodeTestFrame | null {
  const code = ESCAPE_CODE_LIBRARY.find(c => c.code === codeHex);
  if (!code) return null;

  const fullConfig: EscapeCodeTestConfig = {
    type: 'escape-code-test',
    pattern: 'isolated',
    frameRate: 30,
    duration: 1,
    width: config.width || 80,
    height: config.height || 24,
    resetBetween: true,
    ...config,
  };

  const generator = new EscapeCodeFrameGenerator(fullConfig);
  return generator.generateIsolatedFrame(code);
}