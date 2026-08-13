"use client";

import { useEffect, useRef, useCallback, useState } from 'react';

export interface LiveTailOptions {
  /** Interval in milliseconds between fetches (default: 2000) */
  interval?: number;
  /** Whether live tail is enabled */
  enabled: boolean;
  /** Callback when new logs are detected */
  onNewLogs?: (count: number) => void;
}

export interface LiveTailState {
  /** Whether live tail is currently active */
  isActive: boolean;
  /** Whether live tail is paused */
  isPaused: boolean;
  /** Number of new logs since last view */
  newLogCount: number;
  /** Last fetch timestamp */
  lastFetch: Date | null;
  /** Error message if any */
  error: string | null;
}

export interface LiveTailControls {
  /** Start live tailing */
  start: () => void;
  /** Stop live tailing */
  stop: () => void;
  /** Pause live tailing (keeps state) */
  pause: () => void;
  /** Resume live tailing */
  resume: () => void;
  /** Toggle pause/resume */
  toggle: () => void;
  /** Clear new log count */
  clearNewLogCount: () => void;
}

/**
 * Hook for live tailing logs with auto-refresh
 *
 * @param fetchLogs - Function to fetch new logs
 * @param options - Live tail configuration options
 * @returns Live tail state and controls
 */
export function useLiveTail(
  fetchLogs: () => Promise<void>,
  options: LiveTailOptions
): [LiveTailState, LiveTailControls] {
  const { interval = 2000, enabled, onNewLogs } = options;

  const [isActive, setIsActive] = useState(false);
  const [isPaused, setIsPaused] = useState(false);
  const [newLogCount, setNewLogCount] = useState(0);
  const [lastFetch, setLastFetch] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);

  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const fetchLogsRef = useRef(fetchLogs);
  const onNewLogsRef = useRef(onNewLogs);

  // Update refs when callbacks change
  useEffect(() => {
    fetchLogsRef.current = fetchLogs;
  }, [fetchLogs]);

  useEffect(() => {
    onNewLogsRef.current = onNewLogs;
  }, [onNewLogs]);

  // Fetch function that tracks state
  const doFetch = useCallback(async () => {
    try {
      setError(null);
      await fetchLogsRef.current();
      setLastFetch(new Date());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs');
    }
  }, []);

  // Start live tailing
  const start = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }

    setIsActive(true);
    setIsPaused(false);
    setError(null);

    // Immediate fetch
    doFetch();

    // Start interval
    intervalRef.current = setInterval(() => {
      if (!isPaused) {
        doFetch();
      }
    }, interval);
  }, [interval, isPaused, doFetch]);

  // Stop live tailing
  const stop = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    setIsActive(false);
    setIsPaused(false);
  }, []);

  // Pause live tailing
  const pause = useCallback(() => {
    setIsPaused(true);
  }, []);

  // Resume live tailing
  const resume = useCallback(() => {
    setIsPaused(false);
    // Immediate fetch on resume
    doFetch();
  }, [doFetch]);

  // Toggle pause/resume
  const toggle = useCallback(() => {
    if (isPaused) {
      resume();
    } else {
      pause();
    }
  }, [isPaused, pause, resume]);

  // Clear new log count
  const clearNewLogCount = useCallback(() => {
    setNewLogCount(0);
  }, []);

  // Handle enabled state changes
  useEffect(() => {
    if (enabled && !isActive) {
      start();
    } else if (!enabled && isActive) {
      stop();
    }
  }, [enabled, isActive, start, stop]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);

  // Update interval when it changes
  useEffect(() => {
    if (isActive && !isPaused && intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = setInterval(doFetch, interval);
    }
  }, [interval, isActive, isPaused, doFetch]);

  const state: LiveTailState = {
    isActive,
    isPaused,
    newLogCount,
    lastFetch,
    error,
  };

  const controls: LiveTailControls = {
    start,
    stop,
    pause,
    resume,
    toggle,
    clearNewLogCount,
  };

  return [state, controls];
}

export default useLiveTail;
