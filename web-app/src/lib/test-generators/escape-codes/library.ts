/**
 * ANSI Escape Code Library
 * Complete library of all 77 escape codes observed in production
 * Based on ~/Downloads/escape-codes.json (26,974 total occurrences)
 */

import {
  EscapeCodeDefinition,
  EscapeCategory,
  TestPriority,
  PRIORITY_THRESHOLDS,
} from './types';

/**
 * Convert hex string to escape sequence
 */
function hexToSequence(hex: string): string {
  const bytes = hex.match(/.{2}/g) || [];
  return bytes.map(b => String.fromCharCode(parseInt(b, 16))).join('');
}

/**
 * Calculate priority based on production count
 */
function getPriority(count: number): TestPriority {
  if (count > PRIORITY_THRESHOLDS.critical) return 'critical';
  if (count > PRIORITY_THRESHOLDS.high) return 'high';
  if (count > PRIORITY_THRESHOLDS.medium) return 'medium';
  return 'low';
}

/**
 * Complete escape code library from production data
 * Sorted by production frequency (highest first)
 */
export const ESCAPE_CODE_LIBRARY: EscapeCodeDefinition[] = [
  // ============== CRITICAL PRIORITY (>1000 occurrences) ==============
  // These 5 codes account for 53% of all production usage

  {
    code: '1b5b4b',
    sequence: '\x1b[K',
    humanReadable: 'Erase to End of Line',
    category: 'Erase',
    count: 4767,
    priority: 'critical',
    supported: true,
    example: 'Clears from cursor to end of line',
    notes: 'Critical for progress indicators and line updates',
  },
  {
    code: '1b5b33396d',
    sequence: '\x1b[39m',
    humanReadable: 'Default Foreground',
    category: 'SGR',
    count: 3678,
    priority: 'critical',
    supported: true,
    example: 'Resets foreground color to terminal default',
    notes: 'Essential for color state management',
  },
  {
    code: '1b2842',
    sequence: '\x1b(B',
    humanReadable: 'Designate G0 character set (ASCII)',
    category: 'Charset',
    count: 3366,
    priority: 'critical',
    supported: true,
    example: 'Sets standard ASCII character encoding',
    notes: 'Critical for text display accuracy',
  },
  {
    code: '1b5b6d',
    sequence: '\x1b[m',
    humanReadable: 'Reset Attributes',
    category: 'SGR',
    count: 3366,
    priority: 'critical',
    supported: true,
    example: 'Resets all text attributes to default',
    notes: 'Shorthand for \\x1b[0m',
  },
  {
    code: '1b5b33383b353b3233316d',
    sequence: '\x1b[38;5;231m',
    humanReadable: 'Foreground 256-Color 231',
    category: 'SGR',
    count: 2208,
    priority: 'critical',
    supported: true,
    params: [{ name: 'color', type: 'range', min: 0, max: 255, default: 231 }],
    example: 'Sets foreground to color 231 (white)',
  },

  // ============== HIGH PRIORITY (>100 occurrences) ==============

  {
    code: '1b5b33383b353b3234366d',
    sequence: '\x1b[38;5;246m',
    humanReadable: 'Foreground 256-Color 246',
    category: 'SGR',
    count: 2097,
    priority: 'high',
    supported: true,
    params: [{ name: 'color', type: 'range', min: 0, max: 255, default: 246 }],
  },
  {
    code: '1b5b316d',
    sequence: '\x1b[1m',
    humanReadable: 'Bold',
    category: 'SGR',
    count: 1211,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b34396d',
    sequence: '\x1b[49m',
    humanReadable: 'Default Background',
    category: 'SGR',
    count: 1100,
    priority: 'high',
    supported: true,
    example: 'Resets background color to terminal default',
  },
  {
    code: '1b5b34383b353b36356d',
    sequence: '\x1b[48;5;65m',
    humanReadable: 'Background 256-Color 65',
    category: 'SGR',
    count: 771,
    priority: 'high',
    supported: true,
    params: [{ name: 'color', type: 'range', min: 0, max: 255, default: 65 }],
  },
  {
    code: '1b5b34383b353b39356d',
    sequence: '\x1b[48;5;95m',
    humanReadable: 'Background 256-Color 95',
    category: 'SGR',
    count: 439,
    priority: 'high',
    supported: true,
    params: [{ name: 'color', type: 'range', min: 0, max: 255, default: 95 }],
  },
  {
    code: '1b5b33383b353b3135336d',
    sequence: '\x1b[38;5;153m',
    humanReadable: 'Foreground 256-Color 153',
    category: 'SGR',
    count: 438,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b33383b353b3131346d',
    sequence: '\x1b[38;5;114m',
    humanReadable: 'Foreground 256-Color 114',
    category: 'SGR',
    count: 426,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b33383b353b3137346d',
    sequence: '\x1b[38;5;174m',
    humanReadable: 'Foreground 256-Color 174',
    category: 'SGR',
    count: 247,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b326d',
    sequence: '\x1b[2m',
    humanReadable: 'Dim',
    category: 'SGR',
    count: 225,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b48',
    sequence: '\x1b[H',
    humanReadable: 'Cursor Position (Home)',
    category: 'Cursor',
    count: 207,
    priority: 'high',
    supported: true,
    example: 'Moves cursor to position (1,1)',
  },
  {
    code: '1b5b313b3148',
    sequence: '\x1b[1;1H',
    humanReadable: 'Cursor Position (1;1)',
    category: 'Cursor',
    count: 173,
    priority: 'high',
    supported: true,
    params: [
      { name: 'row', type: 'number', min: 1, default: 1 },
      { name: 'col', type: 'number', min: 1, default: 1 },
    ],
  },
  {
    code: '1b5b33383b353b3231316d',
    sequence: '\x1b[38;5;211m',
    humanReadable: 'Foreground 256-Color 211',
    category: 'SGR',
    count: 172,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b34383b353b37326d',
    sequence: '\x1b[48;5;72m',
    humanReadable: 'Background 256-Color 72',
    category: 'SGR',
    count: 123,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b34326d',
    sequence: '\x1b[42m',
    humanReadable: 'Background Green',
    category: 'SGR',
    count: 105,
    priority: 'high',
    supported: true,
  },
  {
    code: '1b5b33306d',
    sequence: '\x1b[30m',
    humanReadable: 'Foreground Black',
    category: 'SGR',
    count: 105,
    priority: 'high',
    supported: true,
  },

  // ============== MEDIUM PRIORITY (>10 occurrences) ==============

  {
    code: '1b5b323464',
    sequence: '\x1b[24d',
    humanReadable: 'Line Position Absolute (24)',
    category: 'Cursor',
    count: 99,
    priority: 'medium',
    supported: true,
    params: [{ name: 'line', type: 'number', min: 1, default: 24 }],
  },
  {
    code: '1b5b393b3148',
    sequence: '\x1b[9;1H',
    humanReadable: 'Cursor Position (9;1)',
    category: 'Cursor',
    count: 79,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b363b3148',
    sequence: '\x1b[6;1H',
    humanReadable: 'Cursor Position (6;1)',
    category: 'Cursor',
    count: 76,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b32313b3148',
    sequence: '\x1b[21;1H',
    humanReadable: 'Cursor Position (21;1)',
    category: 'Cursor',
    count: 74,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b343b3148',
    sequence: '\x1b[4;1H',
    humanReadable: 'Cursor Position (4;1)',
    category: 'Cursor',
    count: 70,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b353b3148',
    sequence: '\x1b[5;1H',
    humanReadable: 'Cursor Position (5;1)',
    category: 'Cursor',
    count: 70,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31373b3148',
    sequence: '\x1b[17;1H',
    humanReadable: 'Cursor Position (17;1)',
    category: 'Cursor',
    count: 69,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31303b3148',
    sequence: '\x1b[10;1H',
    humanReadable: 'Cursor Position (10;1)',
    category: 'Cursor',
    count: 68,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b383b3148',
    sequence: '\x1b[8;1H',
    humanReadable: 'Cursor Position (8;1)',
    category: 'Cursor',
    count: 67,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31393b3148',
    sequence: '\x1b[19;1H',
    humanReadable: 'Cursor Position (19;1)',
    category: 'Cursor',
    count: 66,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31353b3148',
    sequence: '\x1b[15;1H',
    humanReadable: 'Cursor Position (15;1)',
    category: 'Cursor',
    count: 66,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31343b3148',
    sequence: '\x1b[14;1H',
    humanReadable: 'Cursor Position (14;1)',
    category: 'Cursor',
    count: 65,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b32333b3148',
    sequence: '\x1b[23;1H',
    humanReadable: 'Cursor Position (23;1)',
    category: 'Cursor',
    count: 64,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31383b3148',
    sequence: '\x1b[18;1H',
    humanReadable: 'Cursor Position (18;1)',
    category: 'Cursor',
    count: 64,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31333b3148',
    sequence: '\x1b[13;1H',
    humanReadable: 'Cursor Position (13;1)',
    category: 'Cursor',
    count: 64,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b32303b3148',
    sequence: '\x1b[20;1H',
    humanReadable: 'Cursor Position (20;1)',
    category: 'Cursor',
    count: 63,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b373b3148',
    sequence: '\x1b[7;1H',
    humanReadable: 'Cursor Position (7;1)',
    category: 'Cursor',
    count: 63,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b333b3148',
    sequence: '\x1b[3;1H',
    humanReadable: 'Cursor Position (3;1)',
    category: 'Cursor',
    count: 62,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31363b3148',
    sequence: '\x1b[16;1H',
    humanReadable: 'Cursor Position (16;1)',
    category: 'Cursor',
    count: 62,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b34383b353b3137346d',
    sequence: '\x1b[48;5;174m',
    humanReadable: 'Background 256-Color 174',
    category: 'SGR',
    count: 58,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b323b3148',
    sequence: '\x1b[2;1H',
    humanReadable: 'Cursor Position (2;1)',
    category: 'Cursor',
    count: 58,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b32323b3148',
    sequence: '\x1b[22;1H',
    humanReadable: 'Cursor Position (22;1)',
    category: 'Cursor',
    count: 58,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31313b3148',
    sequence: '\x1b[11;1H',
    humanReadable: 'Cursor Position (11;1)',
    category: 'Cursor',
    count: 56,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b31323b3148',
    sequence: '\x1b[12;1H',
    humanReadable: 'Cursor Position (12;1)',
    category: 'Cursor',
    count: 48,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b34383b353b3233376d',
    sequence: '\x1b[48;5;237m',
    humanReadable: 'Background 256-Color 237',
    category: 'SGR',
    count: 36,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b34383b353b31366d',
    sequence: '\x1b[48;5;16m',
    humanReadable: 'Background 256-Color 16',
    category: 'SGR',
    count: 25,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b376d',
    sequence: '\x1b[7m',
    humanReadable: 'Reverse Video',
    category: 'SGR',
    count: 20,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b33346d',
    sequence: '\x1b[34m',
    humanReadable: 'Foreground Blue',
    category: 'SGR',
    count: 14,
    priority: 'medium',
    supported: true,
  },
  {
    code: '1b5b33326d',
    sequence: '\x1b[32m',
    humanReadable: 'Foreground Green',
    category: 'SGR',
    count: 12,
    priority: 'medium',
    supported: true,
  },

  // ============== LOW PRIORITY (<=10 occurrences) ==============

  {
    code: '1b5b33366d',
    sequence: '\x1b[36m',
    humanReadable: 'Foreground Cyan',
    category: 'SGR',
    count: 5,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b323372',
    sequence: '\x1b[1;23r',
    humanReadable: 'Set Scroll Region (1;23)',
    category: 'Scroll',
    count: 4,
    priority: 'low',
    supported: true,
    params: [
      { name: 'top', type: 'number', min: 1, default: 1 },
      { name: 'bottom', type: 'number', min: 1, default: 23 },
    ],
  },
  {
    code: '1b5b323353',
    sequence: '\x1b[23S',
    humanReadable: 'Scroll Up (23)',
    category: 'Scroll',
    count: 4,
    priority: 'low',
    supported: true,
    params: [{ name: 'lines', type: 'number', min: 1, default: 23 }],
  },
  {
    code: '1b5b313b323472',
    sequence: '\x1b[1;24r',
    humanReadable: 'Set Scroll Region (1;24)',
    category: 'Scroll',
    count: 4,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b346d',
    sequence: '\x1b[4m',
    humanReadable: 'Underline',
    category: 'SGR',
    count: 4,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b33316d',
    sequence: '\x1b[31m',
    humanReadable: 'Foreground Red',
    category: 'SGR',
    count: 4,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3758',
    sequence: '\x1b[7X',
    humanReadable: 'Erase Characters (7)',
    category: 'Erase',
    count: 2,
    priority: 'low',
    supported: true,
    params: [{ name: 'count', type: 'number', min: 1, default: 7 }],
  },
  {
    code: '1b5b373b333248',
    sequence: '\x1b[7;32H',
    humanReadable: 'Cursor Position (7;32)',
    category: 'Cursor',
    count: 2,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3743',
    sequence: '\x1b[7C',
    humanReadable: 'Cursor Forward (7)',
    category: 'Cursor',
    count: 2,
    priority: 'low',
    supported: true,
    params: [{ name: 'count', type: 'number', min: 1, default: 7 }],
  },
  {
    code: '1b5b3f313030336c',
    sequence: '\x1b[?1003l',
    humanReadable: 'Disable All Motion Mouse Tracking',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313030306c',
    sequence: '\x1b[?1000l',
    humanReadable: 'Disable X11 Mouse Reporting (Normal)',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3358',
    sequence: '\x1b[3X',
    humanReadable: 'Erase Characters (3)',
    category: 'Erase',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b324a',
    sequence: '\x1b[2J',
    humanReadable: 'Erase All (Full Screen)',
    category: 'Erase',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b363272',
    sequence: '\x1b[1;62r',
    humanReadable: 'Set Scroll Region (1;62)',
    category: 'Scroll',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3343',
    sequence: '\x1b[3C',
    humanReadable: 'Cursor Forward (3)',
    category: 'Cursor',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f323568',
    sequence: '\x1b[?25h',
    humanReadable: 'Enable Cursor Visibility (DECTCEM)',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313034396c',
    sequence: '\x1b[?1049l',
    humanReadable: 'Disable Alternate Screen Buffer',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f31326c',
    sequence: '\x1b[?12l',
    humanReadable: 'Disable Cursor Blink (att610)',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313030346c',
    sequence: '\x1b[?1004l',
    humanReadable: 'Disable Focus In/Out Events',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f316c',
    sequence: '\x1b[?1l',
    humanReadable: 'Disable Application Cursor Keys (DECCKM)',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3158',
    sequence: '\x1b[1X',
    humanReadable: 'Erase Characters (1)',
    category: 'Erase',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b3072',
    sequence: '\x1b[1;0r',
    humanReadable: 'Set Scroll Region (1;0)',
    category: 'Scroll',
    count: 1,
    priority: 'low',
    supported: true,
    notes: 'Unusual - bottom=0 means full screen',
  },
  {
    code: '1b5b3f373732376c',
    sequence: '\x1b[?7727l',
    humanReadable: 'DEC Private Mode 7727',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: false,
    notes: 'Application escape mode - may not be supported',
  },
  {
    code: '1b5b32333b303b3074',
    sequence: '\x1b[23;0;0t',
    humanReadable: 'Window Manipulation (Restore Title)',
    category: 'CSI',
    count: 1,
    priority: 'low',
    supported: true,
    notes: 'Restore window title from stack',
  },
  {
    code: '1b5b3f323030346c',
    sequence: '\x1b[?2004l',
    humanReadable: 'Disable Bracketed Paste Mode',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313030326c',
    sequence: '\x1b[?1002l',
    humanReadable: 'Disable Cell Motion Mouse Tracking',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b43',
    sequence: '\x1b[C',
    humanReadable: 'Cursor Forward (1)',
    category: 'Cursor',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b373072',
    sequence: '\x1b[1;70r',
    humanReadable: 'Set Scroll Region (1;70)',
    category: 'Scroll',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313030356c',
    sequence: '\x1b[?1005l',
    humanReadable: 'Disable UTF-8 Mouse Encoding',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b333872',
    sequence: '\x1b[1;38r',
    humanReadable: 'Set Scroll Region (1;38)',
    category: 'Scroll',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b3f313030366c',
    sequence: '\x1b[?1006l',
    humanReadable: 'Disable SGR Mouse Encoding',
    category: 'DECPriv',
    count: 1,
    priority: 'low',
    supported: true,
  },
  {
    code: '1b5b313b363472',
    sequence: '\x1b[1;64r',
    humanReadable: 'Set Scroll Region (1;64)',
    category: 'Scroll',
    count: 1,
    priority: 'low',
    supported: true,
  },
];

/**
 * Get codes by category
 */
export function getCodesByCategory(category: EscapeCategory): EscapeCodeDefinition[] {
  return ESCAPE_CODE_LIBRARY.filter(code => code.category === category);
}

/**
 * Get codes by priority
 */
export function getCodesByPriority(priority: TestPriority): EscapeCodeDefinition[] {
  return ESCAPE_CODE_LIBRARY.filter(code => code.priority === priority);
}

/**
 * Get codes at or above priority level
 */
export function getCodesAtOrAbovePriority(minPriority: TestPriority): EscapeCodeDefinition[] {
  const priorityOrder: TestPriority[] = ['critical', 'high', 'medium', 'low'];
  const minIndex = priorityOrder.indexOf(minPriority);
  return ESCAPE_CODE_LIBRARY.filter(code => {
    const codeIndex = priorityOrder.indexOf(code.priority);
    return codeIndex <= minIndex;
  });
}

/**
 * Get all categories with stats
 */
export function getCategoryStats(): { category: EscapeCategory; count: number; totalOccurrences: number }[] {
  const categories: EscapeCategory[] = ['SGR', 'Cursor', 'Erase', 'Scroll', 'DECPriv', 'Charset', 'CSI'];
  return categories.map(category => {
    const codes = getCodesByCategory(category);
    return {
      category,
      count: codes.length,
      totalOccurrences: codes.reduce((sum, code) => sum + code.count, 0),
    };
  });
}

/**
 * Get total statistics
 */
export function getLibraryStats() {
  return {
    totalCodes: ESCAPE_CODE_LIBRARY.length,
    totalOccurrences: ESCAPE_CODE_LIBRARY.reduce((sum, code) => sum + code.count, 0),
    byPriority: {
      critical: getCodesByPriority('critical').length,
      high: getCodesByPriority('high').length,
      medium: getCodesByPriority('medium').length,
      low: getCodesByPriority('low').length,
    },
    byCategory: getCategoryStats(),
  };
}

/**
 * Find code by hex
 */
export function findCodeByHex(hex: string): EscapeCodeDefinition | undefined {
  return ESCAPE_CODE_LIBRARY.find(code => code.code === hex);
}

/**
 * Find code by sequence
 */
export function findCodeBySequence(sequence: string): EscapeCodeDefinition | undefined {
  return ESCAPE_CODE_LIBRARY.find(code => code.sequence === sequence);
}
