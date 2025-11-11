"use client";

import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { formatRelativeTime } from "@/lib/utils/datetime";
import styles from "./NotificationPanel.module.css";

/**
 * NotificationPanel - A sidebar that displays notification history
 * Similar to Android's notification panel, persists notifications for review
 */
export function NotificationPanel() {
  const {
    notificationHistory,
    isPanelOpen,
    togglePanel,
    markAsRead,
    markAllAsRead,
    removeFromHistory,
    clearHistory,
    getUnreadCount,
  } = useNotifications();

  const auditLog = useAuditLog();
  const unreadCount = getUnreadCount();

  const handleNotificationClick = (id: string, onView?: () => void, sessionId?: string) => {
    markAsRead(id);
    if (onView && sessionId) {
      auditLog.logNotificationSessionViewed(id, sessionId);
      onView();
    } else if (onView) {
      auditLog.logNotificationViewed(id, sessionId);
      onView();
    }
  };

  const getPriorityColor = (priority?: string) => {
    switch (priority) {
      case "urgent":
        return "var(--color-error, #f44336)";
      case "high":
        return "var(--color-warning, #ff9800)";
      case "medium":
        return "var(--color-info, #2196f3)";
      case "low":
        return "var(--color-success, #4caf50)";
      default:
        return "var(--color-primary, #0070f3)";
    }
  };


  return (
    <>
      {/* Overlay backdrop */}
      {isPanelOpen && (
        <div className={styles.overlay} onClick={togglePanel} aria-hidden="true" />
      )}

      {/* Notification Panel */}
      <div
        className={`${styles.panel} ${isPanelOpen ? styles.open : ""}`}
        role="dialog"
        aria-label="Notification Panel"
        aria-modal="true"
      >
        {/* Header */}
        <div className={styles.header}>
          <h2 className={styles.title}>
            Notifications
            {unreadCount > 0 && (
              <span className={styles.unreadBadge}>{unreadCount}</span>
            )}
          </h2>
          <div className={styles.headerActions}>
            {notificationHistory.length > 0 && (
              <>
                {unreadCount > 0 && (
                  <button
                    className={styles.markAllButton}
                    onClick={markAllAsRead}
                    aria-label="Mark all as read"
                  >
                    Mark all read
                  </button>
                )}
                <button
                  className={styles.clearButton}
                  onClick={clearHistory}
                  aria-label="Clear all notifications"
                >
                  Clear all
                </button>
              </>
            )}
            <button
              className={styles.closeButton}
              onClick={togglePanel}
              aria-label="Close notification panel"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Notification List */}
        <div className={styles.content}>
          {notificationHistory.length === 0 ? (
            <div className={styles.empty}>
              <div className={styles.emptyIcon}>🔔</div>
              <p className={styles.emptyText}>No notifications yet</p>
              <p className={styles.emptySubtext}>
                You'll see notifications from your sessions here
              </p>
            </div>
          ) : (
            <div className={styles.list}>
              {notificationHistory.map((notification) => (
                <div
                  key={notification.id}
                  className={`${styles.item} ${notification.isRead ? styles.read : styles.unread}`}
                  style={
                    {
                      "--priority-color": getPriorityColor(notification.priority),
                    } as React.CSSProperties
                  }
                >
                  <div className={styles.itemHeader}>
                    <div className={styles.itemTitle}>
                      {!notification.isRead && (
                        <span className={styles.unreadDot} aria-label="Unread" />
                      )}
                      <strong>{notification.sessionName}</strong>
                    </div>
                    <button
                      className={styles.removeButton}
                      onClick={() => removeFromHistory(notification.id)}
                      aria-label="Remove notification"
                    >
                      ✕
                    </button>
                  </div>

                  <p className={styles.itemMessage}>{notification.message}</p>

                  <div className={styles.itemFooter}>
                    <span className={styles.timestamp}>
                      {formatRelativeTime(notification.timestamp)}
                    </span>
                    {notification.onView && (
                      <button
                        className={styles.viewButton}
                        onClick={() =>
                          handleNotificationClick(
                            notification.id,
                            notification.onView,
                            notification.sessionId
                          )
                        }
                      >
                        View Session
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
