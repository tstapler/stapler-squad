/**
 * State Applicator for MOSH-Style Terminal State Synchronization
 *
 * Applies complete terminal state snapshots to xterm.js terminal.
 * Based on MOSH (Mobile Shell) State Synchronization Protocol (SSP).
 *
 * Key Benefits:
 * - Idempotent updates: Same sequence number = same result
 * - Out-of-order tolerance: Handle network packet reordering
 * - Self-healing: Complete state prevents desync accumulation
 * - Compression-friendly: LZMA optimizes complete state transmission
 *
 * Protocol:
 * - Server sends TerminalState messages with complete screen buffer
 * - Client applies states by sequence number (monotonic)
 * - ANSI escape sequences preserved in raw byte content
 * - Automatic dimension synchronization
 */

import { Terminal } from '@xterm/xterm';
import { TerminalState, TerminalLine, CursorPosition, TerminalDimensions } from '@/gen/session/v1/events_pb';
import { positionAndClearLine, moveCursorTo, CURSOR_SHOW, CURSOR_HIDE, CLEAR_SCREEN, CLEAR_SCREEN_AND_SCROLLBACK, CURSOR_HOME } from './AnsiCodes';

/**
 * StateApplicator applies complete terminal states to xterm.js terminal.
 * MOSH-inspired approach for robust terminal synchronization.
 */
export class StateApplicator {
  private terminal: Terminal;
  private currentSequence: bigint = BigInt(0);
  private textDecoder: TextDecoder = new TextDecoder();
  private lastAppliedState: TerminalState | null = null;
  private isApplyingState: boolean = false; // Flag to prevent scroll event handling during state application

  constructor(terminal: Terminal) {
    this.terminal = terminal;
  }

  /**
   * Apply a complete terminal state to the terminal.
   * MOSH-inspired approach: Idempotent, out-of-order tolerant, self-healing.
   *
   * @param state - The complete terminal state to apply
   * @returns true if applied successfully, false if should be ignored
   */
  applyState(state: TerminalState): boolean {
    const stateSequence = state.sequence;

    // MOSH-inspired sequence handling
    if (stateSequence <= this.currentSequence) {
      // Old or duplicate state - ignore (idempotent behavior)
      if (stateSequence === this.currentSequence) {
        console.log(`[StateApplicator] Ignoring duplicate state sequence ${stateSequence}`);
      } else {
        console.log(`[StateApplicator] Ignoring old state sequence ${stateSequence} (current: ${this.currentSequence})`);
      }
      return false;
    }

    // Future state - check for reasonable sequence gap
    const sequenceGap = Number(stateSequence) - Number(this.currentSequence);
    const MAX_SEQUENCE_GAP = 100; // Allow larger gaps than deltas since states are self-contained

    if (sequenceGap > MAX_SEQUENCE_GAP) {
      console.warn(
        `[StateApplicator] Large sequence gap detected: current=${this.currentSequence}, ` +
        `received=${stateSequence} (gap=${sequenceGap}). Applying anyway (state is self-contained).`
      );
    }

    console.log(`[StateApplicator] Applying state sequence ${stateSequence} (current: ${this.currentSequence})`);

    // Set flag to prevent scroll event handling during state application
    this.isApplyingState = true;

    try {
      // Apply dimension changes first (prevents out-of-bounds rendering)
      if (state.dimensions) {
        this.applyDimensionChange(state.dimensions);
      }

      // Validate dimension synchronization (essential for correct rendering)
      const stateDims = state.dimensions;
      const terminalDims = { cols: this.terminal.cols, rows: this.terminal.rows };
      if (stateDims && (stateDims.cols !== terminalDims.cols || stateDims.rows !== terminalDims.rows)) {
        console.warn(
          `[StateApplicator] DIMENSION MISMATCH: ` +
          `state=${stateDims.cols}x${stateDims.rows}, ` +
          `terminal=${terminalDims.cols}x${terminalDims.rows}`
        );
      }

      // Validate line count matches terminal rows
      const lineCount = state.lines.length;
      const expectedLines = terminalDims.rows;
      if (lineCount !== expectedLines) {
        console.warn(
          `[StateApplicator] LINE COUNT MISMATCH: ` +
          `state has ${lineCount} lines, ` +
          `terminal has ${expectedLines} rows, ` +
          `difference: ${lineCount - expectedLines} lines`
        );
      }

      // NOTE: Cursor position validation removed - we don't use explicit cursor positioning
      // Content includes ANSI escape sequences that position cursor correctly

      // Apply complete terminal state (includes cursor positioning via ANSI sequences)
      this.applyCompleteState(state);

      // NOTE: Don't explicitly position cursor - let ANSI escape sequences in content handle it
      // Explicit cursor positioning conflicts with MOSH-style complete states and causes
      // out-of-bounds errors when calculated position doesn't match terminal dimensions
      //
      // The terminal content from tmux includes cursor positioning escape sequences,
      // so cursor will be positioned correctly by the content itself

      // Update sequence tracking
      this.currentSequence = stateSequence;
      this.lastAppliedState = state;

      console.log(
        `[StateApplicator] Successfully applied state sequence ${stateSequence} ` +
        `(${lineCount} lines, cursor=(${state.cursor?.col},${state.cursor?.row}))`
      );
    } finally {
      // Always clear the flag, even if an error occurred
      this.isApplyingState = false;
    }

    return true;
  }

  /**
   * Apply complete terminal state (MOSH-style complete screen buffer).
   * This replaces the entire terminal content with the new state.
   */
  private applyCompleteState(state: TerminalState): void {
    // MOSH-STYLE COMPLETE STATE: Clear terminal and write each line at its row position
    // Server splits tmux output by newlines, so each line = one terminal row
    // Each line contains text + ANSI styling codes, but NOT cursor positioning between lines
    this.terminal.clear();
    this.terminal.write(CURSOR_HIDE);

    // Write each line at its specific row position
    for (let i = 0; i < state.lines.length; i++) {
      const line = state.lines[i];

      // Skip out-of-bounds lines (shouldn't happen with proper dimension sync)
      if (i >= this.terminal.rows) {
        console.warn(
          `[StateApplicator] Dropping line ${i} - exceeds terminal rows ${this.terminal.rows}. ` +
          `State has ${state.lines.length} lines, dimension mismatch detected.`
        );
        break;
      }

      // Decode raw content (preserves ANSI escape sequences for styling)
      const lineText = this.textDecoder.decode(line.content);

      // Position at row (1-indexed), clear line, then write content
      // This ensures each line is written at its correct row position
      this.terminal.write(positionAndClearLine(i + 1) + lineText);
    }

    console.log(`[StateApplicator] Applied complete state (sequence ${state.sequence}, ${state.lines.length} lines)`);

    // Log compression statistics if available
    if (state.compression) {
      const ratio = state.compression.compressionRatio;
      const algorithm = state.compression.algorithm;
      console.log(
        `[StateApplicator] Applied state with ${algorithm} compression ` +
        `(ratio: ${(ratio * 100).toFixed(1)}%, ` +
        `${state.compression.uncompressedSize} → ${state.compression.compressedSize} bytes)`
      );
    }
  }

  /**
   * Apply dimension changes (terminal resize).
   * Ensures terminal matches the state dimensions before applying content.
   */
  private applyDimensionChange(dimensions: TerminalDimensions): void {
    const targetRows = Number(dimensions.rows);
    const targetCols = Number(dimensions.cols);
    const currentRows = this.terminal.rows;
    const currentCols = this.terminal.cols;

    if (currentRows !== targetRows || currentCols !== targetCols) {
      console.log(
        `[StateApplicator] Resizing terminal: ${currentCols}x${currentRows} → ${targetCols}x${targetRows}`
      );

      this.terminal.resize(targetCols, targetRows);

      // Verify resize succeeded (browser might constrain it)
      const actualRows = this.terminal.rows;
      const actualCols = this.terminal.cols;

      if (actualRows !== targetRows || actualCols !== targetCols) {
        console.error(
          `[StateApplicator] Terminal resize partially failed! ` +
          `Requested: ${targetCols}x${targetRows}, ` +
          `Actual: ${actualCols}x${actualRows}. ` +
          `Browser constraints may limit terminal size.`
        );
        // Continue anyway - content will be clipped but won't crash
      }
    }
  }

  /**
   * Apply cursor position update.
   * Sets cursor position and visibility based on state.
   * CRITICAL: Position cursor BEFORE showing it to prevent flicker at wrong position.
   */
  private applyCursorPosition(cursor: CursorPosition): void {
    const row = Number(cursor.row);
    const col = Number(cursor.col);

    // Validate cursor position (prevent out-of-bounds)
    const maxRow = this.terminal.rows - 1;
    const maxCol = this.terminal.cols - 1;

    if (row < 0 || row > maxRow || col < 0 || col > maxCol) {
      console.warn(
        `[StateApplicator] Cursor position out of bounds: (${col},${row}) ` +
        `in ${this.terminal.cols}x${this.terminal.rows} terminal. Clamping to bounds.`
      );
    }

    const clampedRow = Math.max(0, Math.min(row, maxRow));
    const clampedCol = Math.max(0, Math.min(col, maxCol));

    // Move cursor to position (1-indexed for ANSI escape codes)
    // Note: ANSI \x1b[row;colH is viewport-relative (relative to top of visible screen)
    // This works correctly as long as viewport is at bottom (no scroll offset)
    this.terminal.write(moveCursorTo(clampedRow + 1, clampedCol + 1));

    // Handle cursor visibility AFTER positioning to prevent flicker
    // Cursor was hidden in applyCompleteState(), now restore correct visibility
    if (cursor.visible) {
      this.terminal.write(CURSOR_SHOW);
    }
    // If cursor should be hidden, it's already hidden from applyCompleteState()
  }

  /**
   * Get current sequence number for synchronization tracking.
   */
  getCurrentSequence(): bigint {
    return this.currentSequence;
  }

  /**
   * Reset sequence tracking (for connection recovery).
   * Call this when reconnecting or recovering from errors.
   */
  resetSequence(): void {
    this.currentSequence = BigInt(0);
    this.lastAppliedState = null;
    console.log('[StateApplicator] Sequence tracking reset to 0');
  }

  /**
   * Get statistics about the last applied state for monitoring.
   */
  getLastStateInfo(): { sequence: bigint; lines: number; compression?: string } | null {
    if (!this.lastAppliedState) {
      return null;
    }

    return {
      sequence: this.lastAppliedState.sequence,
      lines: this.lastAppliedState.lines.length,
      compression: this.lastAppliedState.compression?.algorithm
    };
  }

  /**
   * Check if a sequence number has already been applied.
   * Useful for duplicate detection and debugging.
   */
  hasAppliedSequence(sequence: bigint): boolean {
    return sequence <= this.currentSequence;
  }

  /**
   * Check if the StateApplicator is currently applying a state.
   * Used to prevent scroll event handlers from triggering during programmatic clears.
   */
  getIsApplyingState(): boolean {
    return this.isApplyingState;
  }
}