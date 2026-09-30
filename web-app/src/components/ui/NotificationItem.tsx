"use client";

import Link from "next/link";
import { useCallback, useMemo, useRef } from "react";
import { useFocusRestoreOnRemoval } from "@/lib/hooks/useFocusRestoreOnRemoval";
import { GroupedNotification, groupNotifications } from "@/lib/utils/notificationGrouping";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";
import { visuallyHidden } from "@/styles/a11y.css";
import {
  notificationTypeIcon,
  notificationTypeLabel,
  priorityColor,
  splitCIBlockMessage,
  isReconciledNotification,
} from "@/lib/utils/notificationMapping";
import {
  item,
  read,
  unread,
  itemHeader,
  itemTitle,
  unreadDot,
  typeIcon,
  typeLabel,
  countBadge,
  removeButton,
  itemSubtitle,
  itemContext,
  itemMessage,
  approvalDetails,
  approvalTool,
  approvalCommand,
  approvalCwd,
  itemWorkingDir,
  itemFooter,
  timestamp,
  itemActions,
  resolvedBadge,
  approveButton,
  denyButton,
  ciBlockedRow,
  ciBlockedText,
  ciBlockedLink,
  focusButton,
  viewButton,
  autoHandledSection,
  autoHandledHeader,
  autoHandledHeaderLeft,
  autoHandledBadge,
  autoHandledChevron,
  autoHandledChevronOpen,
  autoHandledList,
  autoHandledItem,
  autoHandledDecision,
  autoHandledContent,
  autoHandledTitle,
  autoHandledMeta,
  autoHandledTimestamp,
  needsDecisionSection,
  needsDecisionHeadingRow,
  needsDecisionHeading,
  stalenessIndicator,
  stalenessRetry,
  needsDecisionEmpty,
  needsDecisionEmptyIcon,
  needsDecisionEmptyText,
  needsDecisionEmptySubtext,
  needsDecisionClearFilterButton,
  list,
} from "./NotificationPanel.css";

const GENERIC_TITLES = new Set(["Claude Notification", "Notification", "Alert", "claude notification"]);

function getContextString(notification: NotificationData): string {
  const projectName = notification.sourceProject;
  const workingDirName = notification.sourceWorkingDir
    ? notification.sourceWorkingDir.split("/").pop()
    : null;
  const contextName = projectName || workingDirName;
  const parts: string[] = [];
  if (contextName) parts.push(contextName);
  if (notification.sourceApp) parts.push(`via ${notification.sourceApp}`);
  return parts.join(" ");
}

export interface NotificationItemProps {
  group: GroupedNotification;
  resolvedApprovals: Record<string, "allow" | "deny" | "expired">;
  pendingApprovals: Record<string, boolean>;
  blockedApprovals: Record<string, string>;
  /** Transient resolveApproval failures (not the CI-block/reconciliation-race case) — the item stays actionable and this message renders next to the still-enabled Approve/Deny buttons. */
  failedApprovals: Record<string, string>;
  resolveApproval: (
    approvalId: string,
    decision: "allow" | "deny",
    notificationIds: string | string[],
    overrideCiBlock?: boolean
  ) => void;
  /**
   * Removes the record from local history. Optional: `NeedsDecisionSection`
   * (Task 3.1.2b) omits this prop so no ✕ control renders for an unread
   * actionable item — dismissing one would silently leave the underlying
   * decision unresolved server-side. Every other caller (Recent Activity,
   * Auto-handled, NotificationPanel) still passes it unchanged.
   */
  removeFromHistory?: (id: string) => void;
  handleNotificationClick: (ids: string | string[], onView?: () => void, sessionId?: string) => void;
  /**
   * Computes the href for the "View Session" link. Defaults to the live-session
   * route (`/?session=<id>`). NotificationsPage passes a variant that falls back
   * to the durable `/sessions/summary` route when the session is no longer live.
   */
  getSessionHref?: (sessionId: string) => string;
  /** Called after handleNotificationClick fires for the Backlog/Session links (e.g. to close the panel). */
  onNavigate?: () => void;
}

const defaultSessionHref = (sessionId: string) => `/?session=${encodeURIComponent(sessionId)}`;

/**
 * Renders a single notification card: type label, count badge, remove button,
 * subtitle/context/message, approval-needed metadata, and the footer action
 * buttons (approve/deny — including the CI-blocked "Approve anyway" variant,
 * focus-window, and View in Backlog / View Session links).
 *
 * Shared between NotificationsPage (full history) and NotificationPanel
 * (compact dropdown) — see NotificationsPage.test.tsx for the session-link
 * fallback behavior `getSessionHref` exists to preserve.
 */
export function NotificationItem({
  group,
  resolvedApprovals,
  pendingApprovals,
  blockedApprovals,
  failedApprovals,
  resolveApproval,
  removeFromHistory,
  handleNotificationClick,
  getSessionHref = defaultSessionHref,
  onNavigate,
}: NotificationItemProps) {
  const notification = group.notification;
  const contextString = getContextString(notification);
  const hasSourceApp = notification.sourceApp || notification.sourceBundleId;

  // Always show the session name as the primary title so users know which
  // session generated the notification. If the stored title is a generic
  // placeholder (e.g. "Claude Notification") or absent, fall back to the
  // session name; otherwise show the specific title.
  const primaryTitle =
    notification.sessionName ||
    (notification.title && !GENERIC_TITLES.has(notification.title) ? notification.title : null) ||
    notification.sessionId ||
    "Notification";
  const subtitleText =
    notification.title && !GENERIC_TITLES.has(notification.title) && notification.title !== primaryTitle
      ? notification.title
      : null;

  const navigate = (ids: string | string[], onView?: () => void, sessionId?: string) => {
    handleNotificationClick(ids, onView, sessionId);
    onNavigate?.();
  };

  return (
    <div
      key={notification.id}
      className={`${item} ${notification.isRead ? read : unread}`}
      style={{ "--priority-color": priorityColor(notification.priority) } as React.CSSProperties}
    >
      <div className={itemHeader}>
        <div className={itemTitle}>
          {!notification.isRead && <span className={unreadDot} role="img" aria-label="Unread" />}
          <span className={typeIcon}>{notificationTypeIcon(notification.notificationType)}</span>
          <strong>{primaryTitle}</strong>
          <span className={typeLabel} style={{ backgroundColor: priorityColor(notification.priority) }}>
            {notificationTypeLabel(notification.notificationType)}
          </span>
          {group.count > 1 && (
            <span className={countBadge} aria-label={`${group.count} occurrences`}>
              x{group.count}
            </span>
          )}
        </div>
        {removeFromHistory && (
          <button
            className={removeButton}
            onClick={() => removeFromHistory(notification.id)}
            aria-label="Remove notification"
          >
            ✕
          </button>
        )}
      </div>

      {subtitleText && <div className={itemSubtitle}>{subtitleText}</div>}
      {contextString && <div className={itemContext}>{contextString}</div>}
      <p className={itemMessage}>{notification.message}</p>

      {notification.notificationType === "approval_needed" && notification.metadata && (
        <div className={approvalDetails}>
          {notification.metadata.tool_name && (
            <span className={approvalTool}>🔧 {notification.metadata.tool_name}</span>
          )}
          {notification.metadata.tool_input_command && (
            <code className={approvalCommand}>{notification.metadata.tool_input_command}</code>
          )}
          {notification.metadata.tool_input_file && !notification.metadata.tool_input_command && (
            <code className={approvalCommand}>{notification.metadata.tool_input_file}</code>
          )}
          {notification.metadata.cwd && (
            <span className={approvalCwd} title={notification.metadata.cwd}>
              📁 {notification.metadata.cwd.split("/").slice(-2).join("/")}
            </span>
          )}
        </div>
      )}

      {notification.sourceWorkingDir && (
        <div className={itemWorkingDir} title={notification.sourceWorkingDir}>
          📁 {notification.sourceWorkingDir.split("/").slice(-2).join("/")}
        </div>
      )}

      <div className={itemFooter}>
        <span className={timestamp}>{formatRelativeTime(notification.timestamp)}</span>
        <div className={itemActions}>
          {notification.notificationType === "approval_needed" &&
            notification.metadata?.approval_id &&
            (() => {
              const approvalId = notification.metadata!.approval_id;
              const resolved = resolvedApprovals[approvalId];
              const isPending = !!pendingApprovals[approvalId];
              const blockedMessage = blockedApprovals[approvalId];
              const failedMessage = failedApprovals[approvalId];
              if (resolved === "allow" || resolved === "deny") {
                if (isReconciledNotification(notification)) {
                  const ruleName = notification.metadata?.["classifier_rule_name"] ?? "a rule";
                  return (
                    <span
                      className={resolvedBadge}
                      data-decision={`${resolved}-reconciled`}
                      title="Re-evaluated with current context (e.g. CI status, session idle time) at resolution time, which may have changed since this was first escalated."
                    >
                      {resolved === "allow" ? "✓" : "✗"} Auto-resolved by rule: {ruleName}
                    </span>
                  );
                }
                return (
                  <span className={resolvedBadge} data-decision={resolved}>
                    {resolved === "allow" ? "✓ Approved" : "✗ Denied"}
                  </span>
                );
              }
              if (resolved === "expired") return <span className={resolvedBadge} data-decision="expired">Expired</span>;
              if (blockedMessage) {
                // AC5/Story 2.2.4: visible inline explanation (not a silent no-op or
                // disabled button) plus an audited "Approve anyway" override.
                const { text: blockedText, checksUrl } = splitCIBlockMessage(blockedMessage);
                return (
                  <div className={ciBlockedRow} data-testid="ci-block-message">
                    <span className={ciBlockedText}>⚠️ {blockedText}</span>
                    {checksUrl && (
                      <a
                        href={checksUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className={ciBlockedLink}
                        data-testid="ci-block-view-run-link"
                      >
                        View CI run
                      </a>
                    )}
                    <div className={itemActions}>
                      {/* No CI-checks URL (e.g. a reconciliation-race message) means there is
                          nothing left to "approve anyway" against — only Deny still applies. */}
                      {checksUrl && (
                        <button className={approveButton} onClick={() => resolveApproval(approvalId, "allow", group.allIds, true)} disabled={isPending} title="Approve despite failing CI">
                          {isPending ? "…" : "Approve anyway"}
                        </button>
                      )}
                      <button className={denyButton} onClick={() => resolveApproval(approvalId, "deny", group.allIds)} disabled={isPending} title="Deny this tool use">
                        {isPending ? "…" : "✗ Deny"}
                      </button>
                    </div>
                  </div>
                );
              }
              return (
                <>
                  {failedMessage && (
                    <span className={ciBlockedText} data-testid="approval-retry-message">
                      {failedMessage}
                    </span>
                  )}
                  <button className={approveButton} onClick={() => resolveApproval(approvalId, "allow", group.allIds)} disabled={isPending} title="Approve this tool use">
                    {isPending ? "…" : "✓ Approve"}
                  </button>
                  <button className={denyButton} onClick={() => resolveApproval(approvalId, "deny", group.allIds)} disabled={isPending} title="Deny this tool use">
                    {isPending ? "…" : "✗ Deny"}
                  </button>
                </>
              );
            })()}
          {hasSourceApp && notification.onFocusWindow && (
            <button className={focusButton} onClick={notification.onFocusWindow} title="Focus the source application window">
              🔗 Focus
            </button>
          )}
          {notification.metadata?.["item_id"] && (
            <Link
              href={`/backlog?item=${encodeURIComponent(notification.metadata["item_id"])}`}
              className={viewButton}
              onClick={() => navigate(group.allIds, undefined, notification.sessionId)}
              data-testid="notification-view-backlog"
            >
              View in Backlog
            </Link>
          )}
          {!notification.metadata?.["item_id"] && notification.sessionId && (
            <Link
              href={getSessionHref(notification.sessionId)}
              className={viewButton}
              onClick={() => navigate(group.allIds, notification.onView, notification.sessionId)}
            >
              View Session
            </Link>
          )}
        </div>
      </div>
    </div>
  );
}

export interface AutoHandledSectionProps {
  notifications: NotificationHistoryItem[];
  isOpen: boolean;
  onToggle: () => void;
}

/**
 * Collapsible section listing auto-handled (classifier-resolved) notifications,
 * shown below the main notification list.
 */
export function AutoHandledSection({ notifications, isOpen, onToggle }: AutoHandledSectionProps) {
  if (notifications.length === 0) return null;

  return (
    <div className={autoHandledSection}>
      <button
        className={autoHandledHeader}
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-controls="auto-handled-list"
      >
        <span className={autoHandledHeaderLeft}>
          Auto-handled
          <span className={autoHandledBadge}>{notifications.length}</span>
        </span>
        <span className={`${autoHandledChevron} ${isOpen ? autoHandledChevronOpen : ""}`}>▼</span>
      </button>
      {isOpen && (
        <div id="auto-handled-list" className={autoHandledList}>
          {notifications.map((n) => {
            const decision = n.metadata?.["approval_decision"] ?? "allow";
            const ruleName = n.metadata?.["classifier_rule_name"];
            const toolName = n.metadata?.["tool_name"] ?? n.title;
            return (
              <div key={n.id} className={autoHandledItem}>
                <span className={autoHandledDecision}>{decision === "deny" ? "✗" : "✓"}</span>
                <div className={autoHandledContent}>
                  <div className={autoHandledTitle}>{toolName}</div>
                  {(n.message || ruleName) && (
                    <div className={autoHandledMeta}>
                      {n.message && <span>{n.message}</span>}
                      {ruleName && <span>· {ruleName}</span>}
                    </div>
                  )}
                </div>
                <span className={autoHandledTimestamp}>{formatRelativeTime(n.timestamp)}</span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export interface NeedsDecisionStaleness {
  /** "Last updated <label> ago" — already minute-rounded/formatted by the caller. */
  label: string;
  onRetry: () => void;
}

export interface NeedsDecisionSectionProps
  extends Pick<
    NotificationItemProps,
    | "resolvedApprovals"
    | "pendingApprovals"
    | "blockedApprovals"
    | "failedApprovals"
    | "resolveApproval"
    | "handleNotificationClick"
    | "getSessionHref"
  > {
  /** Already-filtered (unread + actionable) notifications to render — never the unfiltered set. */
  notifications: NotificationHistoryItem[];
  /**
   * The unread + actionable count over the *unfiltered* notification history
   * (Task 3.1.2c) — used only to distinguish "genuinely nothing needs a
   * decision" from "a filter is hiding something that does." Must never be
   * derived from `notifications.length`.
   */
  totalActionableCount: number;
  /** Resets the page's active filters. Only called from the "hidden by filter" empty state. */
  onClearFilter: () => void;
  /** Background poll/fetch-failure staleness indicator (Task 3.1.2h, AC38) — omitted when there has been no failure. */
  staleness?: NeedsDecisionStaleness;
}

/**
 * "Needs a decision" — always-expanded top tier of unread actionable
 * notifications (approval_needed/question/error/task_failed/warning). See
 * design/ux.md Surface 1 and Surface 4.
 *
 * Deliberately does not pass `removeFromHistory` to its `NotificationItem`s —
 * an item here leaves only by being resolved (Approve/Deny/Open session) or
 * by its underlying state resolving elsewhere, never via the ✕ control.
 */
export function NeedsDecisionSection({
  notifications,
  totalActionableCount,
  onClearFilter,
  resolvedApprovals,
  pendingApprovals,
  blockedApprovals,
  failedApprovals,
  resolveApproval,
  handleNotificationClick,
  getSessionHref,
  staleness,
}: NeedsDecisionSectionProps) {
  // Focus management on removal (design/ux.md AC34): when the item that held
  // keyboard focus leaves the section (resolved elsewhere or acted on),
  // move focus to the item now at the same list position, or to the
  // section heading if none remain. Mirrors ReviewQueuePanel.tsx's identical
  // pattern for the Review Queue's own needs-decision tier (Task 3.2.1's
  // Surface 9 focus handling).
  const headingRef = useRef<HTMLHeadingElement>(null);
  const notificationIds = useMemo(() => notifications.map((n) => n.id), [notifications]);
  const resolveNeedsDecisionElement = useCallback(
    (id: string) => document.querySelector<HTMLElement>(`[data-testid="needs-decision-item-${id}"]`),
    []
  );
  const focusedIdRef = useFocusRestoreOnRemoval(notificationIds, resolveNeedsDecisionElement, headingRef);

  const count = notifications.length;
  const announcement =
    count === 0 ? "" : count === 1 ? "1 item needs a decision" : `${count} items need a decision`;

  const heading = (
    <div className={needsDecisionHeadingRow}>
      <h3 className={needsDecisionHeading} ref={headingRef} tabIndex={-1} data-testid="needs-decision-heading">
        Needs a decision · {count}
      </h3>
      {staleness && (
        <span className={stalenessIndicator} role="status" data-testid="needs-decision-staleness">
          Last updated {staleness.label} ago ·{" "}
          <button type="button" className={stalenessRetry} onClick={staleness.onRetry}>
            Retry
          </button>
        </span>
      )}
    </div>
  );

  // Filter-aware empty state (Product Triad Review round-4 blocker fix):
  // never render the calm "All caught up" copy when a filter — not a lack
  // of actionable items — is what emptied the filtered list.
  if (count === 0) {
    const hiddenByFilter = totalActionableCount > 0;
    return (
      <div className={needsDecisionSection} data-testid="needs-decision-section">
        {heading}
        <div aria-live="polite" className={visuallyHidden} data-testid="needs-decision-announcement">
          {hiddenByFilter
            ? totalActionableCount === 1
              ? "1 item needs a decision, but is hidden by your filter"
              : `${totalActionableCount} items need a decision, but are hidden by your filter`
            : "All caught up"}
        </div>
        {hiddenByFilter ? (
          <div className={needsDecisionEmpty} data-testid="needs-decision-hidden-by-filter">
            <div className={needsDecisionEmptyIcon} aria-hidden="true">⚠️</div>
            <p className={needsDecisionEmptyText}>
              {totalActionableCount === 1
                ? "1 item needs a decision, but is hidden by your filter"
                : `${totalActionableCount} items need a decision, but are hidden by your filter`}
            </p>
            <button type="button" className={needsDecisionClearFilterButton} onClick={onClearFilter}>
              Clear filter
            </button>
          </div>
        ) : (
          <div className={needsDecisionEmpty} data-testid="needs-decision-empty">
            <div className={needsDecisionEmptyIcon} aria-hidden="true">✓</div>
            <p className={needsDecisionEmptyText}>All caught up</p>
            <p className={needsDecisionEmptySubtext}>Nothing needs your attention right now</p>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className={needsDecisionSection} data-testid="needs-decision-section">
      {heading}
      <div aria-live="polite" className={visuallyHidden} data-testid="needs-decision-announcement">
        {announcement}
      </div>
      <div className={list}>
        {groupNotifications(notifications).map((group) => (
          <div
            key={group.notification.id}
            data-testid={`needs-decision-item-${group.notification.id}`}
            tabIndex={-1}
            onFocus={() => {
              focusedIdRef.current = group.notification.id;
            }}
            onBlur={() => {
              if (focusedIdRef.current === group.notification.id) focusedIdRef.current = null;
            }}
          >
            <NotificationItem
              group={group}
              resolvedApprovals={resolvedApprovals}
              pendingApprovals={pendingApprovals}
              blockedApprovals={blockedApprovals}
              failedApprovals={failedApprovals}
              resolveApproval={resolveApproval}
              handleNotificationClick={handleNotificationClick}
              getSessionHref={getSessionHref}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
