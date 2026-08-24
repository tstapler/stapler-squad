"use client";
// +feature: ui:notifications-page

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ResolveApprovalRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { getApiBaseUrl } from "@/lib/config";
import { NotificationData } from "@/lib/types/notification";
import {
  notificationTypeIcon,
  notificationTypeLabel,
  priorityColor,
  notificationTypeFilter,
  splitCIBlockMessage,
} from "@/lib/utils/notificationMapping";
import {
  header,
  title,
  unreadBadge,
  headerActions,
  markAllButton,
  clearButton,
  filterBar,
  searchRow,
  searchInput,
  searchClearButton,
  filterPills,
  filterPill,
  filterPillActive,
  filterPillExcludeActive,
  content,
  empty,
  emptyIcon,
  emptyText,
  emptySubtext,
  list,
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
  loadMore,
  loadMoreButton,
  incompleteSearchNotice,
  incompleteSearchNoticeButton,
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
} from "@/components/ui/NotificationPanel.css";
import { pageRoot } from "./NotificationsPage.css";

type TypeFilter = "all" | "approval_needed" | "error" | "task_complete" | "info";

const TYPE_FILTER_LABELS: Record<TypeFilter, string> = {
  all: "All",
  approval_needed: "Approval",
  error: "Error",
  task_complete: "Task",
  info: "Info",
};

export function NotificationsPage() {
  const {
    notificationHistory,
    markAsRead,
    markAllAsRead,
    removeFromHistory,
    acknowledgeNotification,
    clearHistory,
    getUnreadCount,
    historyLoading,
    historyHasMore,
    loadMoreHistory,
  } = useNotifications();

  const auditLog = useAuditLog();

  // Epic 3.3 (session-completion-summary), Story 3.3.2: a notification's
  // sessionId may reference a session that's since been deleted from the
  // live list (e.g. after DeleteSession). In that case "View Session" falls
  // back to the durable standalone summary route instead of a dead/no-op
  // `/?session=<id>` link.
  const liveSessions = useAppSelector(selectAllSessions);
  const liveSessionIds = useMemo(() => new Set(liveSessions.map((s) => s.id)), [liveSessions]);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
      clientRef.current = createClient(SessionService, transport);
    }
    return clientRef.current;
  }, []);

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [hideBacklogItems, setHideBacklogItems] = useState(false);
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);
  const [resolvedApprovals, setResolvedApprovals] = useState<Record<string, "allow" | "deny" | "expired">>({});
  const [pendingApprovals, setPendingApprovals] = useState<Record<string, boolean>>({});
  // Track approvals blocked by the CI-red guard (AC5) so we can show the inline
  // explanation + "Approve anyway" affordance instead of the generic "expired" state.
  const [blockedApprovals, setBlockedApprovals] = useState<Record<string, string>>({});

  useEffect(() => {
    const seeded: Record<string, "allow" | "deny" | "expired"> = {};
    for (const n of notificationHistory) {
      const decision = n.metadata?.["approval_decision"];
      const approvalId = n.metadata?.["approval_id"];
      if (!approvalId || !decision) continue;
      if (decision === "allow" || decision === "deny") {
        seeded[approvalId] = decision;
      } else if (decision === "timeout" || decision === "canceled") {
        seeded[approvalId] = "expired";
      }
    }
    if (Object.keys(seeded).length > 0) {
      setResolvedApprovals(prev => ({ ...seeded, ...prev }));
    }
  }, [notificationHistory]);

  const resolveApproval = useCallback(async (approvalId: string, decision: "allow" | "deny", notificationIds: string | string[], overrideCiBlock?: boolean) => {
    setPendingApprovals(prev => ({ ...prev, [approvalId]: true }));
    try {
      await getClient().resolveApproval(create(ResolveApprovalRequestSchema, { approvalId, decision, overrideCiBlock }));
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: decision }));
      setBlockedApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
      acknowledgeNotification(notificationIds);
    } catch (err) {
      // AC5: CI-red block — show an inline explanation next to Approve/Deny instead of
      // collapsing into the generic "expired" state (not a silent no-op).
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        setBlockedApprovals(prev => ({ ...prev, [approvalId]: err.rawMessage || err.message }));
        return;
      }
      console.error("Failed to resolve approval:", err);
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: "expired" }));
    } finally {
      setPendingApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
    }
  }, [getClient, acknowledgeNotification]);

  const filteredNotifications = useMemo(() => {
    let items = notificationHistory.filter((n) => n.notificationType !== "auto_approved");
    if (typeFilter !== "all") {
      const allowed = new Set(notificationTypeFilter(typeFilter, items.map((n) => n.notificationType)));
      items = items.filter((n) => allowed.has(n.notificationType));
    }
    if (hideBacklogItems) {
      items = items.filter((n) => !n.metadata?.["item_id"]);
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      items = items.filter(
        (n) =>
          (n.sessionName || "").toLowerCase().includes(q) ||
          (n.message || "").toLowerCase().includes(q) ||
          (n.title || "").toLowerCase().includes(q) ||
          (n.sourceProject || "").toLowerCase().includes(q) ||
          (n.sourceWorkingDir || "").toLowerCase().includes(q) ||
          (n.metadata?.["tool_name"] || "").toLowerCase().includes(q) ||
          (n.metadata?.["tool_input_command"] || "").toLowerCase().includes(q) ||
          (n.metadata?.["tool_input_file"] || "").toLowerCase().includes(q)
      );
    }
    return items;
  }, [notificationHistory, typeFilter, hideBacklogItems, searchQuery]);

  const hasActiveFilter = searchQuery.trim() !== "" || typeFilter !== "all" || hideBacklogItems;

  // The search box and "Hide backlog" toggle only filter over notificationHistory
  // that's already been paged into memory (see filteredNotifications above) — they
  // never re-query the server for older, not-yet-loaded history. When more history
  // remains (historyHasMore) and one of those two filters is active, surface that
  // instead of silently returning incomplete results.
  const hasIncompleteSearch = (searchQuery.trim() !== "" || hideBacklogItems) && historyHasMore;

  const autoHandledNotifications = useMemo(
    () => notificationHistory.filter((n) => n.notificationType === "auto_approved"),
    [notificationHistory]
  );

  const unreadCount = getUnreadCount();

  const handleNotificationClick = (ids: string | string[], onView?: () => void, sessionId?: string) => {
    markAsRead(ids);
    const primaryId = Array.isArray(ids) ? ids[0] : ids;
    if (onView && sessionId) {
      auditLog.logNotificationSessionViewed(primaryId, sessionId);
      onView();
    } else if (onView) {
      auditLog.logNotificationViewed(primaryId, sessionId);
      onView();
    }
  };

  const getContextString = (notification: NotificationData) => {
    const projectName = notification.sourceProject;
    const workingDirName = notification.sourceWorkingDir
      ? notification.sourceWorkingDir.split("/").pop()
      : null;
    const contextName = projectName || workingDirName;
    const parts: string[] = [];
    if (contextName) parts.push(contextName);
    if (notification.sourceApp) parts.push(`via ${notification.sourceApp}`);
    return parts.join(" ");
  };

  return (
    <div className={pageRoot}>
      <div className={header} data-testid="notifications-header">
        <h2 className={title} data-testid="notifications-title">
          Notifications
          {unreadCount > 0 && (
            <span className={unreadBadge} data-testid="notifications-unread-badge">
              {unreadCount}
            </span>
          )}
        </h2>
        <div className={headerActions}>
          {notificationHistory.length > 0 && (
            <>
              {unreadCount > 0 && (
                <button
                  className={markAllButton}
                  onClick={markAllAsRead}
                  aria-label="Mark all as read"
                  data-testid="notifications-mark-all-read"
                >
                  Mark all read
                </button>
              )}
              <button
                className={clearButton}
                onClick={clearHistory}
                aria-label="Clear all notifications"
                data-testid="notifications-clear-all"
              >
                Clear all
              </button>
            </>
          )}
        </div>
      </div>

      <div className={filterBar}>
        <div className={searchRow}>
          <input
            className={searchInput}
            type="search"
            placeholder="Search notifications…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            aria-label="Search notifications"
          />
          {searchQuery && (
            <button
              className={searchClearButton}
              onClick={() => setSearchQuery("")}
              aria-label="Clear search"
              type="button"
            >
              ✕
            </button>
          )}
        </div>
        <div className={filterPills} role="group" aria-label="Filter by type">
          {(Object.keys(TYPE_FILTER_LABELS) as TypeFilter[]).map((filter) => (
            <button
              key={filter}
              className={`${filterPill} ${typeFilter === filter ? filterPillActive : ""}`}
              onClick={() => setTypeFilter(filter)}
              aria-pressed={typeFilter === filter}
            >
              {TYPE_FILTER_LABELS[filter]}
            </button>
          ))}
          <button
            className={`${filterPill} ${hideBacklogItems ? filterPillExcludeActive : ""}`}
            onClick={() => setHideBacklogItems((v) => !v)}
            aria-pressed={hideBacklogItems}
            aria-label="Exclude backlog notifications"
          >
            🚫 Hide backlog
          </button>
        </div>
      </div>

      <div className={content} data-testid="notifications-content">
        {hasIncompleteSearch && (
          <div className={incompleteSearchNotice} data-testid="incomplete-search-notice">
            <span>Showing results from loaded history only — load more to search older notifications.</span>
            <button
              className={incompleteSearchNoticeButton}
              onClick={loadMoreHistory}
              disabled={historyLoading}
              type="button"
              data-testid="incomplete-search-load-more"
            >
              {historyLoading ? "Loading..." : "Load more"}
            </button>
          </div>
        )}
        {historyLoading && notificationHistory.length === 0 ? (
          <div className={empty}>
            <div className={emptyIcon}>⏳</div>
            <p className={emptyText}>Loading notifications...</p>
          </div>
        ) : filteredNotifications.length === 0 ? (
          <div className={empty}>
            <div className={emptyIcon}>{hasActiveFilter ? "🔍" : "🔔"}</div>
            <p className={emptyText}>
              {hasActiveFilter ? "No matching notifications" : "No notifications yet"}
            </p>
            <p className={emptySubtext}>
              {hasActiveFilter
                ? "Try adjusting your search or filter"
                : "You'll see notifications from your sessions here"}
            </p>
          </div>
        ) : (
          <div className={list}>
            {groupNotifications(filteredNotifications).map((group) => {
              const notification = group.notification;
              const contextString = getContextString(notification);
              const hasSourceApp = notification.sourceApp || notification.sourceBundleId;

              const GENERIC_TITLES = new Set(["Claude Notification", "Notification", "Alert", "claude notification"]);
              const primaryTitle =
                notification.sessionName ||
                (notification.title && !GENERIC_TITLES.has(notification.title) ? notification.title : null) ||
                notification.sessionId ||
                "Notification";
              const subtitleText =
                notification.title &&
                !GENERIC_TITLES.has(notification.title) &&
                notification.title !== primaryTitle
                  ? notification.title
                  : null;

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
                    <button
                      className={removeButton}
                      onClick={() => removeFromHistory(notification.id)}
                      aria-label="Remove notification"
                    >
                      ✕
                    </button>
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
                          if (resolved === "allow") return <span className={resolvedBadge} data-decision="allow">✓ Approved</span>;
                          if (resolved === "deny") return <span className={resolvedBadge} data-decision="deny">✗ Denied</span>;
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
                                  <button className={approveButton} onClick={() => resolveApproval(approvalId, "allow", group.allIds, true)} disabled={isPending} title="Approve despite failing CI">
                                    {isPending ? "…" : "Approve anyway"}
                                  </button>
                                  <button className={denyButton} onClick={() => resolveApproval(approvalId, "deny", group.allIds)} disabled={isPending} title="Deny this tool use">
                                    {isPending ? "…" : "✗ Deny"}
                                  </button>
                                </div>
                              </div>
                            );
                          }
                          return (
                            <>
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
                          onClick={() => handleNotificationClick(group.allIds, undefined, notification.sessionId)}
                          data-testid="notification-view-backlog"
                        >
                          View in Backlog
                        </Link>
                      )}
                      {!notification.metadata?.["item_id"] && notification.sessionId && (
                        <Link
                          href={
                            liveSessionIds.has(notification.sessionId)
                              ? `/?session=${encodeURIComponent(notification.sessionId)}`
                              : `/sessions/summary?sessionId=${encodeURIComponent(notification.sessionId)}`
                          }
                          className={viewButton}
                          onClick={() => handleNotificationClick(group.allIds, notification.onView, notification.sessionId)}
                        >
                          View Session
                        </Link>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}

            {historyHasMore && (
              <div className={loadMore}>
                <button className={loadMoreButton} onClick={loadMoreHistory} disabled={historyLoading}>
                  {historyLoading ? "Loading..." : "Load more"}
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {autoHandledNotifications.length > 0 && (
        <div className={autoHandledSection}>
          <button
            className={autoHandledHeader}
            onClick={() => setAutoHandledOpen((v) => !v)}
            aria-expanded={autoHandledOpen}
            aria-controls="auto-handled-list"
          >
            <span className={autoHandledHeaderLeft}>
              Auto-handled
              <span className={autoHandledBadge}>{autoHandledNotifications.length}</span>
            </span>
            <span className={`${autoHandledChevron} ${autoHandledOpen ? autoHandledChevronOpen : ""}`}>▼</span>
          </button>
          {autoHandledOpen && (
            <div id="auto-handled-list" className={autoHandledList}>
              {autoHandledNotifications.map((n) => {
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
      )}
    </div>
  );
}
