"use client";

import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import {
  BacklogService,
  ListStuckBacklogItemsRequestSchema,
  SnoozeStuckItemRequestSchema,
  ResetStuckRemediationRequestSchema,
  BulkResetStuckRemediationRequestSchema,
  TriggerRemediationNowRequestSchema,
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
  /** Clears remediation_attempts/next_remediation_at/notified_at on a single open row. */
  resetRemediation: (itemId: string, reason: StuckReason) => Promise<boolean>;
  /** Resets every open row hitting the attempt cap (parked); returns the count reset. */
  bulkResetParkedRemediation: () => Promise<number>;
  /**
   * Immediately runs the reason's remediation action, bypassing only the
   * backoff timer — still subject to the 5-attempt cap and the wrapped
   * action's own circuit breaker. Throws (does not swallow) on failure so
   * callers can show a specific error, e.g. "already parked — use Reset".
   */
  triggerRemediationNow: (itemId: string, reason: StuckReason) => Promise<void>;
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
 *
 * `enabled=false` skips the fetch+poll effect entirely (state stays at its
 * initial empty/loading values) — used by the public `useStuckBacklogItems`
 * hook below to disable this instance's own polling when a
 * `StuckBacklogItemsProvider` ancestor already owns the single shared poll
 * (research/pitfalls.md #5: StuckItemsSection and StuckNavBadge previously
 * ran two independent 60s pollers that could disagree for up to a minute).
 */
function useStuckBacklogItemsImpl(
  pollIntervalMs: number,
  enabled: boolean
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
    if (!enabled) return;
    fetchItems();
    const interval = setInterval(() => {
      if (!document.hidden) fetchItems();
    }, pollIntervalMs);
    return () => clearInterval(interval);
  }, [enabled, fetchItems, pollIntervalMs]);

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

  const resetRemediation = useCallback(
    async (itemId: string, reason: StuckReason): Promise<boolean> => {
      try {
        const req = create(ResetStuckRemediationRequestSchema, { itemId, reason });
        const res = await client.resetStuckRemediation(req);
        if (res.applied) {
          await fetchItems();
        }
        return res.applied;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to reset stuck remediation"));
        return false;
      }
    },
    [client, fetchItems]
  );

  const bulkResetParkedRemediation = useCallback(async (): Promise<number> => {
    try {
      const req = create(BulkResetStuckRemediationRequestSchema, {
        onlyParked: true,
        onlyParkedExplicitlySet: true,
      });
      const res = await client.bulkResetStuckRemediation(req);
      if (res.resetCount > 0) {
        await fetchItems();
      }
      return res.resetCount;
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to bulk reset stuck remediation"));
      return 0;
    }
  }, [client, fetchItems]);

  const triggerRemediationNow = useCallback(
    async (itemId: string, reason: StuckReason): Promise<void> => {
      const req = create(TriggerRemediationNowRequestSchema, { itemId, reason });
      // Deliberately NOT try/catch-swallowed here (unlike snooze/reset above):
      // callers need the specific error (e.g. "already parked") to show the
      // operator why the retry didn't run, not just a generic failure.
      await client.triggerRemediationNow(req);
      await fetchItems();
    },
    [client, fetchItems]
  );

  return {
    items,
    isLoading,
    error,
    lastFetched,
    refetch: fetchItems,
    snooze,
    resetRemediation,
    bulkResetParkedRemediation,
    triggerRemediationNow,
  };
}

const StuckBacklogItemsContext = createContext<UseStuckBacklogItemsReturn | null>(null);

/**
 * Mount once, high in the tree (Providers.tsx) — every descendant calling
 * `useStuckBacklogItems()` then shares this single poll instead of each
 * mounting its own, closing the "two independently-polled copies can
 * disagree for up to a minute" gap (research/pitfalls.md #5).
 */
export function StuckBacklogItemsProvider({ children }: { children: ReactNode }) {
  const value = useStuckBacklogItemsImpl(DEFAULT_POLL_INTERVAL_MS, true);
  return createElement(StuckBacklogItemsContext.Provider, { value }, children);
}

/**
 * Public hook — unchanged name/shape so every existing call site and every
 * existing `jest.mock("@/lib/hooks/useStuckBacklogItems", ...)` keeps working
 * unmodified. When called under a `StuckBacklogItemsProvider`, returns the
 * shared polled value; otherwise falls back to running its own standalone
 * poll (e.g. a test rendering a consumer with no provider wrapper).
 */
export function useStuckBacklogItems(
  pollIntervalMs: number = DEFAULT_POLL_INTERVAL_MS
): UseStuckBacklogItemsReturn {
  const ctx = useContext(StuckBacklogItemsContext);
  // Always called (Rules of Hooks) — `enabled: ctx === null` just stops it
  // from fetching/polling once a provider ancestor already owns that.
  const standalone = useStuckBacklogItemsImpl(pollIntervalMs, ctx === null);
  return ctx ?? standalone;
}
