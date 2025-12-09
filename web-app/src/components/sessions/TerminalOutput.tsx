"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import { XtermTerminal } from "./XtermTerminal";
import { EscapeSequenceParser } from "@/lib/terminal/EscapeSequenceParser";
import { decompressLZMA, isLZMACompressed } from "@/lib/compression/lzma";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
}

export function TerminalOutput({ sessionId, baseUrl }: TerminalOutputProps) {
  const xtermRef = useRef<any>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const refreshCountRef = useRef(0); // Track number of forced refreshes
  const isMountedRef = useRef(true); // Track component mount state for async operations

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

  // Debug mode state - synced with localStorage
  const [debugMode, setDebugMode] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("debug-terminal") === "true";
    }
    return false;
  });

  // Streaming mode selection (per-session configuration)
  const [streamingMode, setStreamingMode] = useState<"raw" | "raw-compressed" | "state" | "hybrid">("raw");

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
  const handleScrollbackReceived = useCallback((scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => {
    if (!xtermRef.current?.terminal) return;

    const terminal = xtermRef.current.terminal;

    // Reject historical scrollback requests (with metadata) - this is what was garbling output
    if (metadata) {
      console.log(`[TerminalOutput] Ignoring historical scrollback request (${scrollback.length} bytes) - auto-load disabled`, metadata);
      return;
    }

    // Accept initial pane content (no metadata) - this is for fast initial load
    console.log(`[TerminalOutput] Received initial pane content: ${scrollback.length} bytes`);

    // Current pane content - clear terminal first, then write with callback and scroll to bottom
    terminal.clear();
    terminal.write(scrollback, () => {
      // Current pane write completed
      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[FlowControl] Initial pane write completed', {
          bytes: scrollback.length,
        });
      }

      // Use multiple strategies to ensure we scroll to bottom
      // Some terminal content may not render immediately
      terminal.scrollToBottom();
    });

    // Also schedule delayed scrolls in case content is still rendering
    setTimeout(() => {
      terminal.scrollToBottom();
    }, 10);

    setTimeout(() => {
      terminal.scrollToBottom();
    }, 100);
  }, []); // No dependencies needed

  // Flush write buffer to terminal (called by requestAnimationFrame)
  const flushWriteBuffer = useCallback(() => {
    if (writeBufferRef.current && xtermRef.current) {
      const dataToWrite = writeBufferRef.current;
      const byteLength = dataToWrite.length;

      // Track write operation start
      pendingWritesRef.current++;
      totalBytesWrittenRef.current += byteLength;

      // Increase watermark (data queued for xterm.js)
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

      writeBufferRef.current = "";
    }
    writeScheduledRef.current = false;
  }, [HIGH_WATERMARK, LOW_WATERMARK]);

  // Callback to write output directly to terminal
  // For raw/raw-compressed modes: NO BATCHING - write immediately for lowest latency
  // For state/hybrid modes: Use batching to prevent flickering during state application
  const handleOutput = useCallback((output: string) => {
    if (!xtermRef.current) return;

    // RAW MODES: Write immediately without batching (tmux already handles output efficiently)
    if (streamingMode === "raw" || streamingMode === "raw-compressed") {
      // Process chunk through escape sequence parser to prevent splitting ANSI codes
      // This ensures control codes like \x1b[31m (red color) are written atomically
      const safeOutput = escapeParserRef.current.processChunk(output);

      if (safeOutput.length > 0) {
        // Track write operation for flow control
        pendingWritesRef.current++;
        totalBytesWrittenRef.current += safeOutput.length;
        watermarkRef.current += safeOutput.length;

        // Check if we should pause (exceeds HIGH_WATERMARK)
        if (watermarkRef.current > HIGH_WATERMARK && !isPausedRef.current) {
          isPausedRef.current = true;
          console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${watermarkRef.current} bytes)`);
          sendFlowControlRef.current?.(true, watermarkRef.current);
        }

        // Write immediately without batching
        xtermRef.current.terminal?.write(safeOutput, () => {
          // Write completed - decrement pending count and watermark
          pendingWritesRef.current = Math.max(0, pendingWritesRef.current - 1);
          totalBytesCompletedRef.current += safeOutput.length;
          watermarkRef.current = Math.max(0, watermarkRef.current - safeOutput.length);

          // Check if we should resume (below LOW_WATERMARK)
          if (watermarkRef.current < LOW_WATERMARK && isPausedRef.current) {
            isPausedRef.current = false;
            console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${watermarkRef.current} bytes)`);
            sendFlowControlRef.current?.(false, watermarkRef.current);
          }
        });
      }
      return;
    }

    // STATE/HYBRID MODES: Use batching to prevent flickering
    const safeOutput = escapeParserRef.current.processChunk(output);

    // Only accumulate if we have complete data to write
    // Partial escape sequences are buffered in the parser
    if (safeOutput.length > 0) {
      writeBufferRef.current += safeOutput;

      // Schedule flush if not already scheduled (one write per animation frame)
      if (!writeScheduledRef.current) {
        writeScheduledRef.current = true;
        requestAnimationFrame(flushWriteBuffer);
      }
    }
  }, [streamingMode, flushWriteBuffer, HIGH_WATERMARK, LOW_WATERMARK]);

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

  const { isConnected, error, sendInput, resize, connect, disconnect, scrollbackLoaded, requestScrollback, sendFlowControl, getIsApplyingState } = useTerminalStream({
    baseUrl,
    sessionId,
    getTerminal: () => xtermRef.current?.terminal || null, // Getter function for delta compression
    scrollbackLines: 1000,
    autoConnect: false, // Manual connection after terminal is properly sized
    onError: (err) => {
      console.error("Terminal stream error:", err);
      setConnectionAttempts((prev) => prev + 1);
    },
    onScrollbackReceived: handleScrollbackReceived,
    onOutput: handleOutput,
    // Pass actual terminal dimensions (set by first resize before connection)
    initialCols: lastResizeRef.current?.cols,
    initialRows: lastResizeRef.current?.rows,
    streamingMode: streamingMode, // Pass streaming mode to control server output format
  });

  // Update ref when sendFlowControl is available (allows use in callbacks defined earlier)
  useEffect(() => {
    sendFlowControlRef.current = sendFlowControl;
  }, [sendFlowControl]);

  // Disconnect WebSocket on unmount and flush any pending writes
  useEffect(() => {
    return () => {
      isMountedRef.current = false; // Mark as unmounted

      // Flush any buffered output before unmounting
      if (writeBufferRef.current && xtermRef.current) {
        xtermRef.current.write(writeBufferRef.current);
        writeBufferRef.current = "";
      }

      // Reset escape sequence parser to clear any buffered partial sequences
      escapeParserRef.current.reset();

      disconnect();
    };
  }, [disconnect]);

  // Handle terminal data input
  const handleTerminalData = useCallback((data: string) => {
    sendInput(data);
  }, [sendInput]);

  // Handle terminal resize - only send if size actually changed
  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

    // Always save resize dimensions (even if blocked) so they can be used for initial connection
    const lastResize = lastResizeRef.current;
    const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;

    if (sizeChanged) {
      lastResizeRef.current = { cols, rows };
      console.log(`[TerminalOutput] Saved resize dimensions: ${cols}x${rows}`);

      // If this is the first resize and we're not connected yet, initiate connection
      if (!lastResize && !isConnected && !error && isMountedRef.current) {
        console.log(`[TerminalOutput] First resize received, initiating connection with ${cols}x${rows}`);
        connect(cols, rows); // Pass dimensions directly to connect
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
  }, [isConnected, resize]);

  // Monitor connection state changes and show notifications
  useEffect(() => {
    const wasConnected = previousConnectionStateRef.current;
    previousConnectionStateRef.current = isConnected;

    if (!wasConnected && isConnected) {
      // Just connected
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
          <span
            className={`${styles.statusIndicator} ${
              isConnected ? styles.connected : styles.disconnected
            }`}
          />
          <span className={styles.statusText}>
            {isConnected ? "Connected" : "Disconnected"}
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
        <XtermTerminal
          ref={xtermRef}
          onData={handleTerminalData}
          onResize={handleTerminalResize}
          theme={theme}
          fontSize={14}
          scrollback={10000}
        />
      </div>
    </div>
  );
}
