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

interface ScrollbackMetadata {
  hasMore: boolean;
  oldestSequence: number;
  newestSequence: number;
  totalLines: number;
}

interface UseTerminalStreamOptions {
  baseUrl: string;
  sessionId: string;
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
}

export function useTerminalStream({
  baseUrl,
  sessionId,
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

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false);
  const textDecoderRef = useRef(new TextDecoder());

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
              firstMessage = false;
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
            } else if (msg.data.case === "currentPaneResponse") {
              flowControl.handleCurrentPaneResponse(msg.data.value);

              // Write deprecated pane content via scrollback callback
              const response = msg.data.value;
              const content = textDecoderRef.current.decode(response.content);
              console.log(`[useTerminalStream] Received current pane (deprecated): ${content.length} bytes`);

              if (onScrollbackReceived) {
                onScrollbackReceived(content);
              }
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
            } else if (msg.data.case === "error") {
              const err = new Error(msg.data.value.message);
              setError(err);
              onError?.(err);
            }
          }
        } catch (err) {
          handleError(err);
        } finally {
          setIsConnected(false);
        }
      })();
    } catch (err) {
      handleError(err);
      setIsConnected(false);
    }
  }, [sessionId, getTerminal, scrollbackLines, onError, onScrollbackReceived, onOutput,
      streamingMode, flowControl, metrics, handleError]);

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
  }, [getIsResyncingRef]);

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
  };
}
