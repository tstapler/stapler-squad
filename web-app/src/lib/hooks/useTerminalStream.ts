"use client";

import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { TerminalData, TerminalInput, TerminalResize, ScrollbackRequest } from "@/gen/session/v1/events_pb";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { useEffect, useRef, useState, useCallback } from "react";

interface UseTerminalStreamOptions {
  baseUrl: string;
  sessionId: string;
  scrollbackLines?: number; // Number of lines to request from scrollback
  onError?: (error: Error) => void;
  onScrollbackReceived?: (scrollback: string) => void; // Callback when scrollback is received
  onOutput?: (output: string) => void; // Callback when new output is received (bypass React state)
}

interface TerminalStreamResult {
  output: string; // Deprecated: Use onOutput callback for better performance
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number) => void;
  connect: () => void;
  disconnect: () => void;
  scrollbackLoaded: boolean; // Indicates if scrollback has been loaded
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
        yield msg;
      }
    }
  }

  close() {
    this.closed = true;
    if (this.resolve) {
      // Force unblock the iterator
      this.resolve(new TerminalData({ sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
  }
}

export function useTerminalStream({
  baseUrl,
  sessionId,
  scrollbackLines = 1000,
  onError,
  onScrollbackReceived,
  onOutput,
}: UseTerminalStreamOptions): TerminalStreamResult {
  const [output, setOutput] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false); // Track connection state in ref for callbacks
  const lastResizeTimeRef = useRef<number>(0); // Timestamp of last resize message sent
  const clientRef = useRef(createPromiseClient(
    SessionService,
    createWebsocketBasedTransport({
      baseUrl,
      useBinaryFormat: true, // WebSocket supports binary format
    })
  ));

  // Performance optimization: Batch terminal output updates
  const outputBufferRef = useRef<string[]>([]);
  const pendingUpdateRef = useRef<number | null>(null);
  const textDecoderRef = useRef(new TextDecoder()); // Reuse decoder for performance

  // Sync ref with state
  useEffect(() => {
    isConnectedRef.current = isConnected;
  }, [isConnected]);

  // Flush buffered output to state (batched with RAF)
  const flushOutputBuffer = useCallback(() => {
    if (outputBufferRef.current.length > 0) {
      const bufferedText = outputBufferRef.current.join("");
      outputBufferRef.current = [];
      setOutput((prev) => prev + bufferedText);
    }
    pendingUpdateRef.current = null;
  }, []);

  // Schedule output update (debounced with requestAnimationFrame)
  const scheduleOutputUpdate = useCallback((text: string) => {
    outputBufferRef.current.push(text);

    if (!pendingUpdateRef.current) {
      pendingUpdateRef.current = requestAnimationFrame(flushOutputBuffer);
    }
  }, [flushOutputBuffer]);

  const connect = useCallback(async () => {
    if (isConnectedRef.current || !sessionId) {
      return;
    }

    // Reset disconnecting flag on connect
    isDisconnectingRef.current = false;

    try {
      abortControllerRef.current = new AbortController();
      messageQueueRef.current = new MessageQueue();

      // Send initial handshake message
      messageQueueRef.current.push(
        new TerminalData({
          sessionId,
          data: { case: undefined }, // Initial handshake
        })
      );

      // Request scrollback if configured
      if (scrollbackLines > 0) {
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "scrollbackRequest",
              value: new ScrollbackRequest({
                fromSequence: BigInt(0), // Request from oldest
                limit: scrollbackLines,
              }),
            },
          })
        );
      }

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
              firstMessage = false;
            }

            if (msg.data.case === "output") {
              // Decode bytes to string using shared decoder
              const text = textDecoderRef.current.decode(msg.data.value.data, { stream: true });
              // Only log if debug mode is enabled (toggle with: localStorage.setItem('debug-terminal', 'true'))
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
            } else if (msg.data.case === "scrollbackResponse") {
              // Optimize: Use array and join instead of concatenation
              const chunks: string[] = [];
              for (const chunk of msg.data.value.chunks) {
                const text = textDecoderRef.current.decode(chunk.data);
                chunks.push(text);
              }
              const scrollbackText = chunks.join("");

              // Call callback to write directly to terminal (no output state update)
              if (onScrollbackReceived) {
                onScrollbackReceived(scrollbackText);
              }

              setScrollbackLoaded(true);
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
  }, [sessionId, scrollbackLines, onError, onScrollbackReceived, onOutput, scheduleOutputUpdate]); // Include onOutput

  const disconnect = useCallback(async () => {
    // Prevent double-disconnect
    if (isDisconnectingRef.current) {
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

  const resize = useCallback(
    (cols: number, rows: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot resize terminal: stream not connected");
        return;
      }

      // Throttle resize messages to max 1 per second to prevent feedback loops
      const now = Date.now();
      const timeSinceLastResize = now - lastResizeTimeRef.current;
      const THROTTLE_MS = 1000; // 1 second throttle

      if (timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
        console.log(`[useTerminalStream] Resize throttled (${timeSinceLastResize}ms since last, need ${THROTTLE_MS}ms)`);
        return;
      }

      try {
        console.log(`[useTerminalStream] Pushing resize message to queue: ${cols}x${rows}`);
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
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, onError]
  );

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => {
      // Cleanup: Cancel any pending RAF updates
      if (pendingUpdateRef.current) {
        cancelAnimationFrame(pendingUpdateRef.current);
        pendingUpdateRef.current = null;
      }
      // Flush any remaining buffered output
      flushOutputBuffer();
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]); // Only reconnect if sessionId changes

  return {
    output,
    isConnected,
    error,
    sendInput,
    resize,
    connect,
    disconnect,
    scrollbackLoaded,
  };
}
