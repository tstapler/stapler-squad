"use client";

import { useState, useEffect, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { VCSStatus } from "@/gen/session/v1/types_pb";

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

  const fetchVcs = useCallback(
    async (skipCache = false) => {
      if (!skipCache) {
        const cached = getCached(sessionId);
        if (cached) {
          setData(cached.data);
          setError(cached.error);
          setLoading(false);
          return;
        }
      }

      try {
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.getVCSStatus({ id: sessionId });
        const entry: VcsCacheEntry = {
          data: response.vcsStatus ?? null,
          error: response.error || null,
          timestamp: Date.now(),
        };
        vcsCache.set(sessionId, entry);
        setData(entry.data);
        setError(entry.error);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load VCS status");
      } finally {
        setLoading(false);
      }
    },
    [sessionId, baseUrl] // eslint-disable-line react-hooks/exhaustive-deps
  );

  useEffect(() => {
    fetchVcs();
    const interval = setInterval(() => {
      if (!document.hidden) fetchVcs();
    }, pollIntervalMs);
    return () => clearInterval(interval);
  }, [sessionId, baseUrl]); // eslint-disable-line react-hooks/exhaustive-deps

  return { data, loading, error, refetch: () => fetchVcs(true) };
}
