/**
 * TerminalStreamManager - Write buffering, watermark flow control, chunked writes,
 * redraw throttling, and escape sequence safety for xterm.js.
 *
 * This is a pure imperative class (no React dependency). It encapsulates the
 * terminal write pipeline that was previously scattered across TerminalOutput.tsx.
 *
 * Lifecycle: Create an instance in a ref, call methods from effects, call cleanup() on unmount.
 *
 * Follows the same pattern as StateApplicator, EscapeSequenceParser, and EchoOverlay.
 */

import { EscapeSequenceParser } from './EscapeSequenceParser';

/** Minimal terminal interface (subset of xterm.js Terminal) */
export interface ITerminal {
  rows: number;
  cols: number;
  write(data: string | Uint8Array, callback?: () => void): void;
  clear(): void;
  refresh(start: number, end: number): void;
  scrollToBottom(): void;
  buffer: {
    active: { cursorY: number; viewportY: number; length: number };
    normal: { length: number };
  };
}

/** Callback for sending flow control signals upstream */
export type SendFlowControlFn = (paused: boolean, watermark?: number) => void;

// ---- Constants ----
const HIGH_WATERMARK = 100000; // 100KB - pause when buffer exceeds this
const LOW_WATERMARK = 10000;   // 10KB - resume when buffer drops below this
const CHUNK_SIZE = 16384;      // 16KB chunks
const CHUNK_DELAY_MS = 0;      // Yield to event loop between chunks

/**
 * RedrawThrottler - Coalesces rapid full-screen redraws to max 10 FPS.
 *
 * Claude performs complete screen redraws at 12-25 FPS, causing visible flicker.
 * This throttler holds rapid redraws and flushes the latest one at a capped rate.
 */
class RedrawThrottler {
  private pendingRedraw: string | null = null;
  private throttleTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly throttleMs = 100; // 10 FPS max
  private onFlush: (data: string) => void;

  constructor(onFlush: (data: string) => void) {
    this.onFlush = onFlush;
  }

  process(chunk: string): string | null {
    // Detect full redraw pattern (cursor up at start)
    const isFullRedraw = /^\x1b\[\d+A/.test(chunk);

    if (!isFullRedraw) {
      this.flushPending();
      return chunk;
    }

    // This is a full redraw - throttle it
    this.pendingRedraw = chunk;

    if (!this.throttleTimer) {
      this.throttleTimer = setTimeout(() => {
        this.flushPending();
      }, this.throttleMs);
    }

    return null; // Don't output yet
  }

  private flushPending() {
    if (this.pendingRedraw) {
      this.onFlush(this.pendingRedraw);
      this.pendingRedraw = null;
    }
    if (this.throttleTimer) {
      clearTimeout(this.throttleTimer);
      this.throttleTimer = null;
    }
  }

  cleanup() {
    this.flushPending();
  }
}

export class TerminalStreamManager {
  private terminal: ITerminal;
  private sendFlowControl: SendFlowControlFn;

  // Write batching
  private writeBuffer: string = "";
  private writeScheduled: boolean = false;

  // Pending write operations tracking
  private pendingWrites: number = 0;
  private totalBytesWritten: number = 0;
  private totalBytesCompleted: number = 0;

  // Watermark flow control
  private watermark: number = 0;
  private isPaused: boolean = false;

  // Chunked write queue
  private writeQueue: Array<{ data: string; resolve: () => void }> = [];
  private isProcessingQueue: boolean = false;

  // Escape sequence parser
  private escapeParser: EscapeSequenceParser = new EscapeSequenceParser();

  // Redraw throttler
  private redrawThrottler: RedrawThrottler;

  // Debug instrumentation
  private debugMonitorInstalled: boolean = false;
  private originalWrite: ((data: string | Uint8Array, callback?: () => void) => void) | null = null;
  private originalRefresh: ((start: number, end: number) => void) | null = null;
  private lastWriteTime: number = 0;
  private writeCount: number = 0;

  // Write-lock: prevents live output from being written while initial/paged history is loading.
  // Live writes received during the lock are queued and flushed once the history write completes.
  private isWritingInitialContent: boolean = false;
  private pendingLiveWrites: string[] = [];

  // SerializeAddon for prependScrollbackBatch (injected externally after addon is loaded)
  private serializeAddon?: { serialize(): string };

  // First output tracking
  private firstOutputReceived: boolean = false;
  private onFirstOutput: (() => void) | null = null;

  constructor(terminal: ITerminal, sendFlowControl: SendFlowControlFn) {
    this.terminal = terminal;
    this.sendFlowControl = sendFlowControl;

    this.redrawThrottler = new RedrawThrottler((data) => {
      const safeOutput = this.escapeParser.processChunk(data);
      this.handleProcessedOutput(safeOutput);
    });
  }

  /** Set a callback invoked once on first output received. */
  setOnFirstOutput(cb: () => void): void {
    this.onFirstOutput = cb;
  }

  /** Inject a SerializeAddon for prependScrollbackBatch (serialize-clear-rewrite pattern). */
  setSerializeAddon(addon: { serialize(): string }): void {
    this.serializeAddon = addon;
  }

  /** Update the sendFlowControl callback (e.g., after reconnection). */
  updateSendFlowControl(fn: SendFlowControlFn): void {
    this.sendFlowControl = fn;
  }

  /** Install debug instrumentation (write/refresh monkey-patching). */
  installDebugMonitor(): void {
    if (this.debugMonitorInstalled) return;

    this.originalWrite = this.terminal.write.bind(this.terminal);
    this.originalRefresh = this.terminal.refresh.bind(this.terminal);

    const self = this;

    // Wrap write to track output timing
    this.terminal.write = function(data: string | Uint8Array, callback?: () => void) {
      const now = performance.now();
      const timeSinceLastWrite = now - self.lastWriteTime;
      self.lastWriteTime = now;
      self.writeCount++;

      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[XtermWrite]', {
          writeCount: self.writeCount,
          dataLength: typeof data === 'string' ? data.length : data.byteLength,
          timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
          cursorY: self.terminal.buffer.active.cursorY,
          timestamp: new Date().toISOString()
        });
      }

      return self.originalWrite!(data, callback);
    };

    // Wrap refresh to log all calls (only in debug mode)
    this.terminal.refresh = function(start: number, end: number) {
      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        const stackTrace = new Error().stack;
        const caller = stackTrace?.split('\n')[2]?.trim() || 'unknown';
        const timeSinceLastWrite = performance.now() - self.lastWriteTime;

        console.log('[XtermRefresh] Refresh called', {
          start,
          end,
          rows: self.terminal.rows,
          timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
          recentWrites: self.writeCount,
          caller: caller.replace(/^at /, ''),
          possibleRaceCondition: timeSinceLastWrite < 50,
          timestamp: new Date().toISOString()
        });

        self.writeCount = 0;
      }

      return self.originalRefresh!(start, end);
    };

    this.debugMonitorInstalled = true;
    if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
      console.log('[TerminalStreamManager] Refresh and write monitoring installed');
    }
  }

  /**
   * Write output to the terminal with throttling, escape sequence safety, and flow control.
   * This is the primary entry point for streaming output.
   */
  write(output: string): void {
    // Write-lock: if writeInitialContent or prependScrollbackBatch is in progress,
    // queue live output to avoid interleaving history with live data (Pitfall #1 and #8).
    if (this.isWritingInitialContent) {
      this.pendingLiveWrites.push(output);
      return;
    }

    // Track first output
    if (!this.firstOutputReceived) {
      this.firstOutputReceived = true;
      this.onFirstOutput?.();
    }

    // Throttle rapid full-screen redraws to prevent flickering
    const result = this.redrawThrottler.process(output);
    if (result) {
      const safeOutput = this.escapeParser.processChunk(result);
      this.handleProcessedOutput(safeOutput);
    }
  }

  /**
   * Write initial content (e.g., from currentPaneResponse).
   * Clears terminal, enqueues content via chunked write, scrolls to bottom.
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */
  async writeInitialContent(content: string): Promise<void> {
    this.isWritingInitialContent = true;
    try {
      this.terminal.clear();
      await this.enqueueWrite(content);
      this.terminal.scrollToBottom();

      // Delayed scrolls in case content is still rendering.
      // Large scrollback can take >100ms to fully render on slower machines.
      setTimeout(() => this.terminal.scrollToBottom(), 10);
      setTimeout(() => this.terminal.scrollToBottom(), 100);
      setTimeout(() => this.terminal.scrollToBottom(), 500);
    } finally {
      this.isWritingInitialContent = false;
      // Flush any live writes that arrived during history loading (in order)
      const pending = this.pendingLiveWrites.splice(0);
      for (const chunk of pending) {
        this.write(chunk);
      }
    }
  }

  /**
   * Prepend historical scrollback above the current terminal content.
   * Uses the serialize-clear-rewrite pattern (ADR-010):
   *   1. Serialize current buffer (if SerializeAddon available)
   *   2. Clear terminal
   *   3. Write older history
   *   4. Write saved current content
   *   5. Scroll to bottom
   *
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */
  async prependScrollbackBatch(content: string): Promise<void> {
    this.isWritingInitialContent = true;
    try {
      if (!this.serializeAddon) {
        // Degraded mode: SerializeAddon not available — cannot serialize current buffer before
        // clearing, so we skip the clear and simply append the history content. The history
        // will appear after the current content (wrong order) but current content is preserved.
        // This is preferable to clearing and losing the existing terminal output (Bug 6 fix).
        console.warn('[TerminalStreamManager] SerializeAddon not available — appending history without clear (degraded mode)');
        await this.enqueueWrite(content);
        return;
      }

      // Serialize current buffer state before clearing
      const serialized = this.serializeAddon.serialize();

      this.terminal.clear();
      await this.enqueueWrite(content);
      if (serialized) {
        await this.enqueueWrite(serialized);
      }
      this.terminal.scrollToBottom();
    } finally {
      this.isWritingInitialContent = false;
      // Flush any live writes that arrived during history loading (in order)
      const pending = this.pendingLiveWrites.splice(0);
      for (const chunk of pending) {
        this.write(chunk);
      }
    }
  }

  /**
   * Handle processed output by routing through the appropriate write path
   * (raw direct/chunked or state batching).
   */
  private handleProcessedOutput(safeOutput: string): void {
    if (safeOutput.length === 0) return;

    // Detect terminal mode transitions that may need a refresh
    const needsRefresh =
      safeOutput.includes('\x1b[?1049l') ||
      safeOutput.includes('\x1b[?47l') ||
      safeOutput.includes('\x1b[?2026l') ||
      safeOutput.includes('\x1b[?25h');

    if (needsRefresh) {
      requestAnimationFrame(() => {
        this.terminal.refresh(0, this.terminal.rows - 1);
        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log('[TerminalStreamManager] Forced refresh after mode transition', {
            alternateScreenExit: safeOutput.includes('\x1b[?1049l') || safeOutput.includes('\x1b[?47l'),
            syncUpdateEnd: safeOutput.includes('\x1b[?2026l'),
            cursorRestore: safeOutput.includes('\x1b[?25h'),
          });
        }
      });
    }

    // For small writes, write directly for lowest latency with flow control tracking
    if (safeOutput.length <= CHUNK_SIZE) {
      this.writeDirectWithFlowControl(safeOutput);
    } else {
      // Large write - use chunked queue to prevent UI freezing
      this.enqueueWrite(safeOutput);
    }
  }

  /** Direct write with watermark tracking (fast path for small data). */
  private writeDirectWithFlowControl(data: string): void {
    this.pendingWrites++;
    this.totalBytesWritten += data.length;
    this.watermark += data.length;

    // Check if we should pause upstream
    if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
      this.isPaused = true;
      console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${this.watermark} bytes)`);
      this.sendFlowControl(true, this.watermark);
    }

    this.terminal.write(data, () => {
      this.pendingWrites = Math.max(0, this.pendingWrites - 1);
      this.totalBytesCompleted += data.length;
      this.watermark = Math.max(0, this.watermark - data.length);

      if (this.watermark < LOW_WATERMARK && this.isPaused) {
        this.isPaused = false;
        console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${this.watermark} bytes)`);
        this.sendFlowControl(false, this.watermark);
      }

      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[FlowControl] Write completed', {
          bytes: data.length,
          watermark: this.watermark,
          paused: this.isPaused,
          pending: this.pendingWrites,
          totalWritten: this.totalBytesWritten,
          totalCompleted: this.totalBytesCompleted,
          backlog: this.totalBytesWritten - this.totalBytesCompleted,
        });
      }
    });
  }

  /** Enqueue data for chunked writing (large data path). */
  enqueueWrite(data: string): Promise<void> {
    return new Promise((resolve) => {
      this.writeQueue.push({ data, resolve });
      this.processWriteQueue();
    });
  }

  /** Process the write queue sequentially with chunking. */
  private async processWriteQueue(): Promise<void> {
    if (this.isProcessingQueue || this.writeQueue.length === 0) {
      return;
    }

    this.isProcessingQueue = true;

    while (this.writeQueue.length > 0) {
      const item = this.writeQueue[0];
      const data = item.data;

      if (data.length <= CHUNK_SIZE) {
        // Small write - direct with flow control
        await new Promise<void>((resolve) => {
          this.watermark += data.length;

          if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
            this.isPaused = true;
            console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${this.watermark} bytes)`);
            this.sendFlowControl(true, this.watermark);
          }

          this.terminal.write(data, () => {
            this.watermark = Math.max(0, this.watermark - data.length);

            if (this.watermark < LOW_WATERMARK && this.isPaused) {
              this.isPaused = false;
              console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${this.watermark} bytes)`);
              this.sendFlowControl(false, this.watermark);
            }
            resolve();
          });
        });
      } else {
        // Large write - chunk it with yields to the UI
        const totalChunks = Math.ceil(data.length / CHUNK_SIZE);

        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log(`[FlowControl] Chunking large write: ${data.length} bytes into ${totalChunks} chunks`);
        }

        for (let i = 0; i < data.length; i += CHUNK_SIZE) {
          const chunk = data.slice(i, Math.min(i + CHUNK_SIZE, data.length));
          const chunkIndex = Math.floor(i / CHUNK_SIZE) + 1;

          await new Promise<void>((resolve) => {
            this.watermark += chunk.length;

            if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
              this.isPaused = true;
              console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED during chunk ${chunkIndex}/${totalChunks} - Pausing stream (watermark: ${this.watermark} bytes)`);
              this.sendFlowControl(true, this.watermark);
            }

            this.terminal.write(chunk, () => {
              this.watermark = Math.max(0, this.watermark - chunk.length);

              if (this.watermark < LOW_WATERMARK && this.isPaused) {
                this.isPaused = false;
                console.log(`[FlowControl] LOW WATERMARK REACHED after chunk ${chunkIndex}/${totalChunks} - Resuming stream (watermark: ${this.watermark} bytes)`);
                this.sendFlowControl(false, this.watermark);
              }
              resolve();
            });
          });

          // Yield to UI between chunks
          if (i + CHUNK_SIZE < data.length) {
            await new Promise<void>((resolve) => {
              if (CHUNK_DELAY_MS > 0) {
                setTimeout(resolve, CHUNK_DELAY_MS);
              } else {
                requestAnimationFrame(() => resolve());
              }
            });
          }
        }

        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log(`[FlowControl] Completed chunked write: ${data.length} bytes`);
        }
      }

      this.writeQueue.shift();
      item.resolve();
    }

    this.isProcessingQueue = false;
  }

  /** Flush pending write buffer (for state/hybrid mode batching via RAF). */
  flushWriteBuffer(): void {
    if (this.writeBuffer) {
      const dataToWrite = this.writeBuffer;
      const byteLength = dataToWrite.length;

      this.writeBuffer = "";

      if (byteLength > CHUNK_SIZE) {
        this.enqueueWrite(dataToWrite);
        this.writeScheduled = false;
        return;
      }

      this.writeDirectWithFlowControl(dataToWrite);
    }
    this.writeScheduled = false;
  }

  /**
   * Write output in state/hybrid mode (batches with RAF).
   * Used when streaming mode is "state" or "hybrid".
   */
  writeStateBatched(safeOutput: string): void {
    if (safeOutput.length === 0) return;
    this.writeBuffer += safeOutput;

    if (!this.writeScheduled) {
      this.writeScheduled = true;
      requestAnimationFrame(() => this.flushWriteBuffer());
    }
  }

  /** Clean up all pending state on unmount. */
  cleanup(): void {
    // Flush write buffer
    if (this.writeBuffer) {
      // Best-effort direct write
      try {
        this.terminal.write(this.writeBuffer);
      } catch {
        // Terminal may already be disposed
      }
      this.writeBuffer = "";
    }

    // Reset escape sequence parser
    this.escapeParser.reset();

    // Cleanup redraw throttler
    this.redrawThrottler.cleanup();

    // Restore original terminal methods if debug monitoring was installed
    if (this.debugMonitorInstalled && this.originalWrite && this.originalRefresh) {
      this.terminal.write = this.originalWrite;
      this.terminal.refresh = this.originalRefresh;
      this.debugMonitorInstalled = false;
    }

    // Null out refs to terminal internals to prevent GC retention (Pitfall #7):
    // when the terminal is disposed while the manager is still alive, these refs
    // hold closed terminal objects preventing garbage collection.
    this.originalWrite = null;
    this.originalRefresh = null;

    // Clear any pending live writes
    this.pendingLiveWrites.length = 0;
  }

  // ---- Test/debug accessors ----

  /** Get current watermark level (for testing). */
  getWatermark(): number {
    return this.watermark;
  }

  /** Check if flow control is paused (for testing). */
  getIsPaused(): boolean {
    return this.isPaused;
  }
}

// Export constants for testing
export { HIGH_WATERMARK, LOW_WATERMARK, CHUNK_SIZE };
