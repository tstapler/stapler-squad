"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import {
  BacklogService,
  ListStuckBacklogItemsRequestSchema,
  SnoozeStuckItemRequestSchema,
  type StuckBacklogItem,
  type StuckReason,
} from "@/gen/session/v1/backlog_pb";

/** Baseline poll interval — matches the backend's 60s ReconcileStuck tick cadence. */
const DEFAULT_POLL_INTERVAL_MS = 60_000;

export interface UseStuckBacklogItemsReturn {
  items: StuckBacklogItem[];
  isLoading: boolean;
  error: Error | null;
  lastFetched: Date | null;
  refetch: () => Promise<void>;
  snooze: (itemId: string, reason: StuckReason, until: Date) => Promise<boolean>;
}

/**
 * Polled hook exposing the open (unresolved, un-snoozed) stuck backlog items.
 *
 * Modeled on useUnfinishedWork.ts's shape ({ items, isLoading, error, ... })
 * but unary+interval (ListStuckBacklogItems is not a streaming RPC), not
 * WatchUnfinishedWork's subscribe-and-diff pattern. Deliberately avoids
 * useUnfinishedWork's anti-pattern of constructing a fresh transport/client on
 * every render (pitfalls.md §5) — transport/client are memoized here.
 *
 * On a fetch error, the previous `items` are retained (never blanked) and
 * `error` is populated so the UI can show a "may be out of date" banner
 * rather than a false-confidence empty list (design/ux.md Surface 6).
 */
export function useStuckBacklogItems(
  pollIntervalMs: number = DEFAULT_POLL_INTERVAL_MS
): UseStuckBacklogItemsReturn {
  const [items, setItems] = useState<StuckBacklogItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [lastFetched, setLastFetched] = useState<Date | null>(null);

  // Memoized transport/client — the explicit pitfall this hook must NOT
  // reproduce from useUnfinishedWork.ts (which builds a new
  // createConnectTransport/createClient pair on every render).
  const transport = useMemo(() => getConnectTransport(), []);
  const client = useMemo(() => createClient(BacklogService, transport), [transport]);

  const inFlightRef = useRef(false);

  const fetchItems = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    try {
      const req = create(ListStuckBacklogItemsRequestSchema, {});
      const res = await client.listStuckBacklogItems(req);
      setItems(res.items);
      setLastFetched(new Date());
      setError(null);
    } catch (err) {
      // Retain the last-known `items` — do not blank the list on a
      // transient poll failure (design/ux.md Surface 6).
      setError(err instanceof Error ? err : new Error("Failed to load stuck backlog items"));
    } finally {
      setIsLoading(false);
      inFlightRef.current = false;
    }
  }, [client]);

  useEffect(() => {
    fetchItems();
    const interval = setInterval(() => {
      if (!document.hidden) fetchItems();
    }, pollIntervalMs);
    return () => clearInterval(interval);
  }, [fetchItems, pollIntervalMs]);

  const snooze = useCallback(
    async (itemId: string, reason: StuckReason, until: Date): Promise<boolean> => {
      try {
        const req = create(SnoozeStuckItemRequestSchema, {
          itemId,
          reason,
          until: timestampFromDate(until),
        });
        const res = await client.snoozeStuckItem(req);
        if (res.applied) {
          await fetchItems();
        }
        return res.applied;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to snooze stuck item"));
        return false;
      }
    },
    [client, fetchItems]
  );

  return { items, isLoading, error, lastFetched, refetch: fetchItems, snooze };
}
