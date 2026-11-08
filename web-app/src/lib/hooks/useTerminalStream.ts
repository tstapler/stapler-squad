"use client";

import { terminalInputTimer } from "@/lib/telemetry/terminalInputTiming";
import type { VisibilityResyncDecision } from "@/lib/terminal/resyncPolicy";
import { createClient, ConnectError } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { TerminalData, ScrollForwardOutcome, ScrollBlockedReason } from "@/gen/session/v1/events_pb";
import { TerminalDataSchema, CurrentPaneRequest, CurrentPaneRequestSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { createAuthInterceptor } from "@/lib/config";
import { useEffect, useRef, useState, useCallback } from "react";
import {
  BackoffState, CONNECT_TIMEOUT_ABORT_CAP, GAVE_UP_AFTER_ATTEMPTS, RECONNECT_BASE_MS, RECONNECT_CAP_MS,
  SERVER_END_GIVE_UP_MS, STREAM_REFUSED_CLOSE_CODE, connectTimeoutAbortDelay, connectTimeoutMs, getWsCloseCode, isNonRetriableConnectError,
  isWorktreeMissingError, jitteredDelay,
} from "@/lib/utils/backoff";
import { queueForConnectSlot, tryAcquireConnectSlot } from "@/lib/utils/connectGate";
import { MessageQueue } from "@/lib/terminal/MessageQueue";
import { useTerminalFlowControl, type ResizeOptions } from "./useTerminalFlowControl";
import { useTerminalMetrics } from "./useTerminalMetrics";
import type { Terminal } from '@xterm/xterm';
import { ShellStatus } from "@/gen/session/v1/types_pb";
import { isUsableTerminalSize } from "@/lib/terminal/handshakeDims";

/** A stream that lived this long counts as healthy and resets the clean-end streak. */
const STABLE_STREAM_MS = 10_000;
const MAX_CLEAN_SERVER_ENDS = 5;

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
  /** Hidden-session view: no Input or Resize frames are sent (Story 5.3). */
  readOnly?: boolean;
  scrollbackLines?: number; // Number of lines to request from scrollback
  onError?: (error: Error) => void;
  onScrollbackReceived?: (scrollback: string, metadata?: ScrollbackMetadata) => void; // Callback when scrollback is received
  /**
   * Callback when new output is received (bypass React state). `resyncId`
   * echoes CurrentPaneRequest.resync_id (Epic 3.1, AC2) when this output is
   * the reply to a correlation-ID-tagged resync request; empty/undefined
   * otherwise.
   */
  onOutput?: (output: string, resyncId?: string) => void;
  /**
   * Server-authoritative alt-screen-active hint (events.proto's
   * TerminalOutput.alt_screen_active) — set only on an initial-connect or
   * post-resize snapshot, since a capture-pane-derived snapshot can never
   * itself carry the DECSET 1049h marker the client's own byte-scanning
   * (TerminalStreamManager.updateAltScreenActive) watches for on live
   * output. Without this, a client connecting to (or resizing) a session
   * that entered alt screen before this connection existed never learns it.
   */
  onAltScreenActiveHint?: (active: boolean) => void;
  /**
   * Story 1.4.1 — callback when an AppScrollbackResponse frame (Epic 1.3's
   * `Instance.ForwardScroll`) is received. Dispatched from a distinct branch
   * of this hook's message loop (never through onOutput/onScrollbackReceived)
   * so a resize resync's onFullSnapshot can't fire for it and vice versa
   * (research/architecture.md §6, mirrored by TerminalStreamManager's own
   * handleAppScrollback/onFullSnapshot isolation).
   */
  onAppScrollback?: (
    content: string,
    outcome: ScrollForwardOutcome,
    program: string,
    forwardId: string,
    blockedReason: ScrollBlockedReason,
  ) => void;
  /**
   * Shared with useVisibilityResync.ts (Epic 3.1, Task 3.1.2.1) — forwarded
   * to useTerminalFlowControl so both visibility- and resize-triggered
   * resync requests register their resync_id here, letting either flow's
   * stall watchdog be reset when a match arrives on ANY tracked request.
   */
  outstandingResyncIdsRef?: React.MutableRefObject<Map<string, number>>;
  autoConnect?: boolean; // If false, requires manual connect() call (default: true)
  initialCols?: number; // Initial terminal columns (prevents size mismatch on first load)
  initialRows?: number; // Initial terminal rows (prevents size mismatch on first load)
  isExternal?: boolean; // Whether this is an external session (uses /ws/external endpoint)
  /** Called with the number of buffered-but-undelivered messages dropped when a MessageQueue is torn down (superseded connect() or disconnect()). */
  onInputDropped?: (count: number) => void;
  /**
   * True when this terminal is the one currently selected/visible to the user
   * (drives the fast connect-timeout of the automatic reconnect path).
   */
  foreground?: boolean;
}

interface TerminalStreamResult {
  output: string; // Deprecated: Use onOutput callback for better performance
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  /** True while a chunked paste is in flight (see useTerminalFlowControl). */
  isInputChunking: () => boolean;
  resize: (cols: number, rows: number, force?: boolean, opts?: ResizeOptions) => void;
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
  /** Connection-level failures exhausted GAVE_UP_AFTER_ATTEMPTS retries; retries continue (unlike isHardFailed). */
  isServerUnreachable: boolean;
  handleManualReconnect: () => void;
  requestFullResync: (urgent?: boolean, isVisibilityTriggered?: boolean) => string | undefined;
  visibilityResyncDecision: () => VisibilityResyncDecision;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  /**
   * Number of transports attached to this session's StreamHub (Epic 4.2,
   * Story 4.2.1). undefined when unavailable — either no message carrying it
   * has arrived yet, or this is a PathLegacyPerConnection session, which
   * never reports it (see events.proto's TerminalOutput.connection_count doc
   * comment for why that value must never be fabricated).
   */
  connectionCount: number | undefined;
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
  onAppScrollback,
  onAltScreenActiveHint,
  autoConnect = true,
  initialCols,
  initialRows,
  onInputDropped,
  foreground = false,
  outstandingResyncIdsRef,
  readOnly = false,
}: UseTerminalStreamOptions): TerminalStreamResult {
  // ---- Connection state ----
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);
  // Task 4.1.1 — Terminal state machine (R1.4)
  const [terminalState, setTerminalState] = useState<TerminalState>('DISCONNECTED');
  const [isHardFailed, setIsHardFailed] = useState(false);
  // Epic 4.2, Story 4.2.1 — undefined until a PathHubOwned session's first
  // connection_count-carrying message arrives; never fabricated for
  // PathLegacyPerConnection sessions (proto field is absent there).
  const [connectionCount, setConnectionCount] = useState<number | undefined>(undefined);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false);
  // Guards against two independent visibility/focus-triggered reconnect paths
  // (this hook's own Story 3.1.3 listener and useVisibilityResync, composed
  // together in TerminalOutput.tsx) both calling connect() for the same
  // disconnected session before either handshake completes.
  const isConnectingRef = useRef(false);
  const shouldReconnectRef = useRef(false);
  const terminalBackoffRef = useRef(new BackoffState(RECONNECT_BASE_MS, RECONNECT_CAP_MS));
  // Set when a connection-level failure has burned GAVE_UP_AFTER_ATTEMPTS retries: the pane shows a calm
  // "server unreachable" state, but retries continue (only definitive errors set isHardFailed).
  const [isServerUnreachable, setIsServerUnreachable] = useState(false);
  const consecutiveTimeoutAbortsRef = useRef(0);
  const streamRefusedCountRef = useRef(0);
  const lastAttemptRefusedRef = useRef(false);
  // True after a foreground/visibility/online resume: only then is the fast connect-timeout used.
  const resumeReconnectRef = useRef(false);
  // Closes the server ended itself (error frame or clean EndStream), tracked apart from transport
  // failures: own streak, backoff and start time. A server still loading after a restart answers this
  // way for minutes, so it only hard-fails after SERVER_END_GIVE_UP_MS of continuous server-ended closes.
  const serverEndStreakRef = useRef(0);
  const serverEndBackoffRef = useRef(new BackoffState(RECONNECT_BASE_MS, RECONNECT_CAP_MS));
  const serverEndSinceRef = useRef(0);
  // Fires STABLE_STREAM_MS after a first message: only then is "server unreachable" cleared.
  const stableTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelSlotWaitRef = useRef<(() => void) | null>(null);
  const resetServerEnds = useCallback(() => {
    serverEndStreakRef.current = 0;
    serverEndBackoffRef.current.reset();
    serverEndSinceRef.current = 0;
  }, []);
  const isHardFailedRef = useRef(false);
  const terminalDebounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const connectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const foregroundRef = useRef(foreground);
  const foregroundConnectAttemptRef = useRef(0);
  // Shared across the hook's whole lifetime (not per-connect() call) because the
  // connectTimeoutRef callback below is declared outside the async message-processing
  // IIFE and needs to read it. Correctness relies on connect()'s own re-entrancy guard
  // (isConnectedRef/isConnectingRef) preventing two attempts from touching this ref at
  // once — do not relax those guards without re-checking this invariant.
  const firstMessageRef = useRef(true);
  // Set immediately before attemptController.abort() fires from a connect-timeout,
  // so the resulting stream error can be routed past onError (see the catch block
  // below) instead of surfacing as a visible connection failure — a connect-timeout
  // is a deliberate, internal fast-retry optimization, not a real failure the caller
  // should count toward its own attempt/error UI.
  const connectTimeoutAbortedRef = useRef(false);
  const connectRef = useRef<(overrideCols?: number, overrideRows?: number, options?: { isAutoRetry?: boolean }) => Promise<void>>(async () => {});
  const textDecoderRef = useRef(new TextDecoder());
  const scrollbackDecoderRef = useRef(new TextDecoder());
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

  // Clears the pending per-attempt connect-timeout timer, if any. Shared across every
  // exit path (first-message success, stream error/end, disconnect(), unmount, and a
  // synchronous throw before the async message loop starts) so none of them can miss it.
  const clearConnectTimeout = useCallback(() => {
    if (connectTimeoutRef.current) {
      clearTimeout(connectTimeoutRef.current);
      connectTimeoutRef.current = null;
    }
  }, []);

  // Sync ref with state
  useEffect(() => {
    isConnectedRef.current = isConnected;
  }, [isConnected]);

  // Detect the foreground false→true transition (AC3): reset both the
  // existing backoff-delay counter and the new fast-connect-timeout
  // attempt counter so a just-selected terminal gets the full fast
  // window immediately, not whatever was left over from before it was
  // foreground (or from background attempts that happened while it wasn't selected).
  //
  // Also clear any pending reconnectTimerRef and reconnect immediately (pre-mortem
  // Failure #1, P1): without this, a terminal that was mid-backoff-delay while
  // backgrounded would still sit out that stale, potentially-30s wait after being
  // selected, before the fast connect-timeout ever got a chance to apply.
  useEffect(() => {
    const wasForeground = foregroundRef.current;
    foregroundRef.current = foreground;
    if (!wasForeground && foreground) {
      terminalBackoffRef.current.reset();
      foregroundConnectAttemptRef.current = 0;
      resumeReconnectRef.current = true;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
        // !isDisconnectingRef.current is unreachable via any path today — disconnect()
        // always clears reconnectTimerRef before setting isDisconnectingRef, so this
        // branch can never observe both truthy at once — but it's cheap, matches
        // handleVisibilityOrOnline's analogous guard below, and protects a future
        // refactor of disconnect()'s ordering from silently reintroducing a race.
        // Not independently unit-tested for that reason (see sdd:6-verify PR review).
        if (shouldReconnectRef.current && !isConnectingRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
          connectRef.current?.();
        }
      }
    }
  }, [foreground]);

  // ---- Compose sub-hooks ----

  // pushMessageRef bridges the connection's messageQueue to flow control dispatch.
  // Bug Risk 3 mitigation: flow control reads pushMessageRef.current (not a stale closure).
  const pushMessageRef = useRef<((msg: TerminalData) => void) | null>(null);

  // Keep pushMessageRef in sync with current messageQueue
  useEffect(() => {
    pushMessageRef.current = (msg: TerminalData) => {
      if (msg.data.case === "input") terminalInputTimer.markQueued();
      messageQueueRef.current?.push(msg);
    };
    return () => { pushMessageRef.current = null; };
  }, []);

  const flowControl = useTerminalFlowControl({
    sessionId,
    getTerminal: getTerminal ?? (() => null),
    pushMessageRef,
    isConnectedRef,
    onError,
    outstandingResyncIdsRef,
    readOnly,
  });

  const metrics = useTerminalMetrics({ onOutput });

  // ---- Error helper ----
  const handleError = useCallback((err: unknown) => {
    const error = err instanceof Error ? err : new Error(String(err));
    setError(error);
    onError?.(error);
  }, [onError]);

  // ---- Connect ----
  const connect = useCallback(async (overrideCols?: number, overrideRows?: number, options?: { isAutoRetry?: boolean }) => {
    // Refuse to reconnect through a hard failure. The only sanctioned way back
    // in is handleManualReconnect below, which clears isHardFailedRef.current
    // to false *before* calling connect() — so this check never blocks Retry,
    // only callers (resize handlers, visibility/focus fallbacks, stale mount
    // effects) that invoke connect() directly without going through Retry.
    if (isHardFailedRef.current) return;
    if (isConnectedRef.current || isConnectingRef.current || !sessionId) return;
    isConnectingRef.current = true;
    shouldReconnectRef.current = true;
    // Only the finally block's own self-scheduled automatic retry (below) passes
    // isAutoRetry: true. Every other caller — mount, manual reconnect, the
    // visibility/online listener and foreground-transition effect in this file,
    // and useVisibilityResync's two direct connect() calls — omits it and gets the
    // fresh backoff sequence a genuinely new attempt sequence should have. Without
    // this gate, the automatic retry itself reset terminalBackoffRef on every call,
    // so `.attempt` was always 0 when the hard-fail check below ran and the delay
    // below never escalated past uniform(0, 1000ms).
    if (!options?.isAutoRetry) {
      terminalBackoffRef.current.reset();
      serverEndBackoffRef.current.reset(); // delay only: the streak and give-up clock keep running
    }

    // Task 2.2.1 — bump the connection generation immediately so this call's
    // message-processing loop (started below) can identify itself as "the
    // current attempt" and detect being superseded by a later connect().
    const myGeneration = ++connectionGenerationRef.current;
    const connectStartedAt = Date.now();

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
    lastAttemptRefusedRef.current = false;
    setTerminalState('CONNECTING');

    // Bound connects in flight across all panes; a queued attempt stays in CONNECTING.
    let releaseSlot = tryAcquireConnectSlot();
    if (!releaseSlot) {
      const queued = queueForConnectSlot(foregroundRef.current);
      cancelSlotWaitRef.current = queued.cancel;
      releaseSlot = await queued.promise;
      if (cancelSlotWaitRef.current === queued.cancel) cancelSlotWaitRef.current = null;
      // Superseded (a newer connect() owns the flags) or disconnected while waiting: hand the slot
      // back. isDisconnectingRef needs no check of its own, disconnect() always clears shouldReconnectRef first.
      const superseded = myGeneration !== connectionGenerationRef.current;
      if (!releaseSlot || superseded || !shouldReconnectRef.current) {
        releaseSlot?.();
        if (!superseded) {
          isConnectingRef.current = false;
          setTerminalState('DISCONNECTED');
        }
        return;
      }
    }
    const releaseConnectSlot = releaseSlot;

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
      const attemptController = abortControllerRef.current;
      firstMessageRef.current = true;
      connectTimeoutAbortedRef.current = false;
      const foregroundAtSchedule = foregroundRef.current;
      const timeoutMs = connectTimeoutMs(foregroundAtSchedule && resumeReconnectRef.current, foregroundConnectAttemptRef.current);
      foregroundConnectAttemptRef.current += 1;
      const attemptNumber = foregroundConnectAttemptRef.current;
      connectTimeoutRef.current = setTimeout(() => {
        connectTimeoutRef.current = null;
        // Race guard (pre-mortem Failure #3, P2): re-check firstMessageRef
        // immediately before aborting, so a message that already landed and
        // was processed is not retroactively aborted.
        if (!firstMessageRef.current) return;
        console.warn(`[reconnect] stream=terminal trigger=connect-timeout foreground=${foregroundAtSchedule} attempt=${attemptNumber} timeoutMs=${timeoutMs}`);
        connectTimeoutAbortedRef.current = true;
        attemptController.abort();
      }, timeoutMs);
      messageQueueRef.current = new MessageQueue();

      // Send initial handshake with dimensions
      const currentPaneReq = create(CurrentPaneRequestSchema, {
        lines: 50,
        includeEscapes: true,
      });

      if (targetCols !== undefined && targetRows !== undefined && isUsableTerminalSize(targetCols, targetRows)) {
        currentPaneReq.targetCols = targetCols;
        currentPaneReq.targetRows = targetRows;
        console.log(`[useTerminalStream] Sending handshake with dimensions: ${targetCols}x${targetRows}`);
      } else {
        console.warn(`[useTerminalStream] No usable terminal dimensions for handshake (${targetCols}x${targetRows})`);
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
        let serverReportedError = false;
        let endedByServer = false; // the for-await loop returned normally: error frame or clean EndStream
        try {
          for await (const msg of stream) {
            // Task 2.2.3 — a superseded generation's loop must not mutate
            // shared state (or, transitively, deliver buffered input to the
            // wrong connection). Mirrors usePathCompletions.ts's
            // `if (generation !== generationRef.current) return;` guard.
            if (myGeneration !== connectionGenerationRef.current) {
              console.warn(`[useTerminalStream] Discarding message from superseded connection generation ${myGeneration} (current: ${connectionGenerationRef.current})`);
              break;
            }

            if (firstMessageRef.current) {
              clearConnectTimeout();
              releaseConnectSlot();
              consecutiveTimeoutAbortsRef.current = 0;
              streamRefusedCountRef.current = 0;
              resumeReconnectRef.current = false;
              // Cleared only after the stream proves stable, so one that dies right after attaching
              // does not flicker the unreachable state on and off.
              if (stableTimerRef.current) clearTimeout(stableTimerRef.current);
              stableTimerRef.current = setTimeout(() => {
                stableTimerRef.current = null;
                setIsServerUnreachable(false);
              }, STABLE_STREAM_MS);
              isConnectingRef.current = false;
              isConnectedRef.current = true; // sync ref before state setter — mirrors the disconnect-path precedent in the finally block below, closing the window where a same-tick second connect() call could read a stale isConnectedRef before React flushes setIsConnected(true)
              setIsConnected(true);
              setScrollbackLoaded(true);
              setTerminalState('LOADING');
              firstMessageRef.current = false;
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
              // Task 4.2.1c (Epic 4.2) — connection_count rides on TerminalOutput,
              // including on side-channel messages the server sends with no `data`
              // (server/services/connectrpc_websocket.go's sendConnectionCountUpdates).
              // Only ever present for PathHubOwned sessions; undefined otherwise —
              // never fabricated (plan.md Story 4.2.1 AC2).
              if (msg.data.value.connectionCount !== undefined) {
                setConnectionCount(msg.data.value.connectionCount);
              }

              // See onAltScreenActiveHint's doc comment — only ever present
              // on an initial-connect/post-resize snapshot.
              if (msg.data.value.altScreenActive !== undefined) {
                onAltScreenActiveHint?.(msg.data.value.altScreenActive);
              }

              // Handle raw output
              const decodedData = msg.data.value.data;
              if (decodedData.length === 0) {
                // Connection-count-only side-channel message — nothing to render.
                continue;
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
                onOutput(text, msg.data.value.resyncId);
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
            } else if (msg.data.case === "appScrollbackResponse") {
              // Story 1.4.1 — a captured redraw from Instance.ForwardScroll, sent
              // instead of scrollbackResponse when AppScrollGate passes. Decoded
              // and dispatched through its own callback so it can never be
              // confused with either the raw output or tmux-native scrollback path.
              const resp = msg.data.value;
              const text = resp.content.length > 0 ? new TextDecoder().decode(resp.content) : "";
              onAppScrollback?.(text, resp.outcome, resp.program, resp.forwardId, resp.blockedReason);
              setTerminalState((prev) => prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
            } else if (msg.data.case === "error") {
              serverReportedError = true;
              const err = new Error(msg.data.value.message);
              setError(err);
              onError?.(err);
              // design/ux.md Surface 2 — HUB_START_FAILED means the hub-owned
              // path failed to start AND its server-side legacy fallback also
              // failed (server/services/connectrpc_websocket.go's streamViaHub),
              // so this connection has no working path at all. Reconnecting
              // would hit the same failure, so skip the usual 5-attempt
              // backoff-exhaustion path and surface TerminalOutput.tsx's
              // existing hardFailedBanner immediately — the same treatment
              // already given to non-retriable ws-close-codes below.
              if (msg.data.value.code === "HUB_START_FAILED") {
                shouldReconnectRef.current = false;
                isHardFailedRef.current = true;
                setIsHardFailed(true);
                console.warn(`[reconnect] stream=terminal trigger=hub-start-failed, giving up`);
              }
            }
          }
          endedByServer = true;
          // The server ended the stream without an error (e.g. a hub eviction or teardown,
          // sent as a clean EndStream). Surface it as a lost connection so the pane's
          // "Reconnecting (attempt n/5)" status reflects it; the retry itself is scheduled in `finally`.
          // Shell tabs end normally when the shell exits, and an error frame was already surfaced.
          if (myGeneration === connectionGenerationRef.current
              && !shellId
              && !serverReportedError
              && shouldReconnectRef.current
              && !isDisconnectingRef.current
              && !connectTimeoutAbortedRef.current) {
            // TerminalOutput resets its attempt counter on every connect, so a stream that
            // connects and then ends cleanly each time would surface an error forever; cap the streak
            // (finally increments it; a stable stream restarts it).
            const streak = Date.now() - connectStartedAt >= STABLE_STREAM_MS ? 0 : serverEndStreakRef.current;
            if (streak < MAX_CLEAN_SERVER_ENDS) {
              handleError(new Error("terminal stream closed by server"));
            }
          }
        } catch (err) {
          // Task 2.2.3 — a superseded generation's error/teardown must not
          // stomp the newer generation's state (shared backoff/hard-fail
          // refs included — a stale, aborted generation's close should not
          // affect the currently-live generation's reconnect fate).
          if (myGeneration === connectionGenerationRef.current) {
            // isWorktreeMissingError is checked here (terminal-stream-specific),
            // not folded into the shared isNonRetriableConnectError — see that
            // function's doc comment for why.
            lastAttemptRefusedRef.current = getWsCloseCode(err) === STREAM_REFUSED_CLOSE_CODE;
            if (isNonRetriableConnectError(err) || isWorktreeMissingError(err)) {
              shouldReconnectRef.current = false;
              isHardFailedRef.current = true;
              setIsHardFailed(true);
              const code = err instanceof ConnectError ? err.code : "unknown";
              console.warn(`[reconnect] stream=terminal non-retriable error code=${code}, giving up`);
            }
            // A connect-timeout abort is our own deliberate fast-retry optimization, not
            // a real failure — don't surface it via onError/setError, or callers that
            // count onError calls toward a user-visible "connection failed" UI (e.g.
            // TerminalOutput.tsx's connectionAttempts banner) would show churn/false
            // "Terminal unavailable" states for what's meant to be an invisible retry.
            // The internal backoff/reconnect scheduling below is unaffected either way.
            // A disconnect() that aborts an in-flight connect is deliberate too.
            const abortedByDisconnect = attemptController.signal.aborted && !shouldReconnectRef.current;
            // Within the abort cap a timeout abort stays invisible; once exhausted it surfaces like any failure.
            const invisibleTimeoutAbort = connectTimeoutAbortedRef.current && consecutiveTimeoutAbortsRef.current < CONNECT_TIMEOUT_ABORT_CAP;
            if (!invisibleTimeoutAbort && !abortedByDisconnect) {
              handleError(err);
            }
          }
        } finally {
          releaseConnectSlot();
          if (myGeneration === connectionGenerationRef.current) {
            // A superseded generation is a deliberate reconnect; reaching here means this stream ended on its own.
            const livedMs = Date.now() - connectStartedAt;
            // Reflects the retry gate below; disconnect() clears shouldReconnectRef first, so the
            // raw flag alone reads false for a deliberate disconnect-then-connect (e.g. resync recovery).
            const willReconnect = shouldReconnectRef.current && !isDisconnectingRef.current && !isHardFailedRef.current;
            console.warn(
              `[stream-disconnect] session=${sessionId} shell=${shellId ?? ""} livedMs=${livedMs} ` +
              `foreground=${foregroundRef.current} visibility=${document.visibilityState} willReconnect=${willReconnect} ` +
              `hardFailed=${isHardFailedRef.current} connectTimeoutAbort=${connectTimeoutAbortedRef.current}`,
            );
            isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
            isConnectingRef.current = false;
            setIsConnected(false);
            setTerminalState('DISCONNECTED');
            // A stale count from the just-closed connection must not linger
            // and imply this session still has extra viewers attached.
            setConnectionCount(undefined);
            // Reset decoders so stale {stream:true} buffered state from a server-closed
            // connection does not corrupt the next connect() call.
            textDecoderRef.current = new TextDecoder();
            scrollbackDecoderRef.current = new TextDecoder();
            // Task 2.2.3 — guarded by the same myGeneration check as the rest of this
            // block: an already-superseded generation's teardown must not clear the
            // CURRENT generation's pending connect-timeout timer (connectTimeoutRef is
            // shared across the hook's lifetime, not per-generation).
            clearConnectTimeout();
            if (stableTimerRef.current) {
              clearTimeout(stableTimerRef.current);
              stableTimerRef.current = null;
            }
            if (shouldReconnectRef.current && !isDisconnectingRef.current) {
              // A stream that stayed up is a successful attach: restart the backoff so drops
              // hours apart never accumulate toward the hard-fail cap.
              if (livedMs >= STABLE_STREAM_MS) {
                terminalBackoffRef.current.reset();
                resetServerEnds();
              }
              if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
              }
              let delay: number | null;
              let trigger: string;
              if (connectTimeoutAbortedRef.current && consecutiveTimeoutAbortsRef.current < CONNECT_TIMEOUT_ABORT_CAP) {
                // Our own timeout, not a server verdict: retry on an escalating jittered schedule
                // without touching the backoff, but only up to the cap so a black-holed network
                // cannot reopen every pane every few seconds forever.
                consecutiveTimeoutAbortsRef.current += 1;
                delay = connectTimeoutAbortDelay(consecutiveTimeoutAbortsRef.current);
                trigger = "connect-timeout";
              } else if (lastAttemptRefusedRef.current) {
                // Mux admission control (1013): wait it out without spending the unreachable budget.
                streamRefusedCountRef.current += 1;
                delay = jitteredDelay(RECONNECT_BASE_MS, RECONNECT_CAP_MS, streamRefusedCountRef.current - 1);
                trigger = "stream-refused";
              } else if (endedByServer) {
                // The server answered and ended the stream itself. Keep retrying (it may still be
                // loading after a restart) and only give up after SERVER_END_GIVE_UP_MS of it.
                serverEndStreakRef.current += 1;
                if (serverEndSinceRef.current === 0) serverEndSinceRef.current = Date.now();
                trigger = "server-close";
                if (Date.now() - serverEndSinceRef.current >= SERVER_END_GIVE_UP_MS) {
                  delay = null;
                  shouldReconnectRef.current = false;
                  isHardFailedRef.current = true;
                  setIsHardFailed(true);
                  console.warn(`[reconnect] stream=terminal trigger=server-close streak=${serverEndStreakRef.current}, giving up`);
                } else {
                  delay = serverEndBackoffRef.current.next();
                }
              } else {
                delay = terminalBackoffRef.current.next();
                trigger = "close";
                // Transport failures never go terminal (a squad restart outlasts any fixed budget);
                // past the threshold the pane just shows a calmer "server unreachable" state.
                if (terminalBackoffRef.current.attempt >= GAVE_UP_AFTER_ATTEMPTS) setIsServerUnreachable(true);
              }
              if (delay !== null) {
                console.info(`[reconnect] stream=terminal trigger=${trigger} attempt=${terminalBackoffRef.current.attempt} delay=${Math.round(delay)}ms`);
                reconnectTimerRef.current = setTimeout(() => {
                  reconnectTimerRef.current = null;
                  if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                    connectRef.current?.(undefined, undefined, { isAutoRetry: true });
                  }
                }, delay);
              }
            }
          }
        }
      })();
    } catch (err) {
      // Covers a synchronous throw before the async message loop starts (e.g. proto
      // schema validation, MessageQueue construction) — the connect-timeout scheduled
      // above must not outlive this now-dead attempt.
      isConnectingRef.current = false;
      releaseConnectSlot();
      clearConnectTimeout();
      handleError(err);
      setIsConnected(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- flowControl is a fresh object each render; kept so connect's identity churn is unchanged
  }, [sessionId, shellId, onShellStatusChange, getTerminal, onError, onScrollbackReceived, onOutput, onAppScrollback,
      onAltScreenActiveHint, flowControl, metrics, handleError, initialCols, initialRows, onInputDropped, clearConnectTimeout]);

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
    clearConnectTimeout();
    // A cancelled queued attempt settles to DISCONNECTED in connect()'s continuation; the in-flight
    // abort below clears isConnectingRef so a connect() right after this call is not refused.
    cancelSlotWaitRef.current?.();
    cancelSlotWaitRef.current = null;
    if (stableTimerRef.current) {
      clearTimeout(stableTimerRef.current);
      stableTimerRef.current = null;
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
    // An in-flight connect has no live stream to close gracefully, so abort it now; its slot is
    // released when the aborted stream actually ends, which keeps the connect gate honest.
    if (isConnectingRef.current && !isConnectedRef.current) {
      abortControllerRef.current?.abort();
      isConnectingRef.current = false; // the aborted attempt's finally is stale once a new connect() starts
    }
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
    textDecoderRef.current = new TextDecoder();
    scrollbackDecoderRef.current = new TextDecoder();
  }, [getIsResyncingRef, onInputDropped, clearConnectTimeout]);

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
      clearConnectTimeout();
      metrics.flushOutputBuffer();
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, autoConnect]);

  // Story 3.1.3 — reconnect on tab visibility or network online event
  useEffect(() => {
    const handleVisibilityOrOnline = (ev: Event) => {
      if (document.visibilityState !== "visible" && ev.type !== "online") return;
      if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
      terminalDebounceTimerRef.current = setTimeout(() => {
        terminalDebounceTimerRef.current = null;
        if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
          // Also revives the "server unreachable" state: it is retrying, not terminal.
          if (reconnectTimerRef.current) {
            clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = null;
          }
          terminalBackoffRef.current.reset();
          resetServerEnds();
          consecutiveTimeoutAbortsRef.current = 0;
          streamRefusedCountRef.current = 0;
          foregroundConnectAttemptRef.current = 0;
          resumeReconnectRef.current = true;
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
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    terminalBackoffRef.current.reset();
    resetServerEnds();
    consecutiveTimeoutAbortsRef.current = 0;
    streamRefusedCountRef.current = 0;
    connectRef.current();
  }, [resetServerEnds]);

  return {
    output: metrics.output,
    isConnected,
    error,
    sendInput: flowControl.sendInput,
    isInputChunking: flowControl.isInputChunking,
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
    isServerUnreachable,
    handleManualReconnect,
    requestFullResync: flowControl.requestFullResync,
    visibilityResyncDecision: flowControl.visibilityResyncDecision,
    markResyncComplete: flowControl.markResyncComplete,
    markPaneResponseReceived: flowControl.markPaneResponseReceived,
    connectionCount,
  };
}
