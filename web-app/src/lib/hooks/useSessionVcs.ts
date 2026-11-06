"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { VCSStatus } from "@/gen/session/v1/types_pb";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import type { AsyncResult } from "@/lib/types/asyncResult";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";
import { fetchVcsEntry, invalidateVcsEntry } from "@/lib/hooks/useVcsStatus";
import { pollIntervalMs, type PollMode } from "@/lib/hooks/pollMode";

/** Parsed diff stats returned by getSessionDiff. */
export interface SessionDiff {
  content: string;
  added: number;
  removed: number;
}

export interface SessionVcsState extends AsyncResult {
  /** VCS status, null while loading or when the directory is not a VCS repo. */
  status: VCSStatus | null;
  /** Diff content, null while loading or when there are no changes. */
  diff: SessionDiff | null;
  /** True while the VCS status fetch is in-flight (primary loading signal). */
  statusLoading: boolean;
  diffLoading: boolean;
  /** AsyncResult.loading maps to statusLoading (primary loading signal). */
  loading: boolean;
  /** Error from VCS status fetch (diff errors are non-fatal). Implements AsyncResult.error. */
  error: Error | null;
  /** Trigger a fresh VCS status fetch (shared across all consumers). */
  refreshStatus: () => void;
  /** Trigger a fresh diff fetch (shared across all consumers). */
  refreshDiff: () => void;
  /**
   * Unchanged lines of context requested per hunk: 0 = server default (3),
   * -1 = whole file. Changing it refetches the diff.
   */
  diffContextLines: number;
  setDiffContextLines: (contextLines: number) => void;
  /** Refresh status, plus the diff only if a consumer has asked for it. */
  refresh: () => void;
  /**
   * Fetch the diff once per session when a consumer that can show it is
   * visible. The diff is the heaviest read on the reload path (multi-MB, 5-12 s),
   * so nothing fetches it on mount; DiffViewer and FilesTab call this. A no-op
   * when already requested or when pollMode is "off" (hidden window).
   */
  ensureDiff: () => void;
}


/** Fallback poll interval for the currently-focused/visible session. */
const ACTIVE_STATUS_POLL_MS = 60_000;

/**
 * Fallback poll interval for a session whose pane is open but not focused
 * (e.g. a background split-pane, or a peeked session behind the active
 * one). Event-driven refetches (Redux `sessionUpdatedAt`) still fire
 * regardless of focus, so this is just the backstop for state changes the
 * push stream might miss — 10x looser than the active interval is enough
 * for a pane the user isn't looking at, and cuts N-1 background panes'
 * contribution to GetVCSStatus/server load by the same factor.
 */
const INACTIVE_STATUS_POLL_MS = 600_000;

/**
 * Single source of truth for a session's VCS status and diff data.
 *
 * Intended to be instantiated once per SessionDetail via SessionVcsProvider
 * and consumed by VcsPanel, FilesTab, and DiffViewer through
 * useSessionVcsContext(). This eliminates the 3 independent, uncached fetches
 * those components previously made independently.
 *
 * @param pollMode - "fast" (default; matches the prior always-poll-every-60s
 * behavior) for a focused pane, "slow" (INACTIVE_STATUS_POLL_MS) for a visible
 * unfocused pane, "off" for a pane in a hidden window: no fetch at all until
 * the mode changes (the effect refetches on activation).
 */
export function useSessionVcs(sessionId: string, baseUrl: string, pollMode: PollMode = "fast"): SessionVcsState {
  const isActive = pollMode === "fast";
  const [status, setStatus] = useState<VCSStatus | null>(null);
  const [diff, setDiff] = useState<SessionDiff | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [diffLoading, setDiffLoading] = useState(true);
  const [diffContextLines, setDiffContextLines] = useState(0);
  const [error, setError] = useState<Error | null>(null);

  // Set once GetVCSStatus reports the session no longer exists, so the 60s
  // fallback interval below stops calling a deleted session forever.
  const stoppedRef = useRef(false);
  const mountedSessionRef = useRef<string | null>(null);
  // Whether Redux knew this session when the effect last ran; its first appearance is hydration, not a change.
  const sessionKnownRef = useRef(false);
  const mountFetchPendingRef = useRef(false);
  // Fixed per hook instance: re-drawing it on every Redux update would make the slow backstop fire at random.
  const pollPhaseRef = useRef<number | null>(null);
  if (pollPhaseRef.current === null) pollPhaseRef.current = Math.random();
  const pollStartedRef = useRef(false);

  // Stable client reference — initialized once from the shared transport singleton.
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      clientRef.current = createClient(SessionService, getConnectTransport());
    }
    return clientRef.current;
  }, []);

  // Cancel the in-flight status/diff request on the next call or on unmount —
  // this hook is instantiated per-SessionDetail, so switching sessions rapidly
  // without cancellation left every prior request's promise (and its closure
  // over sessionId/setState) alive until it resolved or hit its deadline,
  // growing heap under fast session switching (measured: ~44MB -> ~154MB
  // used JS heap over 138 switches in a profiling run).
  const startStatus = useAbortableRequest();
  const startDiff = useAbortableRequest();

  // `force` skips a fresh prefetch-cache hit; the first fetch after mount reads
  // the cache and joins any in-flight prefetch (Story 2.1.6a).
  const loadStatus = useCallback(async (force: boolean, waitForFresh = false) => {
    if (stoppedRef.current) {
      setStatusLoading(false);
      return;
    }
    const signal = startStatus();
    try {
      const entry = await fetchVcsEntry(sessionId, force, isActive, waitForFresh);
      if (signal.aborted) return;
      if (entry.error) {
        setError(new Error(entry.error));
        setStatus(null);
      } else {
        setStatus(entry.data);
        setError(null);
      }
    } catch (err) {
      if (signal.aborted) return;
      if (err instanceof ConnectError && err.code === Code.NotFound) {
        stoppedRef.current = true;
        invalidateVcsEntry(sessionId);
        setError(err);
        setStatus(null);
        return;
      }
      setError(err instanceof Error ? err : new Error("Failed to load VCS status"));
    } finally {
      if (!signal.aborted) setStatusLoading(false);
    }
  }, [sessionId, startStatus, isActive]);

  // `waitForFresh` is for user-requested refreshes only; the interval poll leaves it false so the
  // server may answer from its stale-while-revalidate entry.
  const fetchStatus = useCallback((waitForFresh = false) => loadStatus(true, waitForFresh), [loadStatus]);

  // "<session>:<contextLines>" the diff was last requested for; null until a
  // consumer asks. Keyed on contextLines so changing it refetches, as before.
  const diffRequestedForRef = useRef<string | null>(null);
  const fetchDiff = useCallback(async () => {
    if (stoppedRef.current) {
      setDiffLoading(false);
      return;
    }
    const signal = startDiff();
    setDiffLoading(true);
    try {
      const response = await getClient().getSessionDiff({ id: sessionId, contextLines: diffContextLines }, { signal });
      if (signal.aborted) return;
      if (response.diffStats) {
        setDiff({
          content: response.diffStats.content,
          added: response.diffStats.added,
          removed: response.diffStats.removed,
        });
      } else {
        setDiff(null);
      }
    } catch (err) {
      if (signal.aborted) return;
      if (err instanceof ConnectError && err.code === Code.NotFound) {
        stoppedRef.current = true;
      }
      // Diff errors are non-fatal — status error is the primary signal.
      console.error("useSessionVcs: failed to load diff:", err);
      diffRequestedForRef.current = null; // let the next ensureDiff() retry
    } finally {
      if (!signal.aborted) setDiffLoading(false);
    }
  }, [sessionId, getClient, startDiff, diffContextLines]);

  const diffKey = `${sessionId}:${diffContextLines}`;
  const ensureDiff = useCallback(() => {
    if (pollMode === "off" || diffRequestedForRef.current === diffKey) return;
    diffRequestedForRef.current = diffKey;
    void fetchDiff();
  }, [pollMode, diffKey, fetchDiff]);

  const refresh = useCallback(() => {
    fetchStatus(true);
    if (diffRequestedForRef.current === diffKey) void fetchDiff();
  }, [fetchStatus, fetchDiff, diffKey]);

  // Watch session state from the Redux store so VCS data re-fetches whenever
  // the session is updated by the server-pushed watchSessions stream, instead
  // of polling on a fixed interval.
  const sessionStatus = useAppSelector((state) =>
    selectAllSessions(state).find((s) => s.id === sessionId)?.status
  );
  const sessionUpdatedAt = useAppSelector((state) => {
    const s = selectAllSessions(state).find((s) => s.id === sessionId);
    return s?.updatedAt?.seconds ?? null;
  });

  // VCS status: re-fetch on mount and whenever the session changes in Redux.
  // Event-driven is the primary trigger; the fallback interval catches git
  // commits that happen mid-session without changing the session's status
  // field. pollMode controls the fallback's cadence, not whether it fires —
  // a backgrounded pane still needs a backstop, just not one tuned for a
  // user actively watching it.
  useEffect(() => {
    stoppedRef.current = false;
    const pollMs = pollIntervalMs(pollMode, ACTIVE_STATUS_POLL_MS, INACTIVE_STATUS_POLL_MS);
    const knownBefore = sessionKnownRef.current;
    sessionKnownRef.current = sessionUpdatedAt !== null; // tracked even while polling is off
    if (pollMs === null) return;
    setStatusLoading(true);
    const firstForSession = mountedSessionRef.current !== sessionId;
    mountedSessionRef.current = sessionId;
    // Redux hydrating the session while the mount fetch is still in flight must join it, not force
    // a duplicate. Once that fetch has settled, hydration may carry newer state than the cached
    // entry, and any change to an already-known session also warrants a forced refetch.
    const hydrating = !firstForSession && !knownBefore && sessionUpdatedAt !== null && mountFetchPendingRef.current;
    // A re-run after the first fetch is event-driven (session changed in Redux): wait for fresh data.
    const eventDriven = !firstForSession && !hydrating;
    const load = loadStatus(eventDriven, eventDriven);
    if (firstForSession) {
      mountFetchPendingRef.current = true;
      void load.finally(() => {
        mountFetchPendingRef.current = false;
      });
    }
    let fallback: ReturnType<typeof setInterval> | undefined;
    const tick = () => {
      if (stoppedRef.current) {
        clearInterval(fallback);
        return;
      }
      if (!document.hidden) fetchStatus();
    };
    // Panes mounted together would otherwise poll in lockstep forever (a 10-minute burst of every
    // unfocused session at once); a random phase spreads them. The focused pane keeps its own cadence.
    const phase = setTimeout(
      () => {
        tick();
        if (!stoppedRef.current) fallback = setInterval(tick, pollMs);
      },
      // Only the hook's first poll start is jittered; a re-run (session update) restarts a full period as before.
      pollMode === "fast" || pollStartedRef.current ? pollMs : Math.floor((pollPhaseRef.current ?? 0) * pollMs),
    );
    pollStartedRef.current = true;
    return () => {
      clearTimeout(phase);
      clearInterval(fallback);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadStatus, fetchStatus, sessionStatus, sessionUpdatedAt, pollMode]);

  // Diff is lazy: consumers call ensureDiff() when visible and refreshDiff()
  // to reload. Cancellation on unmount/re-run is handled by startDiff.
  // A session switch drops the previous session's diff.
  useEffect(() => {
    setDiff(null);
    setDiffLoading(true);
    // Cleanup runs before the next session's consumers re-request, so it only
    // aborts a stale in-flight fetch of the previous session.
    return () => {
      startDiff();
      diffRequestedForRef.current = null; // an aborted fetch must be re-requested on remount (StrictMode)
    };
  }, [sessionId, startDiff]);

  return {
    status,
    diff,
    statusLoading,
    diffLoading,
    loading: statusLoading,
    error,
    refreshStatus: () => fetchStatus(true),
    refreshDiff: fetchDiff,
    diffContextLines,
    setDiffContextLines,
    refresh,
    ensureDiff,
  };
}
