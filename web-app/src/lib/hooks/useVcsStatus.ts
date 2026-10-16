"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { VCSStatus } from "@/gen/session/v1/types_pb";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

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

/**
 * Warm the VCS cache for a session without rendering anything.
 * Call from SessionDetail when a session is selected.
 */
export async function prefetchVcsStatus(sessionId: string, baseUrl: string): Promise<void> {
  if (getCached(sessionId)) return;
  try {
    const client = createClient(SessionService, getConnectTransport());
    // One-shot cache warm with no component lifecycle to cancel against;
    // the TTL cache above (not a signal) is what stops this from
    // compounding on rapid session switches.
    // abort-signal-exempt
    const response = await client.getVCSStatus({ id: sessionId });
    vcsCache.set(sessionId, {
      data: response.vcsStatus ?? null,
      error: response.error || null,
      timestamp: Date.now(),
    });
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
  pollIntervalMs = 15_000
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
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.getVCSStatus({ id: sessionId }, { signal });
        if (signal.aborted) return;
        const entry: VcsCacheEntry = {
          data: response.vcsStatus ?? null,
          error: response.error || null,
          timestamp: Date.now(),
        };
        vcsCache.set(sessionId, entry);
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
    fetchVcs();
    const interval = setInterval(() => {
      if (stoppedRef.current) {
        clearInterval(interval);
        return;
      }
      if (!document.hidden) fetchVcs();
    }, pollIntervalMs);
    return () => clearInterval(interval);
  }, [sessionId, baseUrl]); // eslint-disable-line react-hooks/exhaustive-deps

  return { data, loading, error, refetch: () => fetchVcs(true) };
}
