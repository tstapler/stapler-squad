"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
// useExternalTerminalStream is deprecated - unified WebSocket streaming now uses useTerminalStream for both session types
// import { useExternalTerminalStream } from "@/lib/hooks/useExternalTerminalStream";
import { XtermTerminal } from "./XtermTerminal";
import { EscapeSequenceParser } from "@/lib/terminal/EscapeSequenceParser";
import { decompressLZMA, isLZMACompressed } from "@/lib/compression/lzma";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
  isExternal?: boolean;
  tmuxSessionName?: string; // For external sessions, the tmux session name
}

export function TerminalOutput({ sessionId, baseUrl, isExternal = false, tmuxSessionName }: TerminalOutputProps) {
  const xtermRef = useRef<any>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const [isWaitingForStableSize, setIsWaitingForStableSize] = useState(true); // Wait for stable size before connecting
  const [isLoadingInitialContent, setIsLoadingInitialContent] = useState(true); // Loading overlay state
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const refreshCountRef = useRef(0); // Track number of forced refreshes
  const isMountedRef = useRef(true); // Track component mount state for async operations
  const sizeStabilityTimeoutRef = useRef<NodeJS.Timeout | null>(null); // For size stability detection
  const hasInitiatedConnectionRef = useRef(false); // Prevent multiple connection attempts
  const hasCachedDimensionsRef = useRef(false); // Track if we loaded cached dimensions

  // Terminal loading metrics - tracks time from component mount to first content display
  // This helps identify performance bottlenecks and regressions
  const metricsRef = useRef<{
    mountTime: number;           // When component mounted
    firstResizeTime: number | null;    // First resize event from xterm
    sizeStableTime: number | null;     // When size became stable
    connectionInitTime: number | null; // When WebSocket connection initiated
    connectedTime: number | null;      // When WebSocket connected
    firstOutputTime: number | null;    // When first output received
    resizeCount: number;         // Number of resize events before stable
  }>({
    mountTime: performance.now(),
    firstResizeTime: null,
    sizeStableTime: null,
    connectionInitTime: null,
    connectedTime: null,
    firstOutputTime: null,
    resizeCount: 0,
  });

  // Log terminal loading metrics (called when first output is received)
  const logTerminalMetrics = useCallback(() => {
    const m = metricsRef.current;
    if (!m.firstOutputTime) return; // Metrics not complete yet

    const metrics = {
      totalLoadTime: m.firstOutputTime - m.mountTime,
      timeToFirstResize: m.firstResizeTime ? m.firstResizeTime - m.mountTime : null,
      timeToSizeStable: m.sizeStableTime ? m.sizeStableTime - m.mountTime : null,
      sizeStabilizationDuration: m.sizeStableTime && m.firstResizeTime ? m.sizeStableTime - m.firstResizeTime : null,
      timeToConnectionInit: m.connectionInitTime ? m.connectionInitTime - m.mountTime : null,
      timeToConnected: m.connectedTime ? m.connectedTime - m.mountTime : null,
      connectionDuration: m.connectedTime && m.connectionInitTime ? m.connectedTime - m.connectionInitTime : null,
      timeToFirstOutput: m.firstOutputTime - m.mountTime,
      resizeEventsBeforeStable: m.resizeCount,
      sessionId: isExternal ? tmuxSessionName : sessionId,
      isExternal,
    };

    // Always log metrics summary
    console.log(`[TerminalMetrics] Terminal loaded in ${metrics.totalLoadTime.toFixed(0)}ms`, {
      breakdown: {
        sizeStabilization: `${metrics.sizeStabilizationDuration?.toFixed(0) || 'N/A'}ms (${metrics.resizeEventsBeforeStable} resizes)`,
        wsConnection: `${metrics.connectionDuration?.toFixed(0) || 'N/A'}ms`,
        totalLoad: `${metrics.totalLoadTime.toFixed(0)}ms`,
      },
      detailed: metrics,
    });

    // Report to performance observer if available (for integration with monitoring)
    if (typeof window !== 'undefined' && typeof window.performance?.mark === 'function') {
      try {
        performance.mark('terminal-loaded');
        performance.measure('terminal-load-time', { start: m.mountTime, end: m.firstOutputTime });
      } catch {
        // Ignore if performance API not fully supported
      }
    }
  }, [sessionId, tmuxSessionName, isExternal]);

  // Dimension persistence helpers - cache terminal size per session for instant reconnection
  const getCachedDimensions = useCallback((): { cols: number; rows: number } | null => {
    if (typeof window === 'undefined') return null;
    try {
      const key = `terminal-dimensions-${sessionId}`;
      const cached = localStorage.getItem(key);
      if (cached) {
        const dims = JSON.parse(cached);
        console.log(`[TerminalOutput] Loaded cached dimensions for ${sessionId}: ${dims.cols}x${dims.rows}`);
        return dims;
      }
    } catch (err) {
      console.warn('[TerminalOutput] Failed to load cached dimensions:', err);
    }
    return null;
  }, [sessionId]);

  const saveDimensions = useCallback((cols: number, rows: number) => {
    if (typeof window === 'undefined') return;
    try {
      const key = `terminal-dimensions-${sessionId}`;
      localStorage.setItem(key, JSON.stringify({ cols, rows }));
      console.log(`[TerminalOutput] Saved dimensions for ${sessionId}: ${cols}x${rows}`);
    } catch (err) {
      console.warn('[TerminalOutput] Failed to save dimensions:', err);
    }
  }, [sessionId]);

  // Scrollback loading state - DISABLED (scrollback functionality removed)
  // const [isLoadingHistory, setIsLoadingHistory] = useState(false);
  // const [hasMoreHistory, setHasMoreHistory] = useState(true);
  // const [oldestLoadedSequence, setOldestLoadedSequence] = useState<number>(0);
  // const scrollPositionBeforeLoadRef = useRef<number>(0);
  // const isScrollingToTopRef = useRef(false);

  // Write batching to prevent queue backup during rapid output (e.g., Claude Code animations)
  const writeBufferRef = useRef<string>("");
  const writeScheduledRef = useRef<boolean>(false);

  // Track pending write operations for flow control
  const pendingWritesRef = useRef<number>(0);
  const totalBytesWrittenRef = useRef<number>(0);
  const totalBytesCompletedRef = useRef<number>(0);

  // Watermark-based flow control (xterm.js best practices)
  const HIGH_WATERMARK = 100000; // 100KB - pause when buffer exceeds this
  const LOW_WATERMARK = 10000;   // 10KB - resume when buffer drops below this
  const watermarkRef = useRef<number>(0);
  const isPausedRef = useRef<boolean>(false);

  // Escape sequence parser to prevent splitting ANSI codes mid-sequence
  // Critical: ANSI escape sequences must be written atomically to avoid terminal corruption
  // Reference: https://xtermjs.org/docs/guides/flowcontrol/
  const escapeParserRef = useRef<EscapeSequenceParser>(new EscapeSequenceParser());

  // Ref to hold sendFlowControl function (allows use in callbacks defined before useTerminalStream)
  const sendFlowControlRef = useRef<((paused: boolean, watermark?: number) => void) | null>(null);

  // RedrawThrottler to prevent flickering from rapid full-screen redraws
  // Claude performs complete screen redraws at 12-25 FPS, causing visible flicker
  // This throttler coalesces rapid redraws to a maximum of 10 FPS
  class RedrawThrottler {
    private pendingRedraw: string | null = null;
    private throttleTimer: NodeJS.Timeout | null = null;
    private readonly throttleMs = 100; // 10 FPS max
    private onFlush: (data: string) => void;

    constructor(onFlush: (data: string) => void) {
      this.onFlush = onFlush;
    }

    process(chunk: string): string | null {
      // Detect full redraw pattern (cursor up at start)
      // Claude's status updates always start with \x1b[XXA where XX is line count
      const isFullRedraw = /^\x1b\[\d+A/.test(chunk);

      if (!isFullRedraw) {
        // Not a redraw, flush any pending and return this chunk immediately
        this.flushPending();
        return chunk;
      }

      // This is a full redraw - throttle it
      this.pendingRedraw = chunk; // Replace with latest

      if (!this.throttleTimer) {
        // Start throttle timer
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

  // Ref for the redraw throttler instance
  const redrawThrottlerRef = useRef<RedrawThrottler | null>(null);

  // Chunked write configuration
  // xterm.js has a 50MB hardcoded buffer limit, but we want to yield to UI much sooner
  const CHUNK_SIZE = 16384; // 16KB chunks - balance between throughput and UI responsiveness
  const CHUNK_DELAY_MS = 0; // Yield to event loop between chunks (setTimeout(0) allows UI updates)

  // Queue for managing chunked writes to prevent interleaving
  const writeQueueRef = useRef<Array<{ data: string; resolve: () => void }>>([]);
  const isProcessingQueueRef = useRef<boolean>(false);

  // Process the write queue sequentially with chunking
  const processWriteQueue = useCallback(async () => {
    if (isProcessingQueueRef.current || writeQueueRef.current.length === 0) {
      return;
    }

    isProcessingQueueRef.current = true;
    const terminal = xtermRef.current?.terminal;

    while (writeQueueRef.current.length > 0 && terminal) {
      const item = writeQueueRef.current[0];
      const data = item.data;

      // For small writes, write directly without chunking
      if (data.length <= CHUNK_SIZE) {
        await new Promise<void>((resolve) => {
          watermarkRef.current += data.length;

          // Check if we should pause upstream
          if (watermarkRef.current > HIGH_WATERMARK && !isPausedRef.current) {
            isPausedRef.current = true;
            console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${watermarkRef.current} bytes)`);
            sendFlowControlRef.current?.(true, watermarkRef.current);
          }

          terminal.write(data, () => {
            watermarkRef.current = Math.max(0, watermarkRef.current - data.length);

            // Check if we should resume upstream
            if (watermarkRef.current < LOW_WATERMARK && isPausedRef.current) {
              isPausedRef.current = false;
              console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${watermarkRef.current} bytes)`);
              sendFlowControlRef.current?.(false, watermarkRef.current);
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

          // Write chunk and wait for xterm.js to process it
          await new Promise<void>((resolve) => {
            watermarkRef.current += chunk.length;

            // Check if we should pause upstream
            if (watermarkRef.current > HIGH_WATERMARK && !isPausedRef.current) {
              isPausedRef.current = true;
              console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED during chunk ${chunkIndex}/${totalChunks} - Pausing stream (watermark: ${watermarkRef.current} bytes)`);
              sendFlowControlRef.current?.(true, watermarkRef.current);
            }

            terminal.write(chunk, () => {
              watermarkRef.current = Math.max(0, watermarkRef.current - chunk.length);

              // Check if we should resume upstream
              if (watermarkRef.current < LOW_WATERMARK && isPausedRef.current) {
                isPausedRef.current = false;
                console.log(`[FlowControl] LOW WATERMARK REACHED after chunk ${chunkIndex}/${totalChunks} - Resuming stream (watermark: ${watermarkRef.current} bytes)`);
                sendFlowControlRef.current?.(false, watermarkRef.current);
              }
              resolve();
            });
          });

          // Yield to UI between chunks using requestAnimationFrame for smooth rendering
          // This allows the browser to paint and handle user input between chunks
          if (i + CHUNK_SIZE < data.length) {
            await new Promise<void>((resolve) => {
              if (CHUNK_DELAY_MS > 0) {
                setTimeout(resolve, CHUNK_DELAY_MS);
              } else {
                // Use requestAnimationFrame for optimal UI responsiveness
                requestAnimationFrame(() => resolve());
              }
            });
          }
        }

        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log(`[FlowControl] Completed chunked write: ${data.length} bytes`);
        }
      }

      // Remove processed item and resolve its promise
      writeQueueRef.current.shift();
      item.resolve();
    }

    isProcessingQueueRef.current = false;
  }, [HIGH_WATERMARK, LOW_WATERMARK, CHUNK_SIZE, CHUNK_DELAY_MS]);

  // Enqueue data for chunked writing
  const enqueueWrite = useCallback((data: string): Promise<void> => {
    return new Promise((resolve) => {
      writeQueueRef.current.push({ data, resolve });
      // Start processing if not already running
      processWriteQueue();
    });
  }, [processWriteQueue]);

  // Debug mode state - synced with localStorage
  const [debugMode, setDebugMode] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("debug-terminal") === "true";
    }
    return false;
  });

  // Streaming mode selection (per-session configuration)
  const [streamingMode, setStreamingMode] = useState<"raw" | "raw-compressed" | "state" | "hybrid">("raw");

  // Recording state for debugging terminal flickering
  const [isRecording, setIsRecording] = useState(false);

  // Detect and respond to color scheme changes
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    if (typeof window !== "undefined") {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    return "dark";
  });

  useEffect(() => {
    if (typeof window === "undefined") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleThemeChange = (e: MediaQueryListEvent) => {
      setTheme(e.matches ? "dark" : "light");
    };

    mediaQuery.addEventListener("change", handleThemeChange);
    return () => mediaQuery.removeEventListener("change", handleThemeChange);
  }, []);

  // Callback to write initial pane content to terminal
  // Accepts initial pane content (no metadata) but rejects historical scrollback (with metadata)
  // Uses chunked writes to prevent UI freezing on large initial content
  const handleScrollbackReceived = useCallback(async (scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => {
    if (!xtermRef.current?.terminal) return;

    const terminal = xtermRef.current.terminal;

    // Reject historical scrollback requests (with metadata) - this is what was garbling output
    if (metadata) {
      console.log(`[TerminalOutput] Ignoring historical scrollback request (${scrollback.length} bytes) - auto-load disabled`, metadata);
      return;
    }

    // Accept initial pane content (no metadata) - this is for fast initial load
    console.log(`[TerminalOutput] Received initial pane content: ${scrollback.length} bytes`);

    // Current pane content - clear terminal first
    terminal.clear();

    // Use chunked writing for large initial content to prevent UI freezing
    // This writes data in CHUNK_SIZE pieces, yielding to the UI between chunks
    await enqueueWrite(scrollback);

    // Hide loading overlay now that initial content is displayed
    setIsLoadingInitialContent(false);

    // Write completed - scroll to bottom
    if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
      console.log('[FlowControl] Initial pane write completed (chunked)', {
        bytes: scrollback.length,
      });
    }

    // Use multiple strategies to ensure we scroll to bottom
    // Some terminal content may not render immediately
    terminal.scrollToBottom();

    // Also schedule delayed scrolls in case content is still rendering
    setTimeout(() => {
      terminal.scrollToBottom();
    }, 10);

    setTimeout(() => {
      terminal.scrollToBottom();
    }, 100);
  }, [enqueueWrite]);

  // Flush write buffer to terminal (called by requestAnimationFrame)
  // For large accumulated buffers, delegates to the chunked write queue
  const flushWriteBuffer = useCallback(() => {
    if (writeBufferRef.current && xtermRef.current) {
      const dataToWrite = writeBufferRef.current;
      const byteLength = dataToWrite.length;

      // Clear buffer immediately to prevent re-entrancy issues
      writeBufferRef.current = "";

      // For large accumulated buffers, use the chunked write queue
      // This prevents UI freezing when state updates accumulate a lot of data
      if (byteLength > CHUNK_SIZE) {
        enqueueWrite(dataToWrite);
        writeScheduledRef.current = false;
        return;
      }

      // Small buffer - write directly with flow control tracking
      pendingWritesRef.current++;
      totalBytesWrittenRef.current += byteLength;
      watermarkRef.current += byteLength;

      // Check if we should pause (exceeds HIGH_WATERMARK)
      if (watermarkRef.current > HIGH_WATERMARK && !isPausedRef.current) {
        isPausedRef.current = true;
        console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${watermarkRef.current} bytes)`);
        sendFlowControlRef.current?.(true, watermarkRef.current);
      }

      // Write with callback to track completion
      xtermRef.current.terminal?.write(dataToWrite, () => {
        // Write completed - decrement pending count and watermark
        pendingWritesRef.current = Math.max(0, pendingWritesRef.current - 1);
        totalBytesCompletedRef.current += byteLength;
        watermarkRef.current = Math.max(0, watermarkRef.current - byteLength);

        // Check if we should resume (below LOW_WATERMARK)
        if (watermarkRef.current < LOW_WATERMARK && isPausedRef.current) {
          isPausedRef.current = false;
          console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${watermarkRef.current} bytes)`);
          sendFlowControlRef.current?.(false, watermarkRef.current);
        }

        // Log in debug mode
        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log('[FlowControl] Write completed', {
            bytes: byteLength,
            watermark: watermarkRef.current,
            paused: isPausedRef.current,
            pending: pendingWritesRef.current,
            totalWritten: totalBytesWrittenRef.current,
            totalCompleted: totalBytesCompletedRef.current,
            backlog: totalBytesWrittenRef.current - totalBytesCompletedRef.current,
          });
        }
      });
    }
    writeScheduledRef.current = false;
  }, [HIGH_WATERMARK, LOW_WATERMARK, CHUNK_SIZE, enqueueWrite]);

  // Helper function to handle processed output
  const handleProcessedOutput = useCallback((safeOutput: string) => {
    if (!xtermRef.current) return;

    // Detect terminal mode transitions that may cause rendering issues
    // Claude Code's interactive UI uses alternate screen buffer and sync updates
    // When these modes exit, the terminal may not redraw properly
    const terminal = xtermRef.current.terminal;
    const needsRefresh =
      // Alternate screen buffer exit (used by interactive menus)
      safeOutput.includes('\x1b[?1049l') ||
      safeOutput.includes('\x1b[?47l') ||
      // Synchronous update mode end (batched rendering)
      safeOutput.includes('\x1b[?2026l') ||
      // Cursor visibility restore (often signals end of UI update)
      safeOutput.includes('\x1b[?25h');

    if (needsRefresh && terminal) {
      // Schedule a refresh after the write completes
      // Use requestAnimationFrame to ensure the write is processed first
      requestAnimationFrame(() => {
        if (xtermRef.current?.terminal) {
          const t = xtermRef.current.terminal;
          t.refresh(0, t.rows - 1);
          if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
            console.log('[TerminalOutput] Forced refresh after mode transition', {
              alternateScreenExit: safeOutput.includes('\x1b[?1049l') || safeOutput.includes('\x1b[?47l'),
              syncUpdateEnd: safeOutput.includes('\x1b[?2026l'),
              cursorRestore: safeOutput.includes('\x1b[?25h'),
            });
          }
        }
      });
    }

    if (safeOutput.length > 0) {
      // RAW MODES: Use chunked queue for large outputs, direct write for small
      // This maintains low latency for typical small messages while preventing freeze on bursts
      if (streamingMode === "raw" || streamingMode === "raw-compressed") {
        // For small writes (< CHUNK_SIZE), we can write directly for lowest latency
        // For larger writes, use the chunked queue to prevent UI freezing
        if (safeOutput.length <= CHUNK_SIZE) {
          // Small write - track and write directly (existing fast path)
          pendingWritesRef.current++;
          totalBytesWrittenRef.current += safeOutput.length;
          watermarkRef.current += safeOutput.length;

          // Check if we should pause upstream
          if (watermarkRef.current > HIGH_WATERMARK && !isPausedRef.current) {
            isPausedRef.current = true;
            console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${watermarkRef.current} bytes)`);
            sendFlowControlRef.current?.(true, watermarkRef.current);
          }

          xtermRef.current.terminal?.write(safeOutput, () => {
            pendingWritesRef.current = Math.max(0, pendingWritesRef.current - 1);
            totalBytesCompletedRef.current += safeOutput.length;
            watermarkRef.current = Math.max(0, watermarkRef.current - safeOutput.length);

            if (watermarkRef.current < LOW_WATERMARK && isPausedRef.current) {
              isPausedRef.current = false;
              console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${watermarkRef.current} bytes)`);
              sendFlowControlRef.current?.(false, watermarkRef.current);
            }
          });
        } else {
          // Large write - use chunked queue to prevent UI freezing
          enqueueWrite(safeOutput);
        }
        return;
      }

      // STATE/HYBRID MODES: Use batching with requestAnimationFrame
      // Accumulate writes and flush once per frame for smooth rendering
      writeBufferRef.current += safeOutput;

      // Schedule flush if not already scheduled (one write per animation frame)
      if (!writeScheduledRef.current) {
        writeScheduledRef.current = true;
        requestAnimationFrame(flushWriteBuffer);
      }
    }
  }, [streamingMode, flushWriteBuffer, HIGH_WATERMARK, LOW_WATERMARK, CHUNK_SIZE, enqueueWrite]);

  // Callback to write output directly to terminal
  // All modes now use the unified chunked write queue with flow control
  // This prevents UI freezing on large bursts while maintaining proper backpressure
  const handleOutput = useCallback((output: string) => {
    if (!xtermRef.current) return;

    // Record first output time for metrics (only once)
    if (metricsRef.current.firstOutputTime === null) {
      metricsRef.current.firstOutputTime = performance.now();
      // Log metrics now that we have the complete picture
      logTerminalMetrics();

      // CRITICAL: Hide loading overlay now that we've received first output
      // The initial content comes via state messages from currentPaneRequest,
      // not via scrollback, so we can't wait for handleScrollbackReceived
      setIsLoadingInitialContent(false);
    }

    // Initialize throttler if needed (lazy initialization)
    if (!redrawThrottlerRef.current) {
      redrawThrottlerRef.current = new RedrawThrottler((data) => {
        const safeOutput = escapeParserRef.current.processChunk(data);
        handleProcessedOutput(safeOutput);
      });
    }

    // Throttle rapid full-screen redraws to prevent flickering
    const result = redrawThrottlerRef.current.process(output);
    if (result) {
      // Not a redraw or needs immediate output
      const safeOutput = escapeParserRef.current.processChunk(result);
      handleProcessedOutput(safeOutput);
    }
  }, [handleProcessedOutput, logTerminalMetrics]);

  // Wrap terminal refresh method to detect all refresh calls (only in debug mode)
  useEffect(() => {
    if (xtermRef.current?.terminal && !xtermRef.current.terminal._refreshMonitorInstalled) {
      const terminal = xtermRef.current.terminal;
      const originalRefresh = terminal.refresh.bind(terminal);
      const originalWrite = terminal.write.bind(terminal);

      // Track write operations to detect race conditions
      let lastWriteTime = 0;
      let writeCount = 0;

      // Wrap write to track output timing
      terminal.write = (data: string | Uint8Array, callback?: () => void) => {
        const now = performance.now();
        const timeSinceLastWrite = now - lastWriteTime;
        lastWriteTime = now;
        writeCount++;

        // Only log if verbose debugging is enabled
        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log('[XtermWrite]', {
            writeCount,
            dataLength: data.length,
            timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
            cursorY: terminal.buffer.active.cursorY,
            timestamp: new Date().toISOString()
          });
        }

        return originalWrite(data, callback);
      };

      // Wrap refresh to log all calls and detect race conditions (only in debug mode to avoid overhead)
      terminal.refresh = (start: number, end: number) => {
        // Only run expensive monitoring in debug mode
        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          const stackTrace = new Error().stack;
          const caller = stackTrace?.split('\n')[2]?.trim() || 'unknown';
          const timeSinceLastWrite = performance.now() - lastWriteTime;

          console.log('[XtermRefresh] Refresh called', {
            start,
            end,
            rows: terminal.rows,
            timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
            recentWrites: writeCount,
            caller: caller.replace(/^at /, ''),
            possibleRaceCondition: timeSinceLastWrite < 50, // Flag if refresh happens <50ms after write
            timestamp: new Date().toISOString()
          });

          // Reset write counter after refresh
          writeCount = 0;
        }

        return originalRefresh(start, end);
      };

      terminal._refreshMonitorInstalled = true;
      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[TerminalOutput] Refresh and write monitoring installed');
      }
    }
  }, [xtermRef.current?.terminal]);

  // Unified WebSocket streaming - uses ConnectRPC protocol for both managed and external sessions
  // For external sessions: backend's resolveSession() finds session via ExternalDiscovery using tmuxSessionName
  // For managed sessions: backend finds session via ReviewQueuePoller/Storage using sessionId
  const effectiveSessionId = isExternal && tmuxSessionName ? tmuxSessionName : sessionId;

  // Stable callbacks - must not be inline arrow functions or they recreate on every render,
  // causing connect() to change every render and triggering the auto-reconnect effect in a loop.
  const getTerminal = useCallback(() => xtermRef.current?.terminal || null, []);
  const handleStreamError = useCallback((err: Error) => {
    console.error(`Terminal stream error (${isExternal ? 'external' : 'managed'}):`, err);
    setConnectionAttempts((prev) => prev + 1);
  }, [isExternal]);
  const handleEchoAck = useCallback((_echoNum: bigint, latencyMs: number) => {
    if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
      console.log('[PredictiveEcho] Echo acknowledged:', { latencyMs });
    }
  }, []);

  const { isConnected, error, sendInput, sendInputWithEcho, resize, connect, disconnect, scrollbackLoaded, requestScrollback, sendFlowControl, getIsApplyingState, sspNegotiated, startRecording, stopRecording } = useTerminalStream({
    baseUrl,
    sessionId: effectiveSessionId,
    getTerminal,
    scrollbackLines: 1000,
    autoConnect: false,
    onError: handleStreamError,
    onScrollbackReceived: handleScrollbackReceived,
    onOutput: handleOutput,
    initialCols: lastResizeRef.current?.cols,
    initialRows: lastResizeRef.current?.rows,
    streamingMode: streamingMode,
    isExternal: isExternal, // Pass external flag to hook (for future optimizations)
    enablePredictiveEcho: true, // Enable Mosh-style predictive echo for better responsiveness
    onEchoAck: handleEchoAck,
  });

  // Update ref when sendFlowControl is available (allows use in callbacks defined earlier)
  useEffect(() => {
    sendFlowControlRef.current = sendFlowControl;
  }, [sendFlowControl]);

  // Disconnect WebSocket on unmount and flush any pending writes
  useEffect(() => {
    return () => {
      isMountedRef.current = false; // Mark as unmounted

      // Clear any pending size stability timeout
      if (sizeStabilityTimeoutRef.current) {
        clearTimeout(sizeStabilityTimeoutRef.current);
        sizeStabilityTimeoutRef.current = null;
      }

      // Flush any buffered output before unmounting
      if (writeBufferRef.current && xtermRef.current) {
        xtermRef.current.write(writeBufferRef.current);
        writeBufferRef.current = "";
      }

      // Reset escape sequence parser to clear any buffered partial sequences
      escapeParserRef.current.reset();

      // Cleanup redraw throttler
      if (redrawThrottlerRef.current) {
        redrawThrottlerRef.current.cleanup();
        redrawThrottlerRef.current = null;
      }

      disconnect();
    };
  }, [disconnect]);

  // Handle terminal data input
  const handleTerminalData = useCallback((data: string) => {
    // Use predictive echo when SSP is negotiated for better responsiveness
    // This shows typed characters immediately (dimmed) before server confirmation
    if (sspNegotiated && sendInputWithEcho) {
      const echoNum = sendInputWithEcho(data);
      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[PredictiveEcho] Sent input with echo:', { data, echoNum: echoNum.toString() });
      }
    } else {
      // Fallback to regular input without predictive echo
      sendInput(data);
    }
  }, [sendInput, sendInputWithEcho, sspNegotiated]);

  // Handle terminal resize - uses event-driven size stability detection for initial connection
  // This prevents connecting with wrong dimensions during layout animations/transitions
  //
  // Strategy: Wait for ResizeObserver to stop firing (no resize events for 2 animation frames)
  // This is event-driven because it's triggered by actual layout changes, not arbitrary time.
  // The timeout acts as a "debounce until idle" - each resize event resets the wait.

  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

    // Always save resize dimensions (even if blocked) so they can be used for initial connection
    const lastResize = lastResizeRef.current;
    const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;

    if (sizeChanged) {
      lastResizeRef.current = { cols, rows };
      console.log(`[TerminalOutput] Saved resize dimensions: ${cols}x${rows}`);

      // Save dimensions to localStorage for instant future reconnections
      saveDimensions(cols, rows);

      // Record metrics for first resize
      if (metricsRef.current.firstResizeTime === null) {
        metricsRef.current.firstResizeTime = performance.now();
      }
      metricsRef.current.resizeCount++;

      // OPTIMIZED: Skip size stability wait if we have cached dimensions
      // This dramatically speeds up reconnection to sessions with known sizes
      if (hasCachedDimensionsRef.current && !hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
        console.log(`[TerminalOutput] Using cached dimensions, skipping stability wait`);
        metricsRef.current.sizeStableTime = performance.now();
        metricsRef.current.connectionInitTime = performance.now();
        hasInitiatedConnectionRef.current = true;
        setIsWaitingForStableSize(false);
        connect(cols, rows);
        return;
      }

      // Event-driven size stability detection for initial connection:
      // Wait until no more resize events occur, then wait 2 animation frames to ensure
      // the browser has finished painting. This is more reliable than a fixed timeout.
      if (!hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
        // Clear any pending stability check (size changed, restart the wait)
        if (sizeStabilityTimeoutRef.current) {
          clearTimeout(sizeStabilityTimeoutRef.current);
        }

        console.log(`[TerminalOutput] Size changed, waiting for layout to stabilize...`);
        setIsWaitingForStableSize(true);

        // Use a short timeout to debounce rapid resize events, then use RAF to ensure paint is complete
        // The 50ms debounce catches rapid ResizeObserver callbacks during layout shifts
        sizeStabilityTimeoutRef.current = setTimeout(() => {
          // After debounce, wait for 2 animation frames to ensure browser has painted
          // RAF 1: Ensure we're in the next frame after the last layout change
          // RAF 2: Ensure the paint from RAF 1 has completed
          requestAnimationFrame(() => {
            requestAnimationFrame(() => {
              if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current) {
                const stableSize = lastResizeRef.current;
                if (stableSize) {
                  // Record metrics: size is now stable, about to initiate connection
                  metricsRef.current.sizeStableTime = performance.now();
                  metricsRef.current.connectionInitTime = performance.now();

                  console.log(`[TerminalOutput] Layout stable at ${stableSize.cols}x${stableSize.rows}, initiating connection`);
                  hasInitiatedConnectionRef.current = true;
                  setIsWaitingForStableSize(false);
                  connect(stableSize.cols, stableSize.rows);
                }
              }
            });
          });
          sizeStabilityTimeoutRef.current = null;
        }, 50); // 50ms debounce for rapid resize events, then RAF takes over
      }
    }

    if (!isConnected) {
      console.log(`[TerminalOutput] Resize blocked - not connected (${cols}x${rows})`);
      return;
    }

    // Don't send if size unchanged (already saved above)
    if (!sizeChanged) {
      console.log(`[TerminalOutput] Resize blocked - unchanged (${cols}x${rows})`);
      return;
    }

    console.log(`[TerminalOutput] Sending resize: ${cols}x${rows} (prev: ${lastResize?.cols || 'none'}x${lastResize?.rows || 'none'})`);
    resize(cols, rows);
  }, [isConnected, resize, connect, error]);

  // Monitor connection state changes and show notifications
  useEffect(() => {
    const wasConnected = previousConnectionStateRef.current;
    previousConnectionStateRef.current = isConnected;

    if (!wasConnected && isConnected) {
      // Just connected - record metrics
      if (metricsRef.current.connectedTime === null) {
        metricsRef.current.connectedTime = performance.now();
      }

      console.log("[TerminalOutput] Connection established");
      setShowReconnectButton(false);
      setConnectionAttempts(0);

      // Clear any pending reconnect timeout
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    } else if (wasConnected && !isConnected) {
      // Just disconnected
      console.log("[TerminalOutput] Connection lost, will attempt reconnection");

      // Show reconnect button after 5 seconds if still disconnected
      reconnectTimeoutRef.current = setTimeout(() => {
        if (!isConnected) {
          setShowReconnectButton(true);
        }
      }, 5000);
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [isConnected]);

  // Auto-reconnect with exponential backoff
  useEffect(() => {
    if (!isConnected && error && connectionAttempts > 0 && connectionAttempts < 5) {
      const backoffDelay = Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000);
      console.log(`[TerminalOutput] Auto-reconnecting in ${backoffDelay}ms (attempt ${connectionAttempts})`);

      const timeout = setTimeout(() => {
        console.log("[TerminalOutput] Attempting reconnection...");
        connect();
      }, backoffDelay);

      return () => clearTimeout(timeout);
    }
  }, [isConnected, error, connectionAttempts, connect]);

  // Initialize with cached dimensions on mount
  useEffect(() => {
    const cached = getCachedDimensions();
    if (cached) {
      hasCachedDimensionsRef.current = true;
      lastResizeRef.current = cached;
      console.log(`[TerminalOutput] Initialized with cached dimensions: ${cached.cols}x${cached.rows}`);
    }
  }, [getCachedDimensions]);

  // Reset loading state when switching sessions
  useEffect(() => {
    setIsLoadingInitialContent(true);
    hasInitiatedConnectionRef.current = false;
    return () => {
      setIsLoadingInitialContent(false);
    };
  }, [sessionId]);

  const handleManualReconnect = useCallback(() => {
    console.log("[TerminalOutput] Manual reconnect requested");
    setConnectionAttempts(0);
    setShowReconnectButton(false);
    connect();
  }, [connect]);

  // Toggle debug mode
  const handleToggleDebug = useCallback(() => {
    const newDebugMode = !debugMode;
    setDebugMode(newDebugMode);

    // Sync with localStorage
    if (typeof window !== "undefined") {
      if (newDebugMode) {
        localStorage.setItem("debug-terminal", "true");
        console.log("%c[TerminalOutput] Debug mode ENABLED", "color: #00ff00; font-weight: bold");
        console.log("All terminal refresh and write operations will be logged");
      } else {
        localStorage.removeItem("debug-terminal");
        console.log("%c[TerminalOutput] Debug mode DISABLED", "color: #ff0000; font-weight: bold");
      }
    }
  }, [debugMode]);

  const handleCopyOutput = () => {
    // XtermTerminal handles copy internally via browser selection
    document.execCommand('copy');
  };

  const handleScrollToBottom = () => {
    if (xtermRef.current?.terminal) {
      xtermRef.current.terminal.scrollToBottom();
    }
  };

  const handleClear = () => {
    if (xtermRef.current && xtermRef.current.terminal) {
      const terminal = xtermRef.current.terminal;
      const startTime = performance.now();
      refreshCountRef.current++;

      // Log clear operation start
      console.log('[TerminalOutput] Clear requested', {
        refreshCount: refreshCountRef.current,
        bufferSize: terminal.buffer.active.length,
        rows: terminal.rows,
        cols: terminal.cols,
        scrollbackSize: terminal.buffer.normal.length
      });

      // Clear the terminal buffer
      xtermRef.current.clear();
      const clearTime = performance.now();

      // Force a full screen refresh to prevent corrupted output
      // This ensures xterm.js redraws the entire viewport properly
      terminal.refresh(0, terminal.rows - 1);
      const refreshTime = performance.now();

      // Additionally, reset the cursor to home position for clean state
      terminal.write('\x1b[H');
      const cursorResetTime = performance.now();

      // Log performance metrics and refresh details
      console.log('[TerminalOutput] Clear completed with forced refresh', {
        refreshCount: refreshCountRef.current,
        clearDuration: `${(clearTime - startTime).toFixed(2)}ms`,
        refreshDuration: `${(refreshTime - clearTime).toFixed(2)}ms`,
        cursorResetDuration: `${(cursorResetTime - refreshTime).toFixed(2)}ms`,
        totalDuration: `${(cursorResetTime - startTime).toFixed(2)}ms`,
        refreshedRows: `0-${terminal.rows - 1}`,
        viewport: {
          rows: terminal.rows,
          cols: terminal.cols,
          scrollTop: terminal.buffer.active.viewportY
        }
      });
    }
  };

  const handleManualResize = () => {
    console.log("[TerminalOutput] Manual resize triggered");
    if (xtermRef.current) {
      // Call fit() to resize terminal to container
      xtermRef.current.fit();

      // Get current terminal size after fit
      const terminal = xtermRef.current.terminal;
      if (terminal) {
        const cols = terminal.cols;
        const rows = terminal.rows;
        console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

        // Force send resize message to backend even if blocked by scrollback
        if (isConnected) {
          console.log(`[TerminalOutput] Forcing resize message to backend: ${cols}x${rows}`);
          lastResizeRef.current = { cols, rows };
          resize(cols, rows);
        }
      }
    }
  };

  // Disabled: handleLoadMoreHistory - scrollback functionality removed
  // const handleLoadMoreHistory = useCallback(() => {
  //   console.log(`[TerminalOutput] Scrollback disabled`);
  // }, []);

  // Infinite scroll detection - DISABLED (scrollback functionality removed)
  // useEffect(() => {
  //   // Scrollback auto-load disabled
  // }, [isConnected]);

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <div className={styles.status}>
          {isExternal && (
            <span className={styles.externalLabel} title="External session via claude-mux">
              🔗 External
            </span>
          )}
          <span
            className={`${styles.statusIndicator} ${
              isConnected ? styles.connected : isWaitingForStableSize ? styles.stabilizing : styles.disconnected
            }`}
          />
          <span className={styles.statusText}>
            {isConnected ? "Connected" : isWaitingForStableSize ? "Initializing..." : "Disconnected"}
          </span>
          {!isConnected && connectionAttempts > 0 && connectionAttempts < 5 && (
            <span className={styles.statusText}>
              {" "}• Reconnecting (attempt {connectionAttempts}/5)...
            </span>
          )}
          {!isConnected && connectionAttempts >= 5 && (
            <span className={styles.errorText}> • Connection failed</span>
          )}
          {error && !isConnected && (
            <span className={styles.errorText}> • {error.message}</span>
          )}
        </div>
        <div className={styles.actions}>
          {showReconnectButton && (
            <button
              className={styles.toolbarButton}
              onClick={handleManualReconnect}
              title="Reconnect to terminal"
              aria-label="Reconnect to terminal"
            >
              🔄 Reconnect
            </button>
          )}
          {/* Scrollback button removed - scrollback functionality disabled */}
          <button
            className={`${styles.toolbarButton} ${debugMode ? styles.debugActive : ''}`}
            onClick={handleToggleDebug}
            title={debugMode ? "Disable debug logging" : "Enable debug logging"}
            aria-label={debugMode ? "Disable debug mode" : "Enable debug mode"}
            style={debugMode ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
          >
            🛠️ {debugMode ? 'Debug ON' : 'Debug'}
          </button>
          <button
            className={styles.toolbarButton}
            onClick={() => {
              if (isRecording) {
                stopRecording();
                setIsRecording(false);
              } else {
                startRecording();
                setIsRecording(true);
              }
            }}
            title={isRecording ? "Stop recording" : "Start recording terminal output"}
            style={isRecording ? { backgroundColor: '#ff4444', color: 'white' } : {}}
          >
            {isRecording ? '⏹️ Stop Rec' : '⏺️ Record'}
          </button>
          <select
            value={streamingMode}
            onChange={(e) => setStreamingMode(e.target.value as "raw" | "raw-compressed" | "state" | "hybrid")}
            className={styles.toolbarButton}
            title="Terminal streaming mode - choose how terminal output is delivered"
            aria-label="Select terminal streaming mode"
            disabled={!isConnected}
            style={{ minWidth: '140px' }}
          >
            <option value="raw">🚀 Raw</option>
            <option value="raw-compressed">📦 Raw+LZMA</option>
            <option value="state">🔄 State Sync</option>
            <option value="hybrid">🔬 Hybrid</option>
          </select>
          <button
            className={styles.toolbarButton}
            onClick={handleManualResize}
            title="Resize terminal to fit container"
            aria-label="Resize terminal"
          >
            ↔️ Resize
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleClear}
            title="Clear terminal"
            aria-label="Clear terminal"
          >
            🗑️ Clear
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleScrollToBottom}
            title="Scroll to bottom"
            aria-label="Scroll to bottom"
          >
            ↓ Bottom
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleCopyOutput}
            title="Copy output"
            aria-label="Copy terminal output to clipboard"
          >
            📋 Copy
          </button>
        </div>
      </div>
      <div className={styles.terminal}>
        {isLoadingInitialContent && (
          <div className={styles.loadingOverlay}>
            <div className={styles.loadingSpinner} />
            <div className={styles.loadingText}>
              {isWaitingForStableSize ? "Initializing terminal..." : "Loading terminal content..."}
            </div>
          </div>
        )}
        <XtermTerminal
          ref={xtermRef}
          onData={handleTerminalData}
          onResize={handleTerminalResize}
          theme={theme}
          fontSize={14}
          scrollback={0}  // Disabled - tmux handles scrollback
        />
      </div>
    </div>
  );
}
