"use client";

import { useEffect, useCallback, useRef, useMemo, useState } from "react";
import type { AsyncResult } from "@/lib/types/asyncResult";
import { createClient } from "@connectrpc/connect";
import { getWatchTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import type {
  ReviewQueue,
  ReviewItem,
  Priority,
  AttentionReason} from "@/gen/session/v1/types_pb";
import {
  SessionStatus,
} from "@/gen/session/v1/types_pb";
import {
  GetReviewQueueRequest,
  GetReviewQueueRequestSchema,
  WatchReviewQueueRequest,
  WatchReviewQueueRequestSchema,
  AcknowledgeSessionRequest,
  AcknowledgeSessionRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import type { ReviewQueueEvent } from "@/gen/session/v1/events_pb";
import { SessionEvent } from "@/gen/session/v1/events_pb";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  setReviewQueue as setReviewQueueAction,
  addItem,
  updateItem,
  removeItem,
  setLoading,
  setError,
  selectReviewQueue,
  selectReviewQueueLoading,
  selectReviewQueueError,
  selectReviewQueueLastUpdatedAt,
  selectReviewQueueItemsWithLiveStatus,
} from "@/lib/store/reviewQueueSlice";
import { isReviewQueueVisible } from "@/lib/utils/reviewQueueVisibility";
import { useWatchStream, type WatchConnectionState } from "@/lib/hooks/useWatchStream";

// How long a reconciled row stays visible-but-disabled (ux.md Surface 9) before the
// deferred removeItem dispatch actually removes it from the store.
const AUTO_RESOLVED_DISPLAY_WINDOW_MS = 5000;

interface UseReviewQueueOptions {
  autoRefresh?: boolean;
  refreshInterval?: number; // in milliseconds
  priorityFilter?: Priority;
  reasonFilter?: AttentionReason;
  useWebSocketPush?: boolean; // Enable WebSocket push updates
  fallbackPollInterval?: number; // Fallback polling interval (default: 30000ms)
  /**
   * Called when an `item_removed` event carries `autoResolvedByRule` (Epic 2.3.1) —
   * i.e. rule-reconciliation resolved this item while it may still be visible.
   * Fires immediately, before the ~5s display window elapses and the item is
   * actually removed (see `autoResolvedRules` below for the same information as
   * hook state, which most consumers should prefer over this callback).
   */
  onAutoResolved?: (sessionId: string, ruleName: string) => void;
}

interface UseReviewQueueReturn extends AsyncResult {
  // State
  reviewQueue: ReviewQueue | null;
  items: ReviewItem[];
  // Set on every successful fetch (GetReviewQueue or WatchReviewQueue's initial snapshot
  // arriving via a later refresh); null means "never completed a successful fetch." Drives
  // the staleness indicator / narrowed error-takeover condition (Task 3.2.1d, AC38).
  lastUpdatedAt: number | null;
  // Session ID -> rule display name, for any item currently in its ~5s
  // "disable, don't hide" display window after a reconciliation-driven removal
  // (Epic 2.3.2, ux.md Surface 9). The item stays present in `items` for the
  // whole window; consumers use this map to render it disabled with a banner
  // instead of removing the row immediately.
  autoResolvedRules: Record<string, string>;

  // Statistics
  totalItems: number;
  byPriority: Map<Priority, number>;
  byReason: Map<AttentionReason, number>;
  averageAgeSeconds: bigint;
  oldestItemId: string;
  oldestAgeSeconds: bigint;

  // Methods
  refresh: () => Promise<void>;
  getByPriority: (priority: Priority) => Promise<ReviewQueue | null>;
  getByReason: (reason: AttentionReason) => Promise<ReviewQueue | null>;
  acknowledgeSession: (sessionId: string) => Promise<void>;
  acknowledgeSessions: (sessionIds: string[]) => Promise<{ failed: string[] }>;
}

/**
 * React hook for managing review queue data with real-time push updates and optimistic UI.
 *
 * The review queue tracks sessions that need user attention, ordered by priority.
 * Supports optional filtering by priority level or attention reason.
 *
 * **Update Strategy:**
 * - **Push (WebSocket)**: Direct WatchReviewQueue stream for <100ms queue updates
 * - **Optimistic Updates**: UI updates immediately on user actions (acknowledge)
 * - **Pull (Polling)**: Fallback polling (30s) for eventual consistency
 * - **Initial Snapshot**: Immediate queue state on connection
 *
 * **Performance:**
 * - <100ms latency for queue updates (vs 7-32 seconds with polling)
 * - Zero flickering - direct queue event stream
 * - Reduced server load - targeted updates only
 *
 * @example
 * ```tsx
 * // Default: WebSocket push + optimistic updates
 * const { items, totalItems, acknowledgeSession } = useReviewQueue();
 *
 * // Acknowledge with immediate UI feedback
 * await acknowledgeSession('session-id');
 *
 * // Legacy polling-only mode
 * const { items } = useReviewQueue({
 *   useWebSocketPush: false,
 *   autoRefresh: true,
 *   refreshInterval: 5000,
 * });
 * ```
 */
const LIVE_RECONCILE_MS = 120_000;
const RECONCILE_MIN_GAP_MS = 10_000;

export function useReviewQueue(
  options: UseReviewQueueOptions = {}
): UseReviewQueueReturn {
  const {
    autoRefresh = false,
    refreshInterval = 5000,
    priorityFilter,
    reasonFilter,
    useWebSocketPush = true, // Enable WebSocket push by default
    fallbackPollInterval = 30000, // 30 second fallback polling
    onAutoResolved,
  } = options;

  // Ref'd so the itemRemoved handler (set once, see handleReviewQueueEventRef below)
  // always calls the latest callback without needing it in that effect's deps.
  const onAutoResolvedRef = useRef(onAutoResolved);
  useEffect(() => {
    onAutoResolvedRef.current = onAutoResolved;
  }, [onAutoResolved]);

  // sessionId -> rule name for items in their post-reconciliation display window
  // (ux.md Surface 9's "disable, don't hide"). See autoResolvedRules on the return type.
  const [autoResolvedRules, setAutoResolvedRules] = useState<Record<string, string>>({});
  // Pending deferred-removal timers, keyed by sessionId, so they can be cleared on unmount.
  const autoResolveTimeoutsRef = useRef<Record<string, ReturnType<typeof setTimeout>>>({});
  useEffect(() => {
    const timeouts = autoResolveTimeoutsRef.current;
    return () => {
      for (const id of Object.values(timeouts)) clearTimeout(id);
    };
  }, []);

  const dispatch = useAppDispatch();
  const reviewQueue = useAppSelector(selectReviewQueue);
  const liveItems = useAppSelector(selectReviewQueueItemsWithLiveStatus);
  const loading = useAppSelector(selectReviewQueueLoading);
  const errorStr = useAppSelector(selectReviewQueueError);
  const lastUpdatedAt = useAppSelector(selectReviewQueueLastUpdatedAt);

  // Always-fresh refs for fetchReviewQueue (declared with a stable [dispatch]
  // dep array below) to read current queue/window state without a stale closure.
  const reviewQueueRef = useRef(reviewQueue);
  reviewQueueRef.current = reviewQueue;
  const autoResolvedRulesRef = useRef(autoResolvedRules);
  autoResolvedRulesRef.current = autoResolvedRules;

  // Idle-reason items no longer occupy review-queue slots (Epic 3.2.2, ADR-002) — filtered
  // once here so every consumer (ReviewQueuePanel, ReviewQueueNavBadge, BottomNav, DrawerNav)
  // sees the same list, and totalItems (below) is derived from this same filtered array
  // rather than the raw backend stat, avoiding a count-vs-list mismatch.
  const filteredItems = useMemo(
    () => liveItems.filter(isReviewQueueVisible),
    [liveItems]
  );

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const lastUpdateRef = useRef<number>(Date.now());

  // Initialize ConnectRPC client — uses HTTP for unary, WebSocket for streaming Watch* RPCs
  useEffect(() => {
    clientRef.current = createClient(SessionService, getWatchTransport());
  }, []);

  // Stream events applied so far; a background REST reconcile that started before one is stale and is dropped.
  const eventCountRef = useRef(0);
  const refreshInFlightRef = useRef(false);
  const lastRefreshEndRef = useRef(0);

  // Fetch review queue with optional filters. A background fetch (reconcile while the stream is live) does not
  // flash the loading state and yields to any stream event that landed while it was in flight.
  const fetchReviewQueue = useCallback(
    async (
      filters?: {
        priorityFilter?: Priority;
        reasonFilter?: AttentionReason;
      },
      background = false
    ) => {
      if (!clientRef.current) return;

      refreshInFlightRef.current = true;
      const eventsAtStart = eventCountRef.current;
      if (!background) dispatch(setLoading(true));
      dispatch(setError(null));

      try {
        const request = create(GetReviewQueueRequestSchema, {});

        // Apply filters if provided
        if (filters?.priorityFilter !== undefined) {
          request.priorityFilter = filters.priorityFilter;
        }
        if (filters?.reasonFilter !== undefined) {
          request.reasonFilter = filters.reasonFilter;
        }

        const response = await clientRef.current.getReviewQueue(request);
        if (background && eventCountRef.current !== eventsAtStart) return; // stream state is newer; next reconcile heals

        // A REST fetch (including the fallback poll) fully replaces the queue, but
        // an item in its ~5s post-reconciliation display window (autoResolvedRules)
        // has already been deleted backend-side — a poll landing mid-window would
        // otherwise evict the row early while its banner lingers. Re-splice any such
        // item back in from the current store state before replacing (finding #2).
        let reviewQueueToStore = response.reviewQueue ?? null;
        const preserveIds = Object.keys(autoResolvedRulesRef.current);
        if (preserveIds.length > 0 && reviewQueueToStore) {
          const existingItems = reviewQueueRef.current?.items ?? [];
          const incomingIds = new Set(
            reviewQueueToStore.items.map((item) => item.sessionId)
          );
          const toPreserve = existingItems.filter(
            (item) =>
              preserveIds.includes(item.sessionId) && !incomingIds.has(item.sessionId)
          );
          if (toPreserve.length > 0) {
            reviewQueueToStore = {
              ...reviewQueueToStore,
              items: [...reviewQueueToStore.items, ...toPreserve],
              totalItems: reviewQueueToStore.totalItems + toPreserve.length,
            };
          }
        }

        dispatch(setReviewQueueAction(reviewQueueToStore));
        dispatch(setError(null));
      } catch (err) {
        const error =
          err instanceof Error
            ? err
            : new Error("Failed to fetch review queue");
        dispatch(setError(error.message));
        console.error("Failed to fetch review queue:", error);
      } finally {
        refreshInFlightRef.current = false;
        lastRefreshEndRef.current = Date.now();
        if (!background) dispatch(setLoading(false));
      }
    },
    [dispatch]
  );

  // Refresh with current filters
  const refresh = useCallback(async () => {
    await fetchReviewQueue({ priorityFilter, reasonFilter });
  }, [fetchReviewQueue, priorityFilter, reasonFilter]);

  // Reconcile over REST while the stream is live: never concurrent with another refresh and at most one per
  // RECONCILE_MIN_GAP_MS, so a flapping stream (or the mount fetch right before the first connect) costs nothing.
  const reconcile = useCallback(async () => {
    if (refreshInFlightRef.current || Date.now() - lastRefreshEndRef.current < RECONCILE_MIN_GAP_MS) return;
    await fetchReviewQueue({ priorityFilter, reasonFilter }, true);
  }, [fetchReviewQueue, priorityFilter, reasonFilter]);

  // Get review queue filtered by priority
  const getByPriority = useCallback(
    async (priority: Priority): Promise<ReviewQueue | null> => {
      if (!clientRef.current) return null;

      try {
        const response = await clientRef.current.getReviewQueue({
          priorityFilter: priority,
        });
        return response.reviewQueue ?? null;
      } catch (err) {
        dispatch(
          setError(
            err instanceof Error ? err.message : "Failed to fetch by priority"
          )
        );
        return null;
      }
    },
    [dispatch]
  );

  // Get review queue filtered by reason
  const getByReason = useCallback(
    async (reason: AttentionReason): Promise<ReviewQueue | null> => {
      if (!clientRef.current) return null;

      try {
        const response = await clientRef.current.getReviewQueue({
          reasonFilter: reason,
        });
        return response.reviewQueue ?? null;
      } catch (err) {
        dispatch(
          setError(
            err instanceof Error ? err.message : "Failed to fetch by reason"
          )
        );
        return null;
      }
    },
    [dispatch]
  );

  // Handle review queue events from dedicated WatchReviewQueue stream
  // Use ref callback to avoid dependency issues that would cause stream reconnects.
  // The handler only uses dispatch with functional updates (no stale closures),
  // so it only needs to be set once on mount.
  const handleReviewQueueEventRef = useRef<((event: ReviewQueueEvent) => void) | undefined>(undefined);

  useEffect(() => {
    handleReviewQueueEventRef.current = (event: ReviewQueueEvent) => {
      switch (event.event.case) {
        case "itemAdded":
          if (event.event.value.item) {
            dispatch(addItem(event.event.value.item));
          }
          break;

        case "itemRemoved": {
          const sessionId = event.event.value.sessionId;
          if (!sessionId) break;

          const ruleName = event.event.value.autoResolvedByRule;
          if (ruleName) {
            // Reconciliation-driven removal (Epic 2.3.1): disable the row in place
            // rather than removing it immediately (ux.md Surface 9) — surface the
            // rule name now, defer the actual removeItem dispatch for ~5s so the
            // disabled state + banner is visible first.
            setAutoResolvedRules((prev) => ({ ...prev, [sessionId]: ruleName }));
            onAutoResolvedRef.current?.(sessionId, ruleName);

            const timeoutId = setTimeout(() => {
              dispatch(removeItem(sessionId));
              setAutoResolvedRules((prev) => {
                const next = { ...prev };
                delete next[sessionId];
                return next;
              });
              delete autoResolveTimeoutsRef.current[sessionId];
            }, AUTO_RESOLVED_DISPLAY_WINDOW_MS);
            autoResolveTimeoutsRef.current[sessionId] = timeoutId;
          } else {
            dispatch(removeItem(sessionId));
          }
          break;
        }

        case "itemUpdated":
          if (event.event.value.item && event.event.value.sessionId) {
            dispatch(
              updateItem({
                sessionId: event.event.value.sessionId,
                updates: event.event.value.item,
              })
            );
          }
          break;

        case "statistics":
          // Statistics events are currently not applied to the Redux store; the fallback poll refreshes the full queue.
          break;

        default:
          break;
      }
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dispatch]);

  // Setup WebSocket push updates with dedicated WatchReviewQueue stream.
  // Connection mechanics (backoff reconnect, idle-staleness watchdog) live in
  // the shared useWatchStream.ts -- this only supplies the request shape and
  // event dispatch. ReviewQueueEvent has no seq/afterSeq replay buffer (the
  // review queue manager isn't built on pkg/events.EventBus like the other
  // three Watch* streams), so getSeq is omitted; a stall or filter change
  // just reconnects to a fresh initialSnapshot.
  const subscribeReviewQueue = useCallback(
    (_afterSeq: bigint, signal: AbortSignal) => {
      const request = create(WatchReviewQueueRequestSchema, {
        priorityFilter: priorityFilter !== undefined ? [priorityFilter] : [],
        reasonFilter: reasonFilter !== undefined ? [reasonFilter] : [],
        initialSnapshot: true,
        includeStatistics: true,
      });
      const client = clientRef.current;
      if (!client) throw new Error("review queue client not initialised");
      return client.watchReviewQueue(request, { signal });
    },
    [priorityFilter, reasonFilter]
  );

  const onReviewQueueEvent = useCallback((event: ReviewQueueEvent) => {
    eventCountRef.current++;
    handleReviewQueueEventRef.current?.(event);
  }, []);

  // The REST poll below only runs while the stream is not delivering.
  const [streamLive, setStreamLive] = useState(false);
  const onStreamState = useCallback((state: WatchConnectionState) => setStreamLive(state === "live"), []);

  // A filter change or push toggle restarts the stream loop without emitting a state, so liveness is reset here.
  const streamKey = `${useWebSocketPush}:${priorityFilter ?? ""}:${reasonFilter ?? ""}`;
  useEffect(() => {
    setStreamLive(false);
  }, [streamKey]);

  useWatchStream<ReviewQueueEvent>({
    subscribe: subscribeReviewQueue,
    onConnectionStateChange: onStreamState,
    onEvent: onReviewQueueEvent,
    isHeartbeat: (event) => event.heartbeat,
    enabled: useWebSocketPush,
    restartKey: `${priorityFilter ?? ""}:${reasonFilter ?? ""}`,
  });

  // Keep a ref to the latest refresh so interval callbacks are always current
  // without needing refresh in the interval-setup effect's dep array.
  const refreshRef = useRef(refresh);
  const reconcileRef = useRef(reconcile);
  useEffect(() => {
    refreshRef.current = refresh;
    reconcileRef.current = reconcile;
  }, [refresh, reconcile]);

  // A reconnect snapshot only re-adds items, so items removed during a gap would linger: reconcile over REST
  // on every transition to live.
  useEffect(() => {
    if (streamLive) void reconcileRef.current();
  }, [streamLive]);

  // Always do an initial REST fetch on mount for immediate fresh data.
  // Even in WebSocket push mode the stream may take a moment to connect and
  // deliver its initialSnapshot, leaving the page empty in the meantime.
  // The REST response fills the UI instantly; the stream then overlays updates.
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // intentionally empty -- run once on mount

  // Setup fallback polling or legacy auto-refresh.
  // Intentionally excludes `refresh` from deps; uses refreshRef.current instead
  // so that filter changes (which change `refresh` identity) don't cause an
  // immediate duplicate fetch -- the WatchReviewQueue stream re-connects on
  // filter changes and delivers a fresh initialSnapshot. useWatchStream's own
  // backoff + idle-staleness watchdog self-heal the stream independently, so
  // this poll no longer needs to coordinate a reconnect -- it's just a REST
  // consistency backstop (hybrid mode) or the sole refresh source (legacy).
  useEffect(() => {
    let interval: NodeJS.Timeout | null = null;

    if (useWebSocketPush) {
      // A live stream can still drop an event (the server's client channel is bounded and heartbeats keep the
      // connection looking healthy), so a slow reconcile poll runs while live; the full-rate poll runs when it is not.
      interval = setInterval(
        () => void (streamLive ? reconcileRef.current() : refreshRef.current()),
        streamLive ? Math.max(fallbackPollInterval, LIVE_RECONCILE_MS) : fallbackPollInterval
      );
    } else if (autoRefresh) {
      // Legacy mode: Use original refresh interval
      interval = setInterval(() => {
        refreshRef.current();
      }, refreshInterval);
    }

    return () => {
      if (interval) {
        clearInterval(interval);
      }
    };
  }, [useWebSocketPush, streamLive, autoRefresh, refreshInterval, fallbackPollInterval]);

  // Acknowledge session with optimistic update
  const acknowledgeSession = useCallback(
    async (sessionId: string) => {
      if (!clientRef.current) return;

      // Optimistic update via slice action — no closure dependency on reviewQueue.
      // The removeItem reducer handles the filter + totalItems decrement internally.
      dispatch(removeItem(sessionId));

      try {
        const request = create(AcknowledgeSessionRequestSchema, { id: sessionId });
        await clientRef.current.acknowledgeSession(request);
        // Success - optimistic update was correct
      } catch (err) {
        console.error("Failed to acknowledge session:", err);
        dispatch(
          setError(
            err instanceof Error
              ? err.message
              : "Failed to acknowledge session"
          )
        );
        // Rollback - refetch to get correct state
        await refresh();
      }
    },
    [refresh, dispatch]
  );

  // Acknowledges multiple sessions in parallel. Unlike acknowledgeSession, a per-item
  // RPC failure does not dispatch the global queue error (which would blank the whole
  // panel) — failures are collected and returned so the caller can show a scoped result
  // instead.
  const acknowledgeSessions = useCallback(
    async (sessionIds: string[]): Promise<{ failed: string[] }> => {
      const client = clientRef.current;
      if (!client) return { failed: sessionIds };

      sessionIds.forEach((id) => dispatch(removeItem(id)));

      const results = await Promise.allSettled(
        sessionIds.map((id) => {
          const request = create(AcknowledgeSessionRequestSchema, { id });
          return client.acknowledgeSession(request);
        })
      );

      const failed = sessionIds.filter((_, i) => results[i].status === "rejected");
      if (failed.length > 0) {
        console.error(`Failed to acknowledge ${failed.length} of ${sessionIds.length} sessions`);
        // Reconcile optimistic removals against real server state for the failed items.
        await refresh();
      }

      return { failed };
    },
    [refresh, dispatch]
  );

  // Extract statistics from review queue
  const statistics = {
    // Filtered items.length, not the raw backend stat (reviewQueue?.totalItems counts ALL
    // items including idle ones — session/queue/queue.go's TotalItems is deliberately
    // unchanged per ADR-002's scope boundary). Every ReviewQueuePanel headline-count read
    // sources totalItems from this hook, so fixing it here fixes every read site at once
    // (adversarial-review.md Blocker 2).
    totalItems: filteredItems.length,
    byPriority: new Map<Priority, number>(
      Object.entries(reviewQueue?.byPriority ?? {}).map(([key, value]) => [
        parseInt(key) as Priority,
        value,
      ])
    ),
    byReason: new Map<AttentionReason, number>(
      Object.entries(reviewQueue?.byReason ?? {}).map(([key, value]) => [
        parseInt(key) as AttentionReason,
        value,
      ])
    ),
    averageAgeSeconds: reviewQueue?.averageAgeSeconds ?? BigInt(0),
    oldestItemId: reviewQueue?.oldestItemId ?? "",
    oldestAgeSeconds: reviewQueue?.oldestAgeSeconds ?? BigInt(0),
  };

  // Convert error string back to Error object for backward compatibility.
  // Memoised so the Error identity stays stable across renders.
  const error = useMemo(() => (errorStr ? new Error(errorStr) : null), [errorStr]);

  return {
    reviewQueue,
    items: filteredItems,
    lastUpdatedAt,
    autoResolvedRules,
    loading,
    error,
    ...statistics,
    refresh,
    getByPriority,
    getByReason,
    acknowledgeSession,
    acknowledgeSessions,
  };
}
