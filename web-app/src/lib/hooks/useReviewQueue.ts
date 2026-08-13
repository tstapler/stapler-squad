"use client";

import { useEffect, useCallback, useRef, useMemo } from "react";
import type { AsyncResult } from "@/lib/types/asyncResult";
import { createClient } from "@connectrpc/connect";
import { createWatchTransport } from "@/lib/transport/watch-ws-transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import {
  ReviewQueue,
  ReviewItem,
  Priority,
  AttentionReason,
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
import { SessionEvent, ReviewQueueEvent } from "@/gen/session/v1/events_pb";
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
  selectReviewQueueItemsWithLiveStatus,
} from "@/lib/store/reviewQueueSlice";

interface UseReviewQueueOptions {
  baseUrl?: string;
  autoRefresh?: boolean;
  refreshInterval?: number; // in milliseconds
  priorityFilter?: Priority;
  reasonFilter?: AttentionReason;
  useWebSocketPush?: boolean; // Enable WebSocket push updates
  fallbackPollInterval?: number; // Fallback polling interval (default: 30000ms)
}

interface UseReviewQueueReturn extends AsyncResult {
  // State
  reviewQueue: ReviewQueue | null;
  items: ReviewItem[];

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
export function useReviewQueue(
  options: UseReviewQueueOptions = {}
): UseReviewQueueReturn {
  const {
    baseUrl = getApiBaseUrl(),
    autoRefresh = false,
    refreshInterval = 5000,
    priorityFilter,
    reasonFilter,
    useWebSocketPush = true, // Enable WebSocket push by default
    fallbackPollInterval = 30000, // 30 second fallback polling
  } = options;

  const dispatch = useAppDispatch();
  const reviewQueue = useAppSelector(selectReviewQueue);
  const liveItems = useAppSelector(selectReviewQueueItemsWithLiveStatus);
  const loading = useAppSelector(selectReviewQueueLoading);
  const errorStr = useAppSelector(selectReviewQueueError);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const lastUpdateRef = useRef<number>(Date.now());
  // Stream reconnect state — shared between the WebSocket effect and the fallback poll.
  // streamDeadRef is set when MAX_RETRIES are exhausted; the fallback poll clears it
  // and triggers a reconnect after a successful REST fetch.
  const streamDeadRef = useRef<boolean>(false);
  const streamRetriesRef = useRef<number>(0);

  // Initialize ConnectRPC client — uses HTTP for unary, WebSocket for streaming Watch* RPCs
  useEffect(() => {
    const transport = createWatchTransport({
      baseUrl,
      interceptors: [createAuthInterceptor()],
    });

    clientRef.current = createClient(SessionService, transport);
  }, [baseUrl]);

  // Fetch review queue with optional filters
  const fetchReviewQueue = useCallback(
    async (filters?: {
      priorityFilter?: Priority;
      reasonFilter?: AttentionReason;
    }) => {
      if (!clientRef.current) return;

      dispatch(setLoading(true));
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

        dispatch(setReviewQueueAction(response.reviewQueue ?? null));
        dispatch(setError(null));
      } catch (err) {
        const error =
          err instanceof Error
            ? err
            : new Error("Failed to fetch review queue");
        dispatch(setError(error.message));
        console.error("Failed to fetch review queue:", error);
      } finally {
        dispatch(setLoading(false));
      }
    },
    [dispatch]
  );

  // Refresh with current filters
  const refresh = useCallback(async () => {
    await fetchReviewQueue({ priorityFilter, reasonFilter });
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

        case "itemRemoved":
          if (event.event.value.sessionId) {
            dispatch(removeItem(event.event.value.sessionId));
          }
          break;

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

  // Setup WebSocket push updates with dedicated WatchReviewQueue stream
  useEffect(() => {
    if (!useWebSocketPush || !clientRef.current) return;

    // Stop any existing watch
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    abortControllerRef.current = new AbortController();
    const signal = abortControllerRef.current.signal;

    // Reset reconnect state when the effect re-runs (e.g. filter change).
    streamDeadRef.current = false;
    streamRetriesRef.current = 0;
    const MAX_RETRIES = 5;

    const connect = async () => {
      if (signal.aborted) return;
      try {
        const request = create(WatchReviewQueueRequestSchema, {
          // Apply current filters
          priorityFilter: priorityFilter !== undefined ? [priorityFilter] : [],
          reasonFilter: reasonFilter !== undefined ? [reasonFilter] : [],
          // Get initial snapshot for immediate UI sync
          initialSnapshot: true,
          // Include statistics for queue metrics
          includeStatistics: true,
        });

        const stream = clientRef.current!.watchReviewQueue(request, { signal });

        for await (const event of stream) {
          handleReviewQueueEventRef.current?.(event);
        }
        // Clean close — reset retry counter
        streamRetriesRef.current = 0;
      } catch (err) {
        // Ignore abort errors (intentional cleanup)
        if (err instanceof Error && err.name === "AbortError") return;
        if (signal.aborted) return;

        console.error("WatchReviewQueue stream error:", err);

        if (streamRetriesRef.current < MAX_RETRIES) {
          const delay = Math.min(1000 * Math.pow(2, streamRetriesRef.current), 30000);
          streamRetriesRef.current++;
          setTimeout(() => {
            // F5: Re-check the abort signal before reconnecting — the effect may
            // have cleaned up (filter change, unmount) between the timer being
            // scheduled and firing. Without this check the old closure's connect()
            // would race against the new effect's connect() on the same signal.
            if (signal.aborted) return;
            void connect();
          }, delay);
        } else {
          // Exhausted retries — fallback polling will handle consistency and
          // attempt a reconnect after the next successful REST fetch (F4).
          streamDeadRef.current = true;
          console.warn("WatchReviewQueue: max reconnect attempts reached, relying on fallback poll");
        }
      }
    };

    // Expose reconnect for the fallback poll to call after a successful REST fetch
    // when the stream has given up (streamDeadRef = true). The fallback poll resets
    // streamDeadRef and streamRetriesRef before calling this.
    streamReconnectRef.current = () => void connect();

    void connect();

    return () => {
      streamReconnectRef.current = null;
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }
    };
  // streamDeadRef, streamRetriesRef, and streamReconnectRef are stable refs — no need to list them.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [useWebSocketPush, priorityFilter, reasonFilter]);

  // Keep a ref to the latest refresh so interval callbacks are always current
  // without needing refresh in the interval-setup effect's dep array.
  const refreshRef = useRef(refresh);
  useEffect(() => {
    refreshRef.current = refresh;
  }, [refresh]);

  // Always do an initial REST fetch on mount for immediate fresh data.
  // Even in WebSocket push mode the stream may take a moment to connect and
  // deliver its initialSnapshot, leaving the page empty in the meantime.
  // The REST response fills the UI instantly; the stream then overlays updates.
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // intentionally empty -- run once on mount

  // Ref to a reconnect function that the WebSocket effect exposes for the
  // fallback poll to call when the stream is dead and REST confirms reachability.
  const streamReconnectRef = useRef<(() => void) | null>(null);

  // Setup fallback polling or legacy auto-refresh.
  // Intentionally excludes `refresh` from deps; uses refreshRef.current instead
  // so that filter changes (which change `refresh` identity) don't cause an
  // immediate duplicate fetch -- the WatchReviewQueue stream re-connects on
  // filter changes and delivers a fresh initialSnapshot.
  useEffect(() => {
    let interval: NodeJS.Timeout | null = null;

    if (useWebSocketPush) {
      // Hybrid mode: Use longer fallback polling interval.
      // After a successful REST fetch, if the stream has given up (streamDeadRef),
      // reset retry state and invoke the reconnect thunk (F4).
      interval = setInterval(async () => {
        try {
          await refreshRef.current();
          // REST succeeded — if the stream is dead, attempt reconnect now.
          if (streamDeadRef.current) {
            streamDeadRef.current = false;
            streamRetriesRef.current = 0;
            streamReconnectRef.current?.();
          }
        } catch {
          // fetch error — stream recovery deferred to next poll
        }
      }, fallbackPollInterval);
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
  // streamDeadRef, streamRetriesRef, streamReconnectRef are stable refs.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [useWebSocketPush, autoRefresh, refreshInterval, fallbackPollInterval]);

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

  // Extract statistics from review queue
  const statistics = {
    totalItems: reviewQueue?.totalItems ?? 0,
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
    items: liveItems,
    loading,
    error,
    ...statistics,
    refresh,
    getByPriority,
    getByReason,
    acknowledgeSession,
  };
}
