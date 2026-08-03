"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionSummaryService } from "@/gen/session/v1/session_summary_pb";
import type { SessionSummaryProto } from "@/gen/session/v1/session_summary_pb";
import { SessionSummaryStatus } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { copyToClipboard } from "@/lib/clipboard";

const POLL_INTERVAL_MS = 2000;

/**
 * Cap on consecutive `nil` GetSessionSummary reads before giving up
 * (~10 * 2s = 20s). Closes the "will never resolve" dead end flagged in
 * design/ux.md (surface (g)) for a sessionId that will never produce a row —
 * a typo'd URL, or a session excluded via reconcile-session-missing.
 */
const MAX_NULL_POLL_ATTEMPTS = 10;

interface UseSessionSummaryResult {
  data: SessionSummaryProto | null;
  loading: boolean;
  error: Error | null;
  neverResolved: boolean;
  regenerate: () => Promise<void>;
  copy: () => Promise<boolean>;
}

function isGenerating(status: SessionSummaryStatus): boolean {
  return status === SessionSummaryStatus.PENDING || status === SessionSummaryStatus.GENERATING;
}

/**
 * Fetches a session's completion summary, polling every 2s while generation
 * is in flight. Per design/ux.md surface (b)'s edge case, a `nil` response
 * (no row exists yet) polls identically to PENDING/GENERATING — there is no
 * user-visible distinction between "not started yet" and "in progress."
 * Polling stops once `status` reaches READY/ERROR, or once
 * `MAX_NULL_POLL_ATTEMPTS` consecutive `nil` reads have occurred, at which
 * point `neverResolved` is set so callers can render a terminal empty state
 * instead of polling forever.
 *
 * Shared data layer for both the Summary tab (Epic 3.2) and the standalone
 * post-deletion route (Epic 3.3) — see plan.md Story 3.1.1.
 */
export function useSessionSummary(sessionId: string): UseSessionSummaryResult {
  const [data, setData] = useState<SessionSummaryProto | null>(null);
  const [loading, setLoading] = useState(!!sessionId);
  const [error, setError] = useState<Error | null>(null);
  const [neverResolved, setNeverResolved] = useState(false);

  // Consecutive `nil` GetSessionSummary reads. Only counts consecutive
  // nulls — reset to 0 the moment a non-null summary is observed (even if
  // still GENERATING), per Task 3.1.1a.
  const nullAttemptsRef = useRef(0);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // Holds the latest fetchSummary closure so the interval tick (set up once
  // per sessionId) always calls the current version rather than a stale one.
  const fetchSummaryRef = useRef<() => Promise<void>>(async () => {});

  const stopPolling = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const startPolling = useCallback(() => {
    if (intervalRef.current !== null) return;
    intervalRef.current = setInterval(() => {
      fetchSummaryRef.current();
    }, POLL_INTERVAL_MS);
  }, []);

  const fetchSummary = useCallback(async () => {
    if (!sessionId) {
      setLoading(false);
      return;
    }
    try {
      const client = createClient(SessionSummaryService, getConnectTransport());
      const response = await client.getSessionSummary({ sessionId });
      const summary = response.summary ?? null;
      setData(summary);
      setError(null);

      if (summary === null) {
        nullAttemptsRef.current += 1;
      } else {
        nullAttemptsRef.current = 0;
      }

      const exceededNullAttempts =
        summary === null && nullAttemptsRef.current >= MAX_NULL_POLL_ATTEMPTS;
      if (exceededNullAttempts) {
        setNeverResolved(true);
      }

      const shouldPoll =
        !exceededNullAttempts && (summary === null || isGenerating(summary.status));
      if (shouldPoll) {
        startPolling();
      } else {
        stopPolling();
      }
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to load session summary"));
    } finally {
      setLoading(false);
    }
  }, [sessionId, startPolling, stopPolling]);

  useEffect(() => {
    fetchSummaryRef.current = fetchSummary;
  }, [fetchSummary]);

  useEffect(() => {
    nullAttemptsRef.current = 0;
    setNeverResolved(false);
    stopPolling();
    fetchSummary();
    return () => stopPolling();
    // fetchSummary/stopPolling are recreated only when sessionId changes, so
    // this intentionally keys off sessionId alone to avoid re-running on
    // every fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  const regenerate = useCallback(async () => {
    if (!sessionId) return;
    try {
      const client = createClient(SessionSummaryService, getConnectTransport());
      const response = await client.regenerateSessionSummary({ sessionId });
      const summary = response.summary ?? null;
      setData(summary);
      setError(null);
      nullAttemptsRef.current = 0;
      setNeverResolved(false);

      const shouldPoll = summary === null || isGenerating(summary.status);
      if (shouldPoll) {
        startPolling();
      } else {
        stopPolling();
      }
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to regenerate session summary"));
    }
  }, [sessionId, startPolling, stopPolling]);

  const copy = useCallback(async () => {
    return copyToClipboard(data?.markdown ?? "");
  }, [data]);

  return { data, loading, error, neverResolved, regenerate, copy };
}
