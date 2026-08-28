"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { HandoffSummaryService } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import { getConnectTransport } from "@/lib/api/transport";

const POLL_INTERVAL_MS = 2000;

/**
 * Cap on consecutive `nil` GetHandoffSummary reads before giving up
 * (~10 * 2s = 20s). Mirrors useSessionSummary's MAX_NULL_POLL_ATTEMPTS — a
 * sessionId that will never produce a row (typo'd URL, excluded session)
 * should not poll forever.
 */
const MAX_NULL_POLL_ATTEMPTS = 10;

interface UseHandoffSummaryResult {
  data: HandoffSummaryProto | null;
  loading: boolean;
  error: Error | null;
  neverResolved: boolean;
  trigger: () => Promise<void>;
  /** Re-runs the initial fetch — for retrying after a transport/RPC error. */
  refetch: () => Promise<void>;
}

/**
 * Single canonical definition of "still generating," mirroring
 * useSessionSummary's isGenerating — proto3's zero value (`UNSPECIFIED`) is
 * not a state the backend intentionally sets; it only appears when a row is
 * read before its terminal status has ever been assigned, so it is treated
 * as generating rather than terminal.
 */
export function isGenerating(status: HandoffSummaryStatus): boolean {
  return (
    status === HandoffSummaryStatus.UNSPECIFIED ||
    status === HandoffSummaryStatus.PENDING ||
    status === HandoffSummaryStatus.GENERATING
  );
}

/**
 * Fetches a session's restart-handoff summary, polling every 2s while
 * generation is in flight. A `nil` response (no row exists yet) polls
 * identically to PENDING/GENERATING — there is no user-visible distinction
 * between "not started yet" and "in progress." Polling stops once `status`
 * reaches READY/ERROR, or once `MAX_NULL_POLL_ATTEMPTS` consecutive `nil`
 * reads have occurred, at which point `neverResolved` is set.
 *
 * Structural copy of useSessionSummary.ts (Story 3.1.1) — shared data layer
 * for RestartWithSummaryButton and HandoffSummarySection.
 */
export function useHandoffSummary(sessionId: string): UseHandoffSummaryResult {
  const [data, setData] = useState<HandoffSummaryProto | null>(null);
  const [loading, setLoading] = useState(!!sessionId);
  const [error, setError] = useState<Error | null>(null);
  const [neverResolved, setNeverResolved] = useState(false);

  // Consecutive `nil` GetHandoffSummary reads. Only counts consecutive
  // nulls — reset to 0 the moment a non-null summary is observed (even if
  // still GENERATING).
  const nullAttemptsRef = useRef(0);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  // Holds the latest fetchSummary closure so the interval tick (set up once
  // per sessionId) always calls the current version rather than a stale one.
  const fetchSummaryRef = useRef<() => Promise<void>>(async () => {});
  // The sessionId this hook instance is *currently* rendering for. Updated
  // synchronously every render (not via an effect) so it reflects the latest
  // prop before any async callback scheduled during this render can resolve
  // — guards against a fetch/trigger call started for an old sessionId
  // clobbering freshly-fetched state for a new one.
  const activeSessionIdRef = useRef(sessionId);
  activeSessionIdRef.current = sessionId;
  // True while a getHandoffSummary request (initial fetch, refetch, or poll
  // tick) is in flight. Guards against overlapping poll requests: if a
  // single call takes longer than POLL_INTERVAL_MS, the next interval tick
  // skips issuing a new request rather than firing a second one whose
  // out-of-order response could leave stale data displayed.
  const pollInFlightRef = useRef(false);

  const stopPolling = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const startPolling = useCallback(() => {
    if (intervalRef.current !== null) return;
    intervalRef.current = setInterval(() => {
      if (pollInFlightRef.current) return;
      fetchSummaryRef.current();
    }, POLL_INTERVAL_MS);
  }, []);

  const fetchSummary = useCallback(async () => {
    if (!sessionId) {
      setLoading(false);
      return;
    }
    pollInFlightRef.current = true;
    try {
      const client = createClient(HandoffSummaryService, getConnectTransport());
      const response = await client.getHandoffSummary({ sessionId });

      // A newer sessionId may have become active while this request was in
      // flight — discard this response rather than overwriting the fresher
      // session's state.
      if (activeSessionIdRef.current !== sessionId) return;

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
      if (activeSessionIdRef.current !== sessionId) return;
      setError(err instanceof Error ? err : new Error("Failed to load handoff summary"));
    } finally {
      if (activeSessionIdRef.current === sessionId) {
        setLoading(false);
      }
      pollInFlightRef.current = false;
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

  const trigger = useCallback(async () => {
    if (!sessionId) return;
    pollInFlightRef.current = true;
    try {
      const client = createClient(HandoffSummaryService, getConnectTransport());
      const response = await client.triggerHandoffSummary({ sessionId });

      // See fetchSummary's identical guard — the sessionId this call was
      // issued for may no longer be the active one by the time it resolves.
      if (activeSessionIdRef.current !== sessionId) return;

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
      const e = err instanceof Error ? err : new Error("Failed to trigger handoff summary");
      if (activeSessionIdRef.current === sessionId) {
        setError(e);
      }
      // Rethrow so callers awaiting trigger() can distinguish success from
      // failure via the promise itself.
      throw e;
    } finally {
      pollInFlightRef.current = false;
    }
  }, [sessionId, startPolling, stopPolling]);

  return { data, loading, error, neverResolved, trigger, refetch: fetchSummary };
}
