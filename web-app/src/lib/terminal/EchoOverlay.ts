/**
 * EchoOverlay provides Mosh-style predictive echo for terminal input.
 *
 * When users type, the characters appear immediately (dimmed) before server confirmation.
 * This creates a low-latency feel even on high-latency connections.
 *
 * How it works:
 * 1. User types a character
 * 2. EchoOverlay shows the character immediately at cursor position (dimmed)
 * 3. InputWithEcho message is sent to server with echo_num
 * 4. Server sends EchoAck when input is processed (or after 50ms timeout)
 * 5. EchoOverlay clears predictions up to the acknowledged echo_num
 *
 * References:
 * - Mosh predictive echo: https://mosh.org/#techinfo
 * - Echo timeout: 50ms (Mosh default)
 */

import type { Terminal } from '@xterm/xterm';

/** Represents a single pending echo prediction */
interface PendingEcho {
  echoNum: bigint;
  text: string;
  cursorRow: number;
  cursorCol: number;
  timestamp: number;
  rendered: boolean;
}

/** EchoOverlay options */
export interface EchoOverlayOptions {
  /** Timeout in milliseconds before clearing unconfirmed echoes (default: 50ms) */
  echoTimeoutMs?: number;
  /** ANSI style for predicted characters (default: dim) */
  predictionStyle?: string;
  /** Enable debug logging */
  debug?: boolean;
}

const DEFAULT_OPTIONS: Required<EchoOverlayOptions> = {
  echoTimeoutMs: 50,
  predictionStyle: '\x1b[2m', // Dim text
  debug: false,
};

/**
 * Manages predictive echo overlay for terminal input.
 * Shows typed characters immediately before server confirmation.
 */
export class EchoOverlay {
  private terminal: Terminal | null = null;
  private pendingEchos: Map<bigint, PendingEcho> = new Map();
  private echoCounter: bigint = BigInt(0);
  private options: Required<EchoOverlayOptions>;
  private timeoutCheckInterval: ReturnType<typeof setInterval> | null = null;
  private enabled: boolean = true;

  // Cursor tracking (synced with server state)
  private cursorRow: number = 0;
  private cursorCol: number = 0;

  constructor(options: EchoOverlayOptions = {}) {
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  /**
   * Attach the overlay to a terminal instance.
   */
  attach(terminal: Terminal): void {
    this.terminal = terminal;
    this.startTimeoutCheck();
  }

  /**
   * Detach from the terminal and cleanup.
   */
  detach(): void {
    this.stopTimeoutCheck();
    this.clearAllPredictions();
    this.terminal = null;
  }

  /**
   * Enable or disable predictive echo.
   */
  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    if (!enabled) {
      this.clearAllPredictions();
    }
  }

  /**
   * Check if predictive echo is enabled.
   */
  isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * Show predictive echo for typed input.
   * Returns the echo number for tracking.
   */
  showPredictiveEcho(input: string): bigint {
    if (!this.enabled || !this.terminal) {
      return BigInt(0);
    }

    const echoNum = ++this.echoCounter;

    // Record the pending echo
    const pendingEcho: PendingEcho = {
      echoNum,
      text: input,
      cursorRow: this.cursorRow,
      cursorCol: this.cursorCol,
      timestamp: Date.now(),
      rendered: false,
    };

    this.pendingEchos.set(echoNum, pendingEcho);

    // Render the prediction immediately
    this.renderPrediction(pendingEcho);

    if (this.options.debug) {
      console.log('[EchoOverlay] Showing prediction:', {
        echoNum: echoNum.toString(),
        text: input,
        cursor: { row: this.cursorRow, col: this.cursorCol },
      });
    }

    return echoNum;
  }

  /**
   * Clear echoes up to the acknowledged echo number.
   * Called when server confirms input processing.
   */
  clearAcked(ackedEchoNum: bigint): void {
    let clearedCount = 0;

    for (const [echoNum, echo] of this.pendingEchos) {
      if (echoNum <= ackedEchoNum) {
        // Clear the rendered prediction if it was shown
        if (echo.rendered) {
          this.clearPrediction(echo);
        }
        this.pendingEchos.delete(echoNum);
        clearedCount++;
      }
    }

    if (this.options.debug && clearedCount > 0) {
      console.log('[EchoOverlay] Cleared acked predictions:', {
        ackedUpTo: ackedEchoNum.toString(),
        cleared: clearedCount,
        remaining: this.pendingEchos.size,
      });
    }
  }

  /**
   * Update cursor position (called when receiving server state).
   */
  updateCursor(row: number, col: number): void {
    this.cursorRow = row;
    this.cursorCol = col;
  }

  /**
   * Get the current echo counter value.
   */
  getEchoCounter(): bigint {
    return this.echoCounter;
  }

  /**
   * Get the number of pending predictions.
   */
  getPendingCount(): number {
    return this.pendingEchos.size;
  }

  /**
   * Clear all pending predictions (e.g., on disconnect or error).
   */
  clearAllPredictions(): void {
    for (const echo of this.pendingEchos.values()) {
      if (echo.rendered) {
        this.clearPrediction(echo);
      }
    }
    this.pendingEchos.clear();
  }

  // ============================================================================
  // Private Methods
  // ============================================================================

  private renderPrediction(echo: PendingEcho): void {
    if (!this.terminal) return;

    // For simplicity, we write the prediction directly to the terminal
    // with dim styling. The server update will overwrite this.
    // Note: This is a simplified approach. A more sophisticated implementation
    // would use xterm.js decoration API or overlay canvas.

    const { predictionStyle } = this.options;
    const resetStyle = '\x1b[0m';

    // Write predicted text with dim styling
    // The text will be overwritten when server sends the actual output
    this.terminal.write(`${predictionStyle}${echo.text}${resetStyle}`);

    echo.rendered = true;

    // Update local cursor position estimate
    this.cursorCol += echo.text.length;
  }

  private clearPrediction(echo: PendingEcho): void {
    // In xterm.js, predictions are overwritten by server output
    // No explicit clear needed for the simplified approach
    // A more sophisticated implementation would track and remove decorations
    echo.rendered = false;
  }

  private startTimeoutCheck(): void {
    if (this.timeoutCheckInterval) return;

    // Check for timed-out predictions every 10ms
    this.timeoutCheckInterval = setInterval(() => {
      this.checkTimeouts();
    }, 10);
  }

  private stopTimeoutCheck(): void {
    if (this.timeoutCheckInterval) {
      clearInterval(this.timeoutCheckInterval);
      this.timeoutCheckInterval = null;
    }
  }

  private checkTimeouts(): void {
    const now = Date.now();
    const timeout = this.options.echoTimeoutMs;

    for (const [echoNum, echo] of this.pendingEchos) {
      const age = now - echo.timestamp;

      // If prediction is older than timeout and not yet cleared,
      // we assume the server has processed it (even without explicit ack)
      if (age > timeout * 2) { // Use 2x timeout as safety margin
        if (this.options.debug) {
          console.log('[EchoOverlay] Timeout clearing prediction:', {
            echoNum: echoNum.toString(),
            age,
          });
        }

        if (echo.rendered) {
          this.clearPrediction(echo);
        }
        this.pendingEchos.delete(echoNum);
      }
    }
  }
}

export default EchoOverlay;
