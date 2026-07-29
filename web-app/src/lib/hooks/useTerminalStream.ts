"use client";

import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { TerminalData, TerminalDataSchema, CurrentPaneRequest, CurrentPaneRequestSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { createAuthInterceptor } from "@/lib/config";
import { useEffect, useRef, useState, useCallback } from "react";
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
  /** Called with the number of buffered-but-undelivered messages dropped when a MessageQueue is torn down (superseded connect() or disconnect()). */
  onInputDropped?: (count: number) => void;
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
  onInputDropped,
}: UseTerminalStreamOptions): TerminalStreamResult {
  // ---- Connection state ----
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);
  // Task 4.1.1 — Terminal state machine (R1.4)
  const [terminalState, setTerminalState] = useState<TerminalState>('DISCONNECTED');

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false);
  const textDecoderRef = useRef(new TextDecoder());
  // Task 2.2.1 — Connection-generation fence (mirrors usePathCompletions.ts's
  // generationRef idiom). Bumped once per connect() call; a message-processing
  // loop whose captured generation no longer matches the current value treats
  // itself as superseded and stops mutating shared state.
  const connectionGenerationRef = useRef(0);

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

    // Task 2.2.1 — bump the connection generation immediately so this call's
    // message-processing loop (started below) can identify itself as "the
    // current attempt" and detect being superseded by a later connect().
    const myGeneration = ++connectionGenerationRef.current;

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
      // Task 2.2.2 — unconditionally tear down whatever generation this call
      // is about to replace, regardless of connection state. This removes the
      // previous isConnectedRef-gated skip (the root of the double-live-
      // connection risk documented in architecture.md §1) by making connect()
      // itself always close/abort what it's about to replace.
      if (messageQueueRef.current) {
        const dropped = messageQueueRef.current.close();
        if (dropped > 0) {
          onInputDropped?.(dropped);
        }
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

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
            // Task 2.2.3 — a superseded generation's loop must not mutate
            // shared state (or, transitively, deliver buffered input to the
            // wrong connection). Mirrors usePathCompletions.ts's
            // `if (generation !== generationRef.current) return;` guard.
            if (myGeneration !== connectionGenerationRef.current) {
              console.warn(`[useTerminalStream] Discarding message from superseded connection generation ${myGeneration} (current: ${connectionGenerationRef.current})`);
              break;
            }

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

              // Write deprecated pane content via scrollback callback
              const response = msg.data.value;
              const content = textDecoderRef.current.decode(response.content);
              console.log(`[useTerminalStream] Received current pane (deprecated): ${content.length} bytes`);

              if (onScrollbackReceived) {
                onScrollbackReceived(content);
              }
              setTerminalState('STABLE');
            } else if (msg.data.case === "scrollbackResponse") {
              const chunks: string[] = [];
              for (const chunk of msg.data.value.chunks) {
                const text = textDecoderRef.current.decode(chunk.data);
                chunks.push(text);
              }
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
          if (myGeneration === connectionGenerationRef.current) {
            handleError(err);
          }
        } finally {
          // Task 2.2.3 — a superseded generation's teardown must not stomp
          // the newer generation's state.
          if (myGeneration === connectionGenerationRef.current) {
            setIsConnected(false);
            setTerminalState('DISCONNECTED');
          }
        }
      })();
    } catch (err) {
      handleError(err);
      setIsConnected(false);
    }
  }, [sessionId, shellId, onShellStatusChange, getTerminal, onError, onScrollbackReceived, onOutput,
      streamingMode, flowControl, metrics, handleError, initialCols, initialRows, onInputDropped]);

  // ---- Disconnect ----
  // Use stable method reference to avoid disconnect being recreated on every render.
  // flowControl returns a new object literal each render, but getIsResyncingRef is a
  // stable useCallback(() => isResyncingRef, []) so depending on it keeps disconnect stable.
  const getIsResyncingRef = flowControl.getIsResyncingRef;
  const disconnect = useCallback(async () => {
    const isResyncingRef = getIsResyncingRef();
    if (isDisconnectingRef.current || isResyncingRef.current) {
      if (isResyncingRef.current) {
        console.log("[useTerminalStream] Delaying disconnect - resync in progress");
        setTimeout(() => disconnect(), 500);
      }
      return;
    }
    isDisconnectingRef.current = true;
    // Captured so the delayed callback below can tell whether a newer connect()
    // has since taken over before it mutates shared abortControllerRef/isConnected
    // state — otherwise a stale disconnect() racing a fresh connect() can abort
    // or clobber the newer generation's connection (see connection-generation
    // guard on the read side in connect(), Story 2.2).
    const myGeneration = connectionGenerationRef.current;

    if (messageQueueRef.current) {
      const dropped = messageQueueRef.current.close();
      if (dropped > 0) {
        onInputDropped?.(dropped);
      }
      messageQueueRef.current = null;
    }

    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        if (myGeneration === connectionGenerationRef.current && abortControllerRef.current) {
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

    if (myGeneration === connectionGenerationRef.current) {
      setIsConnected(false);
    }
    isDisconnectingRef.current = false;
  }, [getIsResyncingRef, onInputDropped]);

  // ---- Auto-connect / cleanup ----
  useEffect(() => {
    if (autoConnect) {
      connect();
    }
    return () => {
      metrics.flushOutputBuffer();
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, autoConnect]);

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
  };
}
