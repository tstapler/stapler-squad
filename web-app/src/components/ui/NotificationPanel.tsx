"use client";

import { useMemo, useState } from "react";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useApprovalResolution } from "@/lib/hooks/useApprovalResolution";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import {
  notificationTypeFilter,
  isActionableNotification,
  computeScopedMarkReadIds,
} from "@/lib/utils/notificationMapping";
import { NotificationItem, AutoHandledSection } from "./NotificationItem";
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
  loadMore,
  loadMoreButton,
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
    removeFromHistory,
    acknowledgeNotification,
    clearHistory,
    getUnreadCount,
    historyLoading,
    historyHasMore,
    loadMoreHistory,
  } = useNotifications();

  const auditLog = useAuditLog();

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);

  const { resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval } = useApprovalResolution({
    notificationHistory,
    acknowledgeNotification,
  });

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

  // Task 3.1.5a: reuses the identical scoping helper Task 3.1.2e's Notifications-page
  // button calls — never a second hand-written filter that could drift.
  const scopedMarkReadIds = useMemo(
    () => computeScopedMarkReadIds(notificationHistory),
    [notificationHistory]
  );

  const handleMarkActivityRead = () => markAsRead(scopedMarkReadIds);

  const handleClearHistory = () => {
    // Task 3.1.5d: irreversible, so gate behind a confirm — the actual
    // exclusion of unread actionable records lives server-side (Task 3.1.5c).
    if (window.confirm("Clear read notifications? This can't be undone. Items still needing a decision won't be cleared.")) {
      clearHistory();
    }
  };

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
                {scopedMarkReadIds.length > 0 && (
                  <button
                    className={markAllButton}
                    onClick={handleMarkActivityRead}
                    aria-label="Mark activity as read"
                  >
                    Mark activity read
                  </button>
                )}
                <button
                  className={clearButton}
                  onClick={handleClearHistory}
                  aria-label="Clear notification history"
                >
                  Clear history
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
              {groupNotifications(filteredNotifications).map((group) => (
                <NotificationItem
                  key={group.notification.id}
                  group={group}
                  resolvedApprovals={resolvedApprovals}
                  pendingApprovals={pendingApprovals}
                  blockedApprovals={blockedApprovals}
                  failedApprovals={failedApprovals}
                  resolveApproval={resolveApproval}
                  removeFromHistory={
                    // Task 3.1.5b: exempt an unread actionable item from the ✕
                    // control, same as NeedsDecisionSection — this dropdown has no
                    // tiered sections, so the exemption is applied inline per item.
                    isActionableNotification(group.notification.notificationType) && !group.notification.isRead
                      ? undefined
                      : removeFromHistory
                  }
                  handleNotificationClick={handleNotificationClick}
                  onNavigate={togglePanel}
                />
              ))}

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
        <AutoHandledSection
          notifications={autoHandledNotifications}
          isOpen={autoHandledOpen}
          onToggle={() => setAutoHandledOpen((v) => !v)}
        />
      </div>
    </>
  );
}
