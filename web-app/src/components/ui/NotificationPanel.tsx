"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ResolveApprovalRequest, ResolveApprovalRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
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
} from "@/lib/utils/notificationMapping";
import {
  overlay,
  panel,
  panelOpen,
  header,
  title,
  unreadBadge,
  headerActions,
  markAllButton,
  clearButton,
  closeButton,
  filterBar,
  searchInput,
  filterPills,
  filterPill,
  filterPillActive,
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
  focusButton,
  viewButton,
  loadMore,
  loadMoreButton,
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
} from "./NotificationPanel.css";

type TypeFilter = "all" | "approval_needed" | "error" | "task_complete" | "info";

const TYPE_FILTER_LABELS: Record<TypeFilter, string> = {
  all: "All",
  approval_needed: "Approval",
  error: "Error",
  task_complete: "Task",
  info: "Info",
};

/**
 * NotificationPanel - A sidebar that displays notification history
 * Similar to Android's notification panel, persists notifications for review.
 * Now backed by server-side persistent storage that survives page refreshes.
 */
export function NotificationPanel() {
  const {
    notificationHistory,
    isPanelOpen,
    togglePanel,
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

  // Lightweight RPC client for resolving approvals directly from the panel
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
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);

  // Track per-approval resolution state so buttons update after the user decides.
  // "allow" / "deny" = user resolved it; "expired" = already resolved or timed out.
  const [resolvedApprovals, setResolvedApprovals] = useState<Record<string, "allow" | "deny" | "expired">>({});
  // Track which approval IDs have an in-flight RPC so we can disable buttons while waiting.
  const [pendingApprovals, setPendingApprovals] = useState<Record<string, boolean>>({});

  // Seed resolvedApprovals from persisted metadata when history loads or updates.
  // The server stamps "approval_decision" on the notification record when an approval
  // is resolved, so this survives page refreshes.
  // "timeout" and "canceled" are server-side outcomes where the approval was not handled
  // via the UI; we map them to "expired" for display (same "Expired" badge).
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

  const resolveApproval = useCallback(async (approvalId: string, decision: "allow" | "deny", notificationIds: string | string[]) => {
    setPendingApprovals(prev => ({ ...prev, [approvalId]: true }));
    try {
      await getClient().resolveApproval(create(ResolveApprovalRequestSchema, { approvalId, decision }));
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: decision }));
      // Single call: marks as read in history AND closes the active toast
      acknowledgeNotification(notificationIds);
    } catch (err) {
      console.error("Failed to resolve approval:", err);
      // Approval already timed out or was resolved elsewhere — mark as expired.
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: "expired" }));
    } finally {
      setPendingApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
    }
  }, [getClient, acknowledgeNotification]);
  // Filter notifications by search query and type; auto_approved records are always excluded
  // from the main list and shown in a separate collapsible section.
  const filteredNotifications = useMemo(() => {
    let list = notificationHistory.filter((n) => n.notificationType !== "auto_approved");

    if (typeFilter !== "all") {
      const allowed = new Set(
        notificationTypeFilter(typeFilter, list.map((n) => n.notificationType))
      );
      list = list.filter((n) => allowed.has(n.notificationType));
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(
        (n) =>
          (n.sessionName || "").toLowerCase().includes(q) ||
          (n.message || "").toLowerCase().includes(q) ||
          (n.title || "").toLowerCase().includes(q)
      );
    }

    return list;
  }, [notificationHistory, typeFilter, searchQuery]);

  const autoHandledNotifications = useMemo(() => {
    return notificationHistory.filter((n) => n.notificationType === "auto_approved");
  }, [notificationHistory]);

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

  const getPriorityColor = (priority?: NotificationData["priority"]) => priorityColor(priority);
  const getTypeIcon = (type?: NotificationData["notificationType"]) => notificationTypeIcon(type);
  const getTypeLabel = (type?: NotificationData["notificationType"]) => notificationTypeLabel(type);

  // Build context string for notification (project/directory via app)
  const getContextString = (notification: NotificationData) => {
    const projectName = notification.sourceProject;
    const workingDirName = notification.sourceWorkingDir
      ? notification.sourceWorkingDir.split('/').pop()
      : null;
    const contextName = projectName || workingDirName;

    const parts: string[] = [];
    if (contextName) {
      parts.push(contextName);
    }
    if (notification.sourceApp) {
      parts.push(`via ${notification.sourceApp}`);
    }
    return parts.join(' ');
  };


  return (
    <>
      {/* Overlay backdrop */}
      {isPanelOpen && (
        <div className={overlay} onClick={togglePanel} aria-hidden="true" />
      )}

      {/* Notification Panel */}
      <div
        className={`${panel} ${isPanelOpen ? panelOpen : ""}`}
        role="dialog"
        aria-label="Notification Panel"
        aria-modal="true"
      >
        {/* Header */}
        <div className={header}>
          <h2 className={title}>
            Notifications
            {unreadCount > 0 && (
              <span className={unreadBadge}>{unreadCount}</span>
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
                  >
                    Mark all read
                  </button>
                )}
                <button
                  className={clearButton}
                  onClick={clearHistory}
                  aria-label="Clear all notifications"
                >
                  Clear all
                </button>
              </>
            )}
            <button
              className={closeButton}
              onClick={togglePanel}
              aria-label="Close notification panel"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Search + Filter Bar */}
        <div className={filterBar}>
          <input
            className={searchInput}
            type="search"
            placeholder="Search notifications…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            aria-label="Search notifications"
          />
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
          </div>
        </div>

        {/* Notification List */}
        <div className={content}>
          {historyLoading && notificationHistory.length === 0 ? (
            <div className={empty}>
              <div className={emptyIcon}>⏳</div>
              <p className={emptyText}>Loading notifications...</p>
            </div>
          ) : filteredNotifications.length === 0 ? (
            <div className={empty}>
              <div className={emptyIcon}>{searchQuery || typeFilter !== "all" ? "🔍" : "🔔"}</div>
              <p className={emptyText}>
                {searchQuery || typeFilter !== "all" ? "No matching notifications" : "No notifications yet"}
              </p>
              <p className={emptySubtext}>
                {searchQuery || typeFilter !== "all"
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

                // Always show the session name as the primary title so users know
                // which session generated the notification. If the stored title is a
                // generic placeholder (e.g. "Claude Notification") or absent, fall
                // back to the session name; otherwise show the specific title.
                const GENERIC_TITLES = new Set([
                  "Claude Notification", "Notification", "Alert", "claude notification"
                ]);
                const primaryTitle = notification.sessionName
                  || (notification.title && !GENERIC_TITLES.has(notification.title) ? notification.title : null)
                  || notification.sessionId
                  || "Notification";
                const subtitleText = notification.title && !GENERIC_TITLES.has(notification.title) && notification.title !== primaryTitle
                  ? notification.title
                  : null;

                return (
                  <div
                    key={notification.id}
                    className={`${item} ${notification.isRead ? read : unread}`}
                    style={
                      {
                        "--priority-color": getPriorityColor(notification.priority),
                      } as React.CSSProperties
                    }
                  >
                    <div className={itemHeader}>
                      <div className={itemTitle}>
                        {!notification.isRead && (
                          <span className={unreadDot} aria-label="Unread" />
                        )}
                        <span className={typeIcon}>{getTypeIcon(notification.notificationType)}</span>
                        <strong>{primaryTitle}</strong>
                        <span className={typeLabel} style={{ backgroundColor: getPriorityColor(notification.priority) }}>
                          {getTypeLabel(notification.notificationType)}
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

                    {/* Specific notification title (e.g. "Permission Required: Bash") */}
                    {subtitleText && (
                      <div className={itemSubtitle}>{subtitleText}</div>
                    )}

                    {contextString && (
                      <div className={itemContext}>
                        {contextString}
                      </div>
                    )}

                    <p className={itemMessage}>{notification.message}</p>

                    {/* Approval metadata: tool name + command/file details */}
                    {notification.notificationType === "approval_needed" && notification.metadata && (
                      <div className={approvalDetails}>
                        {notification.metadata.tool_name && (
                          <span className={approvalTool}>
                            🔧 {notification.metadata.tool_name}
                          </span>
                        )}
                        {notification.metadata.tool_input_command && (
                          <code className={approvalCommand}>
                            {notification.metadata.tool_input_command}
                          </code>
                        )}
                        {notification.metadata.tool_input_file && !notification.metadata.tool_input_command && (
                          <code className={approvalCommand}>
                            {notification.metadata.tool_input_file}
                          </code>
                        )}
                        {notification.metadata.cwd && (
                          <span className={approvalCwd} title={notification.metadata.cwd}>
                            📁 {notification.metadata.cwd.split('/').slice(-2).join('/')}
                          </span>
                        )}
                      </div>
                    )}

                    {notification.sourceWorkingDir && (
                      <div className={itemWorkingDir} title={notification.sourceWorkingDir}>
                        📁 {notification.sourceWorkingDir.split('/').slice(-2).join('/')}
                      </div>
                    )}

                    <div className={itemFooter}>
                      <span className={timestamp}>
                        {formatRelativeTime(notification.timestamp)}
                      </span>
                      <div className={itemActions}>
                        {/* Approve/Deny for approval notifications that have a live approval_id */}
                        {notification.notificationType === "approval_needed" && notification.metadata?.approval_id && (() => {
                          const approvalId = notification.metadata!.approval_id;
                          const resolved = resolvedApprovals[approvalId];
                          const isPending = !!pendingApprovals[approvalId];

                          if (resolved === "allow") {
                            return <span className={resolvedBadge} data-decision="allow">✓ Approved</span>;
                          }
                          if (resolved === "deny") {
                            return <span className={resolvedBadge} data-decision="deny">✗ Denied</span>;
                          }
                          if (resolved === "expired") {
                            return <span className={resolvedBadge} data-decision="expired">Expired</span>;
                          }
                          return (
                            <>
                              <button
                                className={approveButton}
                                onClick={() => resolveApproval(approvalId, "allow", group.allIds)}
                                disabled={isPending}
                                title="Approve this tool use"
                              >
                                {isPending ? "…" : "✓ Approve"}
                              </button>
                              <button
                                className={denyButton}
                                onClick={() => resolveApproval(approvalId, "deny", group.allIds)}
                                disabled={isPending}
                                title="Deny this tool use"
                              >
                                {isPending ? "…" : "✗ Deny"}
                              </button>
                            </>
                          );
                        })()}
                        {hasSourceApp && notification.onFocusWindow && (
                          <button
                            className={focusButton}
                            onClick={notification.onFocusWindow}
                            title="Focus the source application window"
                          >
                            🔗 Focus
                          </button>
                        )}
                        {notification.sessionId && (
                          <Link
                            href={`/?session=${encodeURIComponent(notification.sessionId)}`}
                            className={viewButton}
                            onClick={() => {
                              handleNotificationClick(
                                group.allIds,
                                notification.onView,
                                notification.sessionId
                              );
                              togglePanel(); // Close panel after clicking
                            }}
                          >
                            View Session
                          </Link>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}

              {/* Load more button */}
              {historyHasMore && (
                <div className={loadMore}>
                  <button
                    className={loadMoreButton}
                    onClick={loadMoreHistory}
                    disabled={historyLoading}
                  >
                    {historyLoading ? "Loading..." : "Load more"}
                  </button>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Auto-handled section — collapsible, always below main list */}
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
              <span className={`${autoHandledChevron} ${autoHandledOpen ? autoHandledChevronOpen : ""}`}>
                ▼
              </span>
            </button>
            {autoHandledOpen && (
              <div id="auto-handled-list" className={autoHandledList}>
                {autoHandledNotifications.map((n) => {
                  const decision = n.metadata?.["approval_decision"] ?? "allow";
                  const ruleName = n.metadata?.["classifier_rule_name"];
                  const toolName = n.metadata?.["tool_name"] ?? n.title;
                  return (
                    <div key={n.id} className={autoHandledItem}>
                      <span className={autoHandledDecision}>
                        {decision === "deny" ? "✗" : "✓"}
                      </span>
                      <div className={autoHandledContent}>
                        <div className={autoHandledTitle}>{toolName}</div>
                        {(n.message || ruleName) && (
                          <div className={autoHandledMeta}>
                            {n.message && <span>{n.message}</span>}
                            {ruleName && <span>· {ruleName}</span>}
                          </div>
                        )}
                      </div>
                      <span className={autoHandledTimestamp}>
                        {formatRelativeTime(n.timestamp)}
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </>
  );
}
