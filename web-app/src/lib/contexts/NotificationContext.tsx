"use client";

import React, { createContext, useContext, useState, useCallback, useEffect } from "react";
import { NotificationData, NotificationToast } from "@/components/ui/NotificationToast";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";

export interface NotificationHistoryItem extends NotificationData {
  isRead: boolean;
}

interface NotificationContextValue {
  notifications: NotificationData[];
  notificationHistory: NotificationHistoryItem[];
  isPanelOpen: boolean;
  addNotification: (notification: Omit<NotificationData, "id" | "timestamp">) => void;
  removeNotification: (id: string) => void;
  clearAll: () => void;
  /**
   * Show a notification for a review queue item.
   * @param item - The review item to show notification for
   * @param onView - Optional callback when "View Session" is clicked
   * @param onAcknowledge - Optional callback when "Dismiss" is clicked (should call backend acknowledge)
   */
  showSessionNotification: (
    item: ReviewItem,
    onView?: () => void,
    onAcknowledge?: () => void
  ) => void;
  togglePanel: () => void;
  markAsRead: (id: string) => void;
  markAllAsRead: () => void;
  removeFromHistory: (id: string) => void;
  clearHistory: () => void;
  getUnreadCount: () => number;
  /** Whether the initial history fetch is in progress */
  historyLoading: boolean;
  /** Whether there are more history entries to load */
  historyHasMore: boolean;
  /** Load more history entries (pagination) */
  loadMoreHistory: () => Promise<void>;
}

const NotificationContext = createContext<NotificationContextValue | null>(null);

/**
 * Map a protobuf NotificationType to the frontend string union type.
 */
function mapNotificationType(
  protoType: number
): NotificationData["notificationType"] {
  switch (protoType) {
    case NotificationType.APPROVAL_NEEDED:
      return "approval_needed";
    case NotificationType.ERROR:
    case NotificationType.FAILURE:
      return "error";
    case NotificationType.WARNING:
      return "warning";
    case NotificationType.TASK_COMPLETE:
    case NotificationType.PROCESS_FINISHED:
      return "task_complete";
    case NotificationType.INFO:
    case NotificationType.STATUS_CHANGE:
    case NotificationType.DEBUG:
      return "info";
    case NotificationType.INPUT_REQUIRED:
    case NotificationType.CONFIRMATION_NEEDED:
      return "question";
    case NotificationType.PROCESS_STARTED:
      return "progress";
    case NotificationType.CUSTOM:
      return "custom";
    default:
      return "info";
  }
}

/**
 * Map a protobuf NotificationPriority to the frontend string union type.
 */
function mapPriority(
  protoPriority: number
): "urgent" | "high" | "medium" | "low" {
  switch (protoPriority) {
    case NotificationPriority.URGENT:
      return "urgent";
    case NotificationPriority.HIGH:
      return "high";
    case NotificationPriority.MEDIUM:
      return "medium";
    case NotificationPriority.LOW:
      return "low";
    default:
      return "medium";
  }
}

export function NotificationProvider({ children }: { children: React.ReactNode }) {
  const [notifications, setNotifications] = useState<NotificationData[]>([]);
  const [notificationHistory, setNotificationHistory] = useState<NotificationHistoryItem[]>([]);
  const [isPanelOpen, setIsPanelOpen] = useState(false);

  // Initialize audit logging
  const auditLog = useAuditLog();

  // Use the persistent notification history hook
  const history = useNotificationHistory();

  // Hydrate notificationHistory from the backend on initial load
  useEffect(() => {
    if (history.notifications.length > 0) {
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
      }));

      setNotificationHistory((prev) => {
        // Merge backend records with any real-time notifications already in state.
        // Deduplicate by ID, preferring existing (real-time) entries.
        const existingIds = new Set(prev.map((n) => n.id));
        const newFromBackend = backendItems.filter((n) => !existingIds.has(n.id));
        // Combine: real-time items first (newest), then backend items
        return [...prev, ...newFromBackend];
      });
    }
  }, [history.notifications]);

  const addNotification = useCallback(
    (notification: Omit<NotificationData, "id" | "timestamp">) => {
      const id = `notification-${Date.now()}-${Math.random()}`;
      const newNotification: NotificationData = {
        ...notification,
        id,
        timestamp: Date.now(),
      };

      // Add to active toasts
      setNotifications((prev) => [...prev, newNotification]);

      // Add to persistent history with deduplication
      setNotificationHistory((prev) => {
        // Deduplicate by ID
        if (prev.some((n) => n.id === id)) {
          return prev;
        }
        return [
          {
            ...newNotification,
            isRead: false,
          },
          ...prev, // Newest first
        ];
      });
    },
    []
  );

  const removeNotification = useCallback((id: string) => {
    setNotifications((prev) => prev.filter((n) => n.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    setNotifications([]);
  }, []);

  const showSessionNotification = useCallback(
    (item: ReviewItem, onView?: () => void, onAcknowledge?: () => void) => {
      // Map priority from protobuf enum to our notification priority
      let priority: "urgent" | "high" | "medium" | "low" = "medium";
      if (item.priority === 0) priority = "urgent";
      else if (item.priority === 1) priority = "high";
      else if (item.priority === 2) priority = "medium";
      else if (item.priority === 3) priority = "low";

      addNotification({
        sessionId: item.sessionId,
        sessionName: item.sessionName || "Unnamed Session",
        message: item.context || "This session is waiting for your input",
        priority,
        onView,
        onAcknowledge,
      });
    },
    [addNotification]
  );

  const togglePanel = useCallback(() => {
    setIsPanelOpen((prev) => {
      const newState = !prev;
      // Log panel open/close
      if (newState) {
        auditLog.logNotificationPanelOpened();
      } else {
        auditLog.logNotificationPanelClosed();
      }
      return newState;
    });
  }, [auditLog]);

  const markAsRead = useCallback((id: string) => {
    setNotificationHistory((prev) => {
      const notification = prev.find((n) => n.id === id);
      if (notification) {
        auditLog.logNotificationMarkedRead(notification.id, notification.sessionId);
      }
      return prev.map((n) => (n.id === id ? { ...n, isRead: true } : n));
    });
    // Also persist to backend
    history.markAsRead([id]);
  }, [auditLog, history]);

  const markAllAsRead = useCallback(() => {
    setNotificationHistory((prev) => {
      const unreadCount = prev.filter((n) => !n.isRead).length;
      if (unreadCount > 0) {
        auditLog.logNotificationMarkedAllRead(unreadCount);
      }
      return prev.map((n) => ({ ...n, isRead: true }));
    });
    // Also persist to backend
    history.markAllAsRead();
  }, [auditLog, history]);

  const removeFromHistory = useCallback((id: string) => {
    setNotificationHistory((prev) => {
      const notification = prev.find((n) => n.id === id);
      if (notification) {
        auditLog.logNotificationRemoved(notification.id, notification.sessionId);
      }
      return prev.filter((n) => n.id !== id);
    });
  }, [auditLog]);

  const clearHistory = useCallback(() => {
    setNotificationHistory((prev) => {
      if (prev.length > 0) {
        auditLog.logNotificationHistoryCleared(prev.length);
      }
      return [];
    });
    // Also persist to backend
    history.clearHistory();
  }, [auditLog, history]);

  const getUnreadCount = useCallback(() => {
    return notificationHistory.filter((n) => !n.isRead).length;
  }, [notificationHistory]);

  return (
    <NotificationContext.Provider
      value={{
        notifications,
        notificationHistory,
        isPanelOpen,
        addNotification,
        removeNotification,
        clearAll,
        showSessionNotification,
        togglePanel,
        markAsRead,
        markAllAsRead,
        removeFromHistory,
        clearHistory,
        getUnreadCount,
        historyLoading: history.loading,
        historyHasMore: history.hasMore,
        loadMoreHistory: history.loadMore,
      }}
    >
      {children}
      {/* Render notification toasts */}
      <div
        style={{
          position: "fixed",
          bottom: 0,
          right: 0,
          zIndex: 10000,
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
