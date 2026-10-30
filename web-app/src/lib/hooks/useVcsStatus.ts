"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { deferUnless } from "@/lib/api/requestPriority";
import type { VCSStatus } from "@/gen/session/v1/types_pb";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";
import { pollIntervalMs, type PollMode } from "@/lib/hooks/pollMode";

interface VcsCacheEntry {
  data: VCSStatus | null;
  error: string | null;
  timestamp: number;
}

// Module-level cache shared across all hook instances.
// Both VcsPanel and FilesTab read from the same cache, eliminating duplicate requests.
const vcsCache = new Map<string, VcsCacheEntry>();
const VCS_CACHE_TTL_MS = 15_000;

function getCached(sessionId: string): VcsCacheEntry | null {
  const entry = vcsCache.get(sessionId);
  if (entry && Date.now() - entry.timestamp < VCS_CACHE_TTL_MS) return entry;
  return null;
}

const vcsGeneration = new Map<string, number>();
const vcsInflight = new Map<string, { promise: Promise<VcsCacheEntry>; focused: boolean }>();

/**
 * Resolve a session's VCS status through the shared cache and a shared in-flight
 * promise, so a prefetch and any number of hooks mounting at once cost one RPC.
 * `force` skips a fresh cache hit and never joins an in-flight request, which may predate the write that prompted it.
 * `waitForFresh` (server side) opts out of stale-while-revalidate; set it only for event-driven or
 * user-requested refreshes, never for the interval poll, or the server's stale path never serves it.
 * `focused` false marks the call deferred (Story 2.1.9); a focused caller never joins a deferred flight.
 * Rejects on RPC failure; callers own error handling.
 */
export function fetchVcsEntry(sessionId: string, force = false, focused = true, waitForFresh = false): Promise<VcsCacheEntry> {
  if (!force) {
    const cached = getCached(sessionId);
    if (cached) return Promise.resolve(cached);
  }
  const existing = vcsInflight.get(sessionId);
  if (!force && existing && (existing.focused || !focused)) return existing.promise;
  const generation = (vcsGeneration.get(sessionId) ?? 0) + 1;
  vcsGeneration.set(sessionId, generation);
  const client = createClient(SessionService, getConnectTransport());
  // Shared by several callers, so no single caller's signal may cancel it; each
  // caller drops the result itself when its own signal has aborted.
  // abort-signal-exempt
  const promise: Promise<VcsCacheEntry> = client
    .getVCSStatus({ id: sessionId, waitForFresh }, deferUnless(focused))
    .then((response) => {
      const entry: VcsCacheEntry = {
        data: response.vcsStatus ?? null,
        error: response.error || null,
        timestamp: Date.now(),
      };
      // A superseded (older) flight must not overwrite a newer forced result.
      if (vcsGeneration.get(sessionId) === generation) vcsCache.set(sessionId, entry);
      return entry;
    })
    .finally(() => {
      if (vcsInflight.get(sessionId)?.promise === promise) vcsInflight.delete(sessionId);
    });
  vcsInflight.set(sessionId, { promise, focused });
  return promise;
}

/** Drop a session's cached status (session deleted). */
export function invalidateVcsEntry(sessionId: string): void {
  vcsCache.delete(sessionId);
}

/** Test-only: clear the module cache and in-flight map. */
export function resetVcsCacheForTests(): void {
  vcsCache.clear();
  vcsInflight.clear();
  vcsGeneration.clear();
}

/**
 * Warm the VCS cache for a session without rendering anything.
 * Call from SessionDetail when a session is selected.
 */
export async function prefetchVcsStatus(sessionId: string, baseUrl: string): Promise<void> {
  void baseUrl;
  try {
    await fetchVcsEntry(sessionId);
  } catch {
    // Prefetch failures are silent – the hook will retry on mount.
  }
}

interface UseVcsStatusResult {
  data: VCSStatus | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

/**
 * Shared hook for VCS status with module-level caching and visibility-aware polling.
 * Multiple components using this hook for the same sessionId share one cache entry
 * and avoid redundant network requests.
 */
export function useVcsStatus(
  sessionId: string,
  baseUrl: string,
  fastPollMs = 15_000,
  pollMode: PollMode = "fast",
): UseVcsStatusResult {
  const hit = getCached(sessionId);
  const [data, setData] = useState<VCSStatus | null>(hit?.data ?? null);
  const [loading, setLoading] = useState(!hit);
  const [error, setError] = useState<string | null>(hit?.error ?? null);

  // Set once GetVCSStatus reports the session no longer exists, so the
  // polling interval below stops calling a deleted session forever.
  const stoppedRef = useRef(false);

  // Cancel the in-flight request on the next call or on unmount — see
  // useSessionVcs.ts (the sibling hook this pattern was first fixed in) for
  // the measured impact of skipping this under rapid session switching.
  const startFetch = useAbortableRequest();

  const fetchVcs = useCallback(
    async (skipCache = false) => {
      if (!sessionId || stoppedRef.current) {
        setLoading(false);
        return;
      }
      if (!skipCache) {
        const cached = getCached(sessionId);
        if (cached) {
          setData(cached.data);
          setError(cached.error);
          setLoading(false);
          return;
        }
      }

      const signal = startFetch();
      try {
        const entry = await fetchVcsEntry(sessionId, skipCache, true, skipCache);
        if (signal.aborted) return;
        setData(entry.data);
        setError(entry.error);
      } catch (err) {
        if (signal.aborted) return;
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          stoppedRef.current = true;
          vcsCache.delete(sessionId);
          setError(err.message);
          setData(null);
          return;
        }
        setError(err instanceof Error ? err.message : "Failed to load VCS status");
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    },
    [sessionId, baseUrl, startFetch] // eslint-disable-line react-hooks/exhaustive-deps
  );

  useEffect(() => {
    stoppedRef.current = false;
    const intervalMs = pollIntervalMs(pollMode, fastPollMs, fastPollMs * 10);
    if (intervalMs === null) return; // hidden window: the effect reruns and refetches on activation
    fetchVcs();
    const interval = setInterval(() => {
      if (stoppedRef.current) {
        clearInterval(interval);
        return;
      }
      if (!document.hidden) fetchVcs();
    }, intervalMs);
    return () => clearInterval(interval);
  }, [sessionId, baseUrl, pollMode]); // eslint-disable-line react-hooks/exhaustive-deps

  return { data, loading, error, refetch: () => fetchVcs(true) };
}
