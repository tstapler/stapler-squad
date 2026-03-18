"use client";

import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { TerminalData, TerminalInput, TerminalResize, ScrollbackRequest, CurrentPaneRequest, FlowControl, InputWithEcho, SSPNegotiation, SSPCapabilities } from "@/gen/session/v1/events_pb";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { createAuthInterceptor } from "@/lib/config";
import { useEffect, useRef, useState, useCallback } from "react";
import { StateApplicator } from "@/lib/terminal/StateApplicator";
import { EchoOverlay } from "@/lib/terminal/EchoOverlay";
import { decompressLZMA, isLZMACompressed } from "@/lib/compression/lzma";
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

// Queue to manage outgoing terminal messages
class MessageQueue {
  private queue: TerminalData[] = [];
  private resolve: ((value: TerminalData) => void) | null = null;
  private closed = false;

  push(msg: TerminalData) {
    if (this.closed) return;

    if (this.resolve) {
      this.resolve(msg);
      this.resolve = null;
    } else {
      this.queue.push(msg);
    }
  }

  async *[Symbol.asyncIterator]() {
    while (!this.closed || this.queue.length > 0) {
      if (this.queue.length > 0) {
        yield this.queue.shift()!;
      } else {
        const msg = await new Promise<TerminalData>((resolve) => {
          this.resolve = resolve;
        });
        // Don't yield sentinel messages (empty messages used to unblock iterator)
        if (msg.sessionId !== "" || msg.data.case !== undefined) {
          yield msg;
        }
      }
    }
  }

  close() {
    this.closed = true;
    if (this.resolve) {
      // Force unblock the iterator with a sentinel message
      // This message will be filtered out by the iterator and not sent to the server
      this.resolve(new TerminalData({ sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
  }
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
  const [output, setOutput] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);
  const [sspNegotiated, setSspNegotiated] = useState(false);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false); // Track connection state in ref for callbacks
  const isResyncingRef = useRef(false); // Prevent disconnect during resync
  const waitingForPaneResponseRef = useRef(false); // Skip deltas until pane response
  const lastResizeTimeRef = useRef<number>(0); // Timestamp of last resize message sent
  const lastResyncTimeRef = useRef<number>(0); // Timestamp of last resync request
  const stateApplicatorRef = useRef<StateApplicator | null>(null);
  const dimensionSyncRef = useRef<{cols?: number, rows?: number}>({});

  // WebSocket message recording for debugging terminal flickering
  interface RecordedMessage {
    timestamp: number;
    type: 'raw' | 'state' | 'diff';
    data: Uint8Array;
    decoded: string;
    sequenceNumber?: bigint;
  }

  const recordedMessagesRef = useRef<RecordedMessage[]>([]);
  const isRecordingRef = useRef(false);

  // SSP: Echo overlay and tracking refs
  const echoOverlayRef = useRef<EchoOverlay | null>(null);
  const echoCounterRef = useRef<bigint>(BigInt(0));
  const echoTimestampsRef = useRef<Map<bigint, number>>(new Map()); // Track when echoes were sent for RTT calculation
  const clientRef = useRef(createPromiseClient(
    SessionService,
    createWebsocketBasedTransport({
      baseUrl,
      useBinaryFormat: true, // WebSocket supports binary format
      interceptors: [createAuthInterceptor()],
    })
  ));

  // Performance optimization: Adaptive batching with requestAnimationFrame
  // This automatically syncs with display refresh rate (60Hz/120Hz/144Hz)
  const outputBufferRef = useRef<string[]>([]);
  const pendingUpdateRef = useRef<number | null>(null); // RAF ID instead of timeout
  const textDecoderRef = useRef(new TextDecoder()); // Reuse decoder for performance
  const bufferSizeRef = useRef<number>(0); // Track buffer size for adaptive flushing

  // Sync ref with state
  useEffect(() => {
    isConnectedRef.current = isConnected;
  }, [isConnected]);

  // Flush buffered output to state (adaptive batching)
  const flushOutputBuffer = useCallback(() => {
    if (outputBufferRef.current.length > 0) {
      const bufferedText = outputBufferRef.current.join("");
      outputBufferRef.current = [];
      bufferSizeRef.current = 0;
      setOutput((prev) => prev + bufferedText);
    }
    pendingUpdateRef.current = null;
  }, []);

  // Schedule output update with adaptive batching strategy:
  // - Uses requestAnimationFrame for display-synchronized updates (60-144fps)
  // - Flushes immediately if buffer exceeds 4KB (prevents lag on large bursts)
  // - Automatically adapts to high refresh rate displays (120Hz/144Hz)
  const scheduleOutputUpdate = useCallback((text: string) => {
    outputBufferRef.current.push(text);
    bufferSizeRef.current += text.length;

    // Immediate flush for large buffers (>4KB) to prevent lag
    // This handles burst scenarios like build errors or code generation
    if (bufferSizeRef.current > 4096) {
      if (pendingUpdateRef.current !== null) {
        cancelAnimationFrame(pendingUpdateRef.current);
        pendingUpdateRef.current = null;
      }
      flushOutputBuffer();
      return;
    }

    // Otherwise, use RAF for display-synchronized batching
    if (pendingUpdateRef.current === null) {
      pendingUpdateRef.current = requestAnimationFrame(flushOutputBuffer);
    }
  }, [flushOutputBuffer]);

  // Recording functions for debugging terminal flickering
  const startRecording = useCallback(() => {
    recordedMessagesRef.current = [];
    isRecordingRef.current = true;
    console.log('[Recording] Started terminal output recording');
  }, []);

  const stopRecording = useCallback(() => {
    isRecordingRef.current = false;
    const blob = new Blob([JSON.stringify(recordedMessagesRef.current, null, 2)],
      { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `terminal-recording-${Date.now()}.json`;
    a.click();
    console.log('[Recording] Saved recording with', recordedMessagesRef.current.length, 'messages');
  }, []);

  // Request full resync from server (used for desync recovery)
  // MUST be defined before connect() since it's called from within connect's async function
  const requestFullResync = useCallback((urgent: boolean = false) => {
    if (!messageQueueRef.current || !isConnectedRef.current) {
      console.warn("[useTerminalStream] Cannot request resync: stream not connected");
      return;
    }

    const currentTerminal = getTerminal?.();
    if (!currentTerminal) {
      console.warn("[useTerminalStream] Cannot request resync: terminal not available");
      return;
    }

    // Throttle resync requests to prevent spam (max 1 per 2 seconds)
    // But allow urgent requests (dimension mismatches) to bypass throttling
    const now = Date.now();
    const timeSinceLastResync = now - lastResyncTimeRef.current;
    const RESYNC_THROTTLE_MS = 2000; // 2 second throttle

    if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
      console.log(`[useTerminalStream] Resync throttled (${timeSinceLastResync}ms since last, need ${RESYNC_THROTTLE_MS}ms)`);
      return;
    }

    if (urgent) {
      console.log(`[useTerminalStream] Urgent resync bypassing throttle (${timeSinceLastResync}ms since last)`);
    }

    try {
      console.log(`[useTerminalStream] Requesting full resync with current dimensions: ${currentTerminal.cols}x${currentTerminal.rows}`);
      lastResyncTimeRef.current = now;
      isResyncingRef.current = true; // Mark as resyncing to prevent disconnect
      waitingForPaneResponseRef.current = true; // Skip deltas until we get fresh state

      // Store dimensions for consistency checking
      dimensionSyncRef.current = {
        cols: currentTerminal.cols,
        rows: currentTerminal.rows
      };

      const currentPaneReq = new CurrentPaneRequest({
        lines: 50, // Get last 50 lines
        includeEscapes: true, // Preserve colors and formatting
        targetCols: currentTerminal.cols,
        targetRows: currentTerminal.rows,
        streamingMode: streamingMode, // Pass streaming mode to server
      });

      messageQueueRef.current.push(
        new TerminalData({
          sessionId,
          data: {
            case: "currentPaneRequest",
            value: currentPaneReq,
          },
        })
      );
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      console.error("[useTerminalStream] Failed to request resync:", error);
      setError(error);
      onError?.(error);
    }
  }, [sessionId, getTerminal, onError]);

  const connect = useCallback(async (overrideCols?: number, overrideRows?: number) => {
    if (isConnectedRef.current || !sessionId) {
      return;
    }

    // Use provided dimensions (from connect call) or fall back to initial values or current terminal
    let targetCols = overrideCols ?? initialCols;
    let targetRows = overrideRows ?? initialRows;

    // If no dimensions provided, try to get from current terminal
    if (targetCols === undefined || targetRows === undefined) {
      const currentTerminal = getTerminal?.();
      if (currentTerminal) {
        targetCols = currentTerminal.cols;
        targetRows = currentTerminal.rows;
        console.log(`[useTerminalStream] Using current terminal dimensions: ${targetCols}x${targetRows}`);
      }
    }

    // Reset disconnecting flag on connect
    isDisconnectingRef.current = false;

    // Note: StateApplicator is lazily initialized when first state arrives
    // This avoids race conditions with terminal ref availability

    try {
      abortControllerRef.current = new AbortController();
      messageQueueRef.current = new MessageQueue();

      // IMPROVED: Send initial handshake WITH dimensions (single message)
      // This eliminates the need for a separate CurrentPaneRequest message
      // Server can resize tmux and capture content immediately with correct dimensions
      const currentPaneReq = new CurrentPaneRequest({
        lines: 50, // Get last 50 lines (typical terminal viewport)
        includeEscapes: true, // Preserve colors and formatting
        streamingMode: streamingMode, // Pass streaming mode to server
      });

      // Set target dimensions if available (prevents garbled output on first load)
      // Server will resize tmux pane to match before capturing content
      if (targetCols !== undefined && targetRows !== undefined) {
        currentPaneReq.targetCols = targetCols;
        currentPaneReq.targetRows = targetRows;
        console.log(`[useTerminalStream] Sending handshake with dimensions: ${targetCols}x${targetRows}`);
      } else {
        console.warn(`[useTerminalStream] No terminal dimensions available for handshake`);
      }

      // Send as first and only handshake message
      messageQueueRef.current.push(
        new TerminalData({
          sessionId,
          data: {
            case: "currentPaneRequest",
            value: currentPaneReq,
          },
        })
      );

      // Create bidirectional stream
      const stream = clientRef.current.streamTerminal(
        messageQueueRef.current,
        { signal: abortControllerRef.current.signal }
      );

      setError(null);

      // Start reading terminal output in background
      (async () => {
        try {
          let firstMessage = true;
          for await (const msg of stream) {
            // Set connected after receiving first message
            if (firstMessage) {
              setIsConnected(true);
              setScrollbackLoaded(true); // Mark as loaded immediately (no need for scrollback loading state)
              firstMessage = false;
            }

            if (msg.data.case === "state") {
              // Handle complete terminal state (MOSH-style) with lazy initialization
              // Initialize StateApplicator on first state if terminal is now ready
              if (!stateApplicatorRef.current) {
                const currentTerminal = getTerminal?.();
                if (currentTerminal) {
                  stateApplicatorRef.current = new StateApplicator(currentTerminal);
                  console.log('[useTerminalStream] State applicator lazily initialized on first state');

                  // PHASE 1: Set up dimension mismatch handler
                  stateApplicatorRef.current.setOnDimensionMismatch((expectedCols, expectedRows, actualCols, actualRows) => {
                    console.warn(
                      `[useTerminalStream] DIMENSION MISMATCH DETECTED: ` +
                      `server sent ${expectedCols}x${expectedRows}, ` +
                      `but terminal is ${actualCols}x${actualRows}. ` +
                      `Requesting resync with correct dimensions...`
                    );

                    // Request immediate resync with correct terminal dimensions
                    if (!isResyncingRef.current) {
                      console.log(`[useTerminalStream] Triggering resync due to dimension mismatch`);
                      requestFullResync(true); // Mark as urgent
                    } else {
                      console.log(`[useTerminalStream] Already resyncing, skipping duplicate resync request`);
                    }
                  });
                } else {
                  console.warn('[useTerminalStream] Received state but terminal not ready yet');
                  continue; // Skip this state and wait for terminal to be ready
                }
              }

              // If waiting for pane response, this is likely the initial state
              if (waitingForPaneResponseRef.current) {
                console.log('[useTerminalStream] Received complete terminal state as pane response');
                waitingForPaneResponseRef.current = false;
                isResyncingRef.current = false;
              }

              if (stateApplicatorRef.current) {
                // Check for dimension consistency before applying state (for logging/monitoring)
                const currentTerminal = getTerminal?.();
                if (currentTerminal && msg.data.value.dimensions) {
                  const stateCols = Number(msg.data.value.dimensions.cols);
                  const stateRows = Number(msg.data.value.dimensions.rows);

                  if (stateCols !== currentTerminal.cols || stateRows !== currentTerminal.rows) {
                    console.log(
                      `[useTerminalStream] State dimension difference: ` +
                      `state=${stateCols}x${stateRows}, terminal=${currentTerminal.cols}x${currentTerminal.rows}. ` +
                      `StateApplicator will handle resize automatically.`
                    );
                  }
                }

                // Apply complete terminal state (MOSH-style - always succeeds)
                const success = stateApplicatorRef.current.applyState(msg.data.value);
                if (!success) {
                  // State was ignored (old/duplicate sequence) - this is normal
                  console.log(
                    `[useTerminalStream] State sequence ${msg.data.value.sequence} ignored ` +
                    `(current: ${stateApplicatorRef.current.getCurrentSequence()})`
                  );
                } else {
                  // State applied successfully
                  console.log(
                    `[useTerminalStream] Applied state sequence ${msg.data.value.sequence} ` +
                    `(${msg.data.value.lines.length} lines)`
                  );
                }
              }
            } else if (msg.data.case === "diff") {
              // SSP: Handle terminal diff (Mosh-style minimal ANSI sequences)
              // Initialize StateApplicator on first diff if terminal is now ready
              if (!stateApplicatorRef.current) {
                const currentTerminal = getTerminal?.();
                if (currentTerminal) {
                  stateApplicatorRef.current = new StateApplicator(currentTerminal);
                  console.log('[useTerminalStream] State applicator lazily initialized on first diff');

                  // PHASE 1: Set up dimension mismatch handler
                  stateApplicatorRef.current.setOnDimensionMismatch((expectedCols, expectedRows, actualCols, actualRows) => {
                    console.warn(
                      `[useTerminalStream] DIMENSION MISMATCH DETECTED: ` +
                      `server sent ${expectedCols}x${expectedRows}, ` +
                      `but terminal is ${actualCols}x${actualRows}. ` +
                      `Requesting resync with correct dimensions...`
                    );

                    // Request immediate resync with correct terminal dimensions
                    if (!isResyncingRef.current) {
                      console.log(`[useTerminalStream] Triggering resync due to dimension mismatch`);
                      requestFullResync(true); // Mark as urgent
                    } else {
                      console.log(`[useTerminalStream] Already resyncing, skipping duplicate resync request`);
                    }
                  });

                  // Wire up echo overlay if predictive echo is enabled
                  if (enablePredictiveEcho && echoOverlayRef.current) {
                    stateApplicatorRef.current.setEchoOverlay(echoOverlayRef.current);
                    stateApplicatorRef.current.setOnEchoAck((ack) => {
                      // Calculate RTT from stored timestamp
                      const sendTime = echoTimestampsRef.current.get(ack.echoAckNum);
                      if (sendTime) {
                        const latencyMs = Date.now() - sendTime;
                        echoTimestampsRef.current.delete(ack.echoAckNum);
                        console.log(`[useTerminalStream] Echo ack ${ack.echoAckNum}: RTT=${latencyMs}ms`);
                        onEchoAck?.(ack.echoAckNum, latencyMs);
                      }
                    });
                  }
                } else {
                  console.warn('[useTerminalStream] Received diff but terminal not ready yet');
                  continue;
                }
              }

              // Apply diff to terminal
              const diff = msg.data.value;
              const success = stateApplicatorRef.current.applyDiff(diff);

              if (!success) {
                // Sequence mismatch - need resync
                console.warn(
                  `[useTerminalStream] Diff sequence mismatch: ` +
                  `diff.fromSequence=${diff.fromSequence}, ` +
                  `current=${stateApplicatorRef.current.getCurrentSequence()}. ` +
                  `Requesting resync.`
                );
                requestFullResync(true); // Urgent resync
              } else {
                console.log(
                  `[useTerminalStream] Applied diff ${diff.fromSequence}→${diff.toSequence} ` +
                  `(${diff.changedCells} cells changed, ${diff.diffBytes?.length || 0} bytes)`
                );
              }
            } else if (msg.data.case === "sspNegotiation") {
              // SSP: Handle capability negotiation response
              const negotiation = msg.data.value;
              if (!negotiation.isRequest && negotiation.negotiated) {
                console.log('[useTerminalStream] SSP capabilities negotiated:', {
                  predictiveEcho: negotiation.negotiated.supportsPredictiveEcho,
                  diffUpdates: negotiation.negotiated.supportsDiffUpdates,
                  compression: negotiation.negotiated.compressionAlgorithms,
                });
                setSspNegotiated(true);

                // Initialize echo overlay if predictive echo is enabled
                if (negotiation.negotiated.supportsPredictiveEcho && enablePredictiveEcho) {
                  const currentTerminal = getTerminal?.();
                  if (currentTerminal && !echoOverlayRef.current) {
                    echoOverlayRef.current = new EchoOverlay({ debug: true });
                    echoOverlayRef.current.attach(currentTerminal);
                    console.log('[useTerminalStream] Echo overlay initialized after SSP negotiation');
                  }
                }
              }
            } else if (msg.data.case === "output") {
              // Handle raw output (may be compressed in raw-compressed mode)
              const rawData = msg.data.value.data;

              // Detect and decompress LZMA-compressed data
              let decodedData: Uint8Array;
              if (streamingMode === "raw-compressed" && isLZMACompressed(rawData)) {
                try {
                  // Decompress LZMA data
                  decodedData = await decompressLZMA(rawData);
                  if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
                    console.debug(`[useTerminalStream] Decompressed output: ${rawData.length} → ${decodedData.length} bytes`);
                  }
                } catch (err) {
                  console.error(`[useTerminalStream] LZMA decompression failed, using raw data:`, err);
                  decodedData = rawData; // Fallback to raw on decompression error
                }
              } else {
                decodedData = rawData;
              }

              const text = textDecoderRef.current.decode(decodedData, { stream: true });

              // Record the message if recording is enabled
              if (isRecordingRef.current) {
                recordedMessagesRef.current.push({
                  timestamp: Date.now(),
                  type: 'raw',
                  data: decodedData,
                  decoded: text,
                });
              }

              // Only log if debug mode is enabled
              if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
                console.debug(`[useTerminalStream] Received output: ${text.length} bytes`);
              }

              // Use callback if provided (better performance - no React state updates)
              if (onOutput) {
                onOutput(text);
              } else {
                // Fallback: Use batched updates for backward compatibility
                scheduleOutputUpdate(text);
              }
            } else if (msg.data.case === "currentPaneResponse") {
              // DEPRECATED: Server now sends full-sync deltas instead of raw currentPaneResponse
              // This case is kept for backward compatibility with old server versions
              console.warn("[useTerminalStream] Received deprecated currentPaneResponse - server should send full-sync delta instead");

              const response = msg.data.value;
              const content = textDecoderRef.current.decode(response.content);

              console.log(`[useTerminalStream] Received current pane (deprecated): ${content.length} bytes, ` +
                          `cursor at (${response.cursorX},${response.cursorY}), ` +
                          `pane size: ${response.paneWidth}x${response.paneHeight}`);

              // Initialize or reinitialize state applicator when we get fresh pane content
              const currentTerminal = getTerminal?.();
              if (currentTerminal) {
                // Create fresh state applicator with proper terminal reference
                stateApplicatorRef.current = new StateApplicator(currentTerminal);
                console.log("[useTerminalStream] Created fresh state applicator after receiving current pane");

                // PHASE 1: Set up dimension mismatch handler
                stateApplicatorRef.current.setOnDimensionMismatch((expectedCols, expectedRows, actualCols, actualRows) => {
                  console.warn(
                    `[useTerminalStream] DIMENSION MISMATCH DETECTED: ` +
                    `server sent ${expectedCols}x${expectedRows}, ` +
                    `but terminal is ${actualCols}x${actualRows}. ` +
                    `Requesting resync with correct dimensions...`
                  );

                  // Request immediate resync with correct terminal dimensions
                  if (!isResyncingRef.current) {
                    console.log(`[useTerminalStream] Triggering resync due to dimension mismatch`);
                    requestFullResync(true); // Mark as urgent
                  } else {
                    console.log(`[useTerminalStream] Already resyncing, skipping duplicate resync request`);
                  }
                });
              }

              // Write current pane content to terminal (raw write - may cause parsing errors!)
              if (onScrollbackReceived) {
                onScrollbackReceived(content);
              }

              // Mark resync as complete
              isResyncingRef.current = false;

              // Clear pane response wait flag - now safe to process deltas
              waitingForPaneResponseRef.current = false;
              console.log("[useTerminalStream] Pane response received - ready to process deltas");
            } else if (msg.data.case === "scrollbackResponse") {
              // Keep scrollback support for "load more history" feature
              // Optimize: Use array and join instead of concatenation
              const chunks: string[] = [];
              for (const chunk of msg.data.value.chunks) {
                const text = textDecoderRef.current.decode(chunk.data);
                chunks.push(text);
              }
              const scrollbackText = chunks.join("");

              // Extract metadata for smart caching and UI state
              const metadata: ScrollbackMetadata = {
                hasMore: msg.data.value.hasMore,
                oldestSequence: Number(msg.data.value.oldestSequence),
                newestSequence: Number(msg.data.value.newestSequence),
                totalLines: Number(msg.data.value.totalLines),
              };

              console.log(`[useTerminalStream] Scrollback metadata:`, metadata);

              // Call callback to write directly to terminal with metadata
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
          const error = err instanceof Error ? err : new Error(String(err));
          setError(error);
          onError?.(error);
        } finally {
          setIsConnected(false);
        }
      })();
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      setError(error);
      onError?.(error);
      setIsConnected(false);
    }
  }, [sessionId, getTerminal, scrollbackLines, onError, onScrollbackReceived, onOutput, scheduleOutputUpdate, requestFullResync]); // Include getTerminal, onOutput, and requestFullResync

  const disconnect = useCallback(async () => {
    // Prevent double-disconnect or disconnect during resync
    if (isDisconnectingRef.current || isResyncingRef.current) {
      if (isResyncingRef.current) {
        console.log("[useTerminalStream] Delaying disconnect - resync in progress");
        // Wait for resync to complete, then retry disconnect
        setTimeout(() => disconnect(), 500);
      }
      return;
    }
    isDisconnectingRef.current = true;

    // Graceful shutdown: close message queue first to stop sending
    // This allows the server to send EndStreamResponse before closing
    if (messageQueueRef.current) {
      messageQueueRef.current.close();
      messageQueueRef.current = null;
    }

    // Give the stream time to close gracefully (wait for EndStreamResponse)
    // Use Promise-based waiting without polling to avoid setInterval violations
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        if (abortControllerRef.current) {
          console.debug("[useTerminalStream] Timeout waiting for graceful close, forcing abort");
          abortControllerRef.current.abort();
          abortControllerRef.current = null;
        }
        resolve();
      }, 1000); // 1 second timeout for graceful close

      // Use event-driven approach instead of polling
      // Check immediately if already disconnected
      if (!isConnectedRef.current) {
        clearTimeout(timeout);
        resolve();
        return;
      }

      // Otherwise wait for timeout - the connection state change will trigger cleanup
      // This avoids the 100ms polling interval that causes performance violations
    });

    setIsConnected(false);
    isDisconnectingRef.current = false;
  }, []); // No dependencies - use refs for all state checks

  const sendInput = useCallback(
    (input: string) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        return;
      }

      try {
        const inputBytes = new TextEncoder().encode(input);

        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "input",
              value: new TerminalInput({ data: inputBytes }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, onError]
  );

  // SSP: Send input with predictive echo tracking
  // Returns the echo number for tracking (can be used to correlate with acks)
  const sendInputWithEcho = useCallback(
    (input: string): bigint => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        return BigInt(0);
      }

      try {
        // Increment echo counter
        echoCounterRef.current = echoCounterRef.current + BigInt(1);
        const echoNum = echoCounterRef.current;
        const clientTimestamp = Date.now();

        // Store timestamp for RTT calculation when ack arrives
        echoTimestampsRef.current.set(echoNum, clientTimestamp);

        // Show predictive echo immediately (if echo overlay is enabled)
        if (echoOverlayRef.current && enablePredictiveEcho) {
          echoOverlayRef.current.showPredictiveEcho(input);
        }

        const inputBytes = new TextEncoder().encode(input);

        // Send input with echo tracking
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "inputEcho",
              value: new InputWithEcho({
                data: inputBytes,
                echoNum: echoNum,
                clientTimestampMs: BigInt(clientTimestamp),
              }),
            },
          })
        );

        return echoNum;
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
        return BigInt(0);
      }
    },
    [sessionId, onError, enablePredictiveEcho]
  );

  const resize = useCallback(
    (cols: number, rows: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot resize terminal: stream not connected");
        return;
      }

      // Reduced throttle to 200ms (from 1s) for more responsive resizing
      // This prevents feedback loops while still allowing timely dimension updates
      const now = Date.now();
      const timeSinceLastResize = now - lastResizeTimeRef.current;
      const THROTTLE_MS = 200; // 200ms throttle (down from 1000ms)

      if (timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
        console.log(`[useTerminalStream] Resize throttled (${timeSinceLastResize}ms since last, need ${THROTTLE_MS}ms)`);
        return;
      }

      try {
        console.log(`[useTerminalStream] Sending resize to server: ${cols}x${rows}`);
        lastResizeTimeRef.current = now;
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "resize",
              value: new TerminalResize({ cols, rows }),
            },
          })
        );

        // After resizing, request fresh terminal content from tmux
        // Add a small delay to allow tmux to reflow content at new dimensions
        // Without this delay, we capture content at the old width
        setTimeout(() => {
          if (!messageQueueRef.current || !isConnectedRef.current) return;

          console.log(`[useTerminalStream] Requesting fresh pane content after resize`);
          messageQueueRef.current.push(
            new TerminalData({
              sessionId,
              data: {
                case: "currentPaneRequest",
                value: new CurrentPaneRequest({
                  lines: 50,
                  includeEscapes: true,
                  targetCols: cols,
                  targetRows: rows,
                  streamingMode: streamingMode || "raw-compressed",
                }),
              },
            })
          );
        }, 100); // 100ms delay to allow tmux to reflow
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, streamingMode, onError]
  );

  const requestScrollback = useCallback(
    (fromSequence: number, limit: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot request scrollback: stream not connected");
        return;
      }

      try {
        console.log(`[useTerminalStream] Requesting scrollback: fromSeq=${fromSequence}, limit=${limit}`);
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "scrollbackRequest",
              value: new ScrollbackRequest({
                fromSequence: BigInt(fromSequence),
                limit,
              }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, onError]
  );

  const sendFlowControl = useCallback(
    (paused: boolean, watermark?: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot send flow control: stream not connected");
        return;
      }

      try {
        console.log(`[useTerminalStream] Sending flow control: paused=${paused}, watermark=${watermark || 'N/A'}`);
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "flowControl",
              value: new FlowControl({
                paused,
                watermark: watermark !== undefined ? BigInt(watermark) : undefined,
              }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, onError]
  );

  // Helper to check if StateApplicator is currently applying a state
  const getIsApplyingState = useCallback(() => {
    return stateApplicatorRef.current?.getIsApplyingState() ?? false;
  }, []);

  // Auto-connect on mount (if enabled)
  useEffect(() => {
    if (autoConnect) {
      connect();
    }
    return () => {
      // Cleanup: Cancel any pending RAF updates
      if (pendingUpdateRef.current !== null) {
        cancelAnimationFrame(pendingUpdateRef.current);
        pendingUpdateRef.current = null;
      }
      // Flush any remaining buffered output
      flushOutputBuffer();

      // Reset state applicator to prevent stale state
      if (stateApplicatorRef.current) {
        stateApplicatorRef.current.resetSequence();
        stateApplicatorRef.current = null;
      }

      // SSP: Cleanup echo overlay
      if (echoOverlayRef.current) {
        echoOverlayRef.current.detach();
        echoOverlayRef.current = null;
      }
      echoTimestampsRef.current.clear();

      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, autoConnect]); // Only reconnect if sessionId or autoConnect changes

  return {
    output,
    isConnected,
    error,
    sendInput,
    sendInputWithEcho,
    resize,
    connect,
    disconnect,
    scrollbackLoaded,
    requestScrollback,
    sendFlowControl,
    getIsApplyingState,
    sspNegotiated,
    startRecording,
    stopRecording,
  };
}
