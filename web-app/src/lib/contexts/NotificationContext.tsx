"use client";

import React, { createContext, useContext, useState, useCallback } from "react";
import { NotificationData, NotificationToast } from "@/components/ui/NotificationToast";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";

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
  showSessionNotification: (item: ReviewItem, onView?: () => void) => void;
  togglePanel: () => void;
  markAsRead: (id: string) => void;
  markAllAsRead: () => void;
  removeFromHistory: (id: string) => void;
  clearHistory: () => void;
  getUnreadCount: () => number;
}

const NotificationContext = createContext<NotificationContextValue | null>(null);

export function NotificationProvider({ children }: { children: React.ReactNode }) {
  const [notifications, setNotifications] = useState<NotificationData[]>([]);
  const [notificationHistory, setNotificationHistory] = useState<NotificationHistoryItem[]>([]);
  const [isPanelOpen, setIsPanelOpen] = useState(false);

  // Initialize audit logging
  const auditLog = useAuditLog();

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

      // Add to persistent history
      setNotificationHistory((prev) => [
        {
          ...newNotification,
          isRead: false,
        },
        ...prev, // Newest first
      ]);
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
    (item: ReviewItem, onView?: () => void) => {
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
  }, [auditLog]);

  const markAllAsRead = useCallback(() => {
    setNotificationHistory((prev) => {
      const unreadCount = prev.filter((n) => !n.isRead).length;
      if (unreadCount > 0) {
        auditLog.logNotificationMarkedAllRead(unreadCount);
      }
      return prev.map((n) => ({ ...n, isRead: true }));
    });
  }, [auditLog]);

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
  }, [auditLog]);

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
