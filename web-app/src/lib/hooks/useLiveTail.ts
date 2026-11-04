"use client";

import { useEffect, useRef, useCallback, useState } from 'react';
import { usePageVisible } from '@/lib/utils/pageVisibility';

/** Longest idle wait, as a multiple of `interval`: 2s polls back off to 10s when nothing arrives. */
const MAX_IDLE_BACKOFF = 5;

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
 * @param fetchLogs - Function to fetch new logs; resolve `false` when nothing new arrived so polling backs off
 * @param options - Live tail configuration options
 * @returns Live tail state and controls
 */
export function useLiveTail(
  fetchLogs: () => Promise<void | boolean>,
  options: LiveTailOptions
): [LiveTailState, LiveTailControls] {
  const { interval = 2000, enabled, onNewLogs } = options;

  const [isActive, setIsActive] = useState(false);
  const [isPaused, setIsPaused] = useState(false);
  const [newLogCount, setNewLogCount] = useState(0);
  const [lastFetch, setLastFetch] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchLogsRef = useRef(fetchLogs);
  const onNewLogsRef = useRef(onNewLogs);
  const visible = usePageVisible();

  // Update refs when callbacks change
  useEffect(() => {
    fetchLogsRef.current = fetchLogs;
  }, [fetchLogs]);

  useEffect(() => {
    onNewLogsRef.current = onNewLogs;
  }, [onNewLogs]);

  // One fetch; resolves true when it produced new entries (fetchLogs resolving void counts as new).
  const inFlightRef = useRef(false);
  const doFetch = useCallback(async (): Promise<boolean> => {
    inFlightRef.current = true;
    try {
      setError(null);
      const gotNew = await fetchLogsRef.current();
      setLastFetch(new Date());
      return gotNew !== false;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs');
      return false; // a failing endpoint backs off like an idle one
    } finally {
      inFlightRef.current = false;
    }
  }, []);

  // Self-scheduling poll: the next fetch is only armed after the previous one returned, so calls
  // never overlap; the delay doubles (up to MAX_IDLE_BACKOFF x interval) while polls find nothing.
  // Nothing runs while paused or while the tab is hidden; showing the tab fetches immediately.
  useEffect(() => {
    if (!isActive || isPaused || !visible) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let idlePolls = 0;
    const run = async () => {
      // An effect restart (resume, tab shown, interval change) can land while a fetch is still pending.
      if (inFlightRef.current) {
        timer = setTimeout(run, interval);
        return;
      }
      const gotNew = await doFetch();
      if (cancelled) return;
      idlePolls = gotNew ? 0 : idlePolls + 1;
      timer = setTimeout(run, interval * Math.min(2 ** idlePolls, MAX_IDLE_BACKOFF));
    };
    void run();
    return () => {
      cancelled = true;
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [isActive, isPaused, visible, interval, doFetch]);

  const start = useCallback(() => {
    setIsActive(true);
    setIsPaused(false);
    setError(null);
  }, []);

  const stop = useCallback(() => {
    setIsActive(false);
    setIsPaused(false);
  }, []);

  const pause = useCallback(() => {
    setIsPaused(true);
  }, []);

  const resume = useCallback(() => {
    setIsPaused(false);
  }, []);

  const toggle = useCallback(() => {
    setIsPaused((p) => !p);
  }, []);

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
