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
  /**
   * Callback when user clicks "Dismiss" to acknowledge the notification.
   * This should trigger the backend acknowledge API to prevent re-notification.
   */
  onAcknowledge?: () => void;
  /** Called when user approves a pending tool-use request (approval_needed notifications only). */
  onApprove?: () => void;
  /** Called when user denies a pending tool-use request (approval_needed notifications only). */
  onDeny?: () => void;
}

interface NotificationToastProps {
  notification: NotificationData;
  onClose: () => void;
  autoClose?: number; // Auto-close after N milliseconds (0 = no auto-close)
  /** Auto-minimize to compact pill after N milliseconds (0 = disabled). Tier 2 default: 5000ms. */
  autoMinimize?: number;
}

/**
 * Toast notification that appears in the corner of the screen
 * Shows session information and provides action buttons
 */
/**
 * Returns the auto-close duration in ms based on notification type.
 * 0 = never auto-close.
 */
function getAutoCloseMs(type: NotificationData["notificationType"]): number {
  switch (type) {
    case "approval_needed":
    case "question":
      return 0; // Never auto-close — blocks Claude until resolved
    case "error":
    case "task_failed":
      return 12000;
    case "warning":
      return 8000;
    default:
      return 8000;
  }
}

/**
 * Returns the auto-minimize delay in ms based on notification type.
 * 0 = never minimize. Tier 1 types never minimize.
 */
function getAutoMinimizeMs(type: NotificationData["notificationType"]): number {
  switch (type) {
    case "approval_needed":
    case "question":
      return 0; // Never minimize — needs user action
    case "error":
    case "task_failed":
    case "warning":
      return 5000; // Minimize after 5s so it stays visible but compact
    default:
      return 0;
  }
}

function getRelativeTime(timestamp: number): string {
  const seconds = Math.floor((Date.now() - timestamp) / 1000);
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes === 1) return "1 min ago";
  if (minutes < 60) return `${minutes} mins ago`;
  const hours = Math.floor(minutes / 60);
  if (hours === 1) return "1 hr ago";
  return `${hours} hrs ago`;
}

export function NotificationToast({
  notification,
  onClose,
  autoClose,
  autoMinimize,
}: NotificationToastProps) {
  const effectiveAutoClose =
    autoClose !== undefined ? autoClose : getAutoCloseMs(notification.notificationType);
  const effectiveAutoMinimize =
    autoMinimize !== undefined ? autoMinimize : getAutoMinimizeMs(notification.notificationType);

  const [isVisible, setIsVisible] = useState(false);
  const [isExiting, setIsExiting] = useState(false);
  const [isMinimized, setIsMinimized] = useState(false);
  const [relativeTime, setRelativeTime] = useState(() => getRelativeTime(notification.timestamp));
  const auditLog = useAuditLog();

  // Tick every second to keep relative time live
  useEffect(() => {
    const interval = setInterval(() => {
      setRelativeTime(getRelativeTime(notification.timestamp));
    }, 1_000);
    return () => clearInterval(interval);
  }, [notification.timestamp]);

  // Entrance animation
  useEffect(() => {
    const timer = setTimeout(() => setIsVisible(true), 10);
    return () => clearTimeout(timer);
  }, []);

  // Auto-close timer (does NOT acknowledge - user didn't explicitly dismiss)
  useEffect(() => {
    if (effectiveAutoClose > 0) {
      const timer = setTimeout(() => {
        handleClose(false);
      }, effectiveAutoClose);
      return () => clearTimeout(timer);
    }
  }, [effectiveAutoClose]);

  // Auto-minimize timer: shrink to compact pill so it doesn't obscure content
  useEffect(() => {
    if (effectiveAutoMinimize > 0 && !isMinimized) {
      const timer = setTimeout(() => {
        setIsMinimized(true);
      }, effectiveAutoMinimize);
      return () => clearTimeout(timer);
    }
  }, [effectiveAutoMinimize, isMinimized]);

  const handleClose = (shouldAcknowledge: boolean = false) => {
    setIsExiting(true);
    // Log dismissal
    auditLog.logNotificationDismissed(notification.id, notification.sessionId);
    setTimeout(() => {
      notification.onDismiss?.();
      // If acknowledging, call the acknowledge callback to update backend/localStorage
      if (shouldAcknowledge) {
        notification.onAcknowledge?.();
      }
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
      className={`${styles.toast} ${notification.notificationType === "approval_needed" ? styles.toastApproval : ""} ${isVisible ? styles.visible : ""} ${isExiting ? styles.exiting : ""} ${isMinimized ? styles.minimized : ""}`}
      style={{ "--priority-color": getPriorityColor() } as React.CSSProperties}
      role="alert"
      aria-live={notification.notificationType === "approval_needed" ? "assertive" : "polite"}
      onClick={isMinimized ? () => setIsMinimized(false) : undefined}
      title={isMinimized ? "Click to expand" : undefined}
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
            <span className={styles.timestamp} title={new Date(notification.timestamp).toLocaleTimeString()}>
              {relativeTime}
            </span>
          </div>
        </div>
        <button
          className={styles.closeButton}
          onClick={() => handleClose(false)}
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
        {notification.onApprove && (
          <button
            className={styles.approveButton}
            onClick={() => { notification.onApprove?.(); handleClose(true); }}
            title="Allow this tool use"
          >
            ✓ Approve
          </button>
        )}
        {notification.onDeny && (
          <button
            className={styles.denyButton}
            onClick={() => { notification.onDeny?.(); handleClose(true); }}
            title="Deny this tool use"
          >
            ✗ Deny
          </button>
        )}
        <button className={styles.viewButton} onClick={handleView}>
          View Session
        </button>
        <button className={styles.dismissButton} onClick={() => handleClose(true)}>
          Dismiss
        </button>
      </div>
    </div>
  );
}
