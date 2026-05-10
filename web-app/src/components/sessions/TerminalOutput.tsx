"use client";
// +feature: terminal-pre-sizing terminal-dimension-cache terminal-image-upload

import { useEffect, useRef, useCallback, useState } from "react";

// xterm modifier key sequences (CSI parameter convention: modifier 5=Ctrl, 3=Alt).
// Defined at module level to avoid per-render allocation inside sendKey.
const CTRL_KEY_MAP: Record<string, string> = {
  '\x1b[A': '\x1b[1;5A',  // Ctrl+Up
  '\x1b[B': '\x1b[1;5B',  // Ctrl+Down
  '\x1b[C': '\x1b[1;5C',  // Ctrl+Right (word forward)
  '\x1b[D': '\x1b[1;5D',  // Ctrl+Left (word back)
  '\x1b[H': '\x1b[1;5H',  // Ctrl+Home
  '\x1b[F': '\x1b[1;5F',  // Ctrl+End
  '\x1b[5~': '\x1b[5;5~', // Ctrl+PgUp
  '\x1b[6~': '\x1b[6;5~', // Ctrl+PgDn
  '/': '\x1f',             // Ctrl+/ (unit separator)
  '-': '\x1f',             // Ctrl+- (maps to Ctrl+_)
};

const ALT_KEY_MAP: Record<string, string> = {
  '\x1b[A': '\x1b[1;3A',  // Alt+Up
  '\x1b[B': '\x1b[1;3B',  // Alt+Down
  '\x1b[C': '\x1b[1;3C',  // Alt+Right (word forward)
  '\x1b[D': '\x1b[1;3D',  // Alt+Left (word back)
  '\x1b[H': '\x1b[1;3H',  // Alt+Home
  '\x1b[F': '\x1b[1;3F',  // Alt+End
  '\x1b[5~': '\x1b[5;3~', // Alt+PgUp
  '\x1b[6~': '\x1b[6;3~', // Alt+PgDn
};
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import { useBrowserLogStream } from "@/lib/hooks/useBrowserLogStream";
import { XtermTerminal, type XtermTerminalHandle } from "./XtermTerminal";
import { TerminalStreamManager } from "@/lib/terminal/TerminalStreamManager";
import { getCachedDimensions, saveDimensions, validateCellDimensions } from "@/lib/terminal/TerminalDimensionCache";
import { DEFAULT_TERMINAL_CONFIG } from "@/lib/config/terminalConfig";
import { track } from "@/lib/telemetry";
import { useViewport } from "@/components/providers/ViewportProvider";
import * as styles from "./TerminalOutput.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
  isExternal?: boolean;
  tmuxSessionName?: string; // For external sessions, the tmux session name
  isVisible?: boolean; // When provided, triggers fit+focus on visibility change
}

// Minimum dimensions considered "real" — anything smaller is a transient value
// from xterm.js before the CSS container has finished laying out. The first
// resize event often fires at e.g. 10x6 before layout is complete; caching or
// connecting at those dimensions produces a garbled terminal on the next view.
const MIN_COLS = 30;
const MIN_ROWS = 10;

// xterm.js initializes with these default dimensions before FitAddon.fit() runs.
// Cache entries equal to these values are treated as potentially corrupt (see Bug 1)
// and are not used for fast-connect. The actual container size arrives via onResize.
const XTERM_DEFAULT_COLS = 80;
const XTERM_DEFAULT_ROWS = 24;

export function TerminalOutput({ sessionId, baseUrl, isExternal = false, tmuxSessionName, isVisible }: TerminalOutputProps) {
  const xtermRef = useRef<XtermTerminalHandle | null>(null);
  const terminalContainerRef = useRef<HTMLDivElement>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const [isWaitingForStableSize, setIsWaitingForStableSize] = useState(true);
  const [isLoadingInitialContent, setIsLoadingInitialContent] = useState(true);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const refreshCountRef = useRef(0);
  const isMountedRef = useRef(true);
  const isFittingRef = useRef(false);
  const sizeStabilityTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const hasInitiatedConnectionRef = useRef(false);
  const hasCachedDimensionsRef = useRef(false);
  // Set to true when we switch sessions while connected; triggers connect() once disconnect completes
  const pendingConnectAfterDisconnectRef = useRef(false);

  // TerminalStreamManager ref -- lazily initialized when terminal is available
  const streamManagerRef = useRef<TerminalStreamManager | null>(null);

  // Ref to hold sendFlowControl function (allows use in callbacks defined before useTerminalStream)
  const sendFlowControlRef = useRef<((paused: boolean, watermark?: number) => void) | null>(null);

  // Task 4.2.2 — Queue output during RESIZING state (Pitfall #2 / Race 3).
  // A snapshot arriving while the client is mid-resize writes bytes at the old column width.
  // Queuing output during resize prevents stale bytes from being written before the post-resize snapshot.
  const pendingOutputDuringResizeRef = useRef<string[]>([]);

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

  // Remote log streaming state — persisted in localStorage
  const [logStreamEnabled, setLogStreamEnabled] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("stapler-squad-remote-debug") === "true";
    }
    return false;
  });

  // Sticky modifier keys — CTRL and ALT arm on first tap, fire+clear on the next key.
  // Mutually exclusive: arming one disarms the other.
  const [ctrlActive, setCtrlActive] = useState(false);
  const [altActive, setAltActive] = useState(false);

  // Transient paste error shown briefly when clipboard access is denied.
  const [pasteError, setPasteError] = useState<string | null>(null);
  const pasteErrorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Image upload state for the camera/file-picker toolbar button.
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const uploadErrorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Mobile keyboard visibility — persisted in localStorage, scoped per session
  const keyboardStorageKey = `stapler-squad-mobile-keyboard-visible-${sessionId}`;
  const [isKeyboardVisible, setIsKeyboardVisible] = useState(() => {
    if (typeof window === 'undefined') return true;
    try {
      const stored = localStorage.getItem(keyboardStorageKey);
      return stored === null ? true : stored === 'true';
    } catch {
      return true;
    }
  });

  const toggleMobileKeyboard = useCallback(() => {
    setIsKeyboardVisible(prev => {
      const next = !prev;
      try {
        localStorage.setItem(keyboardStorageKey, String(next));
      } catch {
        // localStorage full or disabled — continue without persistence
      }
      return next;
    });
  }, [keyboardStorageKey]);

  // Mobile detection — use shared ViewportProvider hook for consistency
  const { isMobile } = useViewport();

  // Toolbar collapsed/expanded state — persisted in localStorage; collapsed by default
  const [toolbarExpanded, setToolbarExpanded] = useState(() => {
    if (typeof window === 'undefined') return false;
    try {
      const s = localStorage.getItem('stapler-squad-toolbar-expanded');
      return s === null ? false : s === 'true';
    } catch {
      return false;
    }
  });
  const [mobileOverflowOpen, setMobileOverflowOpen] = useState(false);

  useEffect(() => {
    try {
      localStorage.setItem('stapler-squad-toolbar-expanded', String(toolbarExpanded));
    } catch {
      // localStorage unavailable — continue without persistence
    }
  }, [toolbarExpanded]);

  // Mouse tracking mode: "none" on mobile (enables xterm selection + touch scroll),
  // "any" on desktop (forwards mouse events to terminal app for vim/tmux mouse support).
  // User can toggle this via the ⌨️ toolbar to switch modes.
  const [mouseMode, setMouseMode] = useState<'none' | 'any'>('any');

  useEffect(() => {
    setMouseMode(isMobile ? 'none' : 'any');
  }, [isMobile]);

  const toggleMouseMode = useCallback(() => {
    setMouseMode(prev => prev === 'none' ? 'any' : 'none');
  }, []);

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

    // Inject SerializeAddon so prependScrollbackBatch can serialize the current buffer
    // before clearing it (enables correct history order without losing live content).
    const serializeAddon = xtermRef.current?.serializeAddon;
    if (serializeAddon) {
      manager.setSerializeAddon(serializeAddon);
    }

    // Track first output for metrics and loading overlay
    manager.setOnFirstOutput(() => {
      if (metricsRef.current.firstOutputTime === null) {
        metricsRef.current.firstOutputTime = performance.now();
        logTerminalMetrics();
        const totalLoadTime = metricsRef.current.firstOutputTime - metricsRef.current.mountTime;
        track('session_attach', totalLoadTime, { phase: 'attach' }, sessionId);
        const connectionDuration = metricsRef.current.connectedTime && metricsRef.current.connectionInitTime
          ? metricsRef.current.connectedTime - metricsRef.current.connectionInitTime
          : totalLoadTime;
        track('stream_terminal_first_byte', connectionDuration, undefined, sessionId);
        setIsLoadingInitialContent(false);
      }
    });

    // Install debug monitoring (respects debug-terminal localStorage flag)
    manager.installDebugMonitor();

    streamManagerRef.current = manager;
    return manager;
  }, [logTerminalMetrics, sessionId]);

  // Ref to track whether the initial scrollback has been written (Task 2.3.2)
  const isInitialScrollbackDoneRef = useRef(false);
  // Paging state for on-demand scrollback loading (Task 2.3.1 / 2.3.2)
  const isFetchingScrollbackRef = useRef(false);
  const hasMoreScrollbackRef = useRef(false);
  const oldestSequenceReceivedRef = useRef(0);

  // Callback to write initial pane content to terminal.
  // Metadata guard removed (R2.7): all scrollback — both initial and historical —
  // is now allowed to proceed. Initial load calls writeInitialContent(); paged
  // loads (subsequent calls with metadata) call prependScrollbackBatch().
  const handleScrollbackReceived = useCallback(async (scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => {
    if (!xtermRef.current?.terminal) return;

    const manager = getOrCreateStreamManager();
    if (!manager) return;

    if (!isInitialScrollbackDoneRef.current) {
      // Initial load
      console.log(`[TerminalOutput] Writing initial scrollback: ${scrollback.length} bytes`);
      isInitialScrollbackDoneRef.current = true;
      await manager.writeInitialContent(scrollback);
      if (metadata) {
        hasMoreScrollbackRef.current = metadata.hasMore;
        oldestSequenceReceivedRef.current = metadata.oldestSequence;
      }
    } else {
      // Paged history load
      console.log(`[TerminalOutput] Writing paged scrollback: ${scrollback.length} bytes`);
      try {
        await manager.prependScrollbackBatch(scrollback);
        if (metadata) {
          hasMoreScrollbackRef.current = metadata.hasMore;
          oldestSequenceReceivedRef.current = metadata.oldestSequence;
        }
      } catch (err) {
        console.error('[TerminalOutput] prependScrollbackBatch failed:', err);
      } finally {
        isFetchingScrollbackRef.current = false;
      }
    }

    setIsLoadingInitialContent(false);
  }, [getOrCreateStreamManager]);

  // Ref to access current terminalState inside the handleOutput callback
  // (useCallback can't take terminalState as a dep without recreating on every state change)
  const terminalStateRef = useRef<string>('DISCONNECTED');

  // Callback to write output directly to terminal via TerminalStreamManager.
  // Task 4.2.2: Output is queued during RESIZING to prevent bytes at the old column width
  // from being written before the post-resize snapshot.
  const handleOutput = useCallback((output: string) => {
    if (!xtermRef.current) return;

    if (terminalStateRef.current === 'RESIZING') {
      pendingOutputDuringResizeRef.current.push(output);
      return;
    }

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

  const { isConnected, error, sendInput, sendInputWithEcho, resize, connect, disconnect, scrollbackLoaded, requestScrollback, sendFlowControl, getIsApplyingState, sspNegotiated, startRecording, stopRecording, terminalState } = useTerminalStream({
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

  // Sync terminalState into a ref so handleOutput can read it without recreating the callback.
  // Also flushes queued output when transitioning from RESIZING to STABLE (Task 4.2.2).
  useEffect(() => {
    const prevState = terminalStateRef.current;
    terminalStateRef.current = terminalState;

    if (prevState === 'RESIZING' && terminalState === 'STABLE') {
      const manager = streamManagerRef.current;
      if (manager) {
        const pending = pendingOutputDuringResizeRef.current.splice(0);
        for (const chunk of pending) {
          manager.write(chunk);
        }
        console.log(`[TerminalOutput] Flushed ${pending.length} pending output chunks after resize quiescence`);
      }
    }
  }, [terminalState]);

  // Remote browser log streaming for mobile debugging
  useBrowserLogStream({ enabled: logStreamEnabled, sessionId, baseUrl });

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

      if (pasteErrorTimerRef.current) {
        clearTimeout(pasteErrorTimerRef.current);
        pasteErrorTimerRef.current = null;
      }

      if (uploadErrorTimerRef.current) {
        clearTimeout(uploadErrorTimerRef.current);
        uploadErrorTimerRef.current = null;
      }

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

  // Send a key sequence, applying any active sticky modifier (CTRL or ALT) first.
  // Modifier sequences follow xterm's parameter convention:
  //   modifier 3 = Alt (escape prefix or CSI param ;3)
  //   modifier 5 = Ctrl (CSI param ;5)
  const sendKey = useCallback((keyData: string) => {
    let data = keyData;

    if (ctrlActive) {
      data = CTRL_KEY_MAP[keyData] ?? keyData;
      setCtrlActive(false);
    } else if (altActive) {
      data = ALT_KEY_MAP[keyData] ?? '\x1b' + keyData;
      setAltActive(false);
    }

    handleTerminalData(data);
  }, [ctrlActive, altActive, handleTerminalData]);

  // Handle terminal resize with size stability detection
  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

    const lastResize = lastResizeRef.current;
    const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;

    if (sizeChanged) {
      lastResizeRef.current = { cols, rows };
      console.log(`[TerminalOutput] Saved resize dimensions: ${cols}x${rows}`);

      // Only persist dimensions that are plausibly real — transient tiny values
      // (e.g. 10x6) fired before the CSS container finishes layout would otherwise
      // corrupt the cache and cause the next session view to connect at the wrong size.
      if (cols >= MIN_COLS && rows >= MIN_ROWS) {
        // Also capture cell pixel dimensions so TerminalOutput can pre-calculate
        // cols/rows from the container size on the next mount, enabling an immediate
        // connection before xterm fires its first onResize event.
        // Uses xterm.js private API for cell pixel metrics; gracefully falls back
        // to dims-only cache entry if the API changes in a future xterm version.
        // isFinite() guards against NaN/Infinity that a corrupted private API
        // could theoretically return (e.g. during a render-service error state).
        const cell = (xtermRef.current?.terminal as any)?._core?._renderService?.dimensions?.css?.cell;
        const currentFontSize = xtermRef.current?.terminal?.options?.fontSize ?? 14;
        const currentFontFamily = xtermRef.current?.terminal?.options?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace';
        if (cell?.width && cell?.height && isFinite(cell.width) && isFinite(cell.height)) {
          saveDimensions(sessionId, cols, rows, cell.width, cell.height, currentFontSize, currentFontFamily);
        } else {
          saveDimensions(sessionId, cols, rows, undefined, undefined, currentFontSize, currentFontFamily);
        }
      } else {
        console.log(`[TerminalOutput] Skipping cache write for tiny dimensions ${cols}x${rows} (below ${MIN_COLS}x${MIN_ROWS})`);
      }

      if (metricsRef.current.firstResizeTime === null) {
        metricsRef.current.firstResizeTime = performance.now();
      }
      metricsRef.current.resizeCount++;

      // Skip size stability wait if we have cached dimensions, but only when
      // the cached size (lastResize) is itself reasonable — a stale tiny cache
      // entry would otherwise bypass the stability wait and connect at the wrong size.
      if (hasCachedDimensionsRef.current && !hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
        const initDims = { cols, rows };
        if (initDims.cols >= MIN_COLS && initDims.rows >= MIN_ROWS) {
          console.log(`[TerminalOutput] Using cached dimensions, skipping stability wait (${initDims.cols}x${initDims.rows})`);
          metricsRef.current.sizeStableTime = performance.now();
          metricsRef.current.connectionInitTime = performance.now();
          hasInitiatedConnectionRef.current = true;
          setIsWaitingForStableSize(false);
          connect(initDims.cols, initDims.rows);
          return;
        } else {
          // Cached value is too small — treat as no cache and wait for stable size.
          console.log(`[TerminalOutput] Cached dimensions ${initDims.cols}x${initDims.rows} too small, falling through to stability wait`);
          hasCachedDimensionsRef.current = false; // prevents future onResize events from fast-connecting on stale cache
        }
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
      // If connection drops while still loading, content won't arrive — clear the overlay
      // so the user sees the terminal pane and "Disconnected" status instead of a stuck spinner.
      setIsLoadingInitialContent(false);
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

  // Scroll to bottom whenever loading finishes (session open or switch)
  useEffect(() => {
    if (!isLoadingInitialContent && xtermRef.current?.terminal) {
      const terminal = xtermRef.current.terminal;
      requestAnimationFrame(() => {
        terminal.scrollToBottom();
        setTimeout(() => terminal.scrollToBottom(), 100);
      });
    }
  }, [isLoadingInitialContent]);

  // Task 2.3.1 — DOM scroll listener to detect near-top-of-buffer and trigger paged history load.
  // Uses DOM 'scroll' event on terminal.element (NOT terminal.onScroll, which only fires on buffer
  // writes, not user scroll gestures — see xterm.js issues #3201, #3864).
  useEffect(() => {
    if (isLoadingInitialContent) return; // Don't attach until initial content is loaded

    const terminal = xtermRef.current?.terminal;
    if (!terminal?.element) return;

    // xterm.js scrolls the .xterm-viewport child element, not the root terminal.element —
    // attaching the listener to terminal.element would never fire on scroll (Bug 5 fix).
    const scrollEl = terminal.element?.querySelector('.xterm-viewport') ?? terminal.element;

    const onScroll = () => {
      const viewportY = terminal.buffer.active.viewportY;
      if (
        viewportY < 200 &&
        !isFetchingScrollbackRef.current &&
        hasMoreScrollbackRef.current &&
        isConnected
      ) {
        isFetchingScrollbackRef.current = true;
        console.log(`[TerminalOutput] Near top of buffer (viewportY=${viewportY}), requesting older scrollback from seq ${oldestSequenceReceivedRef.current}`);
        requestScrollback(oldestSequenceReceivedRef.current, 500);
      }
    };

    scrollEl?.addEventListener('scroll', onScroll, { passive: true });
    return () => scrollEl?.removeEventListener('scroll', onScroll);
  }, [isLoadingInitialContent, isConnected, requestScrollback]);

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

  // Initialize with cached dimensions on mount.
  // When cell pixel metrics are also cached, pre-calculate cols/rows from the
  // container's current pixel size so the session-switch effect can connect
  // immediately — before xterm.js fires its first onResize event.
  //
  // ORDERING INVARIANT: This effect MUST remain defined before the session-switch
  // effect below (both share the [sessionId] dependency). React runs same-dependency
  // effects in definition order, so this effect runs first and populates
  // lastResizeRef before the session-switch effect reads it to trigger connect().
  // Moving this effect below the session-switch effect will silently break pre-sizing.
  useEffect(() => {
    const rawCached = getCachedDimensions(sessionId);
    // Validate cell dims against current font config (R1.6): stale dims from a different
    // font configuration produce an incorrect initial fit() and wrong initial resize.
    // Use DEFAULT_TERMINAL_CONFIG values so this stays in sync with XtermTerminal's actual
    // font settings rather than being hardcoded independently (Bug 4 fix).
    const currentFontSize = DEFAULT_TERMINAL_CONFIG.fontSize;
    const currentFontFamily = DEFAULT_TERMINAL_CONFIG.fontFamily;
    const cached = rawCached
      ? validateCellDimensions(rawCached, currentFontSize, currentFontFamily)
      : null;
    if (cached && cached.cols >= MIN_COLS && cached.rows >= MIN_ROWS) {
      hasCachedDimensionsRef.current = true;
      console.log(`[TerminalOutput] Initialized with cached dimensions: ${cached.cols}x${cached.rows}`);

      if (cached.cellWidth && cached.cellHeight && terminalContainerRef.current) {
        const rect = terminalContainerRef.current.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
          const preCols = Math.floor(rect.width / cached.cellWidth);
          const preRows = Math.floor(rect.height / cached.cellHeight);
          if (preCols >= MIN_COLS && preRows >= MIN_ROWS) {
            console.log(
              `[TerminalOutput] Pre-sizing: ${rect.width}×${rect.height}px / ` +
              `${cached.cellWidth.toFixed(2)}×${cached.cellHeight.toFixed(2)}px/cell → ${preCols}×${preRows}`
            );
            lastResizeRef.current = { cols: preCols, rows: preRows };
          } else {
            console.log(`[TerminalOutput] Pre-sizing skipped: calculated ${preCols}x${preRows} below minimum`);
          }
        } else {
          console.log(`[TerminalOutput] Pre-sizing skipped: container has zero size`);
        }
      }
    } else if (cached) {
      console.log(`[TerminalOutput] Ignoring stale cached dimensions ${cached.cols}x${cached.rows} (below ${MIN_COLS}x${MIN_ROWS})`);
    }
  }, [sessionId]);

  // When terminal becomes visible (e.g. session switch in pool), trigger fit+focus
  useEffect(() => {
    if (isVisible && xtermRef.current) {
      setTimeout(() => {
        xtermRef.current?.fit();
        xtermRef.current?.terminal?.focus();
      }, 50);
    }
  }, [isVisible]);

  // visualViewport resize listener — re-fits terminal when the on-screen keyboard
  // appears/disappears on mobile (visualViewport changes don't fire window resize).
  // isFittingRef guard prevents resize loops on iOS where fit() triggers another resize event.
  useEffect(() => {
    const vp = window.visualViewport;
    if (!vp) return;

    const onVpResize = () => {
      if (isFittingRef.current) return;
      isFittingRef.current = true;
      // Increase debounce on mobile (400ms) to wait for keyboard animation to finish
      setTimeout(() => {
        xtermRef.current?.fit();
        requestAnimationFrame(() => { isFittingRef.current = false; });
      }, isMobile ? 400 : 300);
    };

    vp.addEventListener('resize', onVpResize);
    return () => vp.removeEventListener('resize', onVpResize);
  }, [isMobile]);

  // Reset loading state when switching sessions and trigger reconnect
  useEffect(() => {
    setIsLoadingInitialContent(true);
    hasInitiatedConnectionRef.current = false;
    metricsRef.current.mountTime = performance.now();
    metricsRef.current.firstOutputTime = null;

    // Reset connection tracking so the new session doesn't inherit stale state
    previousConnectionStateRef.current = false;
    setConnectionAttempts(0);
    setShowReconnectButton(false);
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    // Reset stream manager for new session
    if (streamManagerRef.current) {
      streamManagerRef.current.cleanup();
      streamManagerRef.current = null;
    }

    // Connect immediately if already disconnected (e.g. first load, or was already disconnected)
    // Otherwise set the pending flag so we connect once the in-progress disconnect resolves
    if (!isConnected) {
      const dims = lastResizeRef.current;
      // Guard: don't fast-connect with xterm default dims (80×24). Those are the
      // terminal's initial values before fitAddon.fit() measures the container —
      // if Bug 1 corrupted the cache to 80×24, using them here would cause a thin
      // PTY. The resize handler will fire with the actual dims and connect normally.
      const isXtermDefault = dims?.cols === XTERM_DEFAULT_COLS && dims?.rows === XTERM_DEFAULT_ROWS;
      if (dims && isMountedRef.current && !isXtermDefault) {
        hasInitiatedConnectionRef.current = true;
        setIsWaitingForStableSize(false);
        connect(dims.cols, dims.rows);
      }
      // If no dims yet (or dims are xterm defaults), resize handler will fire and trigger connect normally
    } else {
      // Was connected to previous session — disconnect() is in-flight (async).
      // Mark pending so the isConnected→false transition triggers connect below.
      pendingConnectAfterDisconnectRef.current = true;
    }

    // Safety net: if the container is hidden at mount (display:none, 0×0), the
    // ResizeObserver zero-size guard prevents fitAddon.fit(), so no resize event
    // fires and the stability timer never starts. After 5s, attempt to connect
    // with whatever valid cached dims are available, or skip silently.
    const safetyTimeout = setTimeout(() => {
      if (!hasInitiatedConnectionRef.current && isMountedRef.current) {
        const dims = lastResizeRef.current;
        if (dims && dims.cols >= MIN_COLS && dims.rows >= MIN_ROWS) {
          console.log(`[TerminalOutput] Safety timeout: connecting with cached dims ${dims.cols}x${dims.rows} (container may have been hidden at mount)`);
          hasInitiatedConnectionRef.current = true;
          setIsWaitingForStableSize(false);
          connect(dims.cols, dims.rows);
        } else {
          console.log(`[TerminalOutput] Safety timeout: no valid dims available, container still not visible`);
        }
      }
    }, 5000);

    return () => {
      clearTimeout(safetyTimeout);
      setIsLoadingInitialContent(false);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  // When a session switch happened while connected, the previous disconnect() is async (up to 1s).
  // This effect fires once isConnected transitions to false, completing the switch.
  useEffect(() => {
    if (!isConnected && pendingConnectAfterDisconnectRef.current && !hasInitiatedConnectionRef.current && isMountedRef.current) {
      pendingConnectAfterDisconnectRef.current = false;
      const dims = lastResizeRef.current;
      if (dims) {
        console.log(`[TerminalOutput] Post-disconnect connect for new session: ${dims.cols}x${dims.rows}`);
        hasInitiatedConnectionRef.current = true;
        setIsWaitingForStableSize(false);
        connect(dims.cols, dims.rows);
      }
      // If no dims, the resize handler will fire and connect normally
    }
  }, [isConnected, connect]);

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

  const handleToggleLogStream = useCallback(() => {
    const next = !logStreamEnabled;
    setLogStreamEnabled(next);
    if (typeof window !== "undefined") {
      if (next) {
        localStorage.setItem("stapler-squad-remote-debug", "true");
      } else {
        localStorage.removeItem("stapler-squad-remote-debug");
      }
    }
  }, [logStreamEnabled]);

  const handleCopyOutput = () => {
    const selectedText = xtermRef.current?.terminal?.getSelection();
    if (selectedText) {
      navigator.clipboard.writeText(selectedText).catch(() => {
        document.execCommand('copy');
      });
    }
  };

  // Read from clipboard and inject into the terminal.
  // Text is sent directly; images are uploaded to the server and the resulting
  // file path is inserted so the terminal process (e.g. Claude Code) can read
  // the image from disk via normal file path conventions.
  const handlePaste = useCallback(async () => {
    try {
      // navigator.clipboard.read() supports both text and image items
      if (navigator.clipboard?.read) {
        const items = await navigator.clipboard.read();
        for (const item of items) {
          const imageType = item.types.find(t => t.startsWith('image/'));
          if (imageType) {
            const blob = await item.getType(imageType);
            // Convert to base64 (strip the "data:image/...;base64," prefix)
            const base64 = await new Promise<string>((resolve, reject) => {
              const reader = new FileReader();
              reader.onload = () => resolve((reader.result as string).split(',')[1]);
              reader.onerror = reject;
              reader.readAsDataURL(blob);
            });
            const uploadUrl = `${baseUrl}/upload/image`;
            const resp = await fetch(uploadUrl, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ data: base64, contentType: imageType }),
            });
            if (resp.ok) {
              const { path } = await resp.json();
              handleTerminalData(path);
            } else {
              const msg = await resp.text().catch(() => `HTTP ${resp.status}`);
              console.warn('[TerminalOutput] Image upload failed:', msg);
              if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
              setPasteError('Upload failed');
              pasteErrorTimerRef.current = setTimeout(() => setPasteError(null), 2500);
            }
            return;
          }
          if (item.types.includes('text/plain')) {
            const blob = await item.getType('text/plain');
            const text = await blob.text();
            if (text) handleTerminalData(text);
            return;
          }
        }
      } else {
        // Fallback: text-only clipboard API
        const text = await navigator.clipboard.readText();
        if (text) handleTerminalData(text);
      }
    } catch (err) {
      console.warn('[TerminalOutput] Clipboard access failed:', err);
      if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
      setPasteError('Clipboard access denied');
      pasteErrorTimerRef.current = setTimeout(() => setPasteError(null), 2500);
    }
  }, [handleTerminalData, baseUrl]);

  const handleScrollToBottom = () => {
    if (xtermRef.current?.terminal) {
      xtermRef.current.terminal.scrollToBottom();
    }
  };

  // Synchronous handler — MUST NOT have await before .click() (iOS Safari requirement).
  const handleImageButtonClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileUpload = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    // Reset input so the same file can trigger onChange again.
    e.target.value = "";

    setIsUploading(true);
    setUploadError(null);

    try {
      const formData = new FormData();
      formData.append("session_id", sessionId);
      formData.append("file", file);

      // Do NOT set Content-Type header — browser sets it with multipart boundary.
      const resp = await fetch(`${baseUrl}/v1/upload-image`, {
        method: "POST",
        body: formData,
      });

      if (resp.ok) {
        const data = await resp.json() as { path: string; filename: string };
        // Insert path into terminal with trailing space so user can type after it.
        handleTerminalData(data.path + " ");
      } else {
        let msg = "Upload failed";
        if (resp.status === 413) msg = "File too large (max 10 MB)";
        else if (resp.status === 400 || resp.status === 415) msg = "Invalid image type";
        else if (resp.status === 404) msg = "Session not found";
        setUploadError(msg);
        if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
        uploadErrorTimerRef.current = setTimeout(() => setUploadError(null), 3000);
      }
    } catch {
      setUploadError("Network error");
      if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
      uploadErrorTimerRef.current = setTimeout(() => setUploadError(null), 3000);
    } finally {
      setIsUploading(false);
    }
  }, [sessionId, baseUrl, handleTerminalData]);

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

  const secondaryActions = [
    {
      key: 'mouse',
      icon: '🖱️',
      label: mouseMode === 'none' ? 'Mouse' : 'Mouse ON',
      ariaLabel: mouseMode === 'none' ? 'Enable mouse mode for terminal apps (vim, tmux)' : 'Disable mouse mode — enables text selection',
      title: mouseMode === 'none' ? 'Mouse OFF — tap to enable for vim/tmux' : 'Mouse ON — tap to disable, enables selection',
      extraClass: mouseMode === 'any' ? styles.mouseModeActive : '',
      handler: toggleMouseMode,
    },
    {
      key: 'copy',
      icon: '📋',
      label: 'Copy',
      ariaLabel: 'Copy terminal output to clipboard',
      title: 'Copy selected terminal text to clipboard',
      extraClass: '',
      handler: handleCopyOutput,
    },
    {
      key: 'bottom',
      icon: '↓',
      label: 'Bottom',
      ariaLabel: 'Scroll to bottom',
      title: 'Scroll to bottom',
      extraClass: '',
      handler: handleScrollToBottom,
    },
    {
      key: 'resize',
      icon: '↔️',
      label: 'Resize',
      ariaLabel: 'Resize terminal',
      title: 'Resize terminal to fit container',
      extraClass: '',
      handler: handleManualResize,
    },
    {
      key: 'clear',
      icon: '🗑️',
      label: 'Clear',
      ariaLabel: 'Clear terminal',
      title: 'Clear terminal',
      extraClass: '',
      handler: handleClear,
    },
  ];

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
          {/* Toolbar toggle — always visible on mobile; hidden on desktop via CSS */}
          <button
            className={styles.toolbarToggle}
            onClick={() => setToolbarExpanded(v => !v)}
            aria-label="Toggle toolbar"
            aria-expanded={toolbarExpanded}
            data-testid="toolbar-toggle"
          >
            {toolbarExpanded ? '✕' : '⋯'}
          </button>
          {/* Keyboard toggle — always visible so users can find it without expanding toolbar */}
          <button
            className={`${styles.toolbarButton} ${styles.mobileKeyboardToggle}`}
            onClick={toggleMobileKeyboard}
            aria-label={isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard"}
            aria-expanded={isKeyboardVisible}
            title={isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard"}
          >
            ⌨️
          </button>
          {/* Reconnect always visible when needed, regardless of toolbar state */}
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
          {toolbarExpanded && (
            <div className={styles.toolbarActions} data-testid="toolbar-actions">
              <button
                className={`${styles.toolbarButton} ${styles.devOnly} ${debugMode ? styles.debugActive : ''}`}
                onClick={handleToggleDebug}
                title={debugMode ? "Disable debug logging" : "Enable debug logging"}
                aria-label={debugMode ? "Disable debug mode" : "Enable debug mode"}
                style={debugMode ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
              >
                🛠️ {debugMode ? 'Debug ON' : 'Debug'}
              </button>
              <button
                className={`${styles.toolbarButton} ${styles.devOnly} ${logStreamEnabled ? styles.debugActive : ''}`}
                onClick={handleToggleLogStream}
                title={logStreamEnabled ? "Stop forwarding console logs to server" : "Forward console logs to server (Remote Debug)"}
                aria-label={logStreamEnabled ? "Disable remote log streaming" : "Enable remote log streaming"}
                style={logStreamEnabled ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
              >
                📡 {logStreamEnabled ? 'Log Stream ON' : 'Log Stream'}
              </button>
              <button
                className={`${styles.toolbarButton} ${styles.devOnly}`}
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
                className={`${styles.toolbarButton} ${styles.devOnly}`}
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
                onClick={handlePaste}
                title="Paste from clipboard — text is sent directly, images are saved to a temp file and the path is inserted"
                aria-label="Paste from clipboard"
              >
                {pasteError ? `⚠️ ${pasteError}` : '📎 Paste'}
              </button>
              {/* Hidden file input — no capture attribute so iOS shows camera+library+browse */}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                style={{ display: "none" }}
                onChange={handleFileUpload}
                aria-hidden="true"
              />
              <button
                className={styles.toolbarButton}
                onClick={handleImageButtonClick}
                disabled={isUploading}
                title="Upload image from camera or photo library — saves to session directory and inserts path"
                aria-label={isUploading ? "Uploading image..." : "Attach image from camera or gallery"}
              >
                {isUploading ? "⏳ Uploading..." : uploadError ? `⚠️ ${uploadError}` : "📷 Image"}
              </button>
              {/* Secondary actions — inline on desktop, hidden on mobile (shown in overflow row) */}
              <div className={styles.secondaryGroup} data-testid="toolbar-secondary">
                {secondaryActions.map((action) => (
                  <button
                    key={action.key}
                    className={`${styles.toolbarButton}${action.extraClass ? ` ${action.extraClass}` : ''}`}
                    onClick={action.handler}
                    aria-label={action.ariaLabel}
                    title={action.title}
                  >
                    {action.icon} {action.label}
                  </button>
                ))}
              </div>
              {/* More ▾ trigger — only visible on mobile, opens overflow row below toolbar */}
              <button
                className={`${styles.toolbarButton} ${styles.mobileMoreButton} ${mobileOverflowOpen ? styles.mobileMoreActive : ''}`}
                onClick={() => setMobileOverflowOpen(o => !o)}
                aria-label={mobileOverflowOpen ? 'Close more tools' : 'More tools'}
                aria-expanded={mobileOverflowOpen}
                data-testid="toolbar-more-button"
              >
                {mobileOverflowOpen ? '✕ Less' : 'More ▾'}
              </button>
            </div>
          )}
        </div>
      </div>
      {/* Mobile overflow row — appears below toolbar when More is open; hidden on desktop */}
      {mobileOverflowOpen && toolbarExpanded && (
        <div className={styles.mobileOverflowRow} data-testid="toolbar-overflow-row">
          {secondaryActions.map((action) => (
            <button
              key={action.key}
              className={`${styles.toolbarButton}${action.extraClass ? ` ${action.extraClass}` : ''}`}
              onClick={() => { action.handler(); setMobileOverflowOpen(false); }}
              aria-label={action.ariaLabel}
              title={action.title}
            >
              {action.icon} {action.label}
            </button>
          ))}
        </div>
      )}
      <div className={styles.terminal} ref={terminalContainerRef}>
        {isVisible !== false && isLoadingInitialContent && (
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
        {/* Task 4.2.1 — Non-blocking resizing overlay (R1.4).
            Shown while the server waits for tmux quiescence after a window resize.
            pointer-events: none (set in CSS) allows continued interaction. */}
        {terminalState === 'RESIZING' && (
          <div
            className={styles.resizingOverlay}
            role="status"
            aria-label="Terminal resizing"
          >
            <span className={styles.resizingSpinner} />
          </div>
        )}
<XtermTerminal
  ref={xtermRef}
  onData={handleTerminalData}
  onResize={handleTerminalResize}
  theme={theme}
  fontSize={14}
  scrollback={5000}
/>
      </div>
      {/* Mobile keyboard toolbar — Termux-compatible extra-keys layout.
          Row 1: ESC / - HOME ↑ END PGUP
          Row 2: TAB CTRL ALT ← ↓ → PGDN
          Row 3: ^C  ^D  ^Z  ^L  ^R  ^W  ^U  (direct Ctrl sequences, no sticky needed)
          CTRL and ALT are sticky: tap to arm, next key fires the modified sequence. */}
      {isKeyboardVisible && (
        <div className={styles.mobileKeyboard}>
          <div className={styles.mobileKeyRow}>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b'); }} aria-label="Escape" data-testid="mobile-key">Esc</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('/'); }} aria-label="Forward slash" data-testid="mobile-key">/</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('-'); }} aria-label="Hyphen" data-testid="mobile-key">-</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[H'); }} aria-label="Home" data-testid="mobile-key">Home</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[A'); }} aria-label="Up arrow" data-testid="mobile-key">↑</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[F'); }} aria-label="End" data-testid="mobile-key">End</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[5~'); }} aria-label="Page up" data-testid="mobile-key">PgUp</button>
          </div>
          <div className={styles.mobileKeyRow}>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\t'); }} aria-label="Tab" data-testid="mobile-key">Tab</button>
            <button
              className={`${styles.mobileKey} ${ctrlActive ? styles.mobileKeyActive : ''}`}
              onPointerDown={(e) => { e.preventDefault(); setCtrlActive(p => !p); setAltActive(false); }}
              aria-label={ctrlActive ? 'Ctrl active — press next key' : 'Control modifier'}
              aria-pressed={ctrlActive}
              data-testid="mobile-key"
            >
              Ctrl
            </button>
            <button
              className={`${styles.mobileKey} ${altActive ? styles.mobileKeyActive : ''}`}
              onPointerDown={(e) => { e.preventDefault(); setAltActive(p => !p); setCtrlActive(false); }}
              aria-label={altActive ? 'Alt active — press next key' : 'Alt modifier'}
              aria-pressed={altActive}
              data-testid="mobile-key"
            >
              Alt
            </button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[D'); }} aria-label="Left arrow" data-testid="mobile-key">←</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[B'); }} aria-label="Down arrow" data-testid="mobile-key">↓</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[C'); }} aria-label="Right arrow" data-testid="mobile-key">→</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); sendKey('\x1b[6~'); }} aria-label="Page down" data-testid="mobile-key">PgDn</button>
          </div>
          <div className={styles.mobileKeyRow}>
            <button className={`${styles.mobileKey} ${styles.mobileKeyCtrlC}`} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x03'); }} aria-label="Ctrl+C (interrupt)" title="Interrupt (Ctrl+C)" data-testid="mobile-key">^C</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x1a'); }} aria-label="Ctrl+Z (suspend)" title="Suspend (Ctrl+Z)" data-testid="mobile-key">^Z</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x0c'); }} aria-label="Ctrl+L (clear screen)" title="Clear screen (Ctrl+L)" data-testid="mobile-key">^L</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x12'); }} aria-label="Ctrl+R (reverse search)" title="Reverse search (Ctrl+R)" data-testid="mobile-key">^R</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x17'); }} aria-label="Ctrl+W (delete word)" title="Delete word (Ctrl+W)" data-testid="mobile-key">^W</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x15'); }} aria-label="Ctrl+U (delete line)" title="Delete line (Ctrl+U)" data-testid="mobile-key">^U</button>
            <button className={styles.mobileKey} onPointerDown={(e) => { e.preventDefault(); setCtrlActive(false); setAltActive(false); handleTerminalData('\x04'); }} aria-label="Ctrl+D (EOF)" title="EOF / logout (Ctrl+D)" data-testid="mobile-key">^D</button>
          </div>
        </div>
      )}
    </div>
  );
}
