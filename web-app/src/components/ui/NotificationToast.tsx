"use client";

import { useEffect, useState } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import styles from "./NotificationToast.module.css";

export interface NotificationData {
  id: string;
  sessionId: string;
  sessionName: string;
  message: string;
  timestamp: number;
  priority?: "urgent" | "high" | "medium" | "low";
  onView?: () => void;
  onDismiss?: () => void;
}

interface NotificationToastProps {
  notification: NotificationData;
  onClose: () => void;
  autoClose?: number; // Auto-close after N milliseconds (0 = no auto-close)
}

/**
 * Toast notification that appears in the corner of the screen
 * Shows session information and provides action buttons
 */
export function NotificationToast({
  notification,
  onClose,
  autoClose = 8000, // 8 seconds default
}: NotificationToastProps) {
  const [isVisible, setIsVisible] = useState(false);
  const [isExiting, setIsExiting] = useState(false);
  const auditLog = useAuditLog();

  // Entrance animation
  useEffect(() => {
    const timer = setTimeout(() => setIsVisible(true), 10);
    return () => clearTimeout(timer);
  }, []);

  // Auto-close timer
  useEffect(() => {
    if (autoClose > 0) {
      const timer = setTimeout(() => {
        handleClose();
      }, autoClose);
      return () => clearTimeout(timer);
    }
  }, [autoClose]);

  const handleClose = () => {
    setIsExiting(true);
    // Log dismissal
    auditLog.logNotificationDismissed(notification.id, notification.sessionId);
    setTimeout(() => {
      notification.onDismiss?.();
      onClose();
    }, 300); // Match animation duration
  };

  const handleView = () => {
    // Log view action
    auditLog.logNotificationSessionViewed(notification.id, notification.sessionId);
    notification.onView?.();
    handleClose();
  };

  const getPriorityColor = () => {
    switch (notification.priority) {
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
    <div
      className={`${styles.toast} ${isVisible ? styles.visible : ""} ${isExiting ? styles.exiting : ""}`}
      style={{ "--priority-color": getPriorityColor() } as React.CSSProperties}
      role="alert"
      aria-live="assertive"
    >
      <div className={styles.header}>
        <div className={styles.icon}>🔔</div>
        <div className={styles.title}>
          <strong>{notification.sessionName}</strong>
          <span className={styles.timestamp}>
            {new Date(notification.timestamp).toLocaleTimeString()}
          </span>
        </div>
        <button
          className={styles.closeButton}
          onClick={handleClose}
          aria-label="Close notification"
        >
          ×
        </button>
      </div>

      <div className={styles.body}>
        <p className={styles.message}>{notification.message}</p>
      </div>

      <div className={styles.actions}>
        <button className={styles.viewButton} onClick={handleView}>
          View Session
        </button>
        <button className={styles.dismissButton} onClick={handleClose}>
          Dismiss
        </button>
      </div>
    </div>
  );
}
