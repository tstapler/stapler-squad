"use client";

import { useRef, useCallback, useEffect } from "react";
import { TerminalData, TerminalDataSchema, TerminalInput, TerminalInputSchema, TerminalResize, TerminalResizeSchema, ScrollbackRequest, ScrollbackRequestSchema, CurrentPaneRequest, CurrentPaneRequestSchema, FlowControl, FlowControlSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import type { Terminal } from '@xterm/xterm';

export interface UseTerminalFlowControlOptions {
  sessionId: string;
  getTerminal: () => Terminal | null;
  /** Push a message onto the connection queue. Stored via ref to avoid stale closures. */
  pushMessageRef: React.MutableRefObject<((msg: TerminalData) => void) | null>;
  isConnectedRef: React.MutableRefObject<boolean>;
  onError?: (error: Error) => void;
}

export interface UseTerminalFlowControlResult {
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number) => void;
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
}: UseTerminalFlowControlOptions): UseTerminalFlowControlResult {
  // Resync state machine refs
  const isResyncingRef = useRef(false);
  const waitingForPaneResponseRef = useRef(false);
  const lastResyncTimeRef = useRef<number>(0);
  const lastResizeTimeRef = useRef<number>(0);
  const pendingResizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dimensionSyncRef = useRef<{ cols?: number; rows?: number }>({});

  // Cancel any pending deferred resize timer when the component unmounts to prevent
  // the timer callback from firing against a torn-down component/connection.
  useEffect(() => {
    return () => {
      if (pendingResizeTimerRef.current) {
        clearTimeout(pendingResizeTimerRef.current);
        pendingResizeTimerRef.current = null;
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
    if (!pushMessageRef.current || !isConnectedRef.current) return;

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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const resize = useCallback((cols: number, rows: number) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("Cannot resize terminal: stream not connected");
      return;
    }

    // Cancel any previously deferred resize — we have newer dimensions now.
    if (pendingResizeTimerRef.current) {
      clearTimeout(pendingResizeTimerRef.current);
      pendingResizeTimerRef.current = null;
    }

    const now = Date.now();
    const timeSinceLastResize = now - lastResizeTimeRef.current;
    const THROTTLE_MS = 200;

    // Inner send: stamps the timestamp, sends the resize RPC, then requests a
    // fresh pane capture so xterm.js and tmux are guaranteed to agree on content.
    const doSend = () => {
      if (!pushMessageRef.current || !isConnectedRef.current) return;
      lastResizeTimeRef.current = Date.now();
      try {
        console.log(`[useTerminalFlowControl] Sending resize to server: ${cols}x${rows}`);
        pushMessage(
          create(TerminalDataSchema, {
            sessionId,
            data: { case: "resize", value: create(TerminalResizeSchema, { cols, rows }) },
          })
        );

        // After resizing, request fresh terminal content
        setTimeout(() => {
          if (!pushMessageRef.current || !isConnectedRef.current) return;
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
        }, 100);
      } catch (err) {
        handleError(err);
      }
    };

    if (timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
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
