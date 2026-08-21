/**
 * ANSI Escape Code Constants for Terminal Control
 *
 * These constants provide human-readable names for ANSI escape sequences
 * used to control terminal behavior (cursor positioning, clearing, colors, etc.).
 *
 * Reference: https://en.wikipedia.org/wiki/ANSI_escape_code
 */

/**
 * Clear operations
 */
export const CLEAR_LINE = '\x1b[2K';        // Erase entire line
export const CLEAR_TO_END_OF_LINE = '\x1b[0K';  // Erase from cursor to end of line
export const CLEAR_TO_START_OF_LINE = '\x1b[1K'; // Erase from cursor to start of line
export const CLEAR_SCREEN = '\x1b[2J';      // Erase entire screen
export const CLEAR_SCREEN_AND_SCROLLBACK = '\x1b[3J'; // Erase screen and scrollback buffer

/**
 * Cursor positioning (parameterized - use template literals)
 *
 * Examples:
 *   moveCursorTo(10, 5) // Position cursor at row 10, column 5
 *   moveCursorToColumn(1) // Move cursor to first column
 */
export const moveCursorTo = (row: number, col: number) => `\x1b[${row};${col}H`;
export const moveCursorToColumn = (col: number) => `\x1b[${col}G`;
export const moveCursorUp = (n: number = 1) => `\x1b[${n}A`;
export const moveCursorDown = (n: number = 1) => `\x1b[${n}B`;
export const moveCursorRight = (n: number = 1) => `\x1b[${n}C`;
export const moveCursorLeft = (n: number = 1) => `\x1b[${n}D`;

/**
 * Cursor position shortcuts
 */
export const CURSOR_HOME = '\x1b[H';        // Move cursor to home position (1,1)
export const CURSOR_TO_START_OF_LINE = '\r'; // Move cursor to start of current line

/**
 * Cursor visibility
 */
export const CURSOR_SHOW = '\x1b[?25h';     // Show cursor
export const CURSOR_HIDE = '\x1b[?25l';     // Hide cursor

/**
 * Line operations
 */
export const INSERT_LINE = '\x1b[L';        // Insert blank line at cursor
export const DELETE_LINE = '\x1b[M';        // Delete line at cursor

/**
 * Terminal reset and mode changes
 */
export const RESET_TERMINAL = '\x1b[0m';    // Reset all terminal attributes
export const RESET_COLOR = '\x1b[39m';      // Reset foreground color to default
export const RESET_BACKGROUND = '\x1b[49m'; // Reset background color to default

/**
 * Text formatting (SGR - Select Graphic Rendition)
 */
export const BOLD = '\x1b[1m';
export const DIM = '\x1b[2m';
export const ITALIC = '\x1b[3m';
export const UNDERLINE = '\x1b[4m';
export const BLINK = '\x1b[5m';
export const REVERSE = '\x1b[7m';
export const HIDDEN = '\x1b[8m';
export const STRIKETHROUGH = '\x1b[9m';

/**
 * Helper function to compose multiple escape sequences
 */
export const compose = (...codes: string[]): string => codes.join('');

/**
 * Helper function to position cursor, clear line, and prepare for writing
 * This is a common pattern in terminal state synchronization
 */
export const positionAndClearLine = (row: number, col: number = 1): string =>
  compose(moveCursorTo(row, col), CLEAR_LINE);
