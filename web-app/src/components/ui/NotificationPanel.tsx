"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ResolveApprovalRequest } from "@/gen/session/v1/session_pb";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { getApiBaseUrl } from "@/lib/config";
import { NotificationData } from "./NotificationToast";
import styles from "./NotificationPanel.module.css";

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
    clearHistory,
    getUnreadCount,
    historyLoading,
    historyHasMore,
    loadMoreHistory,
  } = useNotifications();

  const auditLog = useAuditLog();

  // Lightweight RPC client for resolving approvals directly from the panel
  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
      clientRef.current = createPromiseClient(SessionService, transport);
    }
    return clientRef.current;
  }, []);

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");

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
      await getClient().resolveApproval(new ResolveApprovalRequest({ approvalId, decision }));
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: decision }));
      markAsRead(notificationIds);
    } catch (err) {
      console.error("Failed to resolve approval:", err);
      // Approval already timed out or was resolved elsewhere — mark as expired.
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: "expired" }));
    } finally {
      setPendingApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
    }
  }, [getClient, markAsRead]);
  // Filter notifications by search query and type
  const filteredNotifications = useMemo(() => {
    let list = notificationHistory;

    if (typeFilter !== "all") {
      if (typeFilter === "error") {
        // "Error" pill covers error + task_failed + warning
        list = list.filter((n) =>
          n.notificationType === "error" ||
          n.notificationType === "task_failed" ||
          n.notificationType === "warning"
        );
      } else if (typeFilter === "info") {
        // "Info" covers everything not covered by the other explicit filters
        list = list.filter(
          (n) =>
            n.notificationType !== "approval_needed" &&
            n.notificationType !== "error" &&
            n.notificationType !== "task_failed" &&
            n.notificationType !== "warning" &&
            n.notificationType !== "task_complete"
        );
      } else {
        list = list.filter((n) => n.notificationType === typeFilter);
      }
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

  const getTypeIcon = (notificationType?: NotificationData["notificationType"]) => {
    switch (notificationType) {
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

  const getTypeLabel = (notificationType?: NotificationData["notificationType"]) => {
    switch (notificationType) {
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

        {/* Search + Filter Bar */}
        <div className={styles.filterBar}>
          <input
            className={styles.searchInput}
            type="search"
            placeholder="Search notifications…"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            aria-label="Search notifications"
          />
          <div className={styles.filterPills} role="group" aria-label="Filter by type">
            {(Object.keys(TYPE_FILTER_LABELS) as TypeFilter[]).map((filter) => (
              <button
                key={filter}
                className={`${styles.filterPill} ${typeFilter === filter ? styles.filterPillActive : ""}`}
                onClick={() => setTypeFilter(filter)}
                aria-pressed={typeFilter === filter}
              >
                {TYPE_FILTER_LABELS[filter]}
              </button>
            ))}
          </div>
        </div>

        {/* Notification List */}
        <div className={styles.content}>
          {historyLoading && notificationHistory.length === 0 ? (
            <div className={styles.empty}>
              <div className={styles.emptyIcon}>⏳</div>
              <p className={styles.emptyText}>Loading notifications...</p>
            </div>
          ) : filteredNotifications.length === 0 ? (
            <div className={styles.empty}>
              <div className={styles.emptyIcon}>{searchQuery || typeFilter !== "all" ? "🔍" : "🔔"}</div>
              <p className={styles.emptyText}>
                {searchQuery || typeFilter !== "all" ? "No matching notifications" : "No notifications yet"}
              </p>
              <p className={styles.emptySubtext}>
                {searchQuery || typeFilter !== "all"
                  ? "Try adjusting your search or filter"
                  : "You'll see notifications from your sessions here"}
              </p>
            </div>
          ) : (
            <div className={styles.list}>
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
                        <span className={styles.typeIcon}>{getTypeIcon(notification.notificationType)}</span>
                        <strong>{primaryTitle}</strong>
                        <span className={styles.typeLabel} style={{ backgroundColor: getPriorityColor(notification.priority) }}>
                          {getTypeLabel(notification.notificationType)}
                        </span>
                        {group.count > 1 && (
                          <span className={styles.countBadge} aria-label={`${group.count} occurrences`}>
                            x{group.count}
                          </span>
                        )}
                      </div>
                      <button
                        className={styles.removeButton}
                        onClick={() => removeFromHistory(notification.id)}
                        aria-label="Remove notification"
                      >
                        ✕
                      </button>
                    </div>

                    {/* Specific notification title (e.g. "Permission Required: Bash") */}
                    {subtitleText && (
                      <div className={styles.itemSubtitle}>{subtitleText}</div>
                    )}

                    {contextString && (
                      <div className={styles.itemContext}>
                        {contextString}
                      </div>
                    )}

                    <p className={styles.itemMessage}>{notification.message}</p>

                    {/* Approval metadata: tool name + command/file details */}
                    {notification.notificationType === "approval_needed" && notification.metadata && (
                      <div className={styles.approvalDetails}>
                        {notification.metadata.tool_name && (
                          <span className={styles.approvalTool}>
                            🔧 {notification.metadata.tool_name}
                          </span>
                        )}
                        {notification.metadata.tool_input_command && (
                          <code className={styles.approvalCommand}>
                            {notification.metadata.tool_input_command}
                          </code>
                        )}
                        {notification.metadata.tool_input_file && !notification.metadata.tool_input_command && (
                          <code className={styles.approvalCommand}>
                            {notification.metadata.tool_input_file}
                          </code>
                        )}
                        {notification.metadata.cwd && (
                          <span className={styles.approvalCwd} title={notification.metadata.cwd}>
                            📁 {notification.metadata.cwd.split('/').slice(-2).join('/')}
                          </span>
                        )}
                      </div>
                    )}

                    {notification.sourceWorkingDir && (
                      <div className={styles.itemWorkingDir} title={notification.sourceWorkingDir}>
                        📁 {notification.sourceWorkingDir.split('/').slice(-2).join('/')}
                      </div>
                    )}

                    <div className={styles.itemFooter}>
                      <span className={styles.timestamp}>
                        {formatRelativeTime(notification.timestamp)}
                      </span>
                      <div className={styles.itemActions}>
                        {/* Approve/Deny for approval notifications that have a live approval_id */}
                        {notification.notificationType === "approval_needed" && notification.metadata?.approval_id && (() => {
                          const approvalId = notification.metadata!.approval_id;
                          const resolved = resolvedApprovals[approvalId];
                          const isPending = !!pendingApprovals[approvalId];

                          if (resolved === "allow") {
                            return <span className={styles.resolvedBadge} data-decision="allow">✓ Approved</span>;
                          }
                          if (resolved === "deny") {
                            return <span className={styles.resolvedBadge} data-decision="deny">✗ Denied</span>;
                          }
                          if (resolved === "expired") {
                            return <span className={styles.resolvedBadge} data-decision="expired">Expired</span>;
                          }
                          return (
                            <>
                              <button
                                className={styles.approveButton}
                                onClick={() => resolveApproval(approvalId, "allow", group.allIds)}
                                disabled={isPending}
                                title="Approve this tool use"
                              >
                                {isPending ? "…" : "✓ Approve"}
                              </button>
                              <button
                                className={styles.denyButton}
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
                            className={styles.focusButton}
                            onClick={notification.onFocusWindow}
                            title="Focus the source application window"
                          >
                            🔗 Focus
                          </button>
                        )}
                        {notification.sessionId && (
                          <Link
                            href={`/?session=${encodeURIComponent(notification.sessionId)}`}
                            className={styles.viewButton}
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
                <div className={styles.loadMore}>
                  <button
                    className={styles.loadMoreButton}
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
      </div>
    </>
  );
}
