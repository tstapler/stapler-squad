"use client";

import { useState, useEffect, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { BacklogItemShipStatus } from "@/gen/session/v1/backlog_pb";

interface UseBacklogItemShipStatusResult {
  data: BacklogItemShipStatus | null;
  loading: boolean;
  refetch: () => void;
}

/**
 * Fetches whether a backlog item's code actually shipped to main, from
 * durable evidence (repo_path + the most recent work session's commit)
 * rather than a live per-session worktree. Unlike useVcsStatus, this works
 * even after a session's worktree has been cleaned up — the normal end
 * state for a done item — so it's the fallback for the Version Control
 * section once the live VCSStatus widget has nothing to show.
 *
 * No polling: shipped status doesn't change on its own once a session has
 * ended, so a one-shot fetch (plus manual refetch) is enough.
 */
export function useBacklogItemShipStatus(itemId: string): UseBacklogItemShipStatusResult {
  const [data, setData] = useState<BacklogItemShipStatus | null>(null);
  const [loading, setLoading] = useState(!!itemId);

  const fetchStatus = useCallback(async () => {
    if (!itemId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const client = createClient(BacklogService, getConnectTransport());
      const response = await client.getBacklogItemShipStatus({ itemId });
      setData(response.status ?? null);
    } catch {
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [itemId]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  return { data, loading, refetch: fetchStatus };
}
