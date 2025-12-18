"use client";

import { useEffect, useState } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import styles from "./NotificationToast.module.css";

export interface NotificationData {
  id: string;
  sessionId: string;
  sessionName: string;
  title?: string;
  message: string;
  timestamp: number;
  priority?: "urgent" | "high" | "medium" | "low";
  notificationType?: "info" | "approval_needed" | "error" | "warning" | "task_complete" | "task_failed" | "progress" | "question" | "reminder" | "system" | "custom";
  /** Source app name (e.g., "IntelliJ IDEA", "Visual Studio Code") */
  sourceApp?: string;
  /** macOS bundle ID for window activation */
  sourceBundleId?: string;
  /** Working directory where the notification originated */
  sourceWorkingDir?: string;
  /** Project name for additional context */
  sourceProject?: string;
  /** Additional metadata key-value pairs */
  metadata?: Record<string, string>;
  onView?: () => void;
  onDismiss?: () => void;
  onFocusWindow?: () => void;
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

  const getTypeIcon = () => {
    switch (notification.notificationType) {
      case "approval_needed":
        return "⚠️";
      case "error":
        return "❌";
      case "warning":
        return "⚠️";
      case "task_complete":
        return "✅";
      case "task_failed":
        return "💥";
      case "progress":
        return "⏳";
      case "question":
        return "❓";
      case "reminder":
        return "⏰";
      case "system":
        return "⚙️";
      default:
        return "🔔";
    }
  };

  const getTypeLabel = () => {
    switch (notification.notificationType) {
      case "approval_needed":
        return "Approval Needed";
      case "error":
        return "Error";
      case "warning":
        return "Warning";
      case "task_complete":
        return "Task Complete";
      case "task_failed":
        return "Task Failed";
      case "progress":
        return "Progress";
      case "question":
        return "Question";
      case "reminder":
        return "Reminder";
      case "system":
        return "System";
      case "custom":
        return "Custom";
      default:
        return "Info";
    }
  };

  const handleFocusWindow = () => {
    notification.onFocusWindow?.();
  };

  // Determine the display title - use notification title if available, otherwise session name
  const displayTitle = notification.title || notification.sessionName;
  const hasSourceApp = notification.sourceApp || notification.sourceBundleId;

  // Build project/directory context string for better clarity
  const projectName = notification.sourceProject;
  const workingDirName = notification.sourceWorkingDir
    ? notification.sourceWorkingDir.split('/').pop()
    : null;
  const contextName = projectName || workingDirName || notification.sessionName;

  // Build subtitle: "ProjectName via SourceApp" or just "ProjectName" or "SessionName"
  const subtitleParts: string[] = [];
  if (contextName && contextName !== displayTitle) {
    subtitleParts.push(contextName);
  }
  if (hasSourceApp && notification.sourceApp) {
    subtitleParts.push(`via ${notification.sourceApp}`);
  }
  const subtitleText = subtitleParts.join(' ');

  return (
    <div
      className={`${styles.toast} ${isVisible ? styles.visible : ""} ${isExiting ? styles.exiting : ""}`}
      style={{ "--priority-color": getPriorityColor() } as React.CSSProperties}
      role="alert"
      aria-live="assertive"
    >
      <div className={styles.header}>
        <div className={styles.icon}>{getTypeIcon()}</div>
        <div className={styles.title}>
          <div className={styles.titleRow}>
            <strong>{displayTitle}</strong>
            <span className={styles.typeLabel}>{getTypeLabel()}</span>
          </div>
          <div className={styles.subtitleRow}>
            {subtitleText && (
              <span className={styles.sourceApp}>{subtitleText}</span>
            )}
            <span className={styles.timestamp}>
              {new Date(notification.timestamp).toLocaleTimeString()}
            </span>
          </div>
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
        {notification.sourceWorkingDir && (
          <p className={styles.workingDir} title={notification.sourceWorkingDir}>
            📁 {notification.sourceWorkingDir.split('/').slice(-2).join('/')}
          </p>
        )}
      </div>

      <div className={styles.actions}>
        {hasSourceApp && notification.onFocusWindow && (
          <button className={styles.focusButton} onClick={handleFocusWindow} title="Focus the source application window">
            🔗 Focus Window
          </button>
        )}
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
