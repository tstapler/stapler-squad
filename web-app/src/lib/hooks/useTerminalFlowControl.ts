"use client";

import { useRef, useCallback, useEffect } from "react";
import { TerminalData, TerminalDataSchema, TerminalInput, TerminalInputSchema, TerminalResize, TerminalResizeSchema, ScrollbackRequest, ScrollbackRequestSchema, CurrentPaneRequest, CurrentPaneRequestSchema, FlowControl, FlowControlSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { dimensionsEqual, type ResizeDimensions } from "@/lib/terminal/types";
import type { Terminal } from '@xterm/xterm';

export interface UseTerminalFlowControlOptions {
  sessionId: string;
  getTerminal: () => Terminal | null;
  /** Push a message onto the connection queue. Stored via ref to avoid stale closures. */
  pushMessageRef: React.MutableRefObject<((msg: TerminalData) => void) | null>;
  isConnectedRef: React.MutableRefObject<boolean>;
  onError?: (error: Error) => void;
  /**
   * Called when `sendInput` silently discards a keystroke because the
   * connection is already known-disconnected (Task 4.1.1.2) — the same
   * class of input loss as a superseded MessageQueue's drop-on-close, just
   * caught one layer earlier. Only `sendInput`'s guard is wired to this;
   * the other five near-identical `!pushMessageRef.current ||
   * !isConnectedRef.current` guards in this file (resize/scrollback/resync/
   * flow-control) guard non-keystroke messages and are deliberately left
   * unwired — out of scope for this "phantom keystroke" ticket.
   */
  onDrop?: () => void;
}

export interface UseTerminalFlowControlResult {
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number, force?: boolean) => void;
  requestScrollback: (fromSequence: number, limit: number) => void;
  sendFlowControl: (paused: boolean, watermark?: number) => void;
  requestFullResync: (urgent?: boolean) => void;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  getIsResyncingRef: () => React.MutableRefObject<boolean>;
  getWaitingForPaneResponseRef: () => React.MutableRefObject<boolean>;
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
  onDrop,
}: UseTerminalFlowControlOptions): UseTerminalFlowControlResult {
  // Resync state machine refs
  const isResyncingRef = useRef(false);
  const waitingForPaneResponseRef = useRef(false);
  const lastResyncTimeRef = useRef<number>(0);
  const lastResizeTimeRef = useRef<number>(0);
  const lastSentDimsRef = useRef<ResizeDimensions | null>(null);
  const pendingResizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const paneRequestTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dimensionSyncRef = useRef<{ cols?: number; rows?: number }>({});

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

  // ---- Resync ----

  const requestFullResync = useCallback((urgent: boolean = false) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("[useTerminalFlowControl] Cannot request resync: stream not connected");
      return;
    }

    const currentTerminal = getTerminal();
    if (!currentTerminal) {
      console.warn("[useTerminalFlowControl] Cannot request resync: terminal not available");
      return;
    }

    const now = Date.now();
    const timeSinceLastResync = now - lastResyncTimeRef.current;
    const RESYNC_THROTTLE_MS = 2000;

    if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
      console.log(`[useTerminalFlowControl] Resync throttled (${timeSinceLastResync}ms since last, need ${RESYNC_THROTTLE_MS}ms)`);
      return;
    }

    if (urgent) {
      console.log(`[useTerminalFlowControl] Urgent resync bypassing throttle (${timeSinceLastResync}ms since last)`);
    }

    try {
      console.log(`[useTerminalFlowControl] Requesting full resync with current dimensions: ${currentTerminal.cols}x${currentTerminal.rows}`);
      lastResyncTimeRef.current = now;
      isResyncingRef.current = true;
      waitingForPaneResponseRef.current = true;

      dimensionSyncRef.current = {
        cols: currentTerminal.cols,
        rows: currentTerminal.rows,
      };

      const currentPaneReq = create(CurrentPaneRequestSchema, {
        lines: 50,
        includeEscapes: true,
        targetCols: currentTerminal.cols,
        targetRows: currentTerminal.rows,
      });

      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "currentPaneRequest",
            value: currentPaneReq,
          },
        })
      );
    } catch (err) {
      handleError(err);
    }
  }, [sessionId, getTerminal, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  // ---- Message dispatch functions ----

  const PASTE_CHUNK_SIZE = 512; // bytes — fits within tmux's pty write buffer
  const CHUNK_DELAY_MS = 10;   // ms between chunks — yields event loop without stalling input

  const sendInput = useCallback((input: string) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      onDrop?.();
      return;
    }

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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError, onDrop]);

  const resize = useCallback((cols: number, rows: number, force: boolean = false) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("Cannot resize terminal: stream not connected");
      return;
    }

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
        lastSentDimsRef.current = { cols, rows };

        // After resizing, request fresh terminal content
        paneRequestTimerRef.current = setTimeout(() => {
          paneRequestTimerRef.current = null;
          if (!pushMessageRef.current || !isConnectedRef.current) return;
          try {
            console.log(`[useTerminalFlowControl] Requesting fresh pane content after resize`);
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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const requestScrollback = useCallback((fromSequence: number, limit: number) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("Cannot request scrollback: stream not connected");
      return;
    }

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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const sendFlowControl = useCallback((paused: boolean, watermark?: number) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("Cannot send flow control: stream not connected");
      return;
    }

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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const markResyncComplete = useCallback(() => {
    isResyncingRef.current = false;
  }, []);

  const markPaneResponseReceived = useCallback(() => {
    waitingForPaneResponseRef.current = false;
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
