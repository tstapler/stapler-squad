"use client";

import { useEffect, useRef } from "react";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useNowTicker } from "@/lib/hooks/useNowTicker";
import { NotificationData } from "@/lib/types/notification";
import { notificationTypeIcon, notificationTypeLabel, priorityColor } from "@/lib/utils/notificationMapping";
import {
  toast,
  toastStacked,
  toastApproval,
  repeatBadge,
  offlineHint,
  exiting as exitingClass,
  minimized as minimizedClass,
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
  undoButton,
} from "./NotificationToast.css";

export type { NotificationData };

export interface NotificationToastProps {
  notification: NotificationData;
  /** Ask the stack to close this toast. `acknowledge` also fires the toast's onAcknowledge. */
  onClose: (options?: { acknowledge?: boolean }) => void;
  /** True while the stack runs the exit animation. */
  exiting?: boolean;
  /** Inside the capped deck: laid out in flow rather than fixed to the corner. */
  stacked?: boolean;
  /** When set, Approve and Deny (the server-bound actions) are disabled and this reason is shown. */
  offlineReason?: string;
  /** Compact pill; clicking it expands. */
  minimized?: boolean;
  onExpand?: () => void;
  /**
   * Called when the card mounts; the returned cleanup runs on unmount. The stack
   * starts a toast's timers only while its card is mounted, so an overflowing
   * toast with no card yet does not expire unseen.
   */
  onPresent?: () => void | (() => void);
}

function getRelativeTime(timestampMs: number, now: number): string {
  const seconds = Math.floor((now - timestampMs) / 1000);
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
 * One toast card. Presentational: it owns no timers. Auto-close, minimize and
 * exit timing live in the ToastStack through the timer registry, and the timing
 * policy in lib/notification-policy.ts.
 */
export function NotificationToast({
  notification,
  onClose,
  stacked = false,
  offlineReason,
  exiting = false,
  minimized = false,
  onExpand,
  onPresent,
}: NotificationToastProps) {
  const auditLog = useAuditLog();
  const undoButtonRef = useRef<HTMLButtonElement>(null);
  const now = useNowTicker(1_000);
  const relativeTime = getRelativeTime(notification.timestamp, now);

  useEffect(() => onPresent?.(), [onPresent]);

  // WCAG 2.4.3: move focus to Undo button when undo toast mounts
  useEffect(() => {
    if (notification.notificationType === "undo") {
      undoButtonRef.current?.focus();
    }
  }, []);

  const handleView = () => {
    auditLog.logNotificationSessionViewed(notification.id, notification.sessionId);
    notification.onView?.();
    onClose();
  };

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
      className={`${toast} ${notification.notificationType === "approval_needed" ? toastApproval : ""} ${exiting ? exitingClass : ""} ${minimized ? minimizedClass : ""} ${stacked ? toastStacked : ""}`}
      style={{ "--priority-color": priorityColor(notification.priority) } as React.CSSProperties}
      data-testid="toast"
      onClick={minimized ? onExpand : undefined}
      title={minimized ? "Click to expand" : undefined}
    >
      <div className={header}>
        <div className={icon}>{notificationTypeIcon(notification.notificationType)}</div>
        <div className={titleWrapper}>
          <div className={titleRow}>
            <strong>{displayTitle}</strong>
            {(notification.repeatCount ?? 1) > 1 && (
              <span className={repeatBadge} data-testid="toast-repeat-count">
                x{notification.repeatCount}
              </span>
            )}
            <span className={typeLabel}>{notificationTypeLabel(notification.notificationType)}</span>
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
          onClick={() => onClose()}
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
            aria-disabled={offlineReason ? true : undefined}
            onClick={() => {
              if (offlineReason) return;
              notification.onApprove?.();
              onClose({ acknowledge: true });
            }}
            title={offlineReason ?? "Allow this tool use"}
          >
            ✓ Approve
          </button>
        )}
        {notification.onDeny && (
          <button
            className={denyButton}
            aria-disabled={offlineReason ? true : undefined}
            onClick={() => {
              if (offlineReason) return;
              notification.onDeny?.();
              onClose({ acknowledge: true });
            }}
            title={offlineReason ?? "Deny this tool use"}
          >
            ✗ Deny
          </button>
        )}
        {offlineReason && (notification.onApprove || notification.onDeny) && (
          <span className={offlineHint} data-testid="toast-offline-hint">
            {offlineReason}
          </span>
        )}
        {notification.notificationType === "undo" && notification.onUndo && (
          <button
            ref={undoButtonRef}
            className={undoButton}
            data-testid="undo-toast-button"
            aria-label="Undo the last bulk delete"
            onClick={() => { notification.onUndo?.(); onClose(); }}
          >
            Undo
          </button>
        )}
        <button className={viewButton} onClick={handleView}>
          View Session
        </button>
        <button className={dismissButton} onClick={() => onClose({ acknowledge: true })}>
          Dismiss
        </button>
      </div>
    </div>
  );
}
