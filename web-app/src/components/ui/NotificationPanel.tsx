// +feature: notification-tray
"use client";

import { resetBellPos } from "@/lib/utils/deckSettings";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useDeckViewport } from "@/lib/contexts/deckViewportContext";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import { useApprovalResolution } from "@/lib/hooks/useApprovalResolution";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useNotificationConnectivity } from "@/lib/hooks/useNotificationConnectivity";
import { useStableRowOrder } from "@/lib/hooks/useStableRowOrder";
import { EngagementContext, useEngagementTracker } from "@/lib/hooks/useEngagedCard";
import { useFrozenList } from "@/lib/hooks/useFrozenList";
import { useScrollSettle } from "@/lib/hooks/useScrollSettle";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import { useBackgroundActivity } from "@/lib/hooks/useBackgroundActivity";
import { usePeerEscalationAnnouncement } from "@/lib/hooks/usePeerEscalationAnnouncement";
import { usePaneOpenTiming } from "@/lib/hooks/usePaneOpenTiming";
import { useTrayFocus } from "@/lib/hooks/useTrayFocus";
import { useTrayBulkActions } from "@/lib/hooks/useTrayBulkActions";
import { useTrayDismissal } from "@/lib/hooks/useTrayDismissal";
import { routes } from "@/lib/routes";
import { countNeedsAttention, groupNotifications, flattenGroups, type GroupedNotification } from "@/lib/utils/notificationGrouping";
import {
  notificationTypeFilter,
  computeScopedMarkReadIds,
} from "@/lib/utils/notificationMapping";
import type { ActivityRow } from "@/lib/utils/backgroundActivity";
import { readQuietMode, readTrayPinned, readWhatChangedSeen, writeTrayPinned, writeWhatChangedSeen } from "@/lib/utils/deckSettings";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { AutoHandledSection } from "./NotificationItem";
import { SavedForLaterSection } from "./SavedForLaterSection";
import { useSavedAnswers } from "@/lib/hooks/useSavedAnswers";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
import { buildPrefillHref } from "@/lib/ruleBuilderPrefill";
import { rulePrefillFor } from "./notification-actions/inlineActions";
import { BackgroundActivity } from "./BackgroundActivity";
import { ScheduledPromptsSection } from "./ScheduledPromptsSection";
import {
  ActionsDisabledReasonContext,
  OfflineBanner,
  actionsDisabledReason,
  formatClockTime,
  useAwaitingFresh,
} from "./OfflineBanner";
import { asOfLine } from "./OfflineBanner.css";
import { segment, segments } from "./BackgroundActivity.css";
import { TrayConfirm } from "./TrayConfirm";
import { TrayHandle } from "./TrayHandle";
import { TrayList, type ListRow } from "./TrayList";
import { TrayOverflowMenu, type TrayMenuItem } from "./TrayOverflowMenu";
import { TraySettings, WHAT_CHANGED_TEXT, WhatChangedCard } from "./TraySettings";
import { selectTrayBanner, selectTrayState } from "./trayState";
import { selectTrayVariant } from "./trayVariant";
import {
  header,
  title,
  headerActions,
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
  newPill,
  sheetGrabber,
  sheetGrabberRow,
  sheetScrim,
  statusSlot,
  trayAttention,
  trayCounts,
  trayBanner,
  trayButton,
  trayFooter,
  trayUndoBar,
  trayVariant,
} from "./NotificationPanel.css";

type TypeFilter = "all" | "approval_needed" | "error" | "task_complete" | "info";

const TYPE_FILTER_LABELS: Record<TypeFilter, string> = {
  all: "All",
  approval_needed: "Approval",
  error: "Error",
  task_complete: "Task",
  info: "Info",
};

type TraySegment = "notifications" | "background";

const TRAY_ID = "notification-tray";
const HEADING_ID = "notification-tray-heading";
const GRABBER_DRAG_PX = 80;

/**
 * NotificationPanel - the notification history surface.
 *
 * The non-modal tray (ADR-006): no backdrop, no `aria-modal`, `transform`
 * only motion, grouped and virtualized rows, a pinned "Needs attention" group,
 * bulk actions guarded server-side with undo, and layout that never touches the
 * terminal.
 */
export function NotificationPanel() {
  const replyEnabled = useFeatureFlag("hidden_session_reply");
  const {
    notificationHistory,
    isPanelOpen,
    togglePanel,
    markAsRead,
    acknowledgeNotification,
    getUnreadCount,
    historyLoading,
    historyHasMore,
    historyError,
    historyLastUpdatedAt,
    loadMoreHistory,
    refreshHistory,
    clearHistoryByIds,
    broadcastActionResolved,
    answeredElsewhere,
    showActionToast,
    addNotification,
    markBackgroundReviewed,
    quietMode,
    setQuietMode,
  } = useNotifications();

  const auditLog = useAuditLog();
  const { announce } = useAnnounce();
  const viewport = useDeckViewport();
  const { isOffline } = useNotificationConnectivity();
  const awaitingFresh = useAwaitingFresh(isOffline, historyLastUpdatedAt);
  const actionsReason = actionsDisabledReason({
    online: !isOffline,
    reconnecting: historyError !== null,
    awaitingFresh,
  });
  const actionsDisabled = actionsReason !== undefined;
  const coarse = useCoarsePointer();
  const variant = selectTrayVariant(viewport);
  const isBottomSheet = variant === "bottom-sheet";
  // The phone sheet owns a history entry for hardware Back, and drags and expands.
  const isSheet = isBottomSheet;
  const isSideOverlay = variant === "side-overlay";

  const scrollSettle = useScrollSettle();
  const [slotEl, setSlotEl] = useState<HTMLDivElement | null>(null);
  const operatorResolved = useRef<Set<string>>(new Set());
  const trayRef = useRef<HTMLDivElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const headRef = useRef<HTMLDivElement>(null);
  const [headHeight, setHeadHeight] = useState(0);

  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [autoHandledOpen, setAutoHandledOpen] = useState(false);
  const structuredOn = useFeatureFlagValue("askuserquestion_structured", "stage:render", false);
  const savedAnswers = useSavedAnswers(isPanelOpen && structuredOn);
  const sessionNames = useMemo(
    () => Object.fromEntries(notificationHistory.filter((n) => n.sessionId && n.sessionName).map((n) => [n.sessionId, n.sessionName])),
    [notificationHistory],
  );
  const [expanded, setExpanded] = useState(false);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [scrolled, setScrolled] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [whatChangedOpen, setWhatChangedOpen] = useState(() => !readWhatChangedSeen());
  const [pinned, setPinned] = useState(false);
  // True when a Quiet toggle did not persist (storage blocked): it still applies until reload.
  const [quietUnsaved, setQuietUnsaved] = useState(false);
  const [traySegment, setTraySegment] = useState<TraySegment>("notifications");

  const { resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval } = useApprovalResolution({
    notificationHistory,
    acknowledgeNotification,
  });

  useTrayFocus({ isOpen: isPanelOpen, trayRef, headingRef, coarse });

  // Pin tray is read after mount so server and client render the same first frame.
  useEffect(() => setPinned(readTrayPinned()), []);
  const docked = isSideOverlay && pinned && isPanelOpen;
  useEffect(() => {
    if (!docked) return;
    document.documentElement.dataset.trayPinned = "true";
    return () => {
      delete document.documentElement.dataset.trayPinned;
    };
  }, [docked]);

  const modalSheet = isBottomSheet && expanded && !coarse && isPanelOpen;

  const close = useCallback(() => {
    if (isPanelOpen) togglePanel();
  }, [isPanelOpen, togglePanel]);
  const { releaseHistoryEntry } = useTrayDismissal({
    isOpen: isPanelOpen,
    isSheet,
    modal: modalSheet,
    trayRef,
    close,
  });
  const closeForNavigation = useCallback(() => {
    releaseHistoryEntry();
    close();
  }, [releaseHistoryEntry, close]);

  const trapTab = (e: React.KeyboardEvent) => {
    if (!modalSheet || e.key !== "Tab") return;
    const focusable = Array.from(
      trayRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), a[href], input, select, [tabindex]:not([tabindex="-1"])') ?? [],
    ).filter((el) => el.offsetParent !== null || el === document.activeElement);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };

  // Only the grabber drags the sheet; list content never does (C12).
  const dragStartY = useRef<number | null>(null);
  const onGrabberDown = (e: React.PointerEvent) => {
    dragStartY.current = e.clientY;
    e.currentTarget.setPointerCapture?.(e.pointerId);
  };
  const onGrabberUp = (e: React.PointerEvent) => {
    const start = dragStartY.current;
    dragStartY.current = null;
    if (start === null) return;
    const dy = e.clientY - start;
    if (dy >= GRABBER_DRAG_PX) {
      if (expanded) setExpanded(false);
      else close();
    } else if (dy <= -GRABBER_DRAG_PX) setExpanded(true);
  };

  // Reuses the one scoping helper the Notifications page calls — never a second
  // hand-written filter that could drift.
  const scopedMarkReadIds = useMemo(
    () => computeScopedMarkReadIds(notificationHistory),
    [notificationHistory]
  );

  const bulk = useTrayBulkActions({
    notificationHistory,
    isOffline: actionsDisabled,
    needsAttention: countNeedsAttention(notificationHistory),
    scopedMarkReadIds,
    announce,
    markAsRead,
    clearHistoryByIds,
    showActionToast,
  });
  const { undoWindow, clearError } = bulk;

  // The search bar, filters and cards scroll with the list (a peek sheet has little height to spare),
  // so the virtualizer needs their height as its scroll margin.
  useLayoutEffect(() => {
    const head = headRef.current;
    if (!head) return;
    setHeadHeight(head.offsetHeight);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => setHeadHeight(head.offsetHeight));
    observer.observe(head);
    return () => observer.disconnect();
  }, []);

  // Closing resets the sheet so it reopens at peek.
  useEffect(() => {
    if (!isPanelOpen) {
      setExpanded(false);
      setSettingsOpen(false);
      bulk.reset();
    }
    // `bulk.reset` only sets state to its initial values.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isPanelOpen]);

  // Filter notifications by search query and type; auto_approved records are always excluded
  // from the main list and shown in a separate collapsible section.
  const visibleHistory = useMemo(
    () => (bulk.hiddenIds.size === 0 ? notificationHistory : notificationHistory.filter((n) => !bulk.hiddenIds.has(n.id))),
    [notificationHistory, bulk.hiddenIds],
  );

  // A card the operator is working on holds the Needs-you tier still (design/ux.md 7.1).
  const { tracker, frozen } = useEngagementTracker();
  const frozenList = useFrozenList(visibleHistory, frozen, {
    isHeld: isNeedsYou,
    onMerge: (n) => announce(n === 1 ? "1 new item" : `${n} new items`, "polite", "tray-new-items"),
  });
  useEffect(() => {
    if (!isPanelOpen) tracker.reset();
  }, [isPanelOpen, tracker]);
  // A held row whose live record is no longer pending was resolved elsewhere: it stops being
  // engaged by focus or a draft, and stays in place until the list unfreezes.
  useEffect(() => {
    const live = new Map(visibleHistory.map((n) => [n.id, n]));
    for (const n of frozenList.items) if (isNeedsYou(n) && !isNeedsYou(live.get(n.id))) tracker.setResolved(n.id, true);
  }, [visibleHistory, frozenList.items, tracker]);

  const stable = useStableRowOrder(frozenList.items, isPanelOpen, scrolled);
  const newCount = frozenList.queued + stable.heldCount;
  const mergeNew = () => {
    mergeFocusId.current = frozenList.queuedIds[0] ?? null;
    frozenList.merge();
    stable.release();
  };
  // "N new" moves focus to the first merged row's header: the operator's own action.
  const mergeFocusId = useRef<string | null>(null);
  useEffect(() => {
    const id = mergeFocusId.current;
    if (!id) return;
    const el = trayRef.current?.querySelector<HTMLElement>(`[data-tray-row="${CSS.escape(id)}"]`);
    if (!el) return;
    mergeFocusId.current = null;
    el.focus({ preventScroll: true });
  });

  const filteredNotifications = useMemo(() => {
    let rows = stable.items.filter((n) => n.notificationType !== "auto_approved");

    if (typeFilter !== "all") {
      const allowed = new Set(
        notificationTypeFilter(typeFilter, rows.map((n) => n.notificationType))
      );
      rows = rows.filter((n) => allowed.has(n.notificationType));
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      rows = rows.filter(
        (n) =>
          (n.sessionName || "").toLowerCase().includes(q) ||
          (n.message || "").toLowerCase().includes(q) ||
          (n.title || "").toLowerCase().includes(q)
      );
    }

    return rows;
  }, [stable.items, typeFilter, searchQuery]);

  const autoHandledNotifications = useMemo(() => {
    return notificationHistory.filter((n) => n.notificationType === "auto_approved");
  }, [notificationHistory]);

  const unreadCount = getUnreadCount();
  usePeerEscalationAnnouncement(notificationHistory, announce);

  // Background activity (Story 5.4): polled only while its segment is showing; the segment
  // badge is the same join over history, so it adds nothing to the bell count.
  const backgroundVisible = isPanelOpen && traySegment === "background";
  const background = useBackgroundActivity(backgroundVisible && !isOffline, notificationHistory, () =>
    addNotification({
      sessionId: "",
      sessionName: "",
      title: "Background outcome evidence is empty",
      message: "No background session has reported a finished outcome in 24 hours. Stopped sessions are shown as outcome unknown.",
      notificationType: "warning",
    }),
  );
  usePaneOpenTiming(isPanelOpen, notificationHistory.length > 0);
  const announcedBackgroundLoad = useRef(false);
  useEffect(() => {
    if (backgroundVisible && background.poll.loading && !announcedBackgroundLoad.current) {
      announcedBackgroundLoad.current = true;
      announce("Loading background activity", "polite", "background-loading");
    }
    if (!background.poll.loading) announcedBackgroundLoad.current = false;
  }, [backgroundVisible, background.poll.loading, announce]);

  // The one polite region speaks each connectivity change once; the banner itself is plain text.
  const wasOffline = useRef(isOffline);
  useEffect(() => {
    if (wasOffline.current !== isOffline) announce(isOffline ? "Offline" : "Back online", "polite", "connectivity");
    wasOffline.current = isOffline;
  }, [isOffline, announce]);

  const { rows: flatRows, needsAttention, setSize } = useMemo(
    () => flattenGroups(filteredNotifications, collapsed),
    [filteredNotifications, collapsed],
  );

  // Unread items that are not pending decisions, so the two counts never overlap.
  const earlierUnread = Math.max(0, unreadCount - needsAttention);

  const listRows: ListRow[] = useMemo(
    () => (historyHasMore ? [...flatRows, { kind: "load-more", key: "__load_more__" }] : flatRows),
    [flatRows, historyHasMore],
  );
  const groupsById = useMemo(() => {
    const map = new Map<string, GroupedNotification>();
    for (const g of groupNotifications(filteredNotifications)) map.set(g.notification.id, g);
    return map;
  }, [filteredNotifications]);

  const dismissGroup = (group: GroupedNotification) =>
    bulk.startUndoableClear(
      group.allIds.length > 1 ? `Dismissed ${group.allIds.length} notifications` : "Dismissed 1 notification",
      group.allIds,
    );

  const dismissSession = (sessionKey: string) => {
    const ids: string[] = [];
    for (const g of groupNotifications(filteredNotifications)) {
      if ((g.notification.sessionId || "__no_session__") === sessionKey && !g.notification.isPendingDecision) ids.push(...g.allIds);
    }
    bulk.startUndoableClear(`Dismissed ${ids.length} informational`, ids);
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

  const openBackgroundRow = (row: ActivityRow) => {
    if (row.exception) {
      markAsRead(row.exception.recordIds);
      auditLog.logNotificationSessionViewed(row.exception.primaryRecordId, row.sessionId);
    }
    closeForNavigation();
  };

  const toggleGroup = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  const sessionKeys = useMemo(
    () => flatRows.filter((r) => r.kind === "header" && !r.pinned).map((r) => r.key),
    [flatRows],
  );

  const filtered = !!searchQuery.trim() || typeFilter !== "all";
  const trayState = selectTrayState({
    isOffline,
    loading: historyLoading,
    hasLoadError: historyError !== null,
    hasLoadedOnce: historyLastUpdatedAt !== null,
    rowCount: filteredNotifications.length,
    filtered,
    needsAttentionCount: needsAttention,
    hasMore: historyHasMore,
  });
  const banner = selectTrayBanner({ isOffline, hasLoadError: historyError !== null, rowCount: notificationHistory.length });
  const updatedAt = historyLastUpdatedAt ? new Date(historyLastUpdatedAt).toLocaleTimeString() : null;
  const offlineReason = actionsReason;

  // The one-time "What changed" card is announced once, politely, with no focus move (TM-13).
  const announcedWhatChanged = useRef(false);
  useEffect(() => {
    if (isPanelOpen && whatChangedOpen && !announcedWhatChanged.current) {
      announcedWhatChanged.current = true;
      announce(WHAT_CHANGED_TEXT, "polite", "what-changed");
    }
  }, [isPanelOpen, whatChangedOpen, announce]);

  const menuGroups: TrayMenuItem[][] = [
    [
      {
        key: "clear-informational",
        label: `Clear informational (${bulk.informationalIds.length})`,
        caption: "Undo available",
        disabledReason: actionsReason ?? (bulk.informationalIds.length === 0 ? "Nothing to clear" : undefined),
        onSelect: () => bulk.openConfirm("informational"),
      },
    ],
    [
      {
        key: "clear-history",
        label: "Clear history...",
        icon: <span aria-hidden="true">⚠ </span>,
        caption: "cannot be undone",
        disabledReason: actionsReason ?? (bulk.readIds.length === 0 ? "Nothing to clear" : undefined),
        onSelect: () => bulk.openConfirm("history"),
      },
    ],
    [
      { key: "collapse-all", label: "Collapse all groups", onSelect: () => setCollapsed(new Set(sessionKeys)) },
      { key: "expand-all", label: "Expand all groups", onSelect: () => setCollapsed(new Set()) },
    ],
    [
      { key: "settings", label: "Tray settings", onSelect: () => setSettingsOpen((v) => !v) },
      { key: "reset-bell", label: "Reset notification button position", onSelect: () => resetBellPos() },
      { key: "what-changed", label: "What changed", onSelect: () => setWhatChangedOpen(true) },
    ],
  ];

  const emptyStateText = () => {
    if (trayState.kind === "empty-filtered") return { icon: "🔍", text: "No matching notifications", sub: "Try adjusting your search or filter" };
    if (trayState.kind === "offline") return { icon: "📡", text: "Offline. Showing last known state.", sub: updatedAt ? `Updated ${updatedAt}` : "Reconnect to load notifications" };
    if (trayState.kind === "load-error") return { icon: "⚠", text: "Couldn't load notifications.", sub: "" };
    return { icon: "🔔", text: "Nothing needs you right now.", sub: "No recent activity." };
  };

  const renderBody = () => {
    if (trayState.kind === "loading") {
      return (
        <div className={empty} aria-busy="true" data-testid="tray-skeleton">
          <div className={emptyIcon} aria-hidden="true">⏳</div>
          <OfflineBanner
            online
            loading
            hasData={false}
            onRetry={() => refreshHistory()}
            testId="tray-loading"
          />
        </div>
      );
    }
    if (filteredNotifications.length === 0) {
      const e = emptyStateText();
      return (
        <div className={empty} data-testid={`tray-empty-${trayState.kind}`}>
          <div className={emptyIcon} aria-hidden="true">{e.icon}</div>
          <p className={emptyText}>{e.text}</p>
          {e.sub && <p className={emptySubtext}>{e.sub}</p>}
          {trayState.exit === "retry" && (
            <button type="button" className={trayButton} onClick={() => refreshHistory()}>
              Retry
            </button>
          )}
          {trayState.exit === "clear-filters" && (
            <button
              type="button"
              className={trayButton}
              onClick={() => {
                setSearchQuery("");
                setTypeFilter("all");
              }}
            >
              Clear filters
            </button>
          )}
        </div>
      );
    }
    return (
      <>
        <TrayList
          rows={listRows}
          setSize={setSize}
          itemProps={{
            resolvedApprovals,
            pendingApprovals,
            blockedApprovals,
            failedApprovals,
            resolveApproval,
            handleNotificationClick,
            onNavigate: closeForNavigation,
            lookupHidden: isPanelOpen,
            onRefreshActions: () => void refreshHistory(),
            answeredElsewhere,
            onActionApplied: (ids, summary) => {
              ids.forEach((id) => operatorResolved.current.add(id));
              acknowledgeNotification(ids);
              broadcastActionResolved?.(ids, summary);
            },
            onCreateRule: (n) => {
              closeForNavigation();
              window.location.assign(buildPrefillHref(rulePrefillFor(n)));
            },
          }}
          offlineReason={offlineReason}
          scrollRef={scrollRef}
          scrollMargin={headHeight}
          hasMore={historyHasMore}
          loading={historyLoading}
          onLoadMore={loadMoreHistory}
          onToggleGroup={toggleGroup}
          onDismissGroup={dismissGroup}
          onDismissSession={dismissSession}
          groupForId={(id) => groupsById.get(id)}
          onFocusAdvanced={(key) => {
            // A late acknowledgement is silent unless focus was inside the leaving row (D6).
            if (!operatorResolved.current.has(key)) announce("Answer confirmed, moved to Recent activity", "polite", "tray-row-left");
          }}
        />
      </>
    );
  };

  const cardsBlock = (
    <>
{settingsOpen && <TraySettings />}
{whatChangedOpen && (
          <WhatChangedCard
            onDismiss={() => {
              writeWhatChangedSeen();
              setWhatChangedOpen(false);
            }}
          />
        )}

    </>
  );
  const filterBlock = (
    <>
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

    </>
  );

  const variantClass = {
    "side-overlay": trayVariant.sideOverlay,
    "bottom-sheet": trayVariant.bottomSheet,
    "landscape-panel": trayVariant.landscapePanel,
  }[variant];

  return (
    <EngagementContext.Provider value={tracker}>
    <ActionsDisabledReasonContext.Provider value={actionsReason}>
      {modalSheet && <div className={sheetScrim} data-testid="tray-scrim" onClick={close} aria-hidden="true" />}
      <TrayHandle />

      <div
        ref={trayRef}
        id={TRAY_ID}
        className={variantClass}
        data-testid="notification-tray"
        data-gesture-ignore
        data-notification-tray="v2"
        data-variant={variant}
        data-state={isPanelOpen ? "open" : "closed"}
        data-sheet={isBottomSheet ? (expanded ? "expanded" : "peek") : undefined}
        data-keyboard={isBottomSheet && viewport.isVirtualKeyboardOpen ? "open" : undefined}
        role={modalSheet ? "dialog" : "complementary"}
        aria-labelledby={HEADING_ID}
        aria-modal={modalSheet ? true : undefined}
        onKeyDown={trapTab}
      >
        {isBottomSheet && (
          <div className={sheetGrabberRow}>
            <button
              type="button"
              className={sheetGrabber}
              data-testid="tray-grabber"
              aria-label="Drag to resize notifications sheet"
              tabIndex={-1}
              onPointerDown={onGrabberDown}
              onPointerUp={onGrabberUp}
              onPointerCancel={() => (dragStartY.current = null)}
            />
            <button
              type="button"
              className={trayButton}
              data-testid="tray-expand"
              aria-pressed={expanded}
              onClick={() => setExpanded((v) => !v)}
            >
              {expanded ? "Collapse" : "Expand"}
            </button>
          </div>
        )}

        {/* Header */}
        <div className={header}>
          <h2 id={HEADING_ID} ref={headingRef} tabIndex={-1} className={title}>
            Notifications
          </h2>
          {(needsAttention > 0 || earlierUnread > 0) && (
            <span className={trayCounts}>
              {needsAttention > 0 && (
                <span className={trayAttention} data-testid="tray-needs-attention">
                  {needsAttention} need you
                </span>
              )}
              {needsAttention > 0 && earlierUnread > 0 && " \u00b7 "}
              {earlierUnread > 0 && (
                <span data-testid="tray-earlier-count">
                  {earlierUnread} {needsAttention > 0 ? "earlier" : "unread"}
                </span>
              )}
            </span>
          )}
          <div className={headerActions}>
            {notificationHistory.length > 0 && (
              <>
                {scopedMarkReadIds.length > 0 && (
                  <button
                    className={trayButton}
                    onClick={actionsDisabled ? undefined : bulk.markActivityRead}
                    aria-label="Mark activity read"
                    aria-disabled={actionsDisabled ? true : undefined}
                    title={actionsReason}
                  >
                    Mark activity read
                  </button>
                )}
              </>
            )}
            {isSideOverlay && (
              <button
                type="button"
                className={trayButton}
                aria-pressed={pinned}
                aria-label={pinned ? "Unpin notifications tray" : "Pin notifications tray"}
                data-testid="tray-pin"
                onClick={() => {
                  writeTrayPinned(!pinned);
                  setPinned(!pinned);
                }}
              >
                Pin
              </button>
            )}
            <button
              type="button"
              className={trayButton}
              aria-pressed={quietMode}
              data-testid="tray-quiet"
              title="Quiet mode: send every non-urgent toast straight to the tray on this device. Push notifications are unchanged."
              onClick={() => {
                const next = !quietMode;
                setQuietMode(next);
                setQuietUnsaved(next && !readQuietMode());
              }}
            >
              Quiet mode{quietMode ? " on" : ""}
            </button>
            <TrayOverflowMenu groups={menuGroups} />
            <button
              className={closeButton}
              onClick={togglePanel}
              aria-label="Close notification panel"
              style={{ minWidth: 44, minHeight: 44 }}
            >
              ✕
            </button>
          </div>
        </div>

        {(quietMode || quietUnsaved) && (
          <div className={trayBanner} data-testid="tray-quiet-banner">
            <span>
              <strong data-testid="tray-quiet-state">Quiet mode on</strong>
              <span data-testid="tray-quiet-hint">
                {" "}
                - hides toasts on this device. Push notifications are unchanged.
              </span>
            </span>
            {quietUnsaved && <span data-testid="tray-quiet-unsaved">Preference could not be saved</span>}
          </div>
        )}
        {banner === "load-error" && (
          <div className={trayBanner} data-testid="tray-banner-load-error">
            <span>
              Couldn&apos;t load notifications.
              {historyLastUpdatedAt !== null ? ` Showing data from ${formatClockTime(historyLastUpdatedAt)}.` : ""}
            </span>
            <button type="button" className={trayButton} onClick={() => refreshHistory()}>
              Retry
            </button>
          </div>
        )}
        {undoWindow.pending && (
          <div
            key={undoWindow.pending.id}
            className={trayUndoBar}
            data-testid="tray-undo-bar"
            onPointerEnter={() => undoWindow.pause("hover")}
            onPointerLeave={() => undoWindow.resume("hover")}
            onFocus={() => undoWindow.pause("focus")}
            onBlur={() => undoWindow.resume("focus")}
          >
            <span>{undoWindow.pending.label}</span>
            <button type="button" className={trayButton} data-testid="tray-undo" onClick={bulk.undo}>
              Undo
            </button>
          </div>
        )}
        {bulk.keptLine && (
          <div className={trayBanner} data-testid="tray-kept-line">
            <span>{bulk.keptLine}</span>
          </div>
        )}
        {clearError && (
          <div className={trayBanner} data-testid="tray-clear-error">
            <span>Could not clear notifications</span>
            <button type="button" className={trayButton} onClick={() => bulk.startUndoableClear(clearError.label, clearError.ids)}>
              Retry
            </button>
          </div>
        )}
        {bulk.markReadError && (
          <div className={trayBanner} data-testid="tray-mark-read-error">
            <span>Could not mark read</span>
            <button type="button" className={trayButton} onClick={bulk.markActivityRead}>
              Retry
            </button>
          </div>
        )}
        {bulk.confirm === "informational" && (
          <TrayConfirm
            text={`Clear ${bulk.informationalIds.length} informational notifications? ${bulk.decisionCount} awaiting decision kept.`}
            confirmLabel={`Clear ${bulk.informationalIds.length}`}
            onConfirm={bulk.confirmClearInformational}
            onCancel={bulk.cancelConfirm}
          />
        )}
        {bulk.confirm === "history" && (
          <TrayConfirm
            text={`Clear ${bulk.readIds.length} read notifications? This can't be undone. ${bulk.decisionCount} awaiting decision kept.`}
            confirmLabel={`Clear ${bulk.readIds.length}`}
            error={bulk.confirmError}
            onConfirm={bulk.confirmClearHistory}
            onCancel={bulk.cancelConfirm}
          />
        )}
        <div className={segments} role="tablist" aria-label="Tray sections" data-testid="tray-segments">
          {(["notifications", "background"] as const).map((id) => {
            const selected = traySegment === id;
            const count = id === "background" ? background.exceptionCount : 0;
            return (
              // analytics-exempt
              <button
                key={id}
                type="button"
                role="tab"
                id={`tray-tab-${id}`}
                aria-selected={selected}
                aria-label={count > 0 ? `Background, ${count} needs attention` : undefined}
                aria-controls="tray-segment-panel"
                tabIndex={selected ? 0 : -1}
                className={segment}
                data-testid={`tray-tab-${id}`}
                onClick={() => setTraySegment(id)}
                onKeyDown={(e) => {
                  if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
                    e.preventDefault();
                    const next = id === "notifications" ? "background" : "notifications";
                    setTraySegment(next);
                    document.getElementById(`tray-tab-${next}`)?.focus();
                  }
                }}
              >
                {id === "notifications" ? "Notifications" : count > 0 ? `Background (${count})` : "Background"}
              </button>
            );
          })}
        </div>

        {/* Banner, "N new", failure strip and "As of" share one reserved slot, outside the scroll region. */}
        <div ref={setSlotEl} className={statusSlot} data-testid="tray-status-slot">
          <OfflineBanner online={!isOffline} testId="tray-banner-offline" />
          {traySegment !== "background" && newCount > 0 && (
            <button type="button" className={newPill} data-testid="tray-new-pill" onClick={mergeNew}>
              {newCount} new
            </button>
          )}
          {traySegment !== "background" && historyLastUpdatedAt !== null && (
            <p className={asOfLine} data-testid="tray-as-of">
              As of {formatClockTime(historyLastUpdatedAt)}
            </p>
          )}
        </div>

        {/* Notification List */}
        <div
          ref={scrollRef}
          className={content}
          data-testid="tray-scroll"
          id="tray-segment-panel"
          role="tabpanel"
          aria-labelledby={`tray-tab-${traySegment}`}
          {...scrollSettle}
          onScroll={(e) => {
            scrollSettle.onScroll();
            setScrolled((e.currentTarget as HTMLDivElement).scrollTop > 0);
          }}
        >
          <div ref={headRef} data-testid="tray-scroll-head" hidden={traySegment === "background"}>
            {cardsBlock}
            {filterBlock}
          </div>
          {traySegment === "background" ? (
            <>
            <BackgroundActivity
              rows={background.rows}
              rollup={background.rollup}
              loading={background.poll.loading}
              failed={background.poll.failed}
              offline={isOffline}
              lastUpdatedAt={background.poll.lastUpdatedAt}
              onRefresh={background.poll.refresh}
              onOpenRow={openBackgroundRow}
              onReviewed={markBackgroundReviewed}
              onNewFailures={(n) => announce(`${n} background ${n === 1 ? "session needs" : "sessions need"} attention`, "polite", "background-failure")}
              onMergeNew={(n) => announce(`${n} new background ${n === 1 ? "session" : "sessions"}`, "polite", "background-new")}
              replyEnabled={replyEnabled}
              failureStripSlot={slotEl}
              legacy={background.feedEnabled ? undefined : { hasHiddenSessions: background.hasHiddenSessions }}
            />
            <ScheduledPromptsSection active={isPanelOpen && !isOffline} />
            </>
          ) : (
            renderBody()
          )}
        </div>

        {traySegment === "notifications" && <SavedForLaterSection rows={savedAnswers} sessionNames={sessionNames} onNavigate={close} />}

        {/* Auto-handled section — collapsible, always below main list */}
        <AutoHandledSection
          notifications={autoHandledNotifications}
          isOpen={autoHandledOpen}
          onToggle={() => setAutoHandledOpen((v) => !v)}
        />

        <div className={trayFooter}>
          <Link href={routes.notifications} onClick={close}>
            Review all notifications
          </Link>
        </div>
      </div>
    </ActionsDisabledReasonContext.Provider>
    </EngagementContext.Provider>
  );
}

const isNeedsYou = (n: NotificationHistoryItem | undefined) => n?.isPendingDecision === true;
