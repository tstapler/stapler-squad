"use client";
// +feature: ui:notifications-page

import { useCallback, useMemo, useState } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions, selectSessionsHasLoadedOnce } from "@/lib/store/sessionsSlice";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useApprovalResolution } from "@/lib/hooks/useApprovalResolution";
import { groupNotifications } from "@/lib/utils/notificationGrouping";
import {
  notificationTypeFilter,
  isActionableNotification,
  isReconciledNotification,
  computeScopedMarkReadIds,
  capBadgeCount,
} from "@/lib/utils/notificationMapping";
import { NotificationItem, AutoHandledSection, NeedsDecisionSection } from "@/components/ui/NotificationItem";
import { CollapsibleSection } from "@/components/ui/Collapsible";
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
  loadMore,
  loadMoreButton,
  incompleteSearchNotice,
  incompleteSearchNoticeButton,
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

/** Minute-rounding elapsed-time helper for the staleness indicator (Task 3.1.2h) — a
 * one-line helper rather than a new dependency, matching Task 3.1.2h's guidance. */
function minutesAgoLabel(sinceMs: number): string {
  const minutes = Math.round((Date.now() - sinceMs) / 60000);
  return minutes <= 0 ? "just now" : `${minutes}m`;
}

export function NotificationsPage() {
  const {
    notificationHistory,
    markAsRead,
    removeFromHistory,
    acknowledgeNotification,
    clearHistory,
    getUnreadCount,
    historyLoading,
    historyHasMore,
    historyError,
    historyLastUpdatedAt,
    loadMoreHistory,
    refreshHistory,
  } = useNotifications();

  const auditLog = useAuditLog();

  // Epic 3.3 (session-completion-summary), Story 3.3.2: a notification's
  // sessionId may reference a session that's since been deleted from the
  // live list (e.g. after DeleteSession). In that case "View Session" falls
  // back to the durable standalone summary route instead of a dead/no-op
  // `/?session=<id>` link.
  const liveSessions = useAppSelector(selectAllSessions);
  const liveSessionIds = useMemo(() => new Set(liveSessions.map((s) => s.id)), [liveSessions]);
  // Some notifications (e.g. SendNotification's poller-miss fallback in
  // notification_service.go) were recorded with the session's title rather than its
  // stable id — page.tsx's findSessionById already tolerates this by falling back to a
  // title match, so this liveness check has to match it or it wrongly reports a live
  // session as gone. See the reproduced case in NotificationsPage.test.tsx.
  const liveSessionTitles = useMemo(
    () => new Set(liveSessions.map((s) => s.title.toLowerCase())),
    [liveSessions]
  );
  const hasLoadedSessionsOnce = useAppSelector(selectSessionsHasLoadedOnce);

  const { resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval } = useApprovalResolution({
    notificationHistory,
    acknowledgeNotification,
  });

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [hideBacklogItems, setHideBacklogItems] = useState(false);
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);

  // Task 3.1.2c: resets exactly the three state variables hasActiveFilter (below)
  // already tracks, so "Clear filter" can never drift out of sync with what the
  // page itself considers "a filter is active."
  const clearFilters = useCallback(() => {
    setSearchQuery("");
    setTypeFilter("all");
    setHideBacklogItems(false);
  }, []);

  const filteredNotifications = useMemo(() => {
    // Task 3.1.3a: a rule-reconciled item is excluded from the main feed the
    // same way a live auto_approved item is — both surface only in AutoHandledSection.
    let items = notificationHistory.filter(
      (n) => n.notificationType !== "auto_approved" && !isReconciledNotification(n)
    );
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

  // Story 3.1.2: split the filtered set into the always-expanded "needs a
  // decision" tier and everything else ("recent activity" — never
  // "informational", which is Task 3.2.1a's Review Queue tier name).
  const needsDecision = useMemo(
    () => filteredNotifications.filter((n) => !n.isRead && isActionableNotification(n.notificationType)),
    [filteredNotifications]
  );
  const recentActivity = useMemo(
    () => filteredNotifications.filter((n) => n.isRead || !isActionableNotification(n.notificationType)),
    [filteredNotifications]
  );

  // The true unfiltered actionable count — deliberately computed over
  // notificationHistory, never filteredNotifications/needsDecision, so
  // NeedsDecisionSection can tell "nothing needs a decision" apart from "a
  // filter is hiding something that does" (Product Triad Review round-4 fix).
  const totalActionableCount = useMemo(
    () => notificationHistory.filter((n) => !n.isRead && isActionableNotification(n.notificationType)).length,
    [notificationHistory]
  );

  const hasActiveFilter = searchQuery.trim() !== "" || typeFilter !== "all" || hideBacklogItems;

  // The search box and "Hide backlog" toggle only filter over notificationHistory
  // that's already been paged into memory (see filteredNotifications above) — they
  // never re-query the server for older, not-yet-loaded history. When more history
  // remains (historyHasMore) and one of those two filters is active, surface that
  // instead of silently returning incomplete results.
  const hasIncompleteSearch = (searchQuery.trim() !== "" || hideBacklogItems) && historyHasMore;

  const autoHandledNotifications = useMemo(
    () =>
      notificationHistory.filter(
        (n) => n.notificationType === "auto_approved" || isReconciledNotification(n)
      ),
    [notificationHistory]
  );

  const unreadCount = getUnreadCount();

  // Task 3.1.2e: "Mark activity read" only ever touches Recent Activity +
  // Auto-handled — never an item in NeedsDecisionSection. Shared with
  // NotificationPanel's identical button (Task 3.1.5a) via computeScopedMarkReadIds
  // so the scoping rule is defined once.
  const scopedMarkReadIds = useMemo(
    () => computeScopedMarkReadIds([...recentActivity, ...autoHandledNotifications]),
    [recentActivity, autoHandledNotifications]
  );

  const handleMarkActivityRead = useCallback(() => {
    markAsRead(scopedMarkReadIds);
  }, [markAsRead, scopedMarkReadIds]);

  const handleClearHistory = useCallback(() => {
    // Task 3.1.5d: irreversible, so gate behind a confirm — the actual
    // exclusion of unread actionable records lives server-side (Task 3.1.5c).
    if (window.confirm("Clear read notifications? This can't be undone. Items still needing a decision won't be cleared.")) {
      clearHistory();
    }
  }, [clearHistory]);

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
      // Before the sessions store has ever loaded, an absent id means "haven't
      // heard yet," not "confirmed gone" — default to the live route so a fast
      // click right after page load doesn't race the first WatchSessions
      // snapshot and land on the summary route for a session that's actually live.
      // Also accept a title match (see liveSessionTitles above) since some
      // recorded notifications carry the title in place of the stable id.
      !hasLoadedSessionsOnce ||
      liveSessionIds.has(sessionId) ||
      liveSessionTitles.has(sessionId.toLowerCase())
        ? `/?session=${encodeURIComponent(sessionId)}`
        : `/sessions/summary?sessionId=${encodeURIComponent(sessionId)}`,
    [liveSessionIds, liveSessionTitles, hasLoadedSessionsOnce]
  );

  // Task 3.1.2h (AC38): background fetch-failure staleness indicator — only
  // shown once a failure occurs, never implying the last-known list is fresher
  // than it is.
  const staleness = useMemo(
    () =>
      historyError && historyLastUpdatedAt !== null
        ? { label: minutesAgoLabel(historyLastUpdatedAt), onRetry: () => void refreshHistory() }
        : undefined,
    [historyError, historyLastUpdatedAt, refreshHistory]
  );

  // "Recent activity" CollapsibleSection — identical between the two branches
  // below (one totalActionableCount>0-but-empty-needsDecision, one the normal
  // path); only the outer `recentActivity.length > 0` guard differs per call
  // site, so it's applied by each caller rather than baked in here (mirrors
  // ReviewQueuePanel.tsx's renderTierItems/groupTierItems local-function pattern).
  const renderRecentActivitySection = () => (
    <CollapsibleSection sectionKey="recent-activity" title={`Recent activity · ${recentActivity.length}`}>
      <div className={list}>
        {groupNotifications(recentActivity).map((group) => (
          <NotificationItem
            key={group.notification.id}
            group={group}
            resolvedApprovals={resolvedApprovals}
            pendingApprovals={pendingApprovals}
            blockedApprovals={blockedApprovals}
            failedApprovals={failedApprovals}
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
    </CollapsibleSection>
  );

  return (
    <div className={pageRoot}>
      <div className={header} data-testid="notifications-header">
        <h2 className={title} data-testid="notifications-title">
          Notifications
          {unreadCount > 0 && (
            <span className={unreadBadge} data-testid="notifications-unread-badge">
              {capBadgeCount(unreadCount)}
            </span>
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
                  data-testid="notifications-mark-all-read"
                >
                  Mark activity read
                </button>
              )}
              <button
                className={clearButton}
                onClick={handleClearHistory}
                aria-label="Clear notification history"
                data-testid="notifications-clear-all"
              >
                Clear history
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
        ) : totalActionableCount > 0 && needsDecision.length === 0 ? (
          // Product Triad Review round-5 blocker fix: this branch must be
          // reachable even when recentActivity is ALSO empty — see Task 3.1.2c.
          <>
            <NeedsDecisionSection
              notifications={[]}
              totalActionableCount={totalActionableCount}
              onClearFilter={clearFilters}
              resolvedApprovals={resolvedApprovals}
              pendingApprovals={pendingApprovals}
              blockedApprovals={blockedApprovals}
              failedApprovals={failedApprovals}
              resolveApproval={resolveApproval}
              handleNotificationClick={handleNotificationClick}
              getSessionHref={getSessionHref}
              staleness={staleness}
            />
            {recentActivity.length > 0 && renderRecentActivitySection()}
          </>
        ) : filteredNotifications.length === 0 ? (
          // Reachable only when totalActionableCount === 0 — the branch above
          // already caught every case where it's nonzero.
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
          <>
            <NeedsDecisionSection
              notifications={needsDecision}
              totalActionableCount={totalActionableCount}
              onClearFilter={clearFilters}
              resolvedApprovals={resolvedApprovals}
              pendingApprovals={pendingApprovals}
              blockedApprovals={blockedApprovals}
              failedApprovals={failedApprovals}
              resolveApproval={resolveApproval}
              handleNotificationClick={handleNotificationClick}
              getSessionHref={getSessionHref}
              staleness={staleness}
            />
            {renderRecentActivitySection()}
          </>
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
