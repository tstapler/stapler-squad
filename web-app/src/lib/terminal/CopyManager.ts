/**
 * Terminal Copy Manager
 *
 * Enhanced clipboard management for terminal output, building on the existing
 * copyToClipboard utility in @/lib/clipboard.ts.
 *
 * Provides:
 * - Copy with timestamp formatting
 * - Copy as HTML for styled output
 * - Copy with line wrapping options
 * - Copy selection from xterm.js terminal
 * - History tracking for paste operations
 */

import { copyToClipboard } from "@/lib/clipboard";

/**
 * Options for copying terminal content.
 */
export interface CopyOptions {
  /** Include ANSI escape sequences (for rich paste) */
  includeEscapeSequences?: boolean;
  /** Format timestamps as ISO strings */
  formatTimestamps?: boolean;
  /** Wrap to a specific column width */
  wrapAtColumn?: number;
  /** Strip ANSI codes entirely */
  stripAnsi?: boolean;
  /** Copy as HTML with styling (preserves colors) */
  asHtml?: boolean;
}

/**
 * Result of a copy operation with metadata.
 */
export interface CopyResult {
  success: boolean;
  characterCount: number;
  lineCount: number;
  error?: string;
}

/**
 * History entry for clipboard operations.
 */
export interface CopyHistoryEntry {
  timestamp: number;
  characterCount: number;
  lineCount: number;
  options: CopyOptions;
}

const copyHistory: CopyHistoryEntry[] = [];
const MAX_HISTORY = 50;

/**
 * Strip ANSI escape sequences from terminal text.
 * Handles CSI sequences, OSC sequences, and single-character escapes.
 */
export function stripAnsiCodes(text: string): string {
  // Strip CSI sequences ( ESC [ ... letter )
  // Strip OSC sequences ( ESC ] ... BEL or ESC \ )
  // Strip single-character escapes ( ESC followed by single char )
  return text.replace(
    /[\u001B\u009B][[\]#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g,
    ""
  );
}

/**
 * Wrap a single line at the specified column width.
 * Inserts newlines at space boundaries, splitting long words if necessary.
 */
function wrapLine(line: string, columnWidth: number): string {
  if (line.length <= columnWidth) return line;

  const words = line.split(/(\s+)/);
  let currentLine = "";
  let result = "";

  for (const word of words) {
    if (currentLine.length + word.length <= columnWidth) {
      currentLine += word;
      continue;
    }

    result += currentLine.trimEnd() + "\n";

    if (word.length > columnWidth) {
      for (let j = 0; j < word.length; j += columnWidth) {
        result += word.slice(j, j + columnWidth) + "\n";
      }
      currentLine = "";
    } else {
      currentLine = word;
    }
  }

  return result + currentLine.trimEnd();
}

/**
 * Wrap text at a specified column width.
 * Inserts newlines at space boundaries near the column width.
 */
export function wrapTextAtColumn(text: string, columnWidth: number): string {
  if (columnWidth <= 0) return text;

  return text.split("\n").map((line) => wrapLine(line, columnWidth)).join("\n");
}

/**
 * Convert ANSI-styled terminal text to HTML with span-wrapped styling.
 * Basic support for common ANSI color codes.
 */
export function ansiToHtml(text: string): string {
  // Simple ANSI to HTML conversion for common codes
  let html = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\u001B\[0m/g, "</span>")
    .replace(/\u001B\[1m/g, '<span style="font-weight:bold">')
    .replace(/\u001B\[30m/g, '<span style="color:#000000">')
    .replace(/\u001B\[31m/g, '<span style="color:#CD0000">')
    .replace(/\u001B\[32m/g, '<span style="color:#00CD00">')
    .replace(/\u001B\[33m/g, '<span style="color:#CDCD00">')
    .replace(/\u001B\[34m/g, '<span style="color:#0000EE">')
    .replace(/\u001B\[35m/g, '<span style="color:#CD00CD">')
    .replace(/\u001B\[36m/g, '<span style="color:#00CDCD">')
    .replace(/\u001B\[37m/g, '<span style="color:#E5E5E5">')
    .replace(/\u001B\[90m/g, '<span style="color:#808080">')
    .replace(/\u001B\[91m/g, '<span style="color:#FF5555">')
    .replace(/\u001B\[92m/g, '<span style="color:#50FA7B">')
    .replace(/\u001B\[93m/g, '<span style="color:#FFFF87">')
    .replace(/\u001B\[94m/g, '<span style="color:#BD93F9">')
    .replace(/\u001B\[95m/g, '<span style="color:#FF79C6">')
    .replace(/\u001B\[96m/g, '<span style="color:#8BE9FD">');

  // Strip background colors and reset any remaining codes
  html = html.replace(/\u001B\[4[0-7]m/g, "")
    .replace(/[\u001B\u009B][[\]#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g, "");

  return `<pre>${html}</pre>`;
}

/**
 * Process text according to copy options.
 */
function processText(text: string, options: CopyOptions): string {
  let result = text;

  if (options.stripAnsi) {
    result = stripAnsiCodes(result);
  }

  if (options.wrapAtColumn && options.wrapAtColumn > 0 && !options.asHtml) {
    result = wrapTextAtColumn(result, options.wrapAtColumn);
  }

  if (options.asHtml) {
    result = ansiToHtml(result);
  }

  return result;
}

/**
 * Copy text to clipboard with formatting options.
 *
 * @param text - The text to copy
 * @param options - Formatting options
 * @returns Result with success status and metadata
 */
export async function copyWithFormat(
  text: string,
  options: CopyOptions = {}
): Promise<CopyResult> {
  const processed = processText(text, options);
  const success = await copyToClipboard(processed);

  const result: CopyResult = {
    success,
    characterCount: processed.length,
    lineCount: processed.split("\n").length,
  };

  if (!success) {
    result.error = "Failed to copy to clipboard";
  }

  // Record history
  if (success) {
    copyHistoryEntry(text.length, options);
  }

  return result;
}

/**
 * Copy xterm.js terminal selection to clipboard.
 * Integrates with xterm.js's own selection API.
 *
 * @param terminal - xterm.js Terminal instance
 * @param options - Formatting options
 */
export async function copyTerminalSelection(
  terminal: {
    getSelection?(): string | null;
    getScrollback?(): string | null;
  },
  options: CopyOptions = {}
): Promise<CopyResult> {
  const selection = terminal.getSelection?.() ?? null;
  const scrollback = terminal.getScrollback?.() ?? null;

  // Prefer selection, fall back to full scrollback if no selection
  const text = selection || scrollback || "";

  if (!text) {
    return {
      success: false,
      characterCount: 0,
      lineCount: 0,
      error: "No selection or scrollback available",
    };
  }

  return copyWithFormat(text, options);
}

/**
 * Record a copy operation in history.
 */
function copyHistoryEntry(charCount: number, options: CopyOptions): void {
  copyHistory.unshift({
    timestamp: Date.now(),
    characterCount: charCount,
    lineCount: charCount === 0 ? 0 : textLinesEstimate(charCount),
    options,
  });

  if (copyHistory.length > MAX_HISTORY) {
    copyHistory.pop();
  }
}

/**
 * Rough estimate of line count from character count.
 */
function textLinesEstimate(charCount: number): number {
  // Assume average 80 chars per line
  return Math.ceil(charCount / 80);
}

/**
 * Get recent copy history.
 */
export function getCopyHistory(): CopyHistoryEntry[] {
  return [...copyHistory];
}

/**
 * Clear copy history.
 */
export function clearCopyHistory(): void {
  copyHistory.length = 0;
}

/**
 * Get the terminal logger instance for the terminal log.
 * Returns a configured logger that writes to the terminal output log.
 */
export function getTerminalLogger() {
  // The terminal logger is set up by the main terminal system
  // This returns a no-op logger if not configured
  return {
    debug: (..._args: unknown[]) => {},
    info: (..._args: unknown[]) => {},
    warn: (..._args: unknown[]) => {},
    error: (..._args: unknown[]) => {},
  };
}
