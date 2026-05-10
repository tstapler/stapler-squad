"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  NotificationHistoryRecord,
  GetNotificationHistoryRequest,
  GetNotificationHistoryRequestSchema,
  MarkNotificationReadRequest,
  MarkNotificationReadRequestSchema,
  ClearNotificationHistoryRequest,
  ClearNotificationHistoryRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

const DEFAULT_PAGE_SIZE = 50;

interface UseNotificationHistoryReturn {
  notifications: NotificationHistoryRecord[];
  unreadCount: number;
  loading: boolean;
  error: Error | null;
  hasMore: boolean;
  markAsRead: (ids: string[]) => Promise<void>;
  markAllAsRead: () => Promise<void>;
  clearHistory: (beforeTimestamp?: string) => Promise<void>;
  loadMore: () => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for managing persisted notification history.
 *
 * Fetches notification history from the GetNotificationHistory RPC on mount
 * and exposes methods to mark as read, clear, and paginate.
 */
export function useNotificationHistory(): UseNotificationHistoryReturn {
  const [notifications, setNotifications] = useState<NotificationHistoryRecord[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [offset, setOffset] = useState(0);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  // Fetch notification history from the server
  const fetchHistory = useCallback(async (resetOffset = true) => {
    if (!clientRef.current) return;

    setLoading(true);
    setError(null);

    try {
      const currentOffset = resetOffset ? 0 : offset;
      const request = create(GetNotificationHistoryRequestSchema, {
        limit: DEFAULT_PAGE_SIZE,
        offset: currentOffset,
      });

      const response = await clientRef.current.getNotificationHistory(request);

      if (resetOffset) {
        setNotifications(response.notifications ?? []);
        setOffset(DEFAULT_PAGE_SIZE);
      } else {
        // Append for loadMore -- deduplicate by ID
        setNotifications((prev) => {
          const existingIds = new Set(prev.map((n) => n.id));
          const newRecords = (response.notifications ?? []).filter(
            (n) => !existingIds.has(n.id)
          );
          return [...prev, ...newRecords];
        });
        setOffset(currentOffset + DEFAULT_PAGE_SIZE);
      }

      setUnreadCount(response.unreadCount);
      setHasMore(response.hasMore);
    } catch (err) {
      const fetchError =
        err instanceof Error
          ? err
          : new Error("Failed to fetch notification history");
      setError(fetchError);
      console.error("Failed to fetch notification history:", fetchError);
    } finally {
      setLoading(false);
    }
  }, [offset]);

  // Fetch history once on mount. New notifications arrive via the watchSessions
  // stream and are added to local state by NotificationContext.addNotification,
  // so periodic polling is not needed.
  useEffect(() => {
    fetchHistory(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Refresh: reload from the beginning
  const refresh = useCallback(async () => {
    await fetchHistory(true);
  }, [fetchHistory]);

  // Load more: append next page
  const loadMore = useCallback(async () => {
    await fetchHistory(false);
  }, [fetchHistory]);

  // Mark specific notifications as read
  const markAsRead = useCallback(async (ids: string[]) => {
    if (!clientRef.current) return;

    // Optimistic update
    setNotifications((prev) =>
      prev.map((n) =>
        ids.includes(n.id) ? { ...n, isRead: true } as unknown as NotificationHistoryRecord : n
      )
    );
    setUnreadCount((prev) => Math.max(0, prev - ids.length));

    try {
      const request = create(MarkNotificationReadRequestSchema, {
        notificationIds: ids,
      });
      await clientRef.current.markNotificationRead(request);
    } catch (err) {
      console.error("Failed to mark notifications as read:", err);
      // Rollback on failure
      await fetchHistory(true);
    }
  }, [fetchHistory]);

  // Mark all notifications as read
  const markAllAsRead = useCallback(async () => {
    if (!clientRef.current) return;

    // Optimistic update
    setNotifications((prev) =>
      prev.map((n) => ({ ...n, isRead: true }) as unknown as NotificationHistoryRecord)
    );
    setUnreadCount(0);

    try {
      const request = create(MarkNotificationReadRequestSchema, {
        notificationIds: [], // empty = mark all
      });
      await clientRef.current.markNotificationRead(request);
    } catch (err) {
      console.error("Failed to mark all notifications as read:", err);
      // Rollback on failure
      await fetchHistory(true);
    }
  }, [fetchHistory]);

  // Clear notification history
  const clearHistory = useCallback(async (beforeTimestamp?: string) => {
    if (!clientRef.current) return;

    // Optimistic update
    const previousNotifications = notifications;
    setNotifications([]);
    setUnreadCount(0);
    setHasMore(false);

    try {
      const request = create(ClearNotificationHistoryRequestSchema, {
        beforeTimestamp,
      });
      await clientRef.current.clearNotificationHistory(request);
    } catch (err) {
      console.error("Failed to clear notification history:", err);
      // Rollback on failure
      setNotifications(previousNotifications);
      await fetchHistory(true);
    }
  }, [notifications, fetchHistory]);

  return {
    notifications,
    unreadCount,
    loading,
    error,
    hasMore,
    markAsRead,
    markAllAsRead,
    clearHistory,
    loadMore,
    refresh,
  };
}
