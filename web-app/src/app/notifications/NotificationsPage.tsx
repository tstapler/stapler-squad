"use client";
// +feature: ui:notifications-page

import { useCallback, useMemo, useState } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useApprovalResolution } from "@/lib/hooks/useApprovalResolution";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import { notificationTypeFilter } from "@/lib/utils/notificationMapping";
import { NotificationItem, AutoHandledSection } from "@/components/ui/NotificationItem";
import {
  header,
  title,
  unreadBadge,
  headerActions,
  markAllButton,
  clearButton,
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
  loadMore,
  loadMoreButton,
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

  const { resolvedApprovals, pendingApprovals, blockedApprovals, resolveApproval } = useApprovalResolution({
    notificationHistory,
    acknowledgeNotification,
  });

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);

  const filteredNotifications = useMemo(() => {
    let items = notificationHistory.filter((n) => n.notificationType !== "auto_approved");
    if (typeFilter !== "all") {
      const allowed = new Set(notificationTypeFilter(typeFilter, items.map((n) => n.notificationType)));
      items = items.filter((n) => allowed.has(n.notificationType));
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      items = items.filter(
        (n) =>
          (n.sessionName || "").toLowerCase().includes(q) ||
          (n.message || "").toLowerCase().includes(q) ||
          (n.title || "").toLowerCase().includes(q)
      );
    }
    return items;
  }, [notificationHistory, typeFilter, searchQuery]);

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

  const getSessionHref = useCallback(
    (sessionId: string) =>
      liveSessionIds.has(sessionId)
        ? `/?session=${encodeURIComponent(sessionId)}`
        : `/sessions/summary?sessionId=${encodeURIComponent(sessionId)}`,
    [liveSessionIds]
  );

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

      <div className={content} data-testid="notifications-content">
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
            {groupNotifications(filteredNotifications).map((group) => (
              <NotificationItem
                key={group.notification.id}
                group={group}
                resolvedApprovals={resolvedApprovals}
                pendingApprovals={pendingApprovals}
                blockedApprovals={blockedApprovals}
                resolveApproval={resolveApproval}
                removeFromHistory={removeFromHistory}
                handleNotificationClick={handleNotificationClick}
                getSessionHref={getSessionHref}
              />
            ))}

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

      <AutoHandledSection
        notifications={autoHandledNotifications}
        isOpen={autoHandledOpen}
        onToggle={() => setAutoHandledOpen((v) => !v)}
      />
    </div>
  );
}
