"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { VCSStatus } from "@/gen/session/v1/types_pb";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import type { AsyncResult } from "@/lib/types/asyncResult";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

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
  /** Refresh both status and diff. */
  refresh: () => void;
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
 * @param isActive - Whether this session's pane currently has focus.
 * Defaults to true (matches prior always-poll-every-60s behavior) so every
 * caller except a multi-pane layout is unaffected. A multi-pane layout
 * should pass the focused pane's own focus state so unfocused panes fall
 * back to INACTIVE_STATUS_POLL_MS instead of polling as if the user were
 * watching them.
 */
export function useSessionVcs(sessionId: string, baseUrl: string, isActive = true): SessionVcsState {
  const [status, setStatus] = useState<VCSStatus | null>(null);
  const [diff, setDiff] = useState<SessionDiff | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [diffLoading, setDiffLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  // Set once GetVCSStatus reports the session no longer exists, so the 60s
  // fallback interval below stops calling a deleted session forever.
  const stoppedRef = useRef(false);

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

  const fetchStatus = useCallback(async () => {
    if (stoppedRef.current) {
      setStatusLoading(false);
      return;
    }
    const signal = startStatus();
    try {
      const response = await getClient().getVCSStatus({ id: sessionId }, { signal });
      if (signal.aborted) return;
      if (response.error) {
        setError(new Error(response.error));
        setStatus(null);
      } else {
        setStatus(response.vcsStatus ?? null);
        setError(null);
      }
    } catch (err) {
      if (signal.aborted) return;
      if (err instanceof ConnectError && err.code === Code.NotFound) {
        stoppedRef.current = true;
        setError(err);
        setStatus(null);
        return;
      }
      setError(err instanceof Error ? err : new Error("Failed to load VCS status"));
    } finally {
      if (!signal.aborted) setStatusLoading(false);
    }
  }, [sessionId, getClient, startStatus]);

  const fetchDiff = useCallback(async () => {
    if (stoppedRef.current) {
      setDiffLoading(false);
      return;
    }
    const signal = startDiff();
    setDiffLoading(true);
    try {
      const response = await getClient().getSessionDiff({ id: sessionId }, { signal });
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
    } finally {
      if (!signal.aborted) setDiffLoading(false);
    }
  }, [sessionId, getClient, startDiff]);

  const refresh = useCallback(() => {
    fetchStatus();
    fetchDiff();
  }, [fetchStatus, fetchDiff]);

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
  // field. isActive controls the fallback's cadence, not whether it fires —
  // a backgrounded pane still needs a backstop, just not one tuned for a
  // user actively watching it.
  useEffect(() => {
    stoppedRef.current = false;
    setStatusLoading(true);
    fetchStatus();
    const pollMs = isActive ? ACTIVE_STATUS_POLL_MS : INACTIVE_STATUS_POLL_MS;
    const fallback = setInterval(() => {
      if (stoppedRef.current) {
        clearInterval(fallback);
        return;
      }
      if (!document.hidden) fetchStatus();
    }, pollMs);
    return () => clearInterval(fallback);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchStatus, sessionStatus, sessionUpdatedAt, isActive]);

  // Diff: fetch once on mount; consumers call refreshDiff() when needed.
  // Cancellation on unmount/re-run is handled by startDiff's useAbortableRequest.
  useEffect(() => {
    fetchDiff();
  }, [fetchDiff]);

  return {
    status,
    diff,
    statusLoading,
    diffLoading,
    loading: statusLoading,
    error,
    refreshStatus: fetchStatus,
    refreshDiff: fetchDiff,
    refresh,
  };
}
