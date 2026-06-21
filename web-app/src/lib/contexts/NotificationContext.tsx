"use client";

import React, { createContext, useContext, useState, useCallback, useEffect, useMemo } from "react";
import { NotificationToast } from "@/components/ui/NotificationToast";
import { zIndex } from "@/styles/theme.css";
import { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";
import { ReviewItem, AttentionReason } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { mapNotificationType, mapPriority } from "@/lib/utils/notificationMapping";
import { TOAST_STALE_MS, ACTIONABLE_TOAST_STALE_MS, isActionable } from "@/lib/notification-policy";
import { createNotificationSyncChannel } from "@/lib/utils/broadcastChannel";
import { markAcknowledged } from "@/lib/utils/notificationStorage";

export type { NotificationData, NotificationHistoryItem };

interface NotificationContextValue {
  notifications: NotificationData[];
  notificationHistory: NotificationHistoryItem[];
  isPanelOpen: boolean;
  addNotification: (notification: Omit<NotificationData, "id" | "timestamp">) => void;
  /** Add to history panel only — no toast, no sound. For informational events like task_complete. */
  addToHistoryOnly: (notification: Omit<NotificationData, "id" | "timestamp">) => void;
  removeNotification: (id: string) => void;
  /**
   * Remove an active toast whose metadata.approval_id matches the given approvalId.
   * Used to preemptively clear approval toasts when an approval_response event arrives,
   * before refreshHistory() completes.
   */
  removeToastByApprovalId: (approvalId: string) => void;
  /**
   * Acknowledge one or more notifications: removes the active toast(s) and marks
   * them as read in the history panel. Use this for all user-triggered dismissals
   * so the two operations are always kept in sync.
   */
  acknowledgeNotification: (id: string | string[]) => void;
  clearAll: () => void;
  showSessionNotification: (
    item: ReviewItem,
    onView?: () => void,
    onAcknowledge?: () => void
  ) => void;
  togglePanel: () => void;
  markAsRead: (id: string | string[]) => void;
  markAsReadBySessionId: (sessionId: string | string[]) => void;
  /**
   * Remove active toast(s) for the given session ID(s).
   * Does NOT mark history as read — use acknowledgeNotification for that.
   * Used by useReviewQueueNotifications when a stale/queue item resolves,
   * so the toast disappears even if auto-minimize hasn't fired yet.
   */
  removeToastBySessionId: (sessionId: string | string[]) => void;
  markAllAsRead: () => void;
  removeFromHistory: (id: string) => void;
  clearHistory: () => void;
  getUnreadCount: () => number;
  historyLoading: boolean;
  historyHasMore: boolean;
  loadMoreHistory: () => Promise<void>;
  /** Re-fetch the full notification history from the server (e.g. after a stream reconnect). */
  refreshHistory: () => Promise<void>;
}

const NotificationContext = createContext<NotificationContextValue | null>(null);

function reviewItemToNotificationType(reason: AttentionReason): NotificationData["notificationType"] {
  switch (reason) {
    case AttentionReason.APPROVAL_PENDING:
    case AttentionReason.WAITING_FOR_USER:
      return "approval_needed";
    case AttentionReason.INPUT_REQUIRED:
      return "question";
    case AttentionReason.ERROR_STATE:
    case AttentionReason.TESTS_FAILING:
      return "error";
    case AttentionReason.STALE:
      return "warning";
    case AttentionReason.TASK_COMPLETE:
      return "task_complete";
    default:
      return "info";
  }
}

export function NotificationProvider({ children }: { children: React.ReactNode }) {
  const [notifications, setNotifications] = useState<NotificationData[]>([]);
  const [notificationHistory, setNotificationHistory] = useState<NotificationHistoryItem[]>([]);
  const [isPanelOpen, setIsPanelOpen] = useState(false);

  const auditLog = useAuditLog();
  const history = useNotificationHistory();

  // Hydrate and refresh notificationHistory from the backend.
  // Runs on initial load and whenever refreshHistory() is called (e.g. on reconnect
  // or after an approval_response event). Backend data is authoritative: existing
  // local items are UPDATED with the server version so isRead state and metadata
  // (e.g. approval_decision stamped after resolution) always reflect server truth.
  useEffect(() => {
    if (history.notifications.length === 0) return;

    const backendItems: NotificationHistoryItem[] = history.notifications.map((record) => ({
      id: record.id,
      sessionId: record.sessionId,
      sessionName: record.sessionName,
      title: record.title,
      message: record.message,
      timestamp: record.createdAt ? Number(record.createdAt.seconds) * 1000 : Date.now(),
      priority: mapPriority(record.priority),
      notificationType: mapNotificationType(record.notificationType),
      metadata: record.metadata ? Object.fromEntries(Object.entries(record.metadata)) : undefined,
      isRead: record.isRead,
      occurrenceCount: record.occurrenceCount,
    }));

    setNotificationHistory((prev) => {
      const backendById = new Map(backendItems.map((n) => [n.id, n]));
      // Maps dedup key -> backend item, so stream-added items (with client-generated
      // IDs) also get replaced by the authoritative server version.
      const backendByDedupKey = new Map(
        backendItems.map((n) => [`${n.sessionId ?? ""}:${n.notificationType ?? ""}`, n])
      );

      // Pass 1: walk existing local items and replace with server version where available.
      const updated: NotificationHistoryItem[] = [];
      const consumedDedupKeys = new Set<string>();
      for (const n of prev) {
        const dk = `${n.sessionId ?? ""}:${n.notificationType ?? ""}`;
        if (consumedDedupKeys.has(dk)) continue; // skip duplicate local entries
        const serverVersion = backendById.get(n.id) ?? backendByDedupKey.get(dk);
        // Preserve local callbacks (onView, onApprove, etc.) on the server version
        // since they are not persisted and are only meaningful for the current session.
        updated.push(serverVersion ? { ...serverVersion, onView: n.onView, onApprove: n.onApprove, onDeny: n.onDeny, onFocusWindow: n.onFocusWindow } : n);
        consumedDedupKeys.add(dk);
      }

      // Pass 2: add backend items not covered by any local item.
      // Mutate existingDedupKeys as we go so duplicate-type records (e.g. multiple
      // auto_approved entries for the same session) don't all slip through.
      const existingIds = new Set(updated.map((n) => n.id));
      const existingDedupKeys = new Set(updated.map((n) => `${n.sessionId ?? ""}:${n.notificationType ?? ""}`));
      const newFromBackend: NotificationHistoryItem[] = [];
      for (const n of backendItems) {
        if (existingIds.has(n.id)) continue;
        const dk = `${n.sessionId ?? ""}:${n.notificationType ?? ""}`;
        if (existingDedupKeys.has(dk)) continue;
        newFromBackend.push(n);
        existingDedupKeys.add(dk);
      }

      return [...newFromBackend, ...updated];
    });
  }, [history.notifications]);

  const addNotification = useCallback(
    (notification: Omit<NotificationData, "id" | "timestamp">) => {
      const id = `notification-${Date.now()}-${Math.random()}`;
      const newNotification: NotificationData = { ...notification, id, timestamp: Date.now() };

      // Only show the latest toast per session — replace any existing toast for the
      // same sessionId so they don't stack. Older notifications remain in history.
      // Exception: never displace an approval toast (one with onApprove/onDeny) with
      // a notification that lacks those callbacks — approvals require explicit resolution.
      setNotifications((prev) => {
        const existing = prev.find((n) => n.sessionId === notification.sessionId);
        if (
          existing &&
          (existing.onApprove || existing.onDeny) &&
          !notification.onApprove &&
          !notification.onDeny
        ) {
          return prev;
        }
        const without = prev.filter((n) => n.sessionId !== notification.sessionId);
        return [...without, newNotification];
      });

      setNotificationHistory((prev) => {
        if (prev.some((n) => n.id === id)) return prev;
        return [{ ...newNotification, isRead: false }, ...prev];
      });
    },
    []
  );

  const addToHistoryOnly = useCallback(
    (notification: Omit<NotificationData, "id" | "timestamp">) => {
      const id = `notification-${Date.now()}-${Math.random()}`;
      const newNotification: NotificationData = { ...notification, id, timestamp: Date.now() };
      setNotificationHistory((prev) => {
        if (prev.some((n) => n.id === id)) return prev;
        return [{ ...newNotification, isRead: false }, ...prev];
      });
    },
    []
  );

  const removeNotification = useCallback((id: string) => {
    setNotifications((prev) => prev.filter((n) => n.id !== id));
  }, []);

  const removeToastByApprovalId = useCallback((approvalId: string) => {
    setNotifications((prev) =>
      prev.filter((n) => n.metadata?.approval_id !== approvalId)
    );
  }, []);

  const clearAll = useCallback(() => {
    setNotifications([]);
  }, []);

  const showSessionNotification = useCallback(
    (item: ReviewItem, onView?: () => void, onAcknowledge?: () => void) => {
      addNotification({
        sessionId: item.sessionId,
        sessionName: item.sessionName || "Unnamed Session",
        message: item.context || "This session is waiting for your input",
        priority: mapPriority(item.priority),
        notificationType: reviewItemToNotificationType(item.reason),
        onView,
        onAcknowledge,
      });
    },
    [addNotification]
  );

  // Remove stale toasts every minute.
  // Non-actionable: removed after TOAST_STALE_MS (5 min).
  // Actionable (approval_needed, question): removed after ACTIONABLE_TOAST_STALE_MS (6 min).
  // Both remain in the notification history panel regardless.
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now();
      setNotifications((prev) =>
        prev.filter((n) =>
          isActionable(n.notificationType)
            ? now - n.timestamp < ACTIONABLE_TOAST_STALE_MS
            : now - n.timestamp < TOAST_STALE_MS
        )
      );
    }, 60_000);
    return () => clearInterval(interval);
  }, []);

  // Cross-tab sync: when another tab dismisses a notification, reflect it locally.
  useEffect(() => {
    const syncChannel = createNotificationSyncChannel();
    const unsubscribe = syncChannel.subscribe((message) => {
      if (message.type === "NOTIFICATION_DISMISSED") {
        const { notificationId } = message;
        setNotifications((prev) => prev.filter((n) => n.id !== notificationId));
        setNotificationHistory((prev) =>
          prev.map((n) => (n.id === notificationId ? { ...n, isRead: true } : n))
        );
      }
      // NOTIFICATION_ACKNOWLEDGED is intentionally not handled here.
      // Cross-tab session acknowledgement is driven by the sessionAcknowledged
      // event from the server stream (useSessionService), not BroadcastChannel.
    });
    return unsubscribe;
  }, []);

  const togglePanel = useCallback(() => {
    setIsPanelOpen((prev) => {
      const newState = !prev;
      if (newState) auditLog.logNotificationPanelOpened();
      else auditLog.logNotificationPanelClosed();
      return newState;
    });
  }, [auditLog]);

  const markAsRead = useCallback((id: string | string[]) => {
    const ids = Array.isArray(id) ? id : [id];
    const idSet = new Set(ids);
    setNotificationHistory((prev) => {
      for (const n of prev) {
        if (idSet.has(n.id)) auditLog.logNotificationMarkedRead(n.id, n.sessionId);
      }
      return prev.map((n) => (idSet.has(n.id) ? { ...n, isRead: true } : n));
    });
    history.markAsRead(ids);
  }, [auditLog, history]);

  /**
   * Acknowledge one or more notifications: removes the active toast(s) AND marks
   * them as read in the history panel in a single atomic operation.
   *
   * Always prefer this over calling removeNotification + markAsRead separately.
   */
  const acknowledgeNotification = useCallback((id: string | string[]) => {
    const ids = Array.isArray(id) ? id : [id];
    const idSet = new Set(ids);
    const syncChannel = createNotificationSyncChannel();
    setNotifications((prev) => {
      prev.forEach((n) => {
        if (idSet.has(n.id)) {
          syncChannel.broadcast({ type: "NOTIFICATION_DISMISSED", notificationId: n.id });
          if (n.sessionId) markAcknowledged(n.sessionId);
        }
      });
      return prev.filter((n) => !idSet.has(n.id));
    });
    markAsRead(ids);
  }, [markAsRead]);

  const markAsReadBySessionId = useCallback((sessionId: string | string[]) => {
    const sessionIds = new Set(Array.isArray(sessionId) ? sessionId : [sessionId]);
    setNotificationHistory((prev) => {
      const idsToMark: string[] = [];
      const updated = prev.map((n) => {
        if (!n.isRead && n.sessionId != null && sessionIds.has(n.sessionId)) {
          idsToMark.push(n.id);
          return { ...n, isRead: true };
        }
        return n;
      });
      if (idsToMark.length > 0) history.markAsRead(idsToMark);
      return updated;
    });
  }, [history]);

  const removeToastBySessionId = useCallback((sessionId: string | string[]) => {
    const sessionIds = new Set(Array.isArray(sessionId) ? sessionId : [sessionId]);
    sessionIds.delete(""); // never match notifications without a sessionId
    if (sessionIds.size === 0) return;
    setNotifications((prev) => prev.filter((n) => !sessionIds.has(n.sessionId ?? "")));
  }, []);

  const markAllAsRead = useCallback(() => {
    setNotificationHistory((prev) => {
      const unreadCount = prev.filter((n) => !n.isRead).length;
      if (unreadCount > 0) auditLog.logNotificationMarkedAllRead(unreadCount);
      return prev.map((n) => ({ ...n, isRead: true }));
    });
    history.markAllAsRead();
  }, [auditLog, history]);

  const removeFromHistory = useCallback((id: string) => {
    setNotificationHistory((prev) => {
      const notification = prev.find((n) => n.id === id);
      if (notification) auditLog.logNotificationRemoved(notification.id, notification.sessionId);
      return prev.filter((n) => n.id !== id);
    });
  }, [auditLog]);

  const clearHistory = useCallback(() => {
    setNotificationHistory((prev) => {
      if (prev.length > 0) auditLog.logNotificationHistoryCleared(prev.length);
      return [];
    });
    history.clearHistory();
  }, [auditLog, history]);

  const unreadCount = useMemo(() => {
    const unreadGroups = groupNotifications(notificationHistory.filter((n) => !n.isRead));
    return unreadGroups.length;
  }, [notificationHistory]);

  const getUnreadCount = useCallback(() => unreadCount, [unreadCount]);

  return (
    <NotificationContext.Provider
      value={{
        notifications,
        notificationHistory,
        isPanelOpen,
        addNotification,
        addToHistoryOnly,
        removeNotification,
        removeToastByApprovalId,
        acknowledgeNotification,
        clearAll,
        showSessionNotification,
        togglePanel,
        markAsRead,
        markAsReadBySessionId,
        removeToastBySessionId,
        markAllAsRead,
        removeFromHistory,
        clearHistory,
        getUnreadCount,
        historyLoading: history.loading,
        historyHasMore: history.hasMore,
        loadMoreHistory: history.loadMore,
        refreshHistory: history.refresh,
      }}
    >
      {children}
      <div
        style={{
          position: "fixed",
          bottom: 0,
          right: 0,
          zIndex: zIndex.toast,
          pointerEvents: "none",
        }}
      >
        {notifications.map((notification) => (
          <div key={notification.id} style={{ pointerEvents: "auto" }}>
            <NotificationToast
              notification={notification}
              onClose={() => removeNotification(notification.id)}
            />
          </div>
        ))}
      </div>
    </NotificationContext.Provider>
  );
}

export function useNotifications() {
  const context = useContext(NotificationContext);
  if (!context) {
    throw new Error("useNotifications must be used within NotificationProvider");
  }
  return context;
}
