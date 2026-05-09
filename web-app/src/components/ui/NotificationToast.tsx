"use client";

import { useCallback, useEffect, useState } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { NotificationData } from "@/lib/types/notification";
import { toastAutoCloseMs, toastAutoMinimizeMs } from "@/lib/notification-policy";
import { notificationTypeIcon, notificationTypeLabel, priorityColor } from "@/lib/utils/notificationMapping";
import {
  toast,
  toastApproval,
  visible,
  exiting,
  minimized,
  header,
  icon,
  titleWrapper,
  titleRow,
  typeLabel,
  subtitleRow,
  sourceApp,
  timestamp,
  closeButton,
  body,
  message,
  workingDir,
  actions,
  focusButton,
  approveButton,
  denyButton,
  viewButton,
  dismissButton,
  minimizeHint,
} from "./NotificationToast.css";

export type { NotificationData };

interface NotificationToastProps {
  notification: NotificationData;
  onClose: () => void;
  autoClose?: number; // Auto-close after N milliseconds (0 = no auto-close)
  /** Auto-minimize to compact pill after N milliseconds (0 = disabled). Tier 2 default: 5000ms. */
  autoMinimize?: number;
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

/**
 * Toast notification that appears in the corner of the screen.
 * Shows session information and provides action buttons.
 *
 * Timing policy is centralized in lib/notification-policy.ts —
 * do not add dismissal logic here.
 */
export function NotificationToast({
  notification,
  onClose,
  autoClose,
  autoMinimize,
}: NotificationToastProps) {
  const effectiveAutoClose =
    autoClose !== undefined ? autoClose : toastAutoCloseMs(notification.notificationType);
  const effectiveAutoMinimize =
    autoMinimize !== undefined ? autoMinimize : toastAutoMinimizeMs(notification.notificationType);

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

  const handleClose = useCallback((shouldAcknowledge: boolean = false) => {
    setIsExiting(true);
    auditLog.logNotificationDismissed(notification.id, notification.sessionId);
    setTimeout(() => {
      notification.onDismiss?.();
      if (shouldAcknowledge) {
        notification.onAcknowledge?.();
      }
      onClose();
    }, 300);
  }, [auditLog, notification, onClose]);

  // Auto-close timer (does NOT acknowledge - user didn't explicitly dismiss)
  useEffect(() => {
    if (effectiveAutoClose > 0) {
      const timer = setTimeout(() => {
        handleClose(false);
      }, effectiveAutoClose);
      return () => clearTimeout(timer);
    }
  }, [effectiveAutoClose, handleClose]);

  // Auto-minimize timer: shrink to compact pill so it doesn't obscure content
  useEffect(() => {
    if (effectiveAutoMinimize > 0 && !isMinimized) {
      const timer = setTimeout(() => {
        setIsMinimized(true);
      }, effectiveAutoMinimize);
      return () => clearTimeout(timer);
    }
  }, [effectiveAutoMinimize, isMinimized]);

  const handleView = () => {
    auditLog.logNotificationSessionViewed(notification.id, notification.sessionId);
    notification.onView?.();
    handleClose();
  };

  const getPriorityColor = () => priorityColor(notification.priority);
  const getTypeIcon = () => notificationTypeIcon(notification.notificationType);
  const getTypeLabel = () => notificationTypeLabel(notification.notificationType);

  const displayTitle = notification.title || notification.sessionName;
  const hasSourceApp = notification.sourceApp || notification.sourceBundleId;

  const projectName = notification.sourceProject;
  const workingDirName = notification.sourceWorkingDir
    ? notification.sourceWorkingDir.split('/').pop()
    : null;
  const contextName = projectName || workingDirName || notification.sessionName;

  const subtitleParts: string[] = [];
  if (contextName && contextName !== displayTitle) subtitleParts.push(contextName);
  if (hasSourceApp && notification.sourceApp) subtitleParts.push(`via ${notification.sourceApp}`);
  const subtitleText = subtitleParts.join(' ');

  return (
    <div
      className={`${toast} ${notification.notificationType === "approval_needed" ? toastApproval : ""} ${isVisible ? visible : ""} ${isExiting ? exiting : ""} ${isMinimized ? minimized : ""}`}
      style={{ "--priority-color": getPriorityColor() } as React.CSSProperties}
      role="alert"
      aria-live={notification.notificationType === "approval_needed" ? "assertive" : "polite"}
      onClick={isMinimized ? () => setIsMinimized(false) : undefined}
      title={isMinimized ? "Click to expand" : undefined}
    >
      <div className={header}>
        <div className={icon}>{getTypeIcon()}</div>
        <div className={titleWrapper}>
          <div className={titleRow}>
            <strong>{displayTitle}</strong>
            <span className={typeLabel}>{getTypeLabel()}</span>
          </div>
          <div className={subtitleRow}>
            {subtitleText && (
              <span className={sourceApp}>{subtitleText}</span>
            )}
            <span className={timestamp} title={new Date(notification.timestamp).toLocaleTimeString()}>
              {relativeTime}
            </span>
          </div>
        </div>
        <button
          className={closeButton}
          onClick={() => handleClose(false)}
          aria-label="Close notification"
        >
          ×
        </button>
      </div>

      <div className={body}>
        <p className={message}>{notification.message}</p>
        {notification.sourceWorkingDir && (
          <p className={workingDir} title={notification.sourceWorkingDir}>
            📁 {notification.sourceWorkingDir.split('/').slice(-2).join('/')}
          </p>
        )}
      </div>

      <div className={actions}>
        {hasSourceApp && notification.onFocusWindow && (
          <button className={focusButton} onClick={notification.onFocusWindow} title="Focus the source application window">
            🔗 Focus Window
          </button>
        )}
        {notification.onApprove && (
          <button
            className={approveButton}
            onClick={() => { notification.onApprove?.(); handleClose(true); }}
            title="Allow this tool use"
          >
            ✓ Approve
          </button>
        )}
        {notification.onDeny && (
          <button
            className={denyButton}
            onClick={() => { notification.onDeny?.(); handleClose(true); }}
            title="Deny this tool use"
          >
            ✗ Deny
          </button>
        )}
        <button className={viewButton} onClick={handleView}>
          View Session
        </button>
        <button className={dismissButton} onClick={() => handleClose(true)}>
          Dismiss
        </button>
      </div>
    </div>
  );
}
