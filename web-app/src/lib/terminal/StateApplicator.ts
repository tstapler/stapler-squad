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
import { TerminalState, TerminalLine, CursorPosition, TerminalDimensions, TerminalDiff, EchoAck } from '@/gen/session/v1/events_pb';
import { positionAndClearLine, moveCursorTo, CURSOR_SHOW, CURSOR_HIDE, CLEAR_SCREEN, CLEAR_SCREEN_AND_SCROLLBACK, CURSOR_HOME } from './AnsiCodes';
import type { EchoOverlay } from './EchoOverlay';

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
  private pendingDiff: TerminalDiff | null = null;
  private rafId: number | null = null;
  private lastFrameTime: number = 0;

  // Status line buffering for Claude's rapid updates
  private statusLineBuffer: Map<number, { content: string; timestamp: number }> = new Map();
  private statusLineDebounceTimer: number | null = null;

  // SSP: Optional echo overlay for predictive echo acknowledgment
  private echoOverlay: EchoOverlay | null = null;

  // SSP: Callback for echo acknowledgment (allows external handling)
  private onEchoAck: ((ack: EchoAck) => void) | null = null;

  // PHASE 1: Callback for dimension mismatch detection
  private onDimensionMismatch: ((expectedCols: number, expectedRows: number, actualCols: number, actualRows: number) => void) | null = null;

  constructor(terminal: Terminal) {
    this.terminal = terminal;
  }

  /**
   * PHASE 1: Set a callback for dimension mismatch detection.
   * Called when received state dimensions don't match terminal dimensions.
   */
  setOnDimensionMismatch(callback: ((expectedCols: number, expectedRows: number, actualCols: number, actualRows: number) => void) | null): void {
    this.onDimensionMismatch = callback;
  }

  /**
   * Set the echo overlay for predictive echo acknowledgment.
   * The EchoOverlay will receive clearAcked() calls when echo acks arrive.
   */
  setEchoOverlay(overlay: EchoOverlay | null): void {
    this.echoOverlay = overlay;
  }

  /**
   * Set a callback for echo acknowledgment events.
   * Useful for external RTT tracking or statistics.
   */
  setOnEchoAck(callback: ((ack: EchoAck) => void) | null): void {
    this.onEchoAck = callback;
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
   * Apply a terminal diff (SSP mode - Mosh-style minimal ANSI sequences).
   * This is the core of the State Synchronization Protocol optimization:
   * - Server generates minimal ANSI escape sequences to transform old→new state
   * - Client writes these directly to terminal (bypassing line-by-line updates)
   * - Dramatically reduces bandwidth vs full state updates
   *
   * @param diff - The terminal diff containing ANSI escape sequences
   * @returns true if applied successfully, false if sequence mismatch (needs resync)
   */
  applyDiff(diff: TerminalDiff): boolean {
    const fromSeq = diff.fromSequence;
    const toSeq = diff.toSequence;

    // Sequence validation (unless it's a full redraw)
    if (!diff.fullRedraw) {
      // For incremental diffs, verify sequence continuity
      if (fromSeq !== this.currentSequence) {
        console.warn(
          `[StateApplicator] Diff sequence mismatch: ` +
          `diff.fromSequence=${fromSeq}, currentSequence=${this.currentSequence}. ` +
          `Requesting resync.`
        );
        return false; // Caller should request full resync
      }
    } else {
      // Full redraw - no sequence requirement, but log if there's a gap
      if (fromSeq !== BigInt(0) && fromSeq !== this.currentSequence) {
        console.log(
          `[StateApplicator] Full redraw with sequence gap: ` +
          `expected=${this.currentSequence}, received from=${fromSeq}. Accepting (full redraw is self-contained).`
        );
      }
    }

    console.log(
      `[StateApplicator] Queuing diff for RAF batch: ` +
      `seq ${fromSeq}→${toSeq}, fullRedraw=${diff.fullRedraw}, ` +
      `${diff.changedCells} cells changed, ${diff.unchangedCells} unchanged`
    );

    // RAF batching: Buffer diff and apply on next animation frame
    this.pendingDiff = diff;
    this.pendingState = null; // Clear any pending state (diff takes priority)

    if (this.rafId === null) {
      this.rafId = requestAnimationFrame(this.applyPendingUpdates.bind(this));
    }

    return true;
  }

  /**
   * Apply pending updates on animation frame (unified RAF callback).
   * Handles both state and diff updates with RAF batching.
   */
  private applyPendingUpdates(timestamp: number): void {
    this.rafId = null;

    // Process diff if pending (takes priority over state)
    if (this.pendingDiff) {
      this.applyDiffImmediate(this.pendingDiff, timestamp);
      this.pendingDiff = null;
      return;
    }

    // Process state if pending
    if (this.pendingState) {
      this.applyPendingState(timestamp);
      return;
    }
  }

  /**
   * Immediately apply a diff (called from RAF callback).
   */
  private applyDiffImmediate(diff: TerminalDiff, timestamp: number): void {
    const frameDelta = timestamp - this.lastFrameTime;
    this.lastFrameTime = timestamp;

    this.isApplyingState = true;

    try {
      // Handle echo acknowledgment if present (piggy-backed on diff)
      if (diff.echoAck) {
        this.processEchoAck(diff.echoAck);
      }

      // For full redraw, clear line cache since we're getting complete new state
      if (diff.fullRedraw) {
        console.log('[StateApplicator] Full redraw - clearing line cache');
        this.previousLines.clear();
      }

      // Write diff bytes directly to terminal
      // These are pre-computed ANSI escape sequences from the server
      if (diff.diffBytes && diff.diffBytes.length > 0) {
        const diffStr = this.textDecoder.decode(diff.diffBytes);
        this.terminal.write(diffStr);

        console.log(
          `[StateApplicator] Applied diff ${diff.fromSequence}→${diff.toSequence} ` +
          `(${diff.diffBytes.length} bytes, ${diff.changedCells} cells, ` +
          `frame delta: ${frameDelta.toFixed(2)}ms)`
        );
      } else {
        console.log(
          `[StateApplicator] Applied empty diff ${diff.fromSequence}→${diff.toSequence} ` +
          `(no changes, frame delta: ${frameDelta.toFixed(2)}ms)`
        );
      }

      // Log compression statistics if available
      if (diff.compression) {
        const ratio = diff.compression.compressionRatio;
        const algorithm = diff.compression.algorithm;
        console.log(
          `[StateApplicator] Diff with ${algorithm} compression ` +
          `(ratio: ${(ratio * 100).toFixed(1)}%, ` +
          `${diff.compression.uncompressedSize} → ${diff.compression.compressedSize} bytes)`
        );
      }

      // Update sequence tracking
      this.currentSequence = diff.toSequence;

    } finally {
      this.isApplyingState = false;
    }
  }

  /**
   * Process an echo acknowledgment.
   * Clears predictive echoes and notifies external handlers.
   */
  private processEchoAck(ack: EchoAck): void {
    const echoNum = ack.echoAckNum;

    console.log(
      `[StateApplicator] Processing echo ack: echoNum=${echoNum}, ` +
      `serverTimestamp=${ack.serverTimestampMs}`
    );

    // Clear acknowledged predictions in the echo overlay
    if (this.echoOverlay) {
      this.echoOverlay.clearAcked(echoNum);
    }

    // Notify external handler (for RTT tracking, statistics, etc.)
    if (this.onEchoAck) {
      this.onEchoAck(ack);
    }
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

      // PHASE 1: Enhanced dimension synchronization validation with callback
      const stateDims = state.dimensions;
      const terminalDims = { cols: this.terminal.cols, rows: this.terminal.rows };
      if (stateDims && (stateDims.cols !== terminalDims.cols || stateDims.rows !== terminalDims.rows)) {
        console.warn(
          `[StateApplicator] DIMENSION MISMATCH DETECTED: ` +
          `state=${stateDims.cols}x${stateDims.rows}, ` +
          `terminal=${terminalDims.cols}x${terminalDims.rows} ` +
          `[This causes incorrect text positioning - checkboxes appear on right side instead of inline]`
        );

        // PHASE 1: Notify external handler (e.g., to request resync with correct dimensions)
        if (this.onDimensionMismatch) {
          console.log(
            `[StateApplicator] Calling dimension mismatch handler to request resync with correct dimensions`
          );
          this.onDimensionMismatch(stateDims.cols, stateDims.rows, terminalDims.cols, terminalDims.rows);
        }
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
   * Check if a line appears to be a Claude status line
   */
  private isClaudeStatusLine(content: string): boolean {
    return content.includes('esc to interrupt') ||
           (content.includes('↑') && content.includes('tokens')) ||
           /·\s+\w+ing/.test(content);
  }

  /**
   * Check if a status line appears to be complete
   */
  private isLineComplete(content: string): boolean {
    if (content.includes('esc to interrupt')) {
      return /tokens\)?\s*$/.test(content);
    }
    return !content.endsWith('...');
  }

  /**
   * Schedule a debounced flush of buffered status lines
   */
  private scheduleStatusLineFlush(): void {
    if (this.statusLineDebounceTimer) {
      clearTimeout(this.statusLineDebounceTimer);
    }

    this.statusLineDebounceTimer = window.setTimeout(() => {
      this.flushStatusLines();
    }, 100);
  }

  /**
   * Flush all buffered status lines to the terminal
   */
  private flushStatusLines(): void {
    if (this.statusLineBuffer.size === 0) return;

    let writeBuffer = '';
    for (const [lineNum, state] of this.statusLineBuffer) {
      writeBuffer += positionAndClearLine(lineNum + 1) + state.content;
      this.previousLines.set(lineNum, state.content);
    }

    if (writeBuffer.length > 0) {
      this.terminal.write(CURSOR_HIDE + writeBuffer + CURSOR_SHOW);
    }

    this.statusLineBuffer.clear();
    this.statusLineDebounceTimer = null;
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
    // Accumulate all line updates into a single write buffer
    let writeBuffer = CURSOR_HIDE;

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
        // Check if this is a Claude status line that might be incomplete
        if (this.isClaudeStatusLine(lineText)) {
          this.statusLineBuffer.set(i, {
            content: lineText,
            timestamp: Date.now()
          });

          // Only update immediately if the line appears complete
          if (!this.isLineComplete(lineText)) {
            this.scheduleStatusLineFlush();
            continue; // Skip immediate update for incomplete status lines
          }
        }

        // Regular line update or complete status line
        writeBuffer += positionAndClearLine(i + 1) + lineText;
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
        writeBuffer += positionAndClearLine(i + 1);
        this.previousLines.delete(i);
        changedLines++;
      }
    }

    // CRITICAL: Write all line updates in a single terminal.write() call
    // This eliminates the cascading visual effect where lines update one-by-one
    if (writeBuffer.length > CURSOR_HIDE.length) {
      this.terminal.write(writeBuffer);
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

    // Cancel pending RAF and clear pending state/diff
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    this.pendingState = null;
    this.pendingDiff = null;
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