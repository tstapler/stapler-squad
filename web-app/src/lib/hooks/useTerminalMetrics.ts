"use client";

import { useRef, useState, useCallback, useEffect } from "react";

/**
 * Recorded WebSocket message for debugging terminal flickering.
 */
interface RecordedMessage {
  timestamp: number;
  type: 'raw' | 'state' | 'diff';
  data: Uint8Array;
  decoded: string;
  sequenceNumber?: bigint;
}

export interface UseTerminalMetricsOptions {
  /** Callback for direct output mode (bypasses React state). */
  onOutput?: (output: string) => void;
}

export interface UseTerminalMetricsResult {
  /** @deprecated Use onOutput callback for better performance. */
  output: string;
  /** Schedule text for batched output via RAF. Falls back to React state if no onOutput. */
  scheduleOutputUpdate: (text: string) => void;
  /** Flush any pending batched output immediately. */
  flushOutputBuffer: () => void;
  /** Start recording incoming WebSocket messages. */
  startRecording: () => void;
  /** Stop recording and download the recording as JSON. */
  stopRecording: () => void;
  /** Record a single message (called from the message handler). */
  recordMessage: (msg: RecordedMessage) => void;
  /** Whether recording is active. */
  isRecording: boolean;
}

/**
 * useTerminalMetrics - RAF-based output batching and debug recording.
 *
 * Owns the adaptive requestAnimationFrame batching of terminal output
 * and the WebSocket message recording feature for debugging.
 *
 * Output batching strategy:
 * - Uses requestAnimationFrame for display-synchronized updates (60-144fps)
 * - Flushes immediately if buffer exceeds 4KB (prevents lag on large bursts)
 * - Automatically adapts to high refresh rate displays
 */
export function useTerminalMetrics({
  onOutput,
}: UseTerminalMetricsOptions): UseTerminalMetricsResult {
  const [output, setOutput] = useState("");

  // RAF-based output batching refs
  const outputBufferRef = useRef<string[]>([]);
  const pendingUpdateRef = useRef<number | null>(null);
  const textDecoderRef = useRef(new TextDecoder());
  const bufferSizeRef = useRef<number>(0);

  // Recording refs
  const recordedMessagesRef = useRef<RecordedMessage[]>([]);
  const isRecordingRef = useRef(false);
  const [isRecording, setIsRecording] = useState(false);

  // Flush buffered output to state
  const flushOutputBuffer = useCallback(() => {
    if (outputBufferRef.current.length > 0) {
      const bufferedText = outputBufferRef.current.join("");
      outputBufferRef.current = [];
      bufferSizeRef.current = 0;
      setOutput((prev) => prev + bufferedText);
    }
    pendingUpdateRef.current = null;
  }, []);

  // Schedule output update with adaptive batching
  const scheduleOutputUpdate = useCallback((text: string) => {
    // If onOutput callback provided, use it directly (better performance)
    if (onOutput) {
      onOutput(text);
      return;
    }

    // Fallback: batch via RAF into React state
    outputBufferRef.current.push(text);
    bufferSizeRef.current += text.length;

    // Immediate flush for large buffers (>4KB)
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
  }, [flushOutputBuffer, onOutput]);

  // Recording functions
  const startRecording = useCallback(() => {
    recordedMessagesRef.current = [];
    isRecordingRef.current = true;
    setIsRecording(true);
    console.log('[Recording] Started terminal output recording');
  }, []);

  const stopRecording = useCallback(() => {
    isRecordingRef.current = false;
    setIsRecording(false);
    const blob = new Blob([JSON.stringify(recordedMessagesRef.current, null, 2)],
      { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `terminal-recording-${Date.now()}.json`;
    a.click();
    console.log('[Recording] Saved recording with', recordedMessagesRef.current.length, 'messages');
  }, []);

  const recordMessage = useCallback((msg: RecordedMessage) => {
    if (isRecordingRef.current) {
      recordedMessagesRef.current.push(msg);
    }
  }, []);

  // Cleanup on unmount: cancel pending RAF, flush remaining output
  useEffect(() => {
    return () => {
      if (pendingUpdateRef.current !== null) {
        cancelAnimationFrame(pendingUpdateRef.current);
        pendingUpdateRef.current = null;
      }
      // Flush any remaining buffered output
      if (outputBufferRef.current.length > 0) {
        const bufferedText = outputBufferRef.current.join("");
        outputBufferRef.current = [];
        bufferSizeRef.current = 0;
        setOutput((prev) => prev + bufferedText);
      }
    };
  }, []);

  return {
    output,
    scheduleOutputUpdate,
    flushOutputBuffer,
    startRecording,
    stopRecording,
    recordMessage,
    isRecording,
  };
}

// Re-export the RecordedMessage type for use by the parent hook
export type { RecordedMessage };
