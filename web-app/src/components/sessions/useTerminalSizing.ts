"use client";

/**
 * Size-stability and initial-connection logic for TerminalOutput.
 *
 * The refs are owned by the component (not this module) because useTerminalStream
 * is called with `lastResizeRef.current` as an input while its outputs
 * (`connect`, `isConnected`, `resize`) are what this logic consumes. Passing
 * them in keeps that cycle out of the hooks without changing useTerminalStream.
 */

import { useCallback, useEffect } from "react";
import type { RefObject } from "react";
import type { XtermTerminalHandle } from "@/components/sessions/XtermTerminal";
import type { TerminalStreamManager } from "@/lib/terminal/TerminalStreamManager";
import {
  getCachedDimensions,
  saveDimensions,
  validateCellDimensions,
  MIN_COLS,
  MIN_ROWS,
  XTERM_DEFAULT_COLS,
  XTERM_DEFAULT_ROWS,
} from "@/lib/terminal/TerminalDimensionCache";
import { computePreSize, readRenderedCellMetrics } from "@/lib/terminal/preSizing";
import { loadTerminalConfig } from "@/lib/config/terminalConfig";

export interface TerminalLoadMetrics {
  mountTime: number;
  firstResizeTime: number | null;
  sizeStableTime: number | null;
  connectionInitTime: number | null;
  connectedTime: number | null;
  firstOutputTime: number | null;
  resizeCount: number;
}

export interface TerminalSizingRefs {
  lastResizeRef: RefObject<{ cols: number; rows: number } | null>;
  hasInitiatedConnectionRef: RefObject<boolean>;
  hasCachedDimensionsRef: RefObject<boolean>;
  sizeStabilityTimeoutRef: RefObject<ReturnType<typeof setTimeout> | null>;
  pendingConnectAfterDisconnectRef: RefObject<boolean>;
  isMountedRef: RefObject<boolean>;
  metricsRef: RefObject<TerminalLoadMetrics>;
}

type Connect = (cols?: number, rows?: number) => void;

interface ResizeHandlerParams {
  sessionId: string;
  xtermRef: RefObject<XtermTerminalHandle | null>;
  isConnected: boolean;
  error: unknown;
  connect: Connect;
  resize: (cols: number, rows: number, force?: boolean) => void;
  clearBufferBeforeResize: () => void;
  setIsWaitingForStableSize: (waiting: boolean) => void;
  refs: TerminalSizingRefs;
}

/** Handles xterm's onResize: caches dimensions, drives size-stability connect, forwards resizes. */
export function useTerminalResizeHandler({
  sessionId,
  xtermRef,
  isConnected,
  error,
  connect,
  resize,
  clearBufferBeforeResize,
  setIsWaitingForStableSize,
  refs,
}: ResizeHandlerParams) {
  const {
    lastResizeRef,
    hasInitiatedConnectionRef,
    hasCachedDimensionsRef,
    sizeStabilityTimeoutRef,
    isMountedRef,
    metricsRef,
  } = refs;

  return useCallback((cols: number, rows: number) => {
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
        const terminal = xtermRef.current?.terminal;
        saveDimensions(sessionId, cols, rows, {
          ...readRenderedCellMetrics(terminal),
          fontSize: terminal?.options?.fontSize ?? 14,
          fontFamily: terminal?.options?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace',
        });
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
    clearBufferBeforeResize();
    resize(cols, rows);
  }, [isConnected, resize, connect, error, sessionId, clearBufferBeforeResize, xtermRef]);
}

interface ConnectionBootstrapParams {
  sessionId: string;
  containerWidth: number;
  terminalContainerRef: RefObject<HTMLDivElement | null>;
  xtermRef: RefObject<XtermTerminalHandle | null>;
  isConnected: boolean;
  connect: Connect;
  setIsWaitingForStableSize: (waiting: boolean) => void;
  setIsLoadingInitialContent: (loading: boolean) => void;
  setConnectionAttempts: (attempts: number) => void;
  setShowReconnectButton: (show: boolean) => void;
  reconnectTimeoutRef: RefObject<ReturnType<typeof setTimeout> | null>;
  previousConnectionStateRef: RefObject<boolean>;
  streamManagerRef: RefObject<TerminalStreamManager | null>;
  refs: TerminalSizingRefs;
}

/**
 * The three effects that start a connection for a session, in a fixed order:
 * pre-size from cache, session-switch connect, then post-disconnect connect.
 */
export function useSessionConnectionBootstrap({
  sessionId,
  containerWidth,
  terminalContainerRef,
  xtermRef,
  isConnected,
  connect,
  setIsWaitingForStableSize,
  setIsLoadingInitialContent,
  setConnectionAttempts,
  setShowReconnectButton,
  reconnectTimeoutRef,
  previousConnectionStateRef,
  streamManagerRef,
  refs,
}: ConnectionBootstrapParams): void {
  const {
    lastResizeRef,
    hasInitiatedConnectionRef,
    hasCachedDimensionsRef,
    pendingConnectAfterDisconnectRef,
    isMountedRef,
    metricsRef,
  } = refs;

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
    // loadTerminalConfig() is what the pooled XtermTerminal renders with (useConfig), so
    // validating against it keeps cached cell dims in sync with the actual font (Bug 4 fix).
    const { fontSize: currentFontSize, fontFamily: currentFontFamily } = loadTerminalConfig();
    const cached = rawCached
      ? validateCellDimensions(rawCached, currentFontSize, currentFontFamily)
      : null;
    if (cached && cached.cols >= MIN_COLS && cached.rows >= MIN_ROWS) {
      hasCachedDimensionsRef.current = true;
      console.log(`[TerminalOutput] Initialized with cached dimensions: ${cached.cols}x${cached.rows}`);

      if (cached.cellWidth && cached.cellHeight && terminalContainerRef.current) {
        // Guard: only use getBoundingClientRect after the ResizeObserver has confirmed
        // the container has a real constrained width (not the full pre-layout browser width).
        // containerWidth > 0 means layout is complete and the pane has its actual dimensions.
        if (containerWidth > 0) {
          const rect = terminalContainerRef.current.getBoundingClientRect();
          const preSize = computePreSize(rect, { cellWidth: cached.cellWidth, cellHeight: cached.cellHeight });
          if (preSize.ok) {
            const { cols: preCols, rows: preRows } = preSize;
            console.log(
              `[TerminalOutput] Pre-sizing: ${rect.width}×${rect.height}px / ` +
              `${cached.cellWidth.toFixed(2)}×${cached.cellHeight.toFixed(2)}px/cell → ${preCols}×${preRows}`
            );
            lastResizeRef.current = { cols: preCols, rows: preRows };
            // If this effect fired because containerWidth became non-zero (i.e. ResizeObserver
            // fired after the initial render), the session-switch effect won't re-run because
            // sessionId didn't change. We must initiate the connection here in that case.
            // On a sessionId-triggered render, session-switch runs after this and connects,
            // so we only act when no connection has been initiated yet.
            const isXtermDefault = preCols === XTERM_DEFAULT_COLS && preRows === XTERM_DEFAULT_ROWS;
            if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current && !isXtermDefault) {
              hasInitiatedConnectionRef.current = true;
              setIsWaitingForStableSize(false);
              // Grow the xterm buffer to preCols/preRows BEFORE connecting — otherwise the
              // terminal is still at its 80x24 constructor default and the capture-pane
              // snapshot's cursor-positioning sequences for rows beyond 24 are silently
              // dropped, leaving them unpainted until a later resize forces a full repaint.
              xtermRef.current?.resize(preCols, preRows);
              connect(preCols, preRows);
            }
          } else if (preSize.reason === "below-minimum") {
            console.log(`[TerminalOutput] Pre-sizing skipped: calculated ${preSize.cols}x${preSize.rows} below minimum`);
          } else {
            console.log(`[TerminalOutput] Pre-sizing skipped: container has zero size`);
          }
        } else {
          console.log(`[TerminalOutput] Pre-sizing deferred: container layout not yet resolved (containerSize.width=0)`);
        }
      }
    } else if (cached) {
      console.log(`[TerminalOutput] Ignoring stale cached dimensions ${cached.cols}x${cached.rows} (below ${MIN_COLS}x${MIN_ROWS})`);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, containerWidth]);

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
}
