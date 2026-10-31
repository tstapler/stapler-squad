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

/** One GetVCSStatus request shared by every caller for a session. */
interface VcsFlight {
  promise: Promise<VcsCacheEntry>;
  focused: boolean;
  /** Settles `promise`; lets a replacement flight answer the callers of one it supersedes. */
  settle: { resolve: (e: VcsCacheEntry) => void; reject: (err: unknown) => void };
  /** Stops a superseded request (dequeues it, or aborts it on the wire). */
  cancel: () => void;
  /** The single refresh that starts when this flight lands, shared by every caller that needed newer data. */
  trailing?: { promise: Promise<VcsCacheEntry>; focused: boolean };
}
const vcsInflight = new Map<string, VcsFlight>();

/**
 * Unfocused (background) calls: at most this many on the wire at once. Unfocused panes all poll on the
 * same cadence, and a burst of slow GetVCSStatus calls otherwise holds every HTTP/1.1 connection
 * (6 per origin), starving the focused pane and every other RPC behind them.
 */
export const MAX_BACKGROUND_VCS_CALLS = 2;
let backgroundRunning = 0;
const backgroundQueue: Array<() => void> = [];

function acquireBackgroundSlot(): Promise<void> {
  if (backgroundRunning < MAX_BACKGROUND_VCS_CALLS) {
    backgroundRunning++;
    return Promise.resolve();
  }
  return new Promise<void>((resolve) => backgroundQueue.push(resolve));
}

function releaseBackgroundSlot(): void {
  const next = backgroundQueue.shift();
  if (next) next(); // the slot passes straight to the next waiter
  else backgroundRunning--;
}

function startFlight(sessionId: string, focused: boolean, waitForFresh: boolean): VcsFlight {
  const generation = (vcsGeneration.get(sessionId) ?? 0) + 1;
  vcsGeneration.set(sessionId, generation);
  const client = createClient(SessionService, getConnectTransport());
  const controller = new AbortController();
  let cancelled = false;
  const flight = { focused } as VcsFlight;
  flight.promise = new Promise<VcsCacheEntry>((resolve, reject) => {
    flight.settle = { resolve, reject };
  });
  flight.cancel = () => {
    cancelled = true;
    controller.abort();
  };
  const finish = () => {
    if (vcsInflight.get(sessionId) === flight) vcsInflight.delete(sessionId);
  };
  const run = async (): Promise<VcsCacheEntry> => {
    if (!focused) {
      await acquireBackgroundSlot();
      if (cancelled) {
        releaseBackgroundSlot();
        throw new ConnectError("superseded", Code.Canceled);
      }
    }
    try {
      // Shared by several callers, so no caller's own signal may cancel it. The only signal here is
      // ours, fired when a focused caller supersedes a background request; focused calls carry none.
      // abort-signal-exempt
      const response = await client.getVCSStatus(
        { id: sessionId, waitForFresh },
        focused ? undefined : { ...deferUnless(false), signal: controller.signal },
      );
      const entry: VcsCacheEntry = {
        data: response.vcsStatus ?? null,
        error: response.error || null,
        timestamp: Date.now(),
      };
      // A superseded (older) flight must not overwrite a newer forced result.
      if (vcsGeneration.get(sessionId) === generation) vcsCache.set(sessionId, entry);
      return entry;
    } finally {
      if (!focused) releaseBackgroundSlot();
    }
  };
  run().then(
    (entry) => {
      finish();
      flight.settle.resolve(entry);
    },
    (err) => {
      finish();
      if (!cancelled) flight.settle.reject(err); // a superseded flight's callers are answered by its replacement
    },
  );
  vcsInflight.set(sessionId, flight);
  return flight;
}

/** A focused caller needs a request a background flight is only queued or running at low priority: replace it, and answer its callers from the replacement. */
function supersede(sessionId: string, old: VcsFlight, waitForFresh: boolean): VcsFlight {
  old.cancel();
  const next = startFlight(sessionId, true, waitForFresh);
  next.promise.then(old.settle.resolve, old.settle.reject);
  return next;
}

/**
 * Resolve a session's VCS status through the shared cache and a shared in-flight
 * promise, so a prefetch and any number of hooks mounting at once cost one RPC.
 * `force` skips a fresh cache hit. A plain forced poll joins an in-flight request; a forced call
 * with `waitForFresh` (event-driven or user-requested, so the in-flight answer may predate what
 * prompted it) instead waits for that request and runs ONE trailing refresh shared by all such callers.
 * `waitForFresh` (server side) opts out of stale-while-revalidate; never set it for the interval poll,
 * or the server's stale path never serves it.
 * `focused` false marks the call deferred and limited to MAX_BACKGROUND_VCS_CALLS in flight; a focused
 * caller supersedes a background flight rather than starting a duplicate beside it.
 * Rejects on RPC failure; callers own error handling.
 */
export function fetchVcsEntry(sessionId: string, force = false, focused = true, waitForFresh = false): Promise<VcsCacheEntry> {
  if (!force) {
    const cached = getCached(sessionId);
    if (cached) return Promise.resolve(cached);
  }
  const existing = vcsInflight.get(sessionId);
  if (!existing) return startFlight(sessionId, focused, waitForFresh).promise;
  if (focused && !existing.focused) return supersede(sessionId, existing, waitForFresh || force).promise;
  if (!(force && waitForFresh)) return existing.promise;
  if (existing.trailing && (existing.trailing.focused || !focused)) return existing.trailing.promise;
  const settled = () => undefined;
  const promise = existing.promise.then(settled, settled).then(() => fetchVcsEntry(sessionId, true, focused, true));
  existing.trailing = { promise, focused };
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
  backgroundRunning = 0;
  backgroundQueue.length = 0;
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
