"use client";

import { useEffect, useRef, useState } from "react";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import { useNowTicker } from "@/lib/hooks/useNowTicker";
import { useSwipeToDismiss } from "@/lib/hooks/useSwipeToDismiss";
import { useSessionHidden } from "@/lib/hooks/useSessionHidden";
import { BackgroundChip } from "./BackgroundChip";
import { needsApproveConfirm, useDecisionFlow, type Verb } from "@/lib/hooks/useDecisionFlow";
import { isPinned } from "@/lib/notification-policy";
import { NotificationData } from "@/lib/types/notification";
import { notificationTypeIcon, notificationTypeLabel, priorityColor } from "@/lib/utils/notificationMapping";
import { ToastCloseControl, ToastCommand, SwipeReveal } from "./toast/ToastParts";
import {
  toast,
  toastStacked,
  toastApproval,
  repeatBadge,
  offlineHint,
  collapsedChip,
  swipeRow,
  swipeCard,
  swipeCardDragging,
  compactCard,
  decisionPair,
  actionError,
  inlineAction,
  overflowMenu,
  overflowTrigger,
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
  /** Inside the capped deck: laid out in flow, swipeable, with the phone and desktop deck controls. */
  stacked?: boolean;
  /** Phone deck: the one-row card of at most 96px. Approval cards are exempt (they grow to show the command). */
  compact?: boolean;
  /** When set, Approve and Deny (the server-bound actions) are disabled and this reason is shown. */
  offlineReason?: string;
  /** Phone only: a pinned card shrunk to a one-line chip; tapping it expands. */
  collapsed?: boolean;
  onExpandCollapsed?: () => void;
  /** Reports that an Approve or Deny is in flight or its confirm step is open, which exempts the card from collapsing. */
  onBusyChange?: (busy: boolean) => void;
  /** An Approve or Deny failed: the toast stays and is treated as pinned until it is resolved. */
  onActionFailed?: () => void;
  /** Pointer and focus handlers the stack uses to pause this toast's timers. */
  holdHandlers?: React.HTMLAttributes<HTMLDivElement>;
  /** True while the stack runs the exit animation. */
  exiting?: boolean;
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
 * One toast card. Presentational: it owns no timers. Auto-close, minimize,
 * collapse and exit timing live in the ToastStack through the timer registry, and
 * the timing policy in lib/notification-policy.ts.
 */
export function NotificationToast({
  notification,
  onClose,
  stacked = false,
  compact = false,
  offlineReason,
  collapsed = false,
  onExpandCollapsed,
  onBusyChange,
  onActionFailed,
  holdHandlers,
  exiting = false,
  minimized = false,
  onExpand,
  onPresent,
}: NotificationToastProps) {
  const auditLog = useAuditLog();
  const coarse = useCoarsePointer();
  const undoButtonRef = useRef<HTMLButtonElement>(null);
  const rowRef = useRef<HTMLDivElement>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [commandExpanded, setCommandExpanded] = useState(false);
  const now = useNowTicker(1_000);
  const relativeTime = getRelativeTime(notification.timestamp, now);

  const pinned = isPinned(notification);
  const isApproval = Boolean(notification.onApprove || notification.onDeny);
  const riskLevel = notification.metadata?.risk_level;
  const decision = useDecisionFlow({
    notification,
    onClose,
    confirmApprove: coarse && needsApproveConfirm(riskLevel),
    onBusyChange,
    onActionFailed,
  });
  const sending = decision.phase === "sending";

  const swipe = useSwipeToDismiss(rowRef, {
    onDismiss: () => onClose(),
    disabled: !stacked || collapsed || sending,
  });

  useEffect(() => onPresent?.(), [onPresent]);

  // WCAG 2.4.3: move focus to Undo button when undo toast mounts
  useEffect(() => {
    if (notification.notificationType === "undo") {
      undoButtonRef.current?.focus();
    }
  }, []);

  // A hidden session opens read-only: label the action for that expectation (Story 5.3).
  const sessionHidden = useSessionHidden(notification.sessionId) === true;
  const viewLabel = sessionHidden ? "View output" : "View Session";
  const viewTestId = sessionHidden ? "notification-view-output" : "notification-view-session";

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

  if (collapsed) {
    return (
      <div
        className={`${toast} ${toastStacked}`}
        style={{ "--priority-color": priorityColor(notification.priority) } as React.CSSProperties}
        data-testid="toast"
        data-collapsed="true"
        {...holdHandlers}
      >
        <button
          type="button"
          className={collapsedChip}
          data-testid="toast-collapsed-chip"
          onClick={onExpandCollapsed}
        >
          1 needs you - {displayTitle}
        </button>
      </div>
    );
  }

  const commandSource =
    notification.metadata?.tool_input_command ??
    notification.metadata?.tool_input_file ??
    notification.message;
  const focusWindowAvailable = Boolean(hasSourceApp && notification.onFocusWindow) && !sessionHidden;
  // Focus Window is a desktop-terminal action: absent on touch, and tucked into "..." when crowded.
  const focusInOverflow = stacked && !coarse && focusWindowAvailable && isApproval;
  const showFocusInline = focusWindowAvailable && !coarse && !(stacked && isApproval);
  const offline = Boolean(offlineReason);
  const decisionBlocked = offline || sending;

  const cardClass = [
    toast,
    notification.notificationType === "approval_needed" && !stacked ? toastApproval : "",
    exiting ? exitingClass : "",
    minimized ? minimizedClass : "",
    stacked ? toastStacked : "",
    stacked ? swipeCard : "",
    stacked && swipe.dragging ? swipeCardDragging : "",
    stacked && compact && !isApproval ? compactCard : "",
  ]
    .filter(Boolean)
    .join(" ");

  const decisionLabel = (v: Verb) => {
    if (sending && decision.verb === v) return v === "approve" ? "Approving..." : "Denying...";
    return v === "approve" ? "✓ Approve" : "✗ Deny";
  };

  const legacyDecide = (v: Verb) => {
    if (offline) return;
    // Fire-and-forget, as before the deck: the toast closes at once.
    void Promise.resolve(v === "approve" ? notification.onApprove?.() : notification.onDeny?.()).catch(() => {});
    onClose({ acknowledge: true });
  };

  const approveButtonEl = notification.onApprove && (
    <button
      key="approve"
      className={approveButton}
      aria-disabled={decisionBlocked ? true : undefined}
      onClick={() => {
        if (decisionBlocked) return;
        if (stacked) decision.approve();
        else legacyDecide("approve");
      }}
      title={offlineReason ?? "Allow this tool use"}
    >
      {stacked ? decisionLabel("approve") : "✓ Approve"}
    </button>
  );
  const denyButtonEl = notification.onDeny && (
    <button
      key="deny"
      className={denyButton}
      aria-disabled={decisionBlocked ? true : undefined}
      onClick={() => {
        if (decisionBlocked) return;
        if (stacked) void decision.run("deny");
        else legacyDecide("deny");
      }}
      title={offlineReason ?? "Deny this tool use"}
    >
      {stacked ? decisionLabel("deny") : "✗ Deny"}
    </button>
  );

  const closeControl = <ToastCloseControl stacked={stacked} pinned={pinned} onClose={() => onClose()} />;

  const phoneApproval = stacked && isApproval;
  const showViewInline = !(phoneApproval && coarse);
  const showDismissButton = !isApproval ? !(stacked && compact) : !stacked;

  const card = (
    <div
      className={cardClass}
      style={
        {
          "--priority-color": priorityColor(notification.priority),
          ...(stacked && swipe.offset !== 0 ? { transform: `translateX(${swipe.offset}px)` } : {}),
        } as React.CSSProperties
      }
      data-testid="toast"
      data-pinned={pinned ? "true" : undefined}
      tabIndex={stacked ? 0 : undefined}
      onKeyDown={
        stacked
          ? (e) => {
              if ((e.key === "Delete" || e.key === "Backspace") && e.target === e.currentTarget) {
                e.preventDefault();
                onClose();
              }
            }
          : undefined
      }
      onClick={minimized ? onExpand : undefined}
      title={minimized ? "Click to expand" : undefined}
      {...(stacked ? {} : holdHandlers)}
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
            {sessionHidden && <BackgroundChip />}
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
        {closeControl}
      </div>

      <div className={body} data-slot="body">
        {phoneApproval ? (
          <ToastCommand
            command={commandSource}
            riskLevel={riskLevel}
            expanded={commandExpanded}
            onToggle={() => setCommandExpanded((v) => !v)}
          />
        ) : (
          <p className={message}>{notification.message}</p>
        )}
        {notification.sourceWorkingDir && (
          <p className={workingDir} data-slot="workdir" title={notification.sourceWorkingDir}>
            📁 {notification.sourceWorkingDir.split('/').slice(-2).join('/')}
          </p>
        )}
      </div>

      <div className={actions} data-slot="actions">
        {phoneApproval ? (
          <>
            {decision.phase === "confirming" && (
              <div className={actionError} data-testid="toast-approve-confirm">
                <strong>Approve this command?</strong>
                <button type="button" className={inlineAction} onClick={decision.cancelConfirm}>
                  Cancel
                </button>
                <button
                  type="button"
                  className={approveButton}
                  data-testid="toast-confirm-approve"
                  aria-disabled={decisionBlocked ? true : undefined}
                  onClick={() => !decisionBlocked && void decision.run("approve")}
                >
                  Confirm approve
                </button>
              </div>
            )}
            {decision.phase === "failed" && (
              <div className={actionError} data-testid="toast-action-error">
                <span>Could not {decision.verb} - Retry</span>
                <button
                  type="button"
                  className={inlineAction}
                  aria-disabled={offline ? true : undefined}
                  onClick={() => !offline && void decision.run(decision.verb)}
                >
                  Retry
                </button>
              </div>
            )}
            {decision.phase !== "confirming" && (
              <div className={decisionPair}>
                {denyButtonEl}
                {approveButtonEl}
              </div>
            )}
          </>
        ) : (
          <>
            {showFocusInline && (
              <button className={focusButton} onClick={notification.onFocusWindow} title="Focus the source application window">
                🔗 Focus Window
              </button>
            )}
            {approveButtonEl}
            {denyButtonEl}
          </>
        )}
        {offline && isApproval && (
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
        {showViewInline && (
          <button className={viewButton} onClick={handleView} data-testid={viewTestId}>
            {viewLabel}
          </button>
        )}
        {showDismissButton && (
          <button className={dismissButton} onClick={() => onClose({ acknowledge: true })}>
            Dismiss
          </button>
        )}
        {phoneApproval && (focusInOverflow || coarse) && (
          <button
            type="button"
            className={overflowTrigger}
            aria-label="More actions"
            aria-expanded={menuOpen}
            data-testid="toast-overflow-menu"
            onClick={() => setMenuOpen((v) => !v)}
          >
            ...
          </button>
        )}
        {phoneApproval && menuOpen && (
          <div className={overflowMenu}>
            {coarse && (
              <button type="button" className={inlineAction} onClick={handleView} data-testid={viewTestId}>
                {viewLabel}
              </button>
            )}
            {focusInOverflow && (
              <button type="button" className={inlineAction} onClick={notification.onFocusWindow}>
                🔗 Focus Window
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );

  if (!stacked) return card;

  return (
    <div ref={rowRef} className={swipeRow} data-testid="toast-row" {...holdHandlers}>
      {swipe.revealed && <SwipeReveal pinned={pinned} toTheLeft={swipe.offset < 0} />}
      {card}
    </div>
  );
}
