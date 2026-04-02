"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import { XtermTerminal, type XtermTerminalHandle } from "./XtermTerminal";
import { TerminalStreamManager } from "@/lib/terminal/TerminalStreamManager";
import { getCachedDimensions, saveDimensions } from "@/lib/terminal/TerminalDimensionCache";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
  isExternal?: boolean;
  tmuxSessionName?: string; // For external sessions, the tmux session name
}

export function TerminalOutput({ sessionId, baseUrl, isExternal = false, tmuxSessionName }: TerminalOutputProps) {
  const xtermRef = useRef<XtermTerminalHandle | null>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const [isWaitingForStableSize, setIsWaitingForStableSize] = useState(true);
  const [isLoadingInitialContent, setIsLoadingInitialContent] = useState(true);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const refreshCountRef = useRef(0);
  const isMountedRef = useRef(true);
  const sizeStabilityTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const hasInitiatedConnectionRef = useRef(false);
  const hasCachedDimensionsRef = useRef(false);

  // TerminalStreamManager ref -- lazily initialized when terminal is available
  const streamManagerRef = useRef<TerminalStreamManager | null>(null);

  // Ref to hold sendFlowControl function (allows use in callbacks defined before useTerminalStream)
  const sendFlowControlRef = useRef<((paused: boolean, watermark?: number) => void) | null>(null);

  // Terminal loading metrics
  const metricsRef = useRef<{
    mountTime: number;
    firstResizeTime: number | null;
    sizeStableTime: number | null;
    connectionInitTime: number | null;
    connectedTime: number | null;
    firstOutputTime: number | null;
    resizeCount: number;
  }>({
    mountTime: performance.now(),
    firstResizeTime: null,
    sizeStableTime: null,
    connectionInitTime: null,
    connectedTime: null,
    firstOutputTime: null,
    resizeCount: 0,
  });

  const logTerminalMetrics = useCallback(() => {
    const m = metricsRef.current;
    if (!m.firstOutputTime) return;

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

    console.log(`[TerminalMetrics] Terminal loaded in ${metrics.totalLoadTime.toFixed(0)}ms`, {
      breakdown: {
        sizeStabilization: `${metrics.sizeStabilizationDuration?.toFixed(0) || 'N/A'}ms (${metrics.resizeEventsBeforeStable} resizes)`,
        wsConnection: `${metrics.connectionDuration?.toFixed(0) || 'N/A'}ms`,
        totalLoad: `${metrics.totalLoadTime.toFixed(0)}ms`,
      },
      detailed: metrics,
    });

    if (typeof window !== 'undefined' && typeof window.performance?.mark === 'function') {
      try {
        performance.mark('terminal-loaded');
        performance.measure('terminal-load-time', { start: m.mountTime, end: m.firstOutputTime });
      } catch {
        // Ignore if performance API not fully supported
      }
    }
  }, [sessionId, tmuxSessionName, isExternal]);

  // Debug mode state
  const [debugMode, setDebugMode] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("debug-terminal") === "true";
    }
    return false;
  });

  // Streaming mode selection
  const [streamingMode, setStreamingMode] = useState<"raw" | "raw-compressed" | "state" | "hybrid">("raw");

  // Recording state
  const [isRecording, setIsRecording] = useState(false);

  // Theme detection
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    if (typeof window !== "undefined") {
      return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    return "dark";
  });

  useEffect(() => {
    if (typeof window === "undefined") return;
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleThemeChange = (e: MediaQueryListEvent) => setTheme(e.matches ? "dark" : "light");
    mediaQuery.addEventListener("change", handleThemeChange);
    return () => mediaQuery.removeEventListener("change", handleThemeChange);
  }, []);

  // Lazily create or get the TerminalStreamManager
  const getOrCreateStreamManager = useCallback((): TerminalStreamManager | null => {
    if (streamManagerRef.current) return streamManagerRef.current;

    const terminal = xtermRef.current?.terminal;
    if (!terminal) return null;

    const manager = new TerminalStreamManager(
      terminal,
      (paused, watermark) => sendFlowControlRef.current?.(paused, watermark)
    );

    // Track first output for metrics and loading overlay
    manager.setOnFirstOutput(() => {
      if (metricsRef.current.firstOutputTime === null) {
        metricsRef.current.firstOutputTime = performance.now();
        logTerminalMetrics();
        setIsLoadingInitialContent(false);
      }
    });

    // Install debug monitoring (respects debug-terminal localStorage flag)
    manager.installDebugMonitor();

    streamManagerRef.current = manager;
    return manager;
  }, [logTerminalMetrics]);

  // Callback to write initial pane content to terminal
  const handleScrollbackReceived = useCallback(async (scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => {
    if (!xtermRef.current?.terminal) return;

    // Reject historical scrollback requests (with metadata)
    if (metadata) {
      console.log(`[TerminalOutput] Ignoring historical scrollback request (${scrollback.length} bytes) - auto-load disabled`, metadata);
      return;
    }

    console.log(`[TerminalOutput] Received initial pane content: ${scrollback.length} bytes`);

    const manager = getOrCreateStreamManager();
    if (manager) {
      await manager.writeInitialContent(scrollback);
    }

    setIsLoadingInitialContent(false);
  }, [getOrCreateStreamManager]);

  // Callback to write output directly to terminal via TerminalStreamManager
  const handleOutput = useCallback((output: string) => {
    if (!xtermRef.current) return;

    const manager = getOrCreateStreamManager();
    if (manager) {
      manager.write(output);
    }
  }, [getOrCreateStreamManager]);

  // Unified WebSocket streaming
  const effectiveSessionId = isExternal && tmuxSessionName ? tmuxSessionName : sessionId;

  // Stable callbacks
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
    isExternal: isExternal,
    enablePredictiveEcho: true,
    onEchoAck: handleEchoAck,
  });

  // Update sendFlowControl ref when available + sync it into TerminalStreamManager
  useEffect(() => {
    sendFlowControlRef.current = sendFlowControl;
    if (streamManagerRef.current) {
      streamManagerRef.current.updateSendFlowControl(
        (paused, watermark) => sendFlowControlRef.current?.(paused, watermark)
      );
    }
  }, [sendFlowControl]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      isMountedRef.current = false;

      if (sizeStabilityTimeoutRef.current) {
        clearTimeout(sizeStabilityTimeoutRef.current);
        sizeStabilityTimeoutRef.current = null;
      }

      // Cleanup TerminalStreamManager
      if (streamManagerRef.current) {
        streamManagerRef.current.cleanup();
        streamManagerRef.current = null;
      }

      disconnect();
    };
  }, [disconnect]);

  // Handle terminal data input
  const handleTerminalData = useCallback((data: string) => {
    if (sspNegotiated && sendInputWithEcho) {
      const echoNum = sendInputWithEcho(data);
      if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
        console.log('[PredictiveEcho] Sent input with echo:', { data, echoNum: echoNum.toString() });
      }
    } else {
      sendInput(data);
    }
  }, [sendInput, sendInputWithEcho, sspNegotiated]);

  // Handle terminal resize with size stability detection
  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

    const lastResize = lastResizeRef.current;
    const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;

    if (sizeChanged) {
      lastResizeRef.current = { cols, rows };
      console.log(`[TerminalOutput] Saved resize dimensions: ${cols}x${rows}`);

      saveDimensions(sessionId, cols, rows);

      if (metricsRef.current.firstResizeTime === null) {
        metricsRef.current.firstResizeTime = performance.now();
      }
      metricsRef.current.resizeCount++;

      // Skip size stability wait if we have cached dimensions
      if (hasCachedDimensionsRef.current && !hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
        const initDims = lastResize ?? { cols, rows };
        console.log(`[TerminalOutput] Using cached dimensions, skipping stability wait (${initDims.cols}x${initDims.rows})`);
        metricsRef.current.sizeStableTime = performance.now();
        metricsRef.current.connectionInitTime = performance.now();
        hasInitiatedConnectionRef.current = true;
        setIsWaitingForStableSize(false);
        connect(initDims.cols, initDims.rows);
        return;
      }

      // Event-driven size stability detection for initial connection
      if (!hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
        if (sizeStabilityTimeoutRef.current) {
          clearTimeout(sizeStabilityTimeoutRef.current);
        }

        console.log(`[TerminalOutput] Size changed, waiting for layout to stabilize...`);
        setIsWaitingForStableSize(true);

        sizeStabilityTimeoutRef.current = setTimeout(() => {
          requestAnimationFrame(() => {
            requestAnimationFrame(() => {
              if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current) {
                const stableSize = lastResizeRef.current;
                if (stableSize) {
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
        }, 50);
      }
    }

    if (!isConnected) {
      console.log(`[TerminalOutput] Resize blocked - not connected (${cols}x${rows})`);
      return;
    }

    if (!sizeChanged) {
      console.log(`[TerminalOutput] Resize blocked - unchanged (${cols}x${rows})`);
      return;
    }

    console.log(`[TerminalOutput] Sending resize: ${cols}x${rows} (prev: ${lastResize?.cols || 'none'}x${lastResize?.rows || 'none'})`);
    resize(cols, rows);
  }, [isConnected, resize, connect, error, sessionId]);

  // Monitor connection state changes
  useEffect(() => {
    const wasConnected = previousConnectionStateRef.current;
    previousConnectionStateRef.current = isConnected;

    if (!wasConnected && isConnected) {
      if (metricsRef.current.connectedTime === null) {
        metricsRef.current.connectedTime = performance.now();
      }
      console.log("[TerminalOutput] Connection established");
      setShowReconnectButton(false);
      setConnectionAttempts(0);

      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }

      const currentSize = lastResizeRef.current;
      if (currentSize) {
        console.log(`[TerminalOutput] Post-connection resize sync: ${currentSize.cols}x${currentSize.rows}`);
        resize(currentSize.cols, currentSize.rows);
      }
    } else if (wasConnected && !isConnected) {
      console.log("[TerminalOutput] Connection lost, will attempt reconnection");
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
  }, [isConnected, resize]);

  // Clear loading overlay when max reconnect attempts reached
  useEffect(() => {
    if (connectionAttempts >= 5) {
      setIsLoadingInitialContent(false);
    }
  }, [connectionAttempts]);

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
    const cached = getCachedDimensions(sessionId);
    if (cached) {
      hasCachedDimensionsRef.current = true;
      lastResizeRef.current = cached;
      console.log(`[TerminalOutput] Initialized with cached dimensions: ${cached.cols}x${cached.rows}`);
    }
  }, [sessionId]);

  // Reset loading state when switching sessions
  useEffect(() => {
    setIsLoadingInitialContent(true);
    hasInitiatedConnectionRef.current = false;
    metricsRef.current.mountTime = performance.now();
    metricsRef.current.firstOutputTime = null;

    // Reset stream manager for new session
    if (streamManagerRef.current) {
      streamManagerRef.current.cleanup();
      streamManagerRef.current = null;
    }

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

  const handleToggleDebug = useCallback(() => {
    const newDebugMode = !debugMode;
    setDebugMode(newDebugMode);

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
    const selectedText = xtermRef.current?.terminal?.getSelection();
    if (selectedText) {
      navigator.clipboard.writeText(selectedText).catch(() => {
        document.execCommand('copy');
      });
    }
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

      console.log('[TerminalOutput] Clear requested', {
        refreshCount: refreshCountRef.current,
        bufferSize: terminal.buffer.active.length,
        rows: terminal.rows,
        cols: terminal.cols,
        scrollbackSize: terminal.buffer.normal.length
      });

      xtermRef.current.clear();
      const clearTime = performance.now();

      terminal.refresh(0, terminal.rows - 1);
      const refreshTime = performance.now();

      terminal.write('\x1b[H');
      const cursorResetTime = performance.now();

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
      xtermRef.current.fit();

      const terminal = xtermRef.current.terminal;
      if (terminal) {
        const cols = terminal.cols;
        const rows = terminal.rows;
        console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

        if (isConnected) {
          console.log(`[TerminalOutput] Forcing resize message to backend: ${cols}x${rows}`);
          lastResizeRef.current = { cols, rows };
          resize(cols, rows);
        }
      }
    }
  };

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
            <span className={styles.errorText}> • Terminal unavailable</span>
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
        {!isLoadingInitialContent && connectionAttempts >= 5 && (
          <div className={styles.unavailableOverlay}>
            <div className={styles.unavailableIcon}>⚠</div>
            <div className={styles.unavailableText}>Terminal unavailable</div>
            <div className={styles.unavailableSubtext}>Could not connect to terminal session</div>
          </div>
        )}
<XtermTerminal
  ref={xtermRef}
  onData={handleTerminalData}
  onResize={handleTerminalResize}
  theme={theme}
  fontSize={14}
  scrollback={5000}
  mouseTracking="any"
/>
      </div>
      {/* Mobile keyboard toolbar */}
      <div className={styles.mobileKeyboard}>
        <div className={styles.mobileKeyRow}>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b'); }} aria-label="Escape">Esc</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\t'); }} aria-label="Tab">Tab</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x03'); }} aria-label="Control C">Ctrl+C</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x04'); }} aria-label="Control D">Ctrl+D</button>
        </div>
        <div className={styles.mobileKeyRow}>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[D'); }} aria-label="Left arrow">←</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[A'); }} aria-label="Up arrow">↑</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[B'); }} aria-label="Down arrow">↓</button>
          <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); handleTerminalData('\x1b[C'); }} aria-label="Right arrow">→</button>
        </div>
      </div>
    </div>
  );
}
