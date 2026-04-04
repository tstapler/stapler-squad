"use client";

import { useRef, useState, useCallback } from "react";
import { TerminalData, TerminalDataSchema, TerminalInput, TerminalInputSchema, TerminalResize, TerminalResizeSchema, ScrollbackRequest, ScrollbackRequestSchema, CurrentPaneRequest, CurrentPaneRequestSchema, FlowControl, FlowControlSchema, InputWithEcho, InputWithEchoSchema } from "@/gen/session/v1/events_pb";
import { create } from "@bufbuild/protobuf";
import { StateApplicator } from "@/lib/terminal/StateApplicator";
import { EchoOverlay } from "@/lib/terminal/EchoOverlay";
import type { Terminal } from '@xterm/xterm';

export interface UseTerminalFlowControlOptions {
  sessionId: string;
  streamingMode: "raw" | "raw-compressed" | "state" | "hybrid" | "ssp";
  enablePredictiveEcho: boolean;
  getTerminal: () => Terminal | null;
  /** Push a message onto the connection queue. Stored via ref to avoid stale closures. */
  pushMessageRef: React.MutableRefObject<((msg: TerminalData) => void) | null>;
  isConnectedRef: React.MutableRefObject<boolean>;
  onError?: (error: Error) => void;
  onEchoAck?: (echoNum: bigint, latencyMs: number) => void;
}

export interface UseTerminalFlowControlResult {
  sendInput: (input: string) => void;
  sendInputWithEcho: (input: string) => bigint;
  resize: (cols: number, rows: number) => void;
  requestScrollback: (fromSequence: number, limit: number) => void;
  sendFlowControl: (paused: boolean, watermark?: number) => void;
  requestFullResync: (urgent?: boolean) => void;
  getIsApplyingState: () => boolean;
  sspNegotiated: boolean;
  setSspNegotiated: (value: boolean) => void;

  // Internal methods used by the message handler in the parent composition hook
  handleStateMessage: (msg: any) => void;
  handleDiffMessage: (msg: any) => void;
  handleSspNegotiation: (msg: any) => void;
  handleCurrentPaneResponse: (msg: any) => void;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  getIsResyncingRef: () => React.MutableRefObject<boolean>;
  getWaitingForPaneResponseRef: () => React.MutableRefObject<boolean>;
}

/**
 * useTerminalFlowControl - Resync logic, resize throttling, message dispatch,
 * SSP echo tracking, and StateApplicator integration.
 *
 * Bug Risk 1 mitigation: stateApplicatorRef and isResyncingRef are kept in the
 * same hook to preserve synchronous ref access during the SSP state machine.
 *
 * Bug Risk 3 mitigation: pushMessage is accessed via pushMessageRef.current
 * (not captured in closures) to prevent stale closure issues on reconnect.
 */
export function useTerminalFlowControl({
  sessionId,
  streamingMode,
  enablePredictiveEcho,
  getTerminal,
  pushMessageRef,
  isConnectedRef,
  onError,
  onEchoAck,
}: UseTerminalFlowControlOptions): UseTerminalFlowControlResult {
  const [sspNegotiated, setSspNegotiated] = useState(false);

  // Resync state machine refs (Bug Risk 1: keep these together)
  const isResyncingRef = useRef(false);
  const waitingForPaneResponseRef = useRef(false);
  const lastResyncTimeRef = useRef<number>(0);
  const lastResizeTimeRef = useRef<number>(0);
  const dimensionSyncRef = useRef<{ cols?: number; rows?: number }>({});

  // StateApplicator (lazy init) - kept in same hook as resync refs per Bug Risk 1
  const stateApplicatorRef = useRef<StateApplicator | null>(null);

  // SSP echo tracking
  const echoOverlayRef = useRef<EchoOverlay | null>(null);
  const echoCounterRef = useRef<bigint>(BigInt(0));
  const echoTimestampsRef = useRef<Map<bigint, number>>(new Map());

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
        streamingMode: streamingMode,
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
  }, [sessionId, getTerminal, streamingMode, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  // ---- Dimension mismatch handler (shared between state and diff init) ----

  const setupDimensionMismatchHandler = useCallback((applicator: StateApplicator) => {
    applicator.setOnDimensionMismatch((expectedCols, expectedRows, actualCols, actualRows) => {
      console.warn(
        `[useTerminalFlowControl] DIMENSION MISMATCH DETECTED: ` +
        `server sent ${expectedCols}x${expectedRows}, ` +
        `but terminal is ${actualCols}x${actualRows}. ` +
        `Requesting resync with correct dimensions...`
      );

      if (!isResyncingRef.current) {
        console.log(`[useTerminalFlowControl] Triggering resync due to dimension mismatch`);
        requestFullResync(true);
      } else {
        console.log(`[useTerminalFlowControl] Already resyncing, skipping duplicate resync request`);
      }
    });
  }, [requestFullResync]);

  // ---- StateApplicator lazy initialization ----

  const getOrInitStateApplicator = useCallback((messageType: 'state' | 'diff'): StateApplicator | null => {
    if (stateApplicatorRef.current) return stateApplicatorRef.current;

    const currentTerminal = getTerminal();
    if (!currentTerminal) {
      console.warn(`[useTerminalFlowControl] Received ${messageType} but terminal not ready yet`);
      return null;
    }

    const applicator = new StateApplicator(currentTerminal);
    console.log(`[useTerminalFlowControl] State applicator lazily initialized on first ${messageType}`);

    setupDimensionMismatchHandler(applicator);

    // Wire echo overlay for diff messages if predictive echo is enabled
    if (messageType === 'diff' && enablePredictiveEcho && echoOverlayRef.current) {
      applicator.setEchoOverlay(echoOverlayRef.current);
      applicator.setOnEchoAck((ack) => {
        const sendTime = echoTimestampsRef.current.get(ack.echoAckNum);
        if (sendTime) {
          const latencyMs = Date.now() - sendTime;
          echoTimestampsRef.current.delete(ack.echoAckNum);
          console.log(`[useTerminalFlowControl] Echo ack ${ack.echoAckNum}: RTT=${latencyMs}ms`);
          onEchoAck?.(ack.echoAckNum, latencyMs);
        }
      });
    }

    stateApplicatorRef.current = applicator;
    return applicator;
  }, [getTerminal, enablePredictiveEcho, onEchoAck, setupDimensionMismatchHandler]);

  // ---- Message handlers (called from parent hook's message loop) ----

  const handleStateMessage = useCallback((msg: any) => {
    const applicator = getOrInitStateApplicator('state');
    if (!applicator) return;

    // If waiting for pane response, this is the initial state
    if (waitingForPaneResponseRef.current) {
      console.log('[useTerminalFlowControl] Received complete terminal state as pane response');
      waitingForPaneResponseRef.current = false;
      isResyncingRef.current = false;
    }

    // Check dimension consistency
    const currentTerminal = getTerminal();
    if (currentTerminal && msg.dimensions) {
      const stateCols = Number(msg.dimensions.cols);
      const stateRows = Number(msg.dimensions.rows);

      if (stateCols !== currentTerminal.cols || stateRows !== currentTerminal.rows) {
        console.log(
          `[useTerminalFlowControl] State dimension difference: ` +
          `state=${stateCols}x${stateRows}, terminal=${currentTerminal.cols}x${currentTerminal.rows}. ` +
          `StateApplicator will handle resize automatically.`
        );
      }
    }

    const success = applicator.applyState(msg);
    if (!success) {
      console.log(
        `[useTerminalFlowControl] State sequence ${msg.sequence} ignored ` +
        `(current: ${applicator.getCurrentSequence()})`
      );
    } else {
      console.log(
        `[useTerminalFlowControl] Applied state sequence ${msg.sequence} ` +
        `(${msg.lines.length} lines)`
      );
    }
  }, [getOrInitStateApplicator, getTerminal]);

  const handleDiffMessage = useCallback((msg: any) => {
    const applicator = getOrInitStateApplicator('diff');
    if (!applicator) return;

    const success = applicator.applyDiff(msg);

    if (!success) {
      console.warn(
        `[useTerminalFlowControl] Diff sequence mismatch: ` +
        `diff.fromSequence=${msg.fromSequence}, ` +
        `current=${applicator.getCurrentSequence()}. ` +
        `Requesting resync.`
      );
      requestFullResync(true);
    } else {
      console.log(
        `[useTerminalFlowControl] Applied diff ${msg.fromSequence}->${msg.toSequence} ` +
        `(${msg.changedCells} cells changed, ${msg.diffBytes?.length || 0} bytes)`
      );
    }
  }, [getOrInitStateApplicator, requestFullResync]);

  const handleSspNegotiation = useCallback((msg: any) => {
    if (!msg.isRequest && msg.negotiated) {
      console.log('[useTerminalFlowControl] SSP capabilities negotiated:', {
        predictiveEcho: msg.negotiated.supportsPredictiveEcho,
        diffUpdates: msg.negotiated.supportsDiffUpdates,
        compression: msg.negotiated.compressionAlgorithms,
      });
      setSspNegotiated(true);

      // Initialize echo overlay if predictive echo is enabled
      if (msg.negotiated.supportsPredictiveEcho && enablePredictiveEcho) {
        const currentTerminal = getTerminal();
        if (currentTerminal && !echoOverlayRef.current) {
          echoOverlayRef.current = new EchoOverlay({ debug: true });
          echoOverlayRef.current.attach(currentTerminal);
          console.log('[useTerminalFlowControl] Echo overlay initialized after SSP negotiation');
        }
      }
    }
  }, [enablePredictiveEcho, getTerminal]);

  const handleCurrentPaneResponse = useCallback((msg: any) => {
    console.warn("[useTerminalFlowControl] Received deprecated currentPaneResponse");

    const currentTerminal = getTerminal();
    if (currentTerminal) {
      stateApplicatorRef.current = new StateApplicator(currentTerminal);
      console.log("[useTerminalFlowControl] Created fresh state applicator after receiving current pane");
      setupDimensionMismatchHandler(stateApplicatorRef.current);
    }

    isResyncingRef.current = false;
    waitingForPaneResponseRef.current = false;
    console.log("[useTerminalFlowControl] Pane response received - ready to process deltas");
  }, [getTerminal, setupDimensionMismatchHandler]);

  // ---- Message dispatch functions ----

  const sendInput = useCallback((input: string) => {
    if (!pushMessageRef.current || !isConnectedRef.current) return;

    try {
      const inputBytes = new TextEncoder().encode(input);
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
  }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const sendInputWithEcho = useCallback((input: string): bigint => {
    if (!pushMessageRef.current || !isConnectedRef.current) return BigInt(0);

    try {
      echoCounterRef.current = echoCounterRef.current + BigInt(1);
      const echoNum = echoCounterRef.current;
      const clientTimestamp = Date.now();

      echoTimestampsRef.current.set(echoNum, clientTimestamp);

      if (echoOverlayRef.current && enablePredictiveEcho) {
        echoOverlayRef.current.showPredictiveEcho(input);
      }

      const inputBytes = new TextEncoder().encode(input);

      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "inputEcho",
            value: create(InputWithEchoSchema, {
              data: inputBytes,
              echoNum: echoNum,
              clientTimestampMs: BigInt(clientTimestamp),
            }),
          },
        })
      );

      return echoNum;
    } catch (err) {
      handleError(err);
      return BigInt(0);
    }
  }, [sessionId, enablePredictiveEcho, pushMessage, pushMessageRef, isConnectedRef, handleError]);

  const resize = useCallback((cols: number, rows: number) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      console.warn("Cannot resize terminal: stream not connected");
      return;
    }

    const now = Date.now();
    const timeSinceLastResize = now - lastResizeTimeRef.current;
    const THROTTLE_MS = 200;

    if (timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
      console.log(`[useTerminalFlowControl] Resize throttled (${timeSinceLastResize}ms since last, need ${THROTTLE_MS}ms)`);
      return;
    }

    try {
      console.log(`[useTerminalFlowControl] Sending resize to server: ${cols}x${rows}`);
      lastResizeTimeRef.current = now;
      pushMessage(
        create(TerminalDataSchema, {
          sessionId,
          data: {
            case: "resize",
            value: create(TerminalResizeSchema, { cols, rows }),
          },
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
                streamingMode: streamingMode || "raw-compressed",
              }),
            },
          })
        );
      }, 100);
    } catch (err) {
      handleError(err);
    }
  }, [sessionId, streamingMode, pushMessage, pushMessageRef, isConnectedRef, handleError]);

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

  const getIsApplyingState = useCallback(() => {
    return stateApplicatorRef.current?.getIsApplyingState() ?? false;
  }, []);

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
    sendInputWithEcho,
    resize,
    requestScrollback,
    sendFlowControl,
    requestFullResync,
    getIsApplyingState,
    sspNegotiated,
    setSspNegotiated,
    handleStateMessage,
    handleDiffMessage,
    handleSspNegotiation,
    handleCurrentPaneResponse,
    markResyncComplete,
    markPaneResponseReceived,
    getIsResyncingRef,
    getWaitingForPaneResponseRef,
  };
}
