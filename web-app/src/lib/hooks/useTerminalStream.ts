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

/**
 * A single reported occurrence of dropped-but-unsent terminal input (Task
 * 4.1.1.1). This hook reports one occurrence per call site; it does not
 * accumulate a running total across *distinct* occurrences — that
 * cross-render coalescing is `InputDropBadge`'s job (Epic 4.2).
 */
export type DroppedInputEvent = { count: number; at: number };

/**
 * Smart constructor for a drop report. Returns null for count <= 0 so a
 * domain-illegal "0 keystrokes not sent" event can never be constructed.
 */
function recordDrop(count: number): DroppedInputEvent | null {
  return count > 0 ? { count, at: Date.now() } : null;
}

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
  isExternal?: boolean; // Whether this is an external session (uses /ws/external endpoint)
}

interface TerminalStreamResult {
  output: string; // Deprecated: Use onOutput callback for better performance
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number, force?: boolean) => void;
  connect: (cols?: number, rows?: number) => Promise<void>; // Optional dimensions to override initial values
  disconnect: () => Promise<void>;
  scrollbackLoaded: boolean; // Indicates if scrollback has been loaded
  requestScrollback: (fromSequence: number, limit: number) => void; // Request historical scrollback
  sendFlowControl: (paused: boolean, watermark?: number) => void; // Send flow control signal to server
  startRecording: () => void; // Start recording WebSocket messages for debugging
  stopRecording: () => void; // Stop recording and download recorded messages
  /** Terminal state machine (R1.4) — typed lifecycle state driven by server messages. */
  terminalState: TerminalState;
  isHardFailed: boolean;
  handleManualReconnect: () => void;
  requestFullResync: (urgent?: boolean) => void;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  /** Task 4.1.1.1 — most recent single drop occurrence, or null. */
  droppedInputEvent: DroppedInputEvent | null;
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
}: UseTerminalStreamOptions): TerminalStreamResult {
  // ---- Connection state ----
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);
  // Task 4.1.1 — Terminal state machine (R1.4)
  const [terminalState, setTerminalState] = useState<TerminalState>('DISCONNECTED');
  const [isHardFailed, setIsHardFailed] = useState(false);
  // Task 4.1.1.1 — surfaces a single drop occurrence at a time; InputDropBadge
  // (Epic 4.2) owns turning a sequence of these into a running total.
  const [droppedInputEvent, setDroppedInputEvent] = useState<DroppedInputEvent | null>(null);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false);
  // Guards against two independent visibility/focus-triggered reconnect paths
  // (this hook's own Story 3.1.3 listener and useVisibilityResync, composed
  // together in TerminalOutput.tsx) both calling connect() for the same
  // disconnected session before either handshake completes.
  const isConnectingRef = useRef(false);
  // Monotonically-increasing connection epoch; incremented exactly once per
  // connect() call that actually proceeds past the entry guard (never before
  // it — see ADR-001), and checked at every await checkpoint (firstMessage,
  // catch, finally) so a superseded attempt's late-arriving continuation
  // cannot mutate state out from under a newer attempt. disconnect() reads
  // this counter too (never increments it) to detect whether a newer
  // connect() has completed while it was awaiting.
  const connectionEpochRef = useRef(0);
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

  // ---- Drop reporting (Task 4.1.1.1/4.1.1.3) ----

  // Same-React-batch merge guard (pre-mortem.md Failure #3, P2): the three
  // drop call sites below (reconnect-close, disconnect()'s close, and
  // useTerminalFlowControl's onDrop) can fire within the same synchronous
  // React 18 batch (e.g. a reconnect's queue-close and a rejected keystroke
  // landing in the same tick). Two setState calls to the same state in one
  // batch mean only the *last* value survives to the next render — a naive
  // `setDroppedInputEvent(dropEvent)` at each call site would silently drop
  // the earlier occurrence's count. dropBatchRef accumulates same-batch
  // occurrences; the effect below resets it once the batch's single
  // resulting render has committed, so the *next* distinct occurrence starts
  // its own count rather than accumulating onto a stale one.
  const dropBatchRef = useRef<DroppedInputEvent | null>(null);

  const reportDrop = useCallback((count: number) => {
    const event = recordDrop(count);
    if (!event) return;
    const merged = dropBatchRef.current
      ? { count: dropBatchRef.current.count + event.count, at: event.at }
      : event;
    dropBatchRef.current = merged;
    setDroppedInputEvent(merged);
  }, []);

  // Shared by connect()'s reconnect path and disconnect(): both close the
  // current message queue and report any buffered-but-unsent input as a
  // drop, differing only in the log line's reason label.
  const closeQueueAndReportDrop = useCallback((reason: "reconnect" | "disconnect") => {
    const droppedCount = messageQueueRef.current?.close() ?? 0;
    if (droppedCount > 0) {
      reportDrop(droppedCount);
      console.warn(`[useTerminalStream] dropped ${droppedCount} buffered input message(s) on ${reason}`, { sessionId });
    }
    return droppedCount;
  }, [reportDrop, sessionId]);

  useEffect(() => {
    // Runs after the batch that produced this droppedInputEvent has
    // committed — safe to reset here because any same-batch reportDrop()
    // calls have already run and merged synchronously before this effect
    // fires. Skips the `droppedInputEvent === null` mount case (nothing to
    // reset).
    if (droppedInputEvent !== null) {
      dropBatchRef.current = null;
    }
  }, [droppedInputEvent]);

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

  // Task 4.1.1.2 — sendInput's silent drop (keystroke rejected because the
  // hook already knows it's disconnected) is the same class of input loss
  // as a superseded queue's drop-on-close; funnel it through the same
  // reportDrop signal, one reported occurrence per rejected sendInput call.
  // Memoized once (reportDrop is a stable [] useCallback) so this handler's
  // identity never changes across renders — useTerminalFlowControl's
  // sendInput includes `onDrop` in its dependency array, so an unstable
  // inline arrow here would cascade a new sendInput identity through
  // TerminalOutput.tsx's handleTerminalData and the xterm onData wiring on
  // every render.
  const handleFlowControlDrop = useCallback(() => reportDrop(1), [reportDrop]);

  const flowControl = useTerminalFlowControl({
    sessionId,
    getTerminal: getTerminal ?? (() => null),
    pushMessageRef,
    isConnectedRef,
    onError,
    onDrop: handleFlowControlDrop,
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
    if (isConnectedRef.current || isConnectingRef.current || !sessionId) return;
    const epoch = ++connectionEpochRef.current;
    isConnectingRef.current = true;
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

      // Unconditionally close any previous queue before installing a new one —
      // collapses the disconnect-path (which already called close()) vs.
      // implicit-reconnect-path (previously did not) asymmetry. A superseded
      // attempt's buffered-but-unsent input must be dropped, not carried
      // forward into the new queue.
      closeQueueAndReportDrop("reconnect");
      messageQueueRef.current = new MessageQueue();

      // Send initial handshake with dimensions
      const currentPaneReq = create(CurrentPaneRequestSchema, {
        lines: 50,
        includeEscapes: true,
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
            if (epoch !== connectionEpochRef.current) return;
            if (firstMessage) {
              isConnectingRef.current = false;
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

            if (msg.data.case === "output") {
              // Handle raw output
              const decodedData = msg.data.value.data;
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
          if (epoch !== connectionEpochRef.current) return;
          const wsCode = getWsCloseCode(err);
          if (wsCode !== null && !isRetriableCloseCode(wsCode)) {
            shouldReconnectRef.current = false;
            isHardFailedRef.current = true;
            setIsHardFailed(true);
            console.warn(`[reconnect] stream=terminal non-retriable ws-close-code=${wsCode}, giving up`);
          }
          handleError(err);
        } finally {
          // Resource cleanup always runs regardless of epoch — decoder resets
          // must happen even for a superseded generation so its stale buffered
          // decode state can never bleed into whichever generation is current.
          textDecoderRef.current = new TextDecoder();
          scrollbackDecoderRef.current = new TextDecoder();

          if (epoch === connectionEpochRef.current) {
            isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
            isConnectingRef.current = false;
            setIsConnected(false);
            setTerminalState('DISCONNECTED');
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
        }
      })();
    } catch (err) {
      isConnectingRef.current = false;
      handleError(err);
      setIsConnected(false);
    }
  }, [sessionId, shellId, onShellStatusChange, getTerminal, onError, onScrollbackReceived, onOutput,
      flowControl, metrics, handleError, initialCols, initialRows, closeQueueAndReportDrop]);

  // Keep connectRef in sync so visibility/online listeners always call the current closure
  connectRef.current = connect;

  // ---- Disconnect ----
  // Use stable method reference to avoid disconnect being recreated on every render.
  // flowControl returns a new object literal each render, but getIsResyncingRef is a
  // stable useCallback(() => isResyncingRef, []) so depending on it keeps disconnect stable.
  const getIsResyncingRef = flowControl.getIsResyncingRef;
  const disconnect = useCallback(async () => {
    // Read-only capture — disconnect() isn't starting a new attempt, it's
    // checking whether one has started since. Captured before the
    // isResyncingRef/isDisconnectingRef early-return so an early-returning
    // call still captures a value (mirrors connect()'s increment-before-guard
    // placement discipline). See ADR-001 addendum.
    const epochAtDisconnectStart = connectionEpochRef.current;
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
      // An explicit user-initiated disconnect with pending input must fire
      // the same drop signal as a superseded reconnect's close (Task 4.1.1.1).
      closeQueueAndReportDrop("disconnect");
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

    isDisconnectingRef.current = false; // bookkeeping — always reset regardless of epoch

    if (epochAtDisconnectStart === connectionEpochRef.current) {
      setIsConnected(false);
      textDecoderRef.current = new TextDecoder();
      scrollbackDecoderRef.current = new TextDecoder();
    }
  }, [getIsResyncingRef, closeQueueAndReportDrop]);

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
    resize: flowControl.resize,
    connect,
    disconnect,
    scrollbackLoaded,
    requestScrollback: flowControl.requestScrollback,
    sendFlowControl: flowControl.sendFlowControl,
    startRecording: metrics.startRecording,
    stopRecording: metrics.stopRecording,
    terminalState,
    isHardFailed,
    handleManualReconnect,
    requestFullResync: flowControl.requestFullResync,
    markResyncComplete: flowControl.markResyncComplete,
    markPaneResponseReceived: flowControl.markPaneResponseReceived,
    droppedInputEvent,
  };
}
