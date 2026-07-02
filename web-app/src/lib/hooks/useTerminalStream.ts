"use client";

import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { TerminalData, TerminalDataSchema, CurrentPaneRequest, CurrentPaneRequestSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { createAuthInterceptor } from "@/lib/config";
import { useEffect, useRef, useState, useCallback } from "react";
import { BackoffState, getWsCloseCode, isRetriableCloseCode } from "@/lib/utils/backoff";
import { MessageQueue } from "@/lib/terminal/MessageQueue";
import { decompressLZMA, isLZMACompressed } from "@/lib/compression/lzma";
import { useTerminalFlowControl } from "./useTerminalFlowControl";
import { useTerminalMetrics } from "./useTerminalMetrics";
import type { Terminal } from '@xterm/xterm';
import { ShellStatus } from "@/gen/session/v1/types_pb";

interface ScrollbackMetadata {
  hasMore: boolean;
  oldestSequence: number;
  newestSequence: number;
  totalLines: number;
}

/**
 * TerminalState — typed state machine for terminal connection and rendering lifecycle (R1.4).
 * Driven by server-side proto messages rather than ad-hoc client-side booleans.
 */
export type TerminalState =
  | 'DISCONNECTED'
  | 'CONNECTING'
  | 'LOADING'
  | 'STABLE'
  | 'RESIZING'
  | 'FETCHING_SCROLLBACK';

interface UseTerminalStreamOptions {
  baseUrl: string;
  sessionId: string;
  /** When set, routes the terminal stream to a custom shell PTY instead of the main Claude session. */
  shellId?: string;
  /** Callback invoked when a ShellStatusUpdate is received for this shell. */
  onShellStatusChange?: (status: "running" | "stopped" | "error", exitCode?: number) => void;
  getTerminal?: () => Terminal | null; // Getter function for terminal instance (evaluated at connect time)
  scrollbackLines?: number; // Number of lines to request from scrollback
  onError?: (error: Error) => void;
  onScrollbackReceived?: (scrollback: string, metadata?: ScrollbackMetadata) => void; // Callback when scrollback is received
  onOutput?: (output: string) => void; // Callback when new output is received (bypass React state)
  autoConnect?: boolean; // If false, requires manual connect() call (default: true)
  initialCols?: number; // Initial terminal columns (prevents size mismatch on first load)
  initialRows?: number; // Initial terminal rows (prevents size mismatch on first load)
  streamingMode?: "raw" | "raw-compressed" | "state" | "hybrid" | "ssp"; // Terminal streaming mode (default: "raw")
  isExternal?: boolean; // Whether this is an external session (uses /ws/external endpoint)
  enablePredictiveEcho?: boolean; // Enable Mosh-style predictive echo (default: false)
  onEchoAck?: (echoNum: bigint, latencyMs: number) => void; // Callback when echo is acknowledged (for RTT stats)
}

interface TerminalStreamResult {
  output: string; // Deprecated: Use onOutput callback for better performance
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  sendInputWithEcho: (input: string) => bigint; // SSP: Send input with predictive echo tracking, returns echo number
  resize: (cols: number, rows: number) => void;
  connect: (cols?: number, rows?: number) => void; // Optional dimensions to override initial values
  disconnect: () => void;
  scrollbackLoaded: boolean; // Indicates if scrollback has been loaded
  requestScrollback: (fromSequence: number, limit: number) => void; // Request historical scrollback
  sendFlowControl: (paused: boolean, watermark?: number) => void; // Send flow control signal to server
  getIsApplyingState: () => boolean; // Check if StateApplicator is currently applying a state (prevents scrollback auto-load)
  sspNegotiated: boolean; // Whether SSP capabilities have been negotiated
  startRecording: () => void; // Start recording WebSocket messages for debugging
  stopRecording: () => void; // Stop recording and download recorded messages
  /** Terminal state machine (R1.4) — typed lifecycle state driven by server messages. */
  terminalState: TerminalState;
  isHardFailed: boolean;
  handleManualReconnect: () => void;
}

export function useTerminalStream({
  baseUrl,
  sessionId,
  shellId,
  onShellStatusChange,
  getTerminal,
  scrollbackLines = 1000,
  onError,
  onScrollbackReceived,
  onOutput,
  autoConnect = true,
  initialCols,
  initialRows,
  streamingMode = "raw",
  enablePredictiveEcho = false,
  onEchoAck,
}: UseTerminalStreamOptions): TerminalStreamResult {
  // ---- Connection state ----
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);
  // Task 4.1.1 — Terminal state machine (R1.4)
  const [terminalState, setTerminalState] = useState<TerminalState>('DISCONNECTED');
  const [isHardFailed, setIsHardFailed] = useState(false);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false);
  const shouldReconnectRef = useRef(false);
  const terminalBackoffRef = useRef(new BackoffState(1000, 30_000));
  const isHardFailedRef = useRef(false);
  const terminalDebounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const connectRef = useRef<(overrideCols?: number, overrideRows?: number) => Promise<void>>(async () => {});
  const textDecoderRef = useRef(new TextDecoder());
  const scrollbackDecoderRef = useRef(new TextDecoder());

  const clientRef = useRef(createClient(
    SessionService,
    createWebsocketBasedTransport({
      baseUrl,
      useBinaryFormat: true,
      interceptors: [createAuthInterceptor()],
    })
  ));

  // Sync ref with state
  useEffect(() => {
    isConnectedRef.current = isConnected;
  }, [isConnected]);

  // ---- Compose sub-hooks ----

  // pushMessageRef bridges the connection's messageQueue to flow control dispatch.
  // Bug Risk 3 mitigation: flow control reads pushMessageRef.current (not a stale closure).
  const pushMessageRef = useRef<((msg: TerminalData) => void) | null>(null);

  // Keep pushMessageRef in sync with current messageQueue
  useEffect(() => {
    pushMessageRef.current = (msg: TerminalData) => {
      messageQueueRef.current?.push(msg);
    };
    return () => { pushMessageRef.current = null; };
  }, []);

  const flowControl = useTerminalFlowControl({
    sessionId,
    streamingMode,
    enablePredictiveEcho,
    getTerminal: getTerminal ?? (() => null),
    pushMessageRef,
    isConnectedRef,
    onError,
    onEchoAck,
  });

  const metrics = useTerminalMetrics({ onOutput });

  // ---- Error helper ----
  const handleError = useCallback((err: unknown) => {
    const error = err instanceof Error ? err : new Error(String(err));
    setError(error);
    onError?.(error);
  }, [onError]);

  // ---- Connect ----
  const connect = useCallback(async (overrideCols?: number, overrideRows?: number) => {
    if (isConnectedRef.current || !sessionId) return;
    shouldReconnectRef.current = true;
    terminalBackoffRef.current.reset();

    let targetCols = overrideCols ?? initialCols;
    let targetRows = overrideRows ?? initialRows;

    if (targetCols === undefined || targetRows === undefined) {
      const currentTerminal = getTerminal?.();
      if (currentTerminal) {
        targetCols = currentTerminal.cols;
        targetRows = currentTerminal.rows;
        console.log(`[useTerminalStream] Using current terminal dimensions: ${targetCols}x${targetRows}`);
      }
    }

    isDisconnectingRef.current = false;
    setTerminalState('CONNECTING');

    try {
      abortControllerRef.current = new AbortController();
      messageQueueRef.current = new MessageQueue();

      // Send initial handshake with dimensions
      const currentPaneReq = create(CurrentPaneRequestSchema, {
        lines: 50,
        includeEscapes: true,
        streamingMode: streamingMode,
      });

      if (targetCols !== undefined && targetRows !== undefined) {
        currentPaneReq.targetCols = targetCols;
        currentPaneReq.targetRows = targetRows;
        console.log(`[useTerminalStream] Sending handshake with dimensions: ${targetCols}x${targetRows}`);
      } else {
        console.warn(`[useTerminalStream] No terminal dimensions available for handshake`);
      }

      messageQueueRef.current.push(
        create(TerminalDataSchema, {
          sessionId,
          shellId: shellId ?? "",
          data: { case: "currentPaneRequest", value: currentPaneReq },
        })
      );

      const stream = clientRef.current.streamTerminal(
        messageQueueRef.current,
        { signal: abortControllerRef.current.signal }
      );

      setError(null);

      // Message processing loop
      (async () => {
        try {
          let firstMessage = true;
          for await (const msg of stream) {
            if (firstMessage) {
              setIsConnected(true);
              setScrollbackLoaded(true);
              setTerminalState('LOADING');
              firstMessage = false;
            }

            // Task 4.1.2 — Handle ResizeQuiescence message (R1.4).
            // Transitions: resizing=true → RESIZING, resizing=false → STABLE.
            if (msg.data.case === "resizeQuiescence") {
              const rq = msg.data.value;
              if (rq.resizing) {
                setTerminalState('RESIZING');
              } else {
                setTerminalState('STABLE');
              }
              continue; // No further processing for quiescence messages
            }

            // Handle ShellStatusUpdate for custom shell PTY streams.
            // Only fire when msg.shellId matches our shellId (guards against stray updates).
            if (msg.data.case === "shellStatusUpdate") {
              const update = msg.data.value;
              if (onShellStatusChange && update.shellId === shellId) {
                let statusStr: "running" | "stopped" | "error" = "running";
                if (update.newStatus === ShellStatus.STOPPED) statusStr = "stopped";
                else if (update.newStatus === ShellStatus.ERROR) statusStr = "error";
                onShellStatusChange(statusStr, update.exitCode);
              }
              continue;
            }

            // Dispatch to sub-hooks based on message type
            if (msg.data.case === "state") {
              flowControl.handleStateMessage(msg.data.value);
            } else if (msg.data.case === "diff") {
              flowControl.handleDiffMessage(msg.data.value);
            } else if (msg.data.case === "sspNegotiation") {
              flowControl.handleSspNegotiation(msg.data.value);
            } else if (msg.data.case === "output") {
              // Handle raw output (may be compressed)
              const rawData = msg.data.value.data;

              let decodedData: Uint8Array;
              if (streamingMode === "raw-compressed" && isLZMACompressed(rawData)) {
                try {
                  decodedData = await decompressLZMA(rawData);
                  if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
                    console.debug(`[useTerminalStream] Decompressed output: ${rawData.length} -> ${decodedData.length} bytes`);
                  }
                } catch (err) {
                  console.error(`[useTerminalStream] LZMA decompression failed, using raw data:`, err);
                  decodedData = rawData;
                }
              } else {
                decodedData = rawData;
              }

              const text = textDecoderRef.current.decode(decodedData, { stream: true });

              // Record message if recording is active
              metrics.recordMessage({
                timestamp: Date.now(),
                type: 'raw',
                data: decodedData,
                decoded: text,
              });

              if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
                console.debug(`[useTerminalStream] Received output: ${text.length} bytes`);
              }

              // Use callback if provided, otherwise batch via RAF
              if (onOutput) {
                onOutput(text);
              } else {
                metrics.scheduleOutputUpdate(text);
              }
              // First raw output → terminal is stable (not resizing)
              setTerminalState((prev) => prev === 'LOADING' || prev === 'CONNECTING' ? 'STABLE' : prev);
            } else if (msg.data.case === "currentPaneResponse") {
              flowControl.handleCurrentPaneResponse(msg.data.value);

              // Write deprecated pane content via scrollback callback.
              // Use a fresh one-shot TextDecoder — currentPaneResponse is a single complete message,
              // not a streaming sequence, so it must not share stateful decoder state with other messages.
              const response = msg.data.value;
              const content = new TextDecoder().decode(response.content);
              console.log(`[useTerminalStream] Received current pane (deprecated): ${content.length} bytes`);

              if (onScrollbackReceived) {
                onScrollbackReceived(content);
              }
              setTerminalState('STABLE');
            } else if (msg.data.case === "scrollbackResponse") {
              // Use a per-response decoder so chunks within one scrollbackResponse are streamed
              // correctly, but separate responses don't share stateful decoder state.
              const responseDecoder = new TextDecoder();
              const chunks: string[] = [];
              for (const chunk of msg.data.value.chunks) {
                const text = responseDecoder.decode(chunk.data, { stream: true });
                chunks.push(text);
              }
              // Flush any trailing bytes buffered by the streaming decoder.
              const trailing = responseDecoder.decode();
              if (trailing) chunks.push(trailing);
              const scrollbackText = chunks.join("");

              const metadata: ScrollbackMetadata = {
                hasMore: msg.data.value.hasMore,
                oldestSequence: Number(msg.data.value.oldestSequence),
                newestSequence: Number(msg.data.value.newestSequence),
                totalLines: Number(msg.data.value.totalLines),
              };

              console.log(`[useTerminalStream] Scrollback metadata:`, metadata);

              if (onScrollbackReceived) {
                onScrollbackReceived(scrollbackText, metadata);
              }
              // Scrollback response received — return to STABLE
              setTerminalState((prev) => prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
            } else if (msg.data.case === "error") {
              const err = new Error(msg.data.value.message);
              setError(err);
              onError?.(err);
            }
          }
        } catch (err) {
          const wsCode = getWsCloseCode(err);
          if (wsCode !== null && !isRetriableCloseCode(wsCode)) {
            shouldReconnectRef.current = false;
            isHardFailedRef.current = true;
            setIsHardFailed(true);
            console.warn(`[reconnect] stream=terminal non-retriable ws-close-code=${wsCode}, giving up`);
          }
          handleError(err);
        } finally {
          isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
          setIsConnected(false);
          setTerminalState('DISCONNECTED');
          // Reset decoders so stale {stream:true} buffered state from a server-closed
          // connection does not corrupt the next connect() call.
          textDecoderRef.current = new TextDecoder();
          scrollbackDecoderRef.current = new TextDecoder();
          if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"
              && shouldReconnectRef.current
              && !isDisconnectingRef.current) {
            if (terminalBackoffRef.current.attempt >= 5) {
              shouldReconnectRef.current = false;
              isHardFailedRef.current = true;
              setIsHardFailed(true);
            } else {
              const delay = terminalBackoffRef.current.next();
              console.info(`[reconnect] stream=terminal trigger=close attempt=${terminalBackoffRef.current.attempt} delay=${delay}ms`);
              if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
              }
              reconnectTimerRef.current = setTimeout(() => {
                reconnectTimerRef.current = null;
                if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                  connectRef.current?.();
                }
              }, delay);
            }
          }
        }
      })();
    } catch (err) {
      handleError(err);
      setIsConnected(false);
    }
  }, [sessionId, shellId, onShellStatusChange, getTerminal, onError, onScrollbackReceived, onOutput,
      streamingMode, flowControl, metrics, handleError, initialCols, initialRows]);

  // Keep connectRef in sync so visibility/online listeners always call the current closure
  connectRef.current = connect;

  // ---- Disconnect ----
  // Use stable method reference to avoid disconnect being recreated on every render.
  // flowControl returns a new object literal each render, but getIsResyncingRef is a
  // stable useCallback(() => isResyncingRef, []) so depending on it keeps disconnect stable.
  const getIsResyncingRef = flowControl.getIsResyncingRef;
  const disconnect = useCallback(async () => {
    shouldReconnectRef.current = false;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    const isResyncingRef = getIsResyncingRef();
    if (isDisconnectingRef.current || isResyncingRef.current) {
      if (isResyncingRef.current) {
        console.log("[useTerminalStream] Delaying disconnect - resync in progress");
        setTimeout(() => disconnect(), 500);
      }
      return;
    }
    isDisconnectingRef.current = true;

    if (messageQueueRef.current) {
      messageQueueRef.current.close();
      messageQueueRef.current = null;
    }

    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        if (abortControllerRef.current) {
          console.debug("[useTerminalStream] Timeout waiting for graceful close, forcing abort");
          abortControllerRef.current.abort();
          abortControllerRef.current = null;
        }
        resolve();
      }, 1000);

      if (!isConnectedRef.current) {
        clearTimeout(timeout);
        resolve();
        return;
      }
    });

    setIsConnected(false);
    isDisconnectingRef.current = false;
    textDecoderRef.current = new TextDecoder();
    scrollbackDecoderRef.current = new TextDecoder();
  }, [getIsResyncingRef]);

  // ---- Auto-connect / cleanup ----
  useEffect(() => {
    if (autoConnect) {
      connect();
    }
    return () => {
      shouldReconnectRef.current = false;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      metrics.flushOutputBuffer();
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, autoConnect]);

  // Story 3.1.3 — reconnect on tab visibility or network online event
  useEffect(() => {
    if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;

    const handleVisibilityOrOnline = (ev: Event) => {
      if (document.visibilityState !== "visible" && ev.type !== "online") return;
      if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
      terminalDebounceTimerRef.current = setTimeout(() => {
        terminalDebounceTimerRef.current = null;
        if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
          terminalBackoffRef.current.reset();
          console.info("[reconnect] stream=terminal trigger=visibility delay=0ms");
          connectRef.current();
        }
      }, 200);
    };

    document.addEventListener("visibilitychange", handleVisibilityOrOnline);
    window.addEventListener("online", handleVisibilityOrOnline);

    return () => {
      if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
      document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
      window.removeEventListener("online", handleVisibilityOrOnline);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Story 3.1.2 — manual reconnect after hard failure
  const handleManualReconnect = useCallback(() => {
    isHardFailedRef.current = false;
    setIsHardFailed(false);
    shouldReconnectRef.current = true;
    terminalBackoffRef.current.reset();
    connectRef.current();
  }, []);

  return {
    output: metrics.output,
    isConnected,
    error,
    sendInput: flowControl.sendInput,
    sendInputWithEcho: flowControl.sendInputWithEcho,
    resize: flowControl.resize,
    connect,
    disconnect,
    scrollbackLoaded,
    requestScrollback: flowControl.requestScrollback,
    sendFlowControl: flowControl.sendFlowControl,
    getIsApplyingState: flowControl.getIsApplyingState,
    sspNegotiated: flowControl.sspNegotiated,
    startRecording: metrics.startRecording,
    stopRecording: metrics.stopRecording,
    terminalState,
    isHardFailed,
    handleManualReconnect,
  };
}
