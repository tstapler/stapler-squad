/**
 * Delta Applicator for Terminal State Synchronization
 *
 * Applies MOSH-style delta compression protocol to xterm.js terminal.
 * Reduces bandwidth by 70-90% by only sending screen changes instead of raw output.
 *
 * Protocol:
 * - Server sends TerminalDelta messages with only changed lines
 * - Client applies deltas to maintain synchronized terminal state
 * - Version tracking prevents desynchronization
 * - Full sync fallback for recovery
 */

import { Terminal } from '@xterm/xterm';
import { TerminalDelta, LineDelta, CursorPosition, TerminalDimensions } from '@/gen/session/v1/events_pb';
import {
  positionAndClearLine,
  moveCursorTo,
  CURSOR_SHOW,
  CURSOR_HIDE,
  DELETE_LINE,
  INSERT_LINE,
  CLEAR_LINE
} from './AnsiCodes';

/**
 * DeltaApplicator applies terminal state deltas to xterm.js terminal.
 */
export class DeltaApplicator {
  private terminal: Terminal;
  private currentVersion: bigint = BigInt(0);
  private textDecoder: TextDecoder = new TextDecoder();

  constructor(terminal: Terminal) {
    this.terminal = terminal;
  }

  /**
   * Apply a terminal delta to the terminal.
   * MOSH-inspired approach: More forgiving of version gaps, self-healing.
   *
   * @param delta - The delta to apply
   * @returns true if applied successfully, false if desync detected
   */
  applyDelta(delta: TerminalDelta): boolean {
    // Handle full sync first (initial state or recovery)
    if (delta.fullSync) {
      console.log(`[DeltaApplicator] Applying full sync: version ${this.currentVersion} -> ${delta.toState}`);
      this.applyFullSync(delta);
      this.currentVersion = delta.toState;
      return true;
    }

    // MOSH-inspired: Accept deltas with reasonable version gaps (out-of-order tolerance)
    const versionGap = Number(delta.fromState) - Number(this.currentVersion);
    const MAX_VERSION_GAP = 10; // Allow up to 10 versions ahead (like MOSH sequence tolerance)

    if (delta.fromState !== this.currentVersion) {
      if (versionGap > 0 && versionGap <= MAX_VERSION_GAP) {
        // Future delta - apply optimistically and update to newer version
        console.warn(
          `[DeltaApplicator] Accepting future delta: current=${this.currentVersion}, got=${delta.fromState} (gap=${versionGap})`
        );
        this.currentVersion = delta.fromState; // Jump forward to match delta
      } else if (versionGap < 0 && Math.abs(versionGap) <= MAX_VERSION_GAP) {
        // Old delta - apply anyway (idempotent screen updates)
        console.warn(
          `[DeltaApplicator] Accepting old delta: current=${this.currentVersion}, got=${delta.fromState} (age=${Math.abs(versionGap)})`
        );
        // Don't update version for old deltas
      } else {
        // Large gap - request full sync
        console.warn(
          `[DeltaApplicator] Version gap too large: current=${this.currentVersion}, got=${delta.fromState} (gap=${versionGap}). Requesting full sync.`
        );
        return false; // Signal caller to request full sync
      }
    }

    // Check for dimension mismatch (prevents data loss from out-of-bounds line numbers)
    // Server includes dimensions in all deltas to detect resize race conditions
    if (delta.dimensions && !delta.fullSync) {
      const deltaRows = Number(delta.dimensions.rows);
      const deltaCols = Number(delta.dimensions.cols);

      if (deltaRows !== this.terminal.rows || deltaCols !== this.terminal.cols) {
        console.warn(
          `[DeltaApplicator] Dimension mismatch: delta=${deltaCols}x${deltaRows}, ` +
          `terminal=${this.terminal.cols}x${this.terminal.rows}. Requesting full resync.`
        );
        return false; // Triggers automatic resync
      }
    }

    // Handle dimension changes
    if (delta.dimensions) {
      this.applyDimensionChange(delta.dimensions);
    }

    // Apply line changes
    for (const lineDelta of delta.lines) {
      this.applyLineDelta(lineDelta);
    }

    // Update cursor position
    if (delta.cursor) {
      this.applyCursorPosition(delta.cursor);
    }

    // Update version tracking
    this.currentVersion = delta.toState;
    return true;
  }

  /**
   * Apply a full sync (complete terminal state).
   */
  private applyFullSync(delta: TerminalDelta): void {
    // Resize if dimensions provided
    if (delta.dimensions) {
      const targetRows = Number(delta.dimensions.rows);
      const targetCols = Number(delta.dimensions.cols);

      if (this.terminal.rows !== targetRows || this.terminal.cols !== targetCols) {
        console.log(`[DeltaApplicator] Resizing terminal from ${this.terminal.cols}x${this.terminal.rows} to ${targetCols}x${targetRows} for full sync`);
        this.terminal.resize(targetCols, targetRows);

        // Verify resize succeeded (browser might override it)
        if (this.terminal.rows !== targetRows || this.terminal.cols !== targetCols) {
          console.error(
            `[DeltaApplicator] Terminal resize failed! Expected ${targetCols}x${targetRows}, got ${this.terminal.cols}x${this.terminal.rows}. ` +
            `This will cause data loss. Delta has ${delta.lines.length} lines but terminal has ${this.terminal.rows} rows.`
          );
          // Continue anyway - some lines will be skipped but at least we don't crash
        }
      }
    }

    // Clear terminal
    this.terminal.clear();

    // Write all lines
    for (const lineDelta of delta.lines) {
      const lineNum = Number(lineDelta.lineNumber);

      // Get line content
      let lineText = '';
      if (lineDelta.operation.case === 'replaceLine') {
        lineText = this.textDecoder.decode(lineDelta.operation.value);
      }

      // Position cursor and write line
      this.terminal.write(moveCursorTo(lineNum + 1, 1) + lineText);
    }

    // Update cursor
    if (delta.cursor) {
      this.applyCursorPosition(delta.cursor);
    }
  }

  /**
   * Apply dimension changes (terminal resize).
   */
  private applyDimensionChange(dimensions: TerminalDimensions): void {
    const rows = Number(dimensions.rows);
    const cols = Number(dimensions.cols);

    if (this.terminal.rows !== rows || this.terminal.cols !== cols) {
      console.log(`[DeltaApplicator] Resizing terminal to ${cols}x${rows}`);
      this.terminal.resize(cols, rows);
    }
  }

  /**
   * Apply changes to a specific line.
   */
  private applyLineDelta(lineDelta: LineDelta): void {
    const lineNum = Number(lineDelta.lineNumber);

    // Validate line number - skip out-of-bounds lines (can happen during resize race conditions)
    // WARNING: This means we're dropping data! Should only happen during browser resize races.
    if (lineNum < 0 || lineNum >= this.terminal.rows) {
      console.warn(
        `[DeltaApplicator] Dropping out-of-bounds line ${lineNum} (terminal has ${this.terminal.rows} rows). ` +
        `This indicates a dimension mismatch - data loss is occurring!`
      );
      return;
    }

    switch (lineDelta.operation.case) {
      case 'replaceLine': {
        // Replace entire line
        const text = this.textDecoder.decode(lineDelta.operation.value);
        // Move cursor to line, clear it, and write new content
        this.terminal.write(positionAndClearLine(lineNum + 1) + text);
        break;
      }

      case 'edit': {
        // Character-level edit within line
        const edit = lineDelta.operation.value;
        const startCol = Number(edit.startCol);
        const text = this.textDecoder.decode(edit.text);
        // Move cursor to position and write text (overwrites existing)
        this.terminal.write(moveCursorTo(lineNum + 1, startCol + 1) + text);
        break;
      }

      case 'deleteLine': {
        // Delete line (shift lines up)
        // Move to line and delete it
        this.terminal.write(moveCursorTo(lineNum + 1, 1) + DELETE_LINE);
        break;
      }

      case 'insert': {
        // Insert new line (shift lines down)
        const insert = lineDelta.operation.value;
        const text = this.textDecoder.decode(insert.text);
        // Move to line, insert blank line, then write text
        this.terminal.write(moveCursorTo(lineNum + 1, 1) + INSERT_LINE + text);
        break;
      }

      case 'clearLine': {
        // Clear line to empty
        this.terminal.write(moveCursorTo(lineNum + 1, 1) + CLEAR_LINE);
        break;
      }

      default:
        console.warn(`[DeltaApplicator] Unknown line operation: ${lineDelta.operation.case}`);
    }
  }

  /**
   * Apply cursor position update.
   */
  private applyCursorPosition(cursor: CursorPosition): void {
    const row = Number(cursor.row);
    const col = Number(cursor.col);

    // Move cursor to position
    this.terminal.write(moveCursorTo(row + 1, col + 1));

    // Handle cursor visibility
    if (cursor.visible) {
      this.terminal.write(CURSOR_SHOW);
    } else {
      this.terminal.write(CURSOR_HIDE);
    }
  }

  /**
   * Get current version for synchronization.
   */
  getCurrentVersion(): bigint {
    return this.currentVersion;
  }

  /**
   * Reset version tracking (for recovery).
   */
  resetVersion(): void {
    this.currentVersion = BigInt(0);
  }
}
