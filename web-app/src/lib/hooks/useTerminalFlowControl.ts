"use client";

import { useRef, useCallback, useEffect } from "react";
import { TerminalData, TerminalDataSchema, TerminalInput, TerminalInputSchema, TerminalResize, TerminalResizeSchema, ScrollbackRequest, ScrollbackRequestSchema, CurrentPaneRequest, CurrentPaneRequestSchema, FlowControl, FlowControlSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { dimensionsEqual, type ResizeDimensions } from "@/lib/terminal/types";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { generateSecureId } from "@/lib/pane/paneUtils";
import type { Terminal } from '@xterm/xterm';

// Epic 3.1 (AC2) — client-generated correlation ID echoed back on the
// resulting TerminalOutput.resync_id. Flag off preserves pre-project wire
// behavior: CurrentPaneRequest.resync_id is left empty and
// requestFullResync()'s return value is always undefined.
export const RESYNC_CORRELATION_ID_FLAG = 'terminal:resync-correlation-id';

export interface UseTerminalFlowControlOptions {
  sessionId: string;
  getTerminal: () => Terminal | null;
  /** Push a message onto the connection queue. Stored via ref to avoid stale closures. */
  pushMessageRef: React.MutableRefObject<((msg: TerminalData) => void) | null>;
  isConnectedRef: React.MutableRefObject<boolean>;
  onError?: (error: Error) => void;
  /**
   * Shared with useVisibilityResync.ts (Epic 3.1, Task 3.1.2.1) so the two
   * independently-tracked resync flows (visibility-triggered vs
   * resize-triggered) can reconcile a stall watchdog reset when *either*
   * flow's outstanding resync_id is echoed back on a TerminalOutput message.
   * Populated only when a resync_id was actually generated (i.e. the
   * correlation-ID flag is on) — never gated on `isVisibilityTriggered`.
   * Values are the `Date.now()` this ID was added (Epic 3.1, Task 3.1.2.4a)
   * so useVisibilityResync's escalation logic can tell how long a specific
   * ID has actually been outstanding, independent of the shared watchdog
   * reset.
   */
  outstandingResyncIdsRef?: React.MutableRefObject<Map<string, number>>;
}

export interface UseTerminalFlowControlResult {
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number, force?: boolean) => void;
  requestScrollback: (fromSequence: number, limit: number) => void;
  sendFlowControl: (paused: boolean, watermark?: number) => void;
  /**
   * @param isVisibilityTriggered - True when this resync was triggered by a
   * visibility/focus event (useVisibilityResync.ts) rather than a resize.
   * Drives the `stale_dimensions` flag — never set for resize-triggered
   * resyncs, since those already carry fresh dimensions by construction.
   * @returns the generated resync_id, or undefined when
   * terminal:resync-correlation-id is off or the request could not be sent.
   */
  requestFullResync: (urgent?: boolean, isVisibilityTriggered?: boolean) => string | undefined;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  getIsResyncingRef: () => React.MutableRefObject<string | null>;
  getWaitingForPaneResponseRef: () => React.MutableRefObject<string | null>;
}

/**
 * useTerminalFlowControl - Resync logic, resize throttling, and message dispatch
 * for terminal input/resize/scrollback/flow-control.
 */
export function useTerminalFlowControl({
  sessionId,
  getTerminal,
  pushMessageRef,
  isConnectedRef,
  onError,
  outstandingResyncIdsRef,
}: UseTerminalFlowControlOptions): UseTerminalFlowControlResult {
  const correlationIdEnabled = useFeatureFlag(RESYNC_CORRELATION_ID_FLAG);

  // Resync state machine refs. Hold the pending resync's generated tracking
  // ID (null when no resync is pending) rather than a plain boolean — Epic
  // 3.1, Task 3.1.1.2. Truthiness is preserved for existing consumers
  // (useTerminalStream.ts's disconnect()): a non-empty string is truthy,
  // null is falsy, so the existing `if (isResyncingRef.current)` checks keep
  // working unchanged. An internal tracking ID is always generated
  // (independent of the correlation-ID flag) so this pending-state machinery
  // behaves identically whether or not the flag is on; only the wire
  // encoding (CurrentPaneRequest.resync_id) and requestFullResync's return
  // value are gated on the flag.
  const isResyncingRef = useRef<string | null>(null);
  const waitingForPaneResponseRef = useRef<string | null>(null);
  const lastResyncTimeRef = useRef<number>(0);
  const lastResizeTimeRef = useRef<number>(0);
  const lastSentDimsRef = useRef<ResizeDimensions | null>(null);
  // Last BOUNCE_HISTORY_SIZE sizes sent (oldest first; current lastSentDimsRef excluded,
  // handled by value-dedup above). Lets resize() catch a repeat of any recent size, not
  // just two sends back, since a real oscillating viewport can cycle through 3+ values.
  const sentHistoryRef = useRef<ResizeDimensions[]>([]);
  const BOUNCE_HISTORY_SIZE = 4;
  // Consecutive bounces held back-to-back with no genuinely-new size sent in
  // between. Drives an escalating hold (doubling, capped) so a viewport
  // stuck oscillating for an extended period backs off further apart instead
  // of retrying every flat BOUNCE_HOLD_MS — resets to 0 the moment a resize
  // actually sends (a real new size, or a held one whose hold elapsed).
  const bounceStreakRef = useRef(0);
  const pendingResizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const paneRequestTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dimensionSyncRef = useRef<{ cols?: number; rows?: number }>({});
  // Epic 3.1, Task 3.1.1.4a — last dimensions a resync response was actually
  // applied for (set in markPaneResponseReceived). Compared against the
  // terminal's CURRENT dimensions in requestFullResync to compute
  // stale_dimensions: null until the first resync response lands, so the
  // flag defaults to false on a cold start rather than an unconditional true
  // (pre-mortem P1 risk — must not reintroduce the corruption bug an
  // always-true stale_dimensions would cause).
  const lastSyncedDimensionsRef = useRef<{ cols: number; rows: number } | null>(null);

  // Cancel any pending deferred resize/pane-request timers when the component
  // unmounts to prevent the timer callback from firing against a torn-down
  // component/connection.
  useEffect(() => {
    return () => {
      if (pendingResizeTimerRef.current) {
        clearTimeout(pendingResizeTimerRef.current);
        pendingResizeTimerRef.current = null;
      }
      if (paneRequestTimerRef.current) {
        clearTimeout(paneRequestTimerRef.current);
        paneRequestTimerRef.current = null;
      }
    };
  }, []);

  // Helper to set error state
  const handleError = useCallback((err: unknown) => {
    const error = err instanceof Error ? err : new Error(String(err));
    onError?.(error);
  }, [onError]);

  // Helper to push a message (reads from ref to avoid stale closures)
  const pushMessage = useCallback((msg: TerminalData) => {
    pushMessageRef.current?.(msg);
  }, [pushMessageRef]);

  // Shared connection gate for every public dispatch function below — a copy-pasted
  // per-callsite check is how sendInput's copy silently lacked a console.warn. Internal
  // per-chunk/per-tick continuation checks stay inline (a background retry after the
  // initial call already logged once shouldn't log again).
  const ensureConnected = useCallback((action: string): boolean => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn(`[useTerminalFlowControl] Cannot ${action}: stream not connected`);
      return false;
    }
    return true;
  }, [pushMessageRef, isConnectedRef]);

  // ---- Resync ----

  const requestFullResync = useCallback((urgent: boolean = false, isVisibilityTriggered: boolean = false): string | undefined => {
    if (!ensureConnected("request resync")) {
      return undefined;
    }

    const currentTerminal = getTerminal();
    if (!currentTerminal) {
      console.warn("[useTerminalFlowControl] Cannot request resync: terminal not available");
      return undefined;
    }

    const now = Date.now();
    const timeSinceLastResync = now - lastResyncTimeRef.current;
    const RESYNC_THROTTLE_MS = 2000;

    if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
      console.log(`[useTerminalFlowControl] Resync throttled (${timeSinceLastResync}ms since last, need ${RESYNC_THROTTLE_MS}ms)`);
      return undefined;
    }

    if (urgent) {
      console.log(`[useTerminalFlowControl] Urgent resync bypassing throttle (${timeSinceLastResync}ms since last)`);
    }

    try {
      console.log(`[useTerminalFlowControl] Requesting full resync with current dimensions: ${currentTerminal.cols}x${currentTerminal.rows}`);
      lastResyncTimeRef.current = now;
      const trackingId = generateSecureId();
      isResyncingRef.current = trackingId;
      waitingForPaneResponseRef.current = trackingId;

      dimensionSyncRef.current = {
        cols: currentTerminal.cols,
        rows: currentTerminal.rows,
      };

      // Epic 3.1, Task 3.1.1.4b — only a visibility/focus-triggered resync
      // can be "stale" in this sense (a resize-triggered one always carries
      // just-measured dimensions). Defaults to false until a resync response
      // has actually been applied at least once (lastSyncedDimensionsRef
      // starts null), and correctly stays false when the terminal's own
      // dimensions genuinely changed while backgrounded (currentTerminal.cols/
      // rows would then differ from lastSyncedDimensionsRef).
      const staleDimensions = isVisibilityTriggered
        && lastSyncedDimensionsRef.current !== null
        && currentTerminal.cols === lastSyncedDimensionsRef.current.cols
        && currentTerminal.rows === lastSyncedDimensionsRef.current.rows;

      const resyncId = correlationIdEnabled ? trackingId : "";

      const currentPaneReq = create(CurrentPaneRequestSchema, {
        lines: 50,
        includeEscapes: true,
        targetCols: currentTerminal.cols,
        targetRows: currentTerminal.rows,
        resyncId,
        staleDimensions,
      });

      if (resyncId && outstandingResyncIdsRef) {
        outstandingResyncIdsRef.current.set(resyncId, Date.now());
      }

      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "currentPaneRequest",
            value: currentPaneReq,
          },
        })
      );

      return resyncId || undefined;
    } catch (err) {
      handleError(err);
      return undefined;
    }
  }, [sessionId, getTerminal, pushMessage, ensureConnected, handleError, correlationIdEnabled, outstandingResyncIdsRef]);

  // ---- Message dispatch functions ----

  const PASTE_CHUNK_SIZE = 512; // bytes — fits within tmux's pty write buffer
  const CHUNK_DELAY_MS = 10;   // ms between chunks — yields event loop without stalling input

  const sendInput = useCallback((input: string) => {
    if (!ensureConnected("send input")) return;

    const encoder = new TextEncoder();
    const inputBytes = encoder.encode(input);

    if (inputBytes.length <= PASTE_CHUNK_SIZE) {
      try {
        pushMessage(
          create(TerminalDataSchema, {
            sessionId,
            data: {
              case: "input",
              value: create(TerminalInputSchema, { data: inputBytes }),
            },
          })
        );
      } catch (err) {
        handleError(err);
      }
      return;
    }

    // Large input: send in chunks to avoid tmux/WebSocket buffer limits.
    // Capture sessionId at call-time — if the session changes mid-paste the
    // pending chunks are aborted (sessionIdAtStart !== current sessionId).
    const sessionIdAtStart = sessionId;
    let offset = 0;
    const sendChunk = () => {
      if (!pushMessageRef.current || !isConnectedRef.current) return;
      if (sessionId !== sessionIdAtStart) return; // session changed; abort
      if (offset >= inputBytes.length) return;
      const chunk = inputBytes.slice(offset, offset + PASTE_CHUNK_SIZE);
      offset += PASTE_CHUNK_SIZE;
      try {
        pushMessage(
          create(TerminalDataSchema, {
            sessionId: sessionIdAtStart,
            data: {
              case: "input",
              value: create(TerminalInputSchema, { data: chunk }),
            },
          })
        );
      } catch (err) {
        handleError(err);
        return;
      }
      if (offset < inputBytes.length) {
        setTimeout(sendChunk, CHUNK_DELAY_MS);
      }
    };
    sendChunk();
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError, ensureConnected]);

  const resize = useCallback((cols: number, rows: number, force: boolean = false) => {
    if (!ensureConnected("resize terminal")) return;

    // Cancel any previously deferred resize — we have newer dimensions now.
    // This MUST run before the value-dedup early-return below: otherwise a
    // bounce-back call whose dimensions match lastSentDimsRef (e.g. A -> B
    // deferred within the throttle window -> back to A) would dedup-return
    // without cancelling the still-pending deferred send for B, letting that
    // stale send fire later with the wrong dimensions.
    if (pendingResizeTimerRef.current) {
      clearTimeout(pendingResizeTimerRef.current);
      pendingResizeTimerRef.current = null;
    }

    // Value-dedup: skip if this exact (cols, rows) pair was the last one actually
    // sent, independent of (and checked before) the time throttle below. An
    // unchanged value must not keep the throttle window "warm" — lastResizeTimeRef
    // is deliberately left untouched here.
    if (
      !force &&
      lastSentDimsRef.current !== null &&
      dimensionsEqual(lastSentDimsRef.current, { cols, rows })
    ) {
      console.log(`[useTerminalFlowControl] Resize skipped, value unchanged (${cols}x${rows})`);
      return;
    }

    const now = Date.now();
    const timeSinceLastResize = now - lastResizeTimeRef.current;
    const THROTTLE_MS = 200;

    // Inner send: stamps the timestamp, sends the resize RPC, then requests a
    // fresh pane capture so xterm.js and tmux are guaranteed to agree on content.
    const doSend = () => {
      if (!pushMessageRef.current || !isConnectedRef.current) return;
      try {
        console.log(`[useTerminalFlowControl] Sending resize to server: ${cols}x${rows}`);
        pushMessage(
          create(TerminalDataSchema, {
            sessionId,
            data: { case: "resize", value: create(TerminalResizeSchema, { cols, rows }) },
          })
        );

        // Only record success (and refresh the throttle/dedup state) after the
        // send above completed without throwing.
        lastResizeTimeRef.current = Date.now();
        if (lastSentDimsRef.current !== null) {
          sentHistoryRef.current.push(lastSentDimsRef.current);
          if (sentHistoryRef.current.length > BOUNCE_HISTORY_SIZE) {
            sentHistoryRef.current.shift();
          }
        }
        lastSentDimsRef.current = { cols, rows };
        bounceStreakRef.current = 0;

        // After resizing, request fresh terminal content
        paneRequestTimerRef.current = setTimeout(() => {
          paneRequestTimerRef.current = null;
          if (!pushMessageRef.current || !isConnectedRef.current) return;
          try {
            console.log(`[useTerminalFlowControl] Requesting fresh pane content after resize`);
            // Epic 3.1, Task 3.1.1.3 — a resize-triggered pane request always
            // carries just-measured dimensions, so stale_dimensions is always
            // explicitly false here (never the isVisibilityTriggered formula
            // used in requestFullResync).
            const resyncId = correlationIdEnabled ? generateSecureId() : "";
            if (resyncId && outstandingResyncIdsRef) {
              outstandingResyncIdsRef.current.set(resyncId, Date.now());
            }
            pushMessage(
              create(TerminalDataSchema, {
                sessionId,
                data: {
                  case: "currentPaneRequest",
                  value: create(CurrentPaneRequestSchema, {
                    lines: 50,
                    includeEscapes: true,
                    targetCols: cols,
                    targetRows: rows,
                    resyncId,
                    staleDimensions: false,
                  }),
                },
              })
            );
          } catch (err) {
            handleError(err);
          }
        }, 100);
      } catch (err) {
        handleError(err);
      }
    };

    // Bounce detection: matches ANY of the last BOUNCE_HISTORY_SIZE sizes sent, not just
    // the one two sends ago, since a real oscillating viewport can wander through 3+
    // values on a slower cadence than THROTTLE_MS catches. Held out past an escalating
    // BOUNCE_HOLD_MS instead of sent immediately, coalescing the oscillation into one settled resize.
    const isBounce = !force && sentHistoryRef.current.some((d) => dimensionsEqual(d, { cols, rows }));
    if (isBounce) {
      const BOUNCE_HOLD_BASE_MS = 3000;
      const BOUNCE_HOLD_MAX_MS = 15000;
      const holdMs = Math.min(BOUNCE_HOLD_BASE_MS * 2 ** bounceStreakRef.current, BOUNCE_HOLD_MAX_MS);
      bounceStreakRef.current += 1;
      console.log(`[useTerminalFlowControl] Resize bounce detected (${cols}x${rows} matches recent history), holding ${holdMs}ms (streak ${bounceStreakRef.current})`);
      pendingResizeTimerRef.current = setTimeout(() => {
        pendingResizeTimerRef.current = null;
        doSend();
      }, holdMs);
      return;
    }

    if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
      // Defer instead of drop: schedule the trailing-edge send so the final
      // settled size always reaches the server after rapid resize sequences.
      const remaining = THROTTLE_MS - timeSinceLastResize;
      console.log(`[useTerminalFlowControl] Resize deferred ${remaining}ms (${cols}x${rows})`);
      pendingResizeTimerRef.current = setTimeout(() => {
        pendingResizeTimerRef.current = null;
        doSend();
      }, remaining + 1);
      return;
    }

    doSend();
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, ensureConnected, handleError, correlationIdEnabled, outstandingResyncIdsRef]);

  const requestScrollback = useCallback((fromSequence: number, limit: number) => {
    if (!ensureConnected("request scrollback")) return;

    try {
      console.log(`[useTerminalFlowControl] Requesting scrollback: fromSeq=${fromSequence}, limit=${limit}`);
      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "scrollbackRequest",
            value: create(ScrollbackRequestSchema, {
              fromSequence: BigInt(fromSequence),
              limit,
            }),
          },
        })
      );
    } catch (err) {
      handleError(err);
    }
  }, [sessionId, pushMessage, ensureConnected, handleError]);

  const sendFlowControl = useCallback((paused: boolean, watermark?: number) => {
    if (!ensureConnected("send flow control")) return;

    try {
      console.log(`[useTerminalFlowControl] Sending flow control: paused=${paused}, watermark=${watermark || 'N/A'}`);
      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "flowControl",
            value: create(FlowControlSchema, {
              paused,
              watermark: watermark !== undefined ? BigInt(watermark) : undefined,
            }),
          },
        })
      );
    } catch (err) {
      handleError(err);
    }
  }, [sessionId, pushMessage, ensureConnected, handleError]);

  const markResyncComplete = useCallback(() => {
    isResyncingRef.current = null;
  }, []);

  const markPaneResponseReceived = useCallback(() => {
    waitingForPaneResponseRef.current = null;
    // Epic 3.1, Task 3.1.1.4a — this is the single existing "resync response
    // applied" signal (invoked, via useVisibilityResync.ts's
    // notifyResyncOutputReceived, whenever a visibility/focus-triggered
    // resync completes), so it's the correct integration point to record
    // which dimensions the client believes are now in sync with the server.
    if (dimensionSyncRef.current.cols !== undefined && dimensionSyncRef.current.rows !== undefined) {
      lastSyncedDimensionsRef.current = {
        cols: dimensionSyncRef.current.cols,
        rows: dimensionSyncRef.current.rows,
      };
    }
  }, []);

  const getIsResyncingRef = useCallback(() => isResyncingRef, []);
  const getWaitingForPaneResponseRef = useCallback(() => waitingForPaneResponseRef, []);

  return {
    sendInput,
    resize,
    requestScrollback,
    sendFlowControl,
    requestFullResync,
    markResyncComplete,
    markPaneResponseReceived,
    getIsResyncingRef,
    getWaitingForPaneResponseRef,
  };
}
