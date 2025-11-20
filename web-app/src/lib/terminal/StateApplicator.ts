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

  // Incremental diff optimization: Track rendered lines to avoid full clear
  private previousLines: Map<number, string> = new Map();
  private previousDimensions: { cols: number; rows: number } | null = null;

  // RAF batching: Group rapid updates into single frame (60fps target)
  private pendingState: TerminalState | null = null;
  private rafId: number | null = null;
  private lastFrameTime: number = 0;

  constructor(terminal: Terminal) {
    this.terminal = terminal;
  }

  /**
   * Apply a complete terminal state to the terminal.
   * MOSH-inspired approach: Idempotent, out-of-order tolerant, self-healing.
   * RAF batching: Groups rapid updates into single frames for 60fps target.
   *
   * @param state - The complete terminal state to apply
   * @returns true if queued/applied successfully, false if should be ignored
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

    console.log(`[StateApplicator] Queuing state sequence ${stateSequence} for RAF batch (current: ${this.currentSequence})`);

    // RAF batching: Buffer state and apply on next animation frame
    // This groups rapid updates (like Claude's fast status bar pulses) into single renders
    this.pendingState = state;

    if (this.rafId === null) {
      // Schedule RAF callback if not already scheduled
      this.rafId = requestAnimationFrame(this.applyPendingState.bind(this));
    }

    return true;
  }

  /**
   * Apply pending state on animation frame (RAF callback).
   * This is called once per browser frame (~16.67ms at 60fps).
   */
  private applyPendingState(timestamp: number): void {
    this.rafId = null;

    if (!this.pendingState) {
      return; // No pending state
    }

    const state = this.pendingState;
    this.pendingState = null;

    // Calculate frame timing for monitoring
    const frameDelta = timestamp - this.lastFrameTime;
    this.lastFrameTime = timestamp;

    console.log(
      `[StateApplicator] Applying batched state sequence ${state.sequence} ` +
      `(frame delta: ${frameDelta.toFixed(2)}ms)`
    );

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

      // Apply incremental state update (only changed lines)
      this.applyIncrementalState(state);

      // Update sequence tracking
      this.currentSequence = state.sequence;
      this.lastAppliedState = state;

      console.log(
        `[StateApplicator] Successfully applied state sequence ${state.sequence} ` +
        `(${lineCount} lines, cursor=(${state.cursor?.col},${state.cursor?.row}))`
      );
    } finally {
      // Always clear the flag, even if an error occurred
      this.isApplyingState = false;
    }
  }

  /**
   * Apply incremental terminal state update (only changed lines).
   * This is the key optimization that eliminates flickering:
   * - Compares new state with previous rendered state
   * - Only updates lines that have changed
   * - No full terminal clear (prevents blank screen flicker)
   * - Perfect for rapid animations like Claude's status bar pulses
   */
  private applyIncrementalState(state: TerminalState): void {
    const currentDims = { cols: this.terminal.cols, rows: this.terminal.rows };

    // Check if dimensions changed - if so, need full redraw
    const dimensionsChanged =
      !this.previousDimensions ||
      this.previousDimensions.cols !== currentDims.cols ||
      this.previousDimensions.rows !== currentDims.rows;

    if (dimensionsChanged) {
      console.log(
        `[StateApplicator] Terminal dimensions changed, performing full redraw ` +
        `(${this.previousDimensions?.cols}x${this.previousDimensions?.rows} → ${currentDims.cols}x${currentDims.rows})`
      );

      // Dimension change requires full clear and redraw
      this.terminal.clear();
      this.previousLines.clear();
      this.previousDimensions = currentDims;
    }

    // Hide cursor during update to prevent cursor jump flicker
    this.terminal.write(CURSOR_HIDE);

    let changedLines = 0;
    let unchangedLines = 0;

    // Update only changed lines
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
      const previousLine = this.previousLines.get(i);

      // Only update if line changed
      if (lineText !== previousLine) {
        // Position at row (1-indexed), clear line, then write content
        this.terminal.write(positionAndClearLine(i + 1) + lineText);
        this.previousLines.set(i, lineText);
        changedLines++;
      } else {
        unchangedLines++;
      }
    }

    // Clear lines that no longer exist (terminal shrunk or content removed)
    const previousLineCount = this.previousLines.size;
    if (state.lines.length < previousLineCount) {
      for (let i = state.lines.length; i < previousLineCount; i++) {
        this.terminal.write(positionAndClearLine(i + 1));
        this.previousLines.delete(i);
        changedLines++;
      }
    }

    console.log(
      `[StateApplicator] Applied incremental state (sequence ${state.sequence}, ` +
      `${changedLines} changed, ${unchangedLines} unchanged, ${state.lines.length} total lines)`
    );

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
   * Apply complete terminal state (MOSH-style complete screen buffer).
   * Legacy method - kept for compatibility but now uses incremental approach.
   * @deprecated Use applyIncrementalState instead
   */
  private applyCompleteState(state: TerminalState): void {
    // Redirect to incremental implementation
    this.applyIncrementalState(state);
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
   * Clears all caches and cancels pending RAF callbacks.
   */
  resetSequence(): void {
    this.currentSequence = BigInt(0);
    this.lastAppliedState = null;

    // Clear incremental diff caches
    this.previousLines.clear();
    this.previousDimensions = null;

    // Cancel pending RAF and clear pending state
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    this.pendingState = null;
    this.lastFrameTime = 0;

    console.log('[StateApplicator] Sequence tracking and caches reset to initial state');
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