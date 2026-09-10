"use client";
// +feature: backlog:list-page

import { useState, useEffect, useLayoutEffect, useCallback, useMemo, useRef, Suspense } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { resizeHandle as resizeHandleCss } from "@/styles/pane/resizeHandle.css";
import { useRouter, useSearchParams } from "next/navigation";
import { useAnalytics } from "@/lib/analytics";
import { usePageView } from "@/lib/analytics/usePageView";
import { AppLink } from "@/components/ui/AppLink";
import { BacklogItemDetail } from "@/components/backlog/BacklogItemDetail";
import { BacklogItemForm } from "@/components/backlog/BacklogItemForm";
import { BacklogEmptyState, FilterZeroState, FooterNudge } from "@/components/backlog/BacklogEmptyState";
import { VaguenessPromptModal } from "@/components/backlog/VaguenessPromptModal";
import { BacklogTourModal } from "@/components/backlog/BacklogTourModal";
import { useBacklogTour } from "@/components/backlog/useBacklogTour";
import { GitHubIssuePicker } from "@/components/backlog/GitHubIssuePicker";
import { ConnectionIndicator } from "@/components/backlog/ConnectionIndicator";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import {
  useBacklogService,
  type BacklogItem,
  type BacklogItemStatus,
  type BacklogItemInput,
  type GitHubIssue,
} from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { usePersistedViewState, type PersistedFieldsConfig } from "@/lib/hooks/usePersistedViewState";
import { useAppDispatch } from "@/lib/store";
import { upsertItem } from "@/lib/store/backlogItemsSlice";
import { getStatusLabel } from "@/lib/backlog/status";
import { compareByRepoPath, groupByRepoPath } from "@/lib/backlog/sortGroup";
import { ALL_STATUSES, BACKLOG_FILTER_FIELDS, filterBacklogItems } from "@/lib/hooks/useBacklogFilters";
import { BacklogFilterBar } from "@/components/backlog/BacklogFilterBar";
import * as styles from "./backlog.css";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SortColumn = "title" | "status" | "priority" | "updatedAt" | "repoPath";
type GroupBy = "none" | "repoPath";

const SORT_COLUMNS: SortColumn[] = ["title", "status", "priority", "updatedAt", "repoPath"];
const GROUP_BY_VALUES: GroupBy[] = ["none", "repoPath"];

interface BacklogViewState {
  search: string;
  statusFilter: BacklogItemStatus[];
  priorityFilter: number[];
  showArchived: boolean;
  sortCol: SortColumn;
  sortAsc: boolean;
  groupBy: GroupBy;
}

// Persisted under stapler-squad-backlog-* keys (see usePersistedViewState) —
// namespaced separately from SessionList's stapler-squad-* keys. The
// search/statusFilter/priorityFilter/showArchived fields are spread in from
// BACKLOG_FILTER_FIELDS (useBacklogFilters.ts) — the single source of truth
// shared with the board view — so the two views can't silently diverge on
// keys/validators. sortCol/sortAsc/groupBy are list-view-only extras.
const BACKLOG_VIEW_FIELDS: PersistedFieldsConfig<BacklogViewState> = {
  ...BACKLOG_FILTER_FIELDS,
  sortCol: {
    key: "stapler-squad-backlog-sort-col",
    defaultValue: "updatedAt",
    isValid: (v) => SORT_COLUMNS.includes(v as SortColumn),
  },
  sortAsc: {
    key: "stapler-squad-backlog-sort-asc",
    defaultValue: false,
    isValid: (v) => typeof v === "boolean",
  },
  groupBy: {
    key: "stapler-squad-backlog-group-by",
    defaultValue: "none",
    isValid: (v) => GROUP_BY_VALUES.includes(v as GroupBy),
  },
};

const STATUS_CSS: Record<string, string> = {
  idea: styles.statusIdea,
  refining: styles.statusRefining,
  ready: styles.statusReady,
  queued: styles.statusQueued,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  pr_pending: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const getStatusClass = (s: string): string => STATUS_CSS[s] ?? styles.statusArchived;

// Epic 6.3 (backlog-event-driven-updates): how long a row's fade-out plays
// before it's removed from the DOM after dropping out of the active filter
// (ux.md §7 — "~200ms"). Under `prefers-reduced-motion: reduce` this is
// bypassed to 0ms (instant removal) at the call site.
const EXIT_TRANSITION_MS = 200;

const PRIORITY_LABELS: Record<number, string> = {
  1: "P1",
  2: "P2",
  3: "P3",
  4: "P4",
  5: "P5",
};

function formatDateShort(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

// Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep,
// 2026-07-22): ux.md UX AC #6 requires the list row itself to play "a
// background flash that fades within ~1 second" on a genuine live update —
// the page previously only tracked `item.liveVersion` for the Epic 6.3 exit
// transition, never applying the Epic 6.1 in-place flash BacklogItemCard.tsx
// (used by the Kanban board) already has. Extracted into its own component,
// mirroring BacklogItemCard's exact flash-tracking hook, since a plain row
// function has no hook of its own to track the previous liveVersion.
function BacklogTableRow({
  item,
  isActive,
  isExiting,
  onRowClick,
}: {
  item: BacklogItem;
  isActive: boolean;
  isExiting: boolean;
  onRowClick: (itemId: string) => void;
}) {
  const acDone = item.acCriteria.filter((c) => c.status === "done").length;

  const prevLiveVersionRef = useRef(item.liveVersion);
  const [justChanged, setJustChanged] = useState(false);

  useEffect(() => {
    const prev = prevLiveVersionRef.current;
    const next = item.liveVersion;
    prevLiveVersionRef.current = next;
    if (prev === undefined || next === undefined || next === prev) return;

    setJustChanged(true);
    const timer = setTimeout(() => setJustChanged(false), 250);
    return () => clearTimeout(timer);
  }, [item.liveVersion]);

  return (
    <tr
      className={`${styles.tableRow} ${isActive ? styles.tableRowActive : ""} ${isExiting ? styles.tableRowExiting : ""} ${justChanged ? styles.tableRowJustChanged : ""}`}
      tabIndex={isExiting ? -1 : 0}
      role="row"
      aria-selected={isActive}
      aria-hidden={isExiting || undefined}
      data-testid="backlog-table-row"
      data-item-id={item.id}
      data-exiting={isExiting ? "true" : undefined}
      onClick={() => {
        if (isExiting) return;
        onRowClick(item.id);
      }}
      onKeyDown={(e) => {
        if (isExiting) return;
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onRowClick(item.id);
        }
      }}
    >
      <td className={`${styles.tableCell} ${styles.titleCell}`}>
        {item.title}
      </td>
      <td className={styles.tableCell}>
        <span
          className={`${styles.statusBadge} ${getStatusClass(item.status)}`}
          aria-label={`Status: ${getStatusLabel(item.status)}`}
          data-testid={item.status === "queued" ? "backlog-status-queued" : undefined}
        >
          {getStatusLabel(item.status)}
        </span>
      </td>
      <td className={styles.tableCell}>
        <span
          className={styles.priorityBadge}
          data-testid="priority-badge"
          aria-label={`Priority: ${PRIORITY_LABELS[item.priority] ?? "Unknown"}`}
        >
          {PRIORITY_LABELS[item.priority] ?? "P?"}
        </span>
      </td>
      <td className={`${styles.tableCell} ${styles.acProgressCell}`}>
        {item.acCriteria.length > 0
          ? `${acDone}/${item.acCriteria.length}`
          : "—"}
      </td>
      <td className={styles.tableCell} style={{ whiteSpace: "nowrap" }}>
        {formatDateShort(item.updatedAt)}
      </td>
      <td className={`${styles.tableCell} ${styles.repoPathCell}`} data-testid="backlog-repo-path-cell">
        {item.repoPath || "—"}
      </td>
    </tr>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

function BacklogPageInner() {
  usePageView();
  const { track } = useAnalytics();
  const { createBacklogItem, importGitHubIssue, triggerTriage } = useBacklogService();
  const router = useRouter();
  const searchParams = useSearchParams();
  const dispatch = useAppDispatch();

  const selectedItemId = searchParams.get("item");

  // Epic 5.1 (backlog-event-driven-updates): live-updating list, replacing
  // the former fetch-once-on-mount pattern. useWatchBacklogItems streams
  // every backlog item unfiltered — per design/ux.md Surface 1's interaction
  // flow ("subscribes ... unfiltered ... then filters client-side same as
  // today") — so status/priority/search/archived filtering below is purely
  // client-side and never tears down/reconnects the stream when a filter
  // chip changes. The hook already returns domain-shaped BacklogItem[]
  // (mapped internally, see useWatchBacklogItems.ts's file-header note 3),
  // so no further proto->domain mapping is needed here.
  const { items, connectionState } = useWatchBacklogItems();
  // Only the very first connect (before any items have ever loaded) shows a
  // loading state — a reconnect must keep showing last-known data, not blank
  // or spinner out (design/ux.md Surface 1 "Error / edge cases").
  const loading = connectionState === "connecting" && items.length === 0;

  // Filters, sort, and grouping — persisted across page loads under
  // stapler-squad-backlog-* keys (see usePersistedViewState).
  const backlogViewState = usePersistedViewState<BacklogViewState>(BACKLOG_VIEW_FIELDS);
  const {
    search,
    statusFilter,
    priorityFilter,
    // showArchived: archived items are excluded client-side by default (Epic
    // 5.1 — see filteredItems below); enabling this reveals them from the
    // already-loaded live store. Mirrors SessionList's "Show Archived" toggle.
    showArchived,
    sortCol,
    sortAsc,
    groupBy,
  } = backlogViewState.state;
  const {
    search: setSearch,
    statusFilter: setStatusFilter,
    priorityFilter: setPriorityFilter,
    showArchived: setShowArchived,
    sortCol: setSortCol,
    sortAsc: setSortAsc,
    groupBy: setGroupBy,
  } = backlogViewState.setters;
  const resetViewState = backlogViewState.resetToDefaults;

  // Detail pane resize
  const [detailWidth, setDetailWidth] = useState(420);
  const dragRef = useRef({ active: false, startX: 0, startWidth: 0 });

  const handleResizePointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    (e.currentTarget as HTMLDivElement).setPointerCapture(e.pointerId);
    dragRef.current = { active: true, startX: e.clientX, startWidth: detailWidth };
  }, [detailWidth]);

  const handleResizePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current.active) return;
    const delta = dragRef.current.startX - e.clientX;
    setDetailWidth(Math.max(240, Math.min(800, dragRef.current.startWidth + delta)));
  }, []);

  const handleResizePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    dragRef.current.active = false;
    (e.currentTarget as HTMLDivElement).releasePointerCapture(e.pointerId);
  }, []);

  // New-item modal
  const [showForm, setShowForm] = useState(false);
  const [formMode, setFormMode] = useState<"manual" | "github">("manual");
  const [githubIssueUrl, setGithubIssueUrl] = useState("");
  const [githubImporting, setGithubImporting] = useState(false);
  const [githubImportError, setGithubImportError] = useState<string | null>(null);
  const [githubImportProgress, setGithubImportProgress] = useState<{ done: number; total: number } | null>(null);

  // First-visit walkthrough
  const { showTour, setTourComplete, hideTour, resetTour } = useBacklogTour();

  // Vagueness prompt modal state
  const [vaguenessItem, setVaguenessItem] = useState<BacklogItem | null>(null);

  // Raw (unmapped) ConnectRPC client — used only to hydrate a just-created
  // item into the shared backlogItemsSlice store immediately. The
  // WatchBacklogItems event stream (Phases 1-3) has no "item created" oneof
  // variant (see plan.md's BacklogItemEvent oneof: status_changed /
  // verdict_recorded / session_attached / item_updated / item_archived /
  // item_removed only), so a freshly created/imported item would not
  // otherwise appear in this live list until some other event touches it.
  // This is a known gap in the event proto's scope, not something Epic 5.1
  // can fix on its own — flagged here rather than silently worked around by
  // re-adding a full listBacklogItems fetch-on-every-mutation path.
  const rawClientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    rawClientRef.current = createClient(BacklogService, transport);
  }, []);

  const hydrateItemIntoStore = useCallback(
    async (itemId: string) => {
      if (!rawClientRef.current) return;
      try {
        const resp = await rawClientRef.current.getBacklogItem({ itemId });
        if (resp.item) dispatch(upsertItem(resp.item));
      } catch (err) {
        console.error("[BacklogPage] failed to hydrate newly created item into the live store:", err);
      }
    },
    [dispatch]
  );

  // Epic 5.1: client-side filtering (search/status/priority/archived),
  // mirroring what the server-side listBacklogItems filter used to do before
  // this page moved to the shared live stream (design/ux.md Surface 1).
  const filteredItems = useMemo(
    () => filterBacklogItems(items, { search, statusFilter, priorityFilter, showArchived }),
    [items, search, statusFilter, priorityFilter, showArchived],
  );

  // Epic 6.3 (backlog-event-driven-updates): when an item's fields change
  // such that it no longer matches the active filter, keep rendering it
  // briefly with a fade-out instead of letting it vanish in the same render
  // the filter re-evaluates (ux.md §7 "reads as moved, not vanished"). Only
  // genuinely live, one-at-a-time departures animate — gated on
  // `item.liveVersion` advancing (the same signal BacklogItemCard's flash
  // uses), so a bulk resnapshot on reconnect (liveVersion never advances for
  // a snapshot-flagged event, per backlogItemsSlice.ts) or the user simply
  // toggling a filter chip (no liveVersion change at all) both fall through
  // to an ordinary instant removal, matching ux.md's edge cases.
  const [exitingItems, setExitingItems] = useState<Map<string, BacklogItem>>(new Map());
  const exitingMapRef = useRef<Map<string, BacklogItem>>(new Map());
  const prevMatchedIdsRef = useRef<Set<string> | null>(null);
  const prevLiveVersionsRef = useRef<Map<string, number | undefined>>(new Map());
  const exitTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const reducedMotionRef = useRef(false);

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    reducedMotionRef.current = mq.matches;
    const onChange = () => {
      reducedMotionRef.current = mq.matches;
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  // useLayoutEffect (not useEffect): runs before the browser paints, so a
  // departing row is re-added to the exiting set within the same commit it
  // was excluded from `filteredItems` — no visible blank frame in between.
  useLayoutEffect(() => {
    const currentMatchedIds = new Set(filteredItems.map((i) => i.id));
    const prevMatchedIds = prevMatchedIdsRef.current;
    const itemsById = new Map(items.map((i) => [i.id, i]));
    const exitingMap = exitingMapRef.current;
    let changed = false;

    if (prevMatchedIds) {
      // Flap protection: if a pending exit's item re-matches the filter
      // before its timer fires, cancel the exit and let it settle back to a
      // normal in-place row (ux.md §7 "Error / edge cases").
      for (const id of Array.from(exitingMap.keys())) {
        if (currentMatchedIds.has(id)) {
          const timer = exitTimersRef.current.get(id);
          if (timer) clearTimeout(timer);
          exitTimersRef.current.delete(id);
          exitingMap.delete(id);
          changed = true;
        }
      }

      for (const id of prevMatchedIds) {
        if (currentMatchedIds.has(id) || exitingMap.has(id)) continue;
        const fullItem = itemsById.get(id);
        if (!fullItem) continue; // item removed entirely -- not a filter departure

        const prevVersion = prevLiveVersionsRef.current.get(id);
        const isGenuineLiveChange =
          fullItem.liveVersion !== undefined && fullItem.liveVersion !== prevVersion;
        if (!isGenuineLiveChange) continue; // bulk resnapshot / manual filter change -> instant

        exitingMap.set(id, fullItem);
        changed = true;
        const duration = reducedMotionRef.current ? 0 : EXIT_TRANSITION_MS;
        const timer = setTimeout(() => {
          if (exitingMapRef.current.delete(id)) {
            setExitingItems(new Map(exitingMapRef.current));
          }
          exitTimersRef.current.delete(id);
        }, duration);
        exitTimersRef.current.set(id, timer);
      }
    }

    prevMatchedIdsRef.current = currentMatchedIds;
    for (const item of items) {
      prevLiveVersionsRef.current.set(item.id, item.liveVersion);
    }
    if (changed) setExitingItems(new Map(exitingMap));
  }, [filteredItems, items]);

  // Clear any in-flight exit timers on unmount.
  useEffect(() => {
    return () => {
      for (const timer of exitTimersRef.current.values()) clearTimeout(timer);
    };
  }, []);

  // Re-merge still-fading items into the visible set so they keep sorting
  // into a natural position instead of jumping to the end of the list.
  const visibleItems = useMemo(() => {
    if (exitingItems.size === 0) return filteredItems;
    const presentIds = new Set(filteredItems.map((i) => i.id));
    const extra = Array.from(exitingItems.values()).filter((i) => !presentIds.has(i.id));
    return extra.length === 0 ? filteredItems : [...filteredItems, ...extra];
  }, [filteredItems, exitingItems]);

  // Sort items client-side
  const sortedItems = [...visibleItems].sort((a, b) => {
    let cmp = 0;
    if (sortCol === "title") {
      cmp = a.title.localeCompare(b.title);
    } else if (sortCol === "status") {
      cmp = ALL_STATUSES.indexOf(a.status) - ALL_STATUSES.indexOf(b.status);
    } else if (sortCol === "priority") {
      cmp = a.priority - b.priority;
    } else if (sortCol === "updatedAt") {
      cmp = (a.updatedAt ?? "").localeCompare(b.updatedAt ?? "");
    } else if (sortCol === "repoPath") {
      cmp = compareByRepoPath(a, b);
    }
    return sortAsc ? cmp : -cmp;
  });

  // Grouping is applied after sorting so items within each group keep the
  // currently selected sort order (mirrors SessionList's sort-then-group).
  const groups = groupBy === "repoPath" ? groupByRepoPath(sortedItems) : null;

  const handleSortClick = (col: SortColumn) => {
    if (sortCol === col) {
      setSortAsc((prev) => !prev);
    } else {
      setSortCol(col);
      setSortAsc(false);
    }
  };

  const handleHeaderKeyDown = (e: React.KeyboardEvent, col: SortColumn) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleSortClick(col);
    }
  };

  const handleRowClick = (itemId: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("item", itemId);
    router.push(`/backlog?${params.toString()}`);
  };

  const handleDetailClose = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("item");
    const qs = params.toString();
    router.push(qs ? `/backlog?${qs}` : "/backlog");
  };

  const handleCreateItem = useCallback(
    async (data: BacklogItemInput) => {
      const result = await createBacklogItem(data);
      setShowForm(false);
      if (!result) return;
      await hydrateItemIntoStore(result.item.id);
      // Show vagueness prompt if item was created with skip_triage=true
      if (data.skipTriage) {
        setVaguenessItem(result.item);
        // Navigate to the new item
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", result.item.id);
        router.push(`/backlog?${params.toString()}`);
      } else {
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", result.item.id);
        router.push(`/backlog?${params.toString()}`);
      }
    },
    [createBacklogItem, hydrateItemIntoStore, router, searchParams]
  );

  const handleImportGitHubIssue = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!githubIssueUrl.trim()) return;
      setGithubImportError(null);
      setGithubImporting(true);
      try {
        const result = await importGitHubIssue(githubIssueUrl.trim());
        if (result) {
          setShowForm(false);
          setGithubIssueUrl("");
          await hydrateItemIntoStore(result.item.id);
          const params = new URLSearchParams(searchParams.toString());
          params.set("item", result.item.id);
          router.push(`/backlog?${params.toString()}`);
        } else {
          setGithubImportError("Import failed. Check the URL and try again.");
        }
      } catch (err) {
        setGithubImportError(err instanceof Error ? err.message : "Import failed.");
      } finally {
        setGithubImporting(false);
      }
    },
    [githubIssueUrl, importGitHubIssue, hydrateItemIntoStore, router, searchParams]
  );

  const handlePickerSelect = useCallback(
    async (owner: string, repo: string, issues: GitHubIssue[]) => {
      setGithubImportError(null);
      setGithubImporting(true);
      setGithubImportProgress({ done: 0, total: issues.length });
      const createdIds: string[] = [];
      let failures = 0;
      let duplicates = 0;
      try {
        for (const issue of issues) {
          const url = issue.url || `https://github.com/${owner}/${repo}/issues/${issue.number}`;
          const result = await importGitHubIssue(url.trim());
          if (result) {
            await hydrateItemIntoStore(result.item.id);
            createdIds.push(result.item.id);
            if (result.alreadyExisted) duplicates++;
          } else {
            failures++;
          }
          setGithubImportProgress((prev) => (prev ? { ...prev, done: prev.done + 1 } : prev));
        }
      } finally {
        setGithubImporting(false);
        setGithubImportProgress(null);
      }
      if (failures > 0) {
        // Leave the modal open (don't setShowForm(false) below) so this error
        // is actually visible — closing the form first would unmount it
        // before the message could ever render.
        setGithubImportError(
          issues.length === 1
            ? "Import failed. Check that this is a real issue, not a pull request, and try again."
            : `Imported ${createdIds.length} of ${issues.length} — ${failures} failed. Pull requests can't be imported as backlog items.`
        );
        return;
      }
      if (duplicates > 0) {
        // Same reasoning as the failures branch above: leave the modal open
        // so this message is actually visible.
        setGithubImportError(
          duplicates === issues.length
            ? "Already imported — no new items created."
            : `Imported ${createdIds.length - duplicates} new item${createdIds.length - duplicates === 1 ? "" : "s"}; ${duplicates} already imported.`
        );
        return;
      }
      setShowForm(false);
      // Only navigate to the item detail when a single issue was imported —
      // with multiple, there's no single item to land on.
      if (createdIds.length === 1) {
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", createdIds[0]);
        router.push(`/backlog?${params.toString()}`);
      }
    },
    [importGitHubIssue, hydrateItemIntoStore, router, searchParams]
  );

  const sortIndicator = (col: SortColumn) => {
    if (sortCol !== col) return null;
    return sortAsc ? " ↑" : " ↓";
  };

  const renderItemRow = (item: BacklogItem) => {
    const isActive = selectedItemId === item.id;
    const isExiting = exitingItems.has(item.id);
    return (
      <BacklogTableRow
        key={item.id}
        item={item}
        isActive={isActive}
        isExiting={isExiting}
        onRowClick={handleRowClick}
      />
    );
  };

  return (
    <div className={styles.pageWrapper} data-testid="backlog-page">
      {/* Header */}
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>Backlog</h1>
        <div className={styles.headerActions}>
          <ConnectionIndicator connectionState={connectionState} />
          <button
            className={styles.helpButton}
            onClick={() => { track({ name: "backlog_open_tour", category: "user_action", component: "BacklogPage" }); resetTour(); }}
            aria-label="How this page works"
            data-testid="backlog-tour-button"
          >
            ?
          </button>
          <button
            className={styles.newItemButton}
            onClick={() => { track({ name: "backlog_new_item", category: "user_action", component: "BacklogPage" }); setFormMode("manual"); setShowForm(true); }}
            aria-label="Create new backlog item"
            data-testid="backlog-new-item-button"
          >
            + New Item
          </button>
        </div>
      </div>

      {/* Tab Bar */}
      <nav className={styles.tabBar} aria-label="Backlog views">
        <button
          type="button"
          className={`${styles.tab} ${styles.tabActive}`}
          aria-current="page"
          data-testid="backlog-tab-list"
        >
          List
        </button>
        <AppLink
          href="/backlog/board"
          className={styles.tab}
          data-testid="backlog-tab-board"
        >
          Board
        </AppLink>
      </nav>

      {/* Filter Bar */}
      <BacklogFilterBar
        search={search}
        onSearchChange={setSearch}
        statusFilter={statusFilter}
        onStatusFilterChange={setStatusFilter}
        priorityFilter={priorityFilter}
        onPriorityFilterChange={setPriorityFilter}
        showArchived={showArchived}
        onShowArchivedChange={setShowArchived}
        onResetView={resetViewState}
        showSortGroupControls={true}
        groupBy={groupBy}
        onGroupByChange={setGroupBy}
      />

      {/* Content */}
      <div className={styles.contentArea}>
        <div className={styles.listPane}>
          {loading ? (
            <div role="status" aria-label="Loading backlog items" style={{ padding: "32px", textAlign: "center", color: "inherit", opacity: 0.6 }}>
              Loading…
            </div>
          ) : sortedItems.length === 0 && items.length === 0 ? (
            <BacklogEmptyState onCreateItem={() => { setFormMode("manual"); setShowForm(true); }} />
          ) : sortedItems.length === 0 ? (
            <FilterZeroState onClearFilters={resetViewState} />
          ) : (
            <table className={styles.table} aria-label="Backlog items">
              <thead className={styles.tableHead}>
                <tr>
                  <th
                    scope="col"
                    className={`${styles.tableHeaderCell} ${styles.tableHeaderCellSortable}`}
                    aria-sort={sortCol === "title" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    <button
                      type="button"
                      className={styles.tableHeaderSortButton}
                      onClick={() => handleSortClick("title")}
                      onKeyDown={(e) => handleHeaderKeyDown(e, "title")}
                    >
                      Title{sortIndicator("title")}
                    </button>
                  </th>
                  <th
                    scope="col"
                    className={`${styles.tableHeaderCell} ${styles.tableHeaderCellSortable}`}
                    aria-sort={sortCol === "status" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    <button
                      type="button"
                      className={styles.tableHeaderSortButton}
                      onClick={() => handleSortClick("status")}
                      onKeyDown={(e) => handleHeaderKeyDown(e, "status")}
                    >
                      Status{sortIndicator("status")}
                    </button>
                  </th>
                  <th
                    scope="col"
                    className={`${styles.tableHeaderCell} ${styles.tableHeaderCellSortable}`}
                    aria-sort={sortCol === "priority" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    <button
                      type="button"
                      className={styles.tableHeaderSortButton}
                      onClick={() => handleSortClick("priority")}
                      onKeyDown={(e) => handleHeaderKeyDown(e, "priority")}
                    >
                      Priority{sortIndicator("priority")}
                    </button>
                  </th>
                  <th scope="col" className={styles.tableHeaderCell}>
                    AC
                  </th>
                  <th
                    scope="col"
                    className={`${styles.tableHeaderCell} ${styles.tableHeaderCellSortable}`}
                    aria-sort={sortCol === "updatedAt" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    <button
                      type="button"
                      className={styles.tableHeaderSortButton}
                      onClick={() => handleSortClick("updatedAt")}
                      onKeyDown={(e) => handleHeaderKeyDown(e, "updatedAt")}
                    >
                      Updated{sortIndicator("updatedAt")}
                    </button>
                  </th>
                  <th
                    scope="col"
                    className={`${styles.tableHeaderCell} ${styles.tableHeaderCellSortable}`}
                    aria-sort={sortCol === "repoPath" ? (sortAsc ? "ascending" : "descending") : "none"}
                    data-testid="backlog-col-repo-path"
                  >
                    <button
                      type="button"
                      className={styles.tableHeaderSortButton}
                      onClick={() => handleSortClick("repoPath")}
                      onKeyDown={(e) => handleHeaderKeyDown(e, "repoPath")}
                    >
                      Repository{sortIndicator("repoPath")}
                    </button>
                  </th>
                </tr>
              </thead>
              {groups ? (
                groups.map((group) => (
                  <tbody key={group.groupKey}>
                    <tr>
                      <td
                        colSpan={6}
                        className={styles.groupHeaderCell}
                        data-testid="backlog-group-header"
                      >
                        {group.displayName} ({group.items.length})
                      </td>
                    </tr>
                    {group.items.map((item) => renderItemRow(item))}
                  </tbody>
                ))
              ) : (
                <tbody>{sortedItems.map((item) => renderItemRow(item))}</tbody>
              )}
            </table>
          )}
          {filteredItems.length > 0 && !filteredItems.some((i) => i.status === "in_progress") && <FooterNudge />}
        </div>

        {/* Detail pane */}
        {selectedItemId && (
          <>
            <div
              className={resizeHandleCss({ direction: "vertical" })}
              style={{ touchAction: "none" }}
              aria-label="Resize detail panel"
              onPointerDown={handleResizePointerDown}
              onPointerMove={handleResizePointerMove}
              onPointerUp={handleResizePointerUp}
              onPointerCancel={handleResizePointerUp}
            />
            <aside
              className={styles.detailPane}
              style={{ width: detailWidth }}
              aria-label="Item detail"
            >
              <BacklogItemDetail
                key={selectedItemId}
                itemId={selectedItemId}
                onClose={handleDetailClose}
              />
            </aside>
          </>
        )}
      </div>

      {/* New Item Modal */}
      {showForm && (
        <div
          className={styles.modalOverlay}
          role="dialog"
          aria-modal="true"
          aria-label="Create new backlog item"
          onClick={(e) => {
            if (e.target === e.currentTarget) { setShowForm(false); setGithubIssueUrl(""); setGithubImportError(null); }
          }}
          data-testid="backlog-form-modal"
        >
          <div className={styles.modalBox} onClick={(e) => e.stopPropagation()}>
            <h2 className={styles.modalTitle}>New Backlog Item</h2>
            {/* Mode toggle */}
            <div style={{ display: "flex", gap: "8px", marginBottom: "16px" }}>
              <button
                type="button"
                onClick={() => setFormMode("manual")}
                style={{
                  padding: "6px 16px",
                  borderRadius: "6px",
                  border: "1px solid",
                  cursor: "pointer",
                  fontSize: "13px",
                  fontWeight: formMode === "manual" ? 600 : 400,
                  background: formMode === "manual" ? "var(--primary)" : "transparent",
                  color: formMode === "manual" ? "var(--primary-text)" : "var(--text-secondary)",
                  borderColor: formMode === "manual" ? "var(--primary)" : "var(--border-color)",
                }}
              >
                Manual
              </button>
              <button
                type="button"
                onClick={() => setFormMode("github")}
                style={{
                  padding: "6px 16px",
                  borderRadius: "6px",
                  border: "1px solid",
                  cursor: "pointer",
                  fontSize: "13px",
                  fontWeight: formMode === "github" ? 600 : 400,
                  background: formMode === "github" ? "var(--primary)" : "transparent",
                  color: formMode === "github" ? "var(--primary-text)" : "var(--text-secondary)",
                  borderColor: formMode === "github" ? "var(--primary)" : "var(--border-color)",
                }}
              >
                Import from GitHub Issue
              </button>
            </div>

            {formMode === "manual" ? (
              <BacklogItemForm
                onSubmit={handleCreateItem}
                onCancel={() => setShowForm(false)}
              />
            ) : (
              <>
                <GitHubIssuePicker
                  onSelect={handlePickerSelect}
                  onCancel={() => { setShowForm(false); setGithubIssueUrl(""); setGithubImportError(null); }}
                  importing={githubImporting}
                  importProgress={githubImportProgress}
                />
                {githubImportError && (
                  <p style={{ fontSize: "12px", color: "var(--error)", margin: "8px 0 0" }}>
                    {githubImportError}
                  </p>
                )}
              </>
            )}

            {false && (
              <form onSubmit={handleImportGitHubIssue} style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                  <input
                    type="url"
                    value={githubIssueUrl}
                    onChange={(e) => setGithubIssueUrl(e.target.value)}
                    placeholder="https://github.com/owner/repo/issues/123"
                    required
                  />
                </div>
                {githubImportError && (
                  <p style={{ fontSize: "12px", color: "var(--error)", margin: 0 }}>{githubImportError}</p>
                )}
                <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
                  <button
                    type="submit"
                    disabled={githubImporting || !githubIssueUrl.trim()}
                  >
                    {githubImporting ? "Importing…" : "Import Issue"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* First-visit walkthrough */}
      <BacklogTourModal
        isOpen={showTour}
        onComplete={(persist) => (persist ? setTourComplete() : hideTour())}
      />

      {/* Vagueness Prompt Modal */}
      {vaguenessItem && (
        <VaguenessPromptModal
          itemTitle={vaguenessItem.title}
          onRefine={() => {
            setVaguenessItem(null);
            setFormMode("manual");
            setShowForm(true);
          }}
          onProceed={() => {
            const item = vaguenessItem;
            setVaguenessItem(null);
            void triggerTriage(item.id);
          }}
        />
      )}
    </div>
  );
}

export default function BacklogPage() {
  return (
    <Suspense>
      <BacklogPageInner />
    </Suspense>
  );
}
