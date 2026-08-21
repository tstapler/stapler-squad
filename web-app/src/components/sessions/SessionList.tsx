"use client";

import React, { useState, useMemo, useEffect, useCallback, useRef } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { GroupedVirtuoso } from "react-virtuoso";
import { createClient } from "@connectrpc/connect";
import { SessionService, Project } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { AppLink } from "@/components/ui/AppLink";
import { Session, SessionStatus, SubStatus, CheckpointProto } from "@/gen/session/v1/types_pb";
import { SessionCard } from "./SessionCard";
import { SessionRow } from "./SessionRow";
import { SessionListEmptyState } from "./SessionListEmptyState";
import { SessionListSkeleton } from "./SessionListSkeleton";
import { BulkActions } from "./BulkActions";
import { TagEditor } from "./TagEditor";
import { GroupingStrategy, GroupingStrategyLabels, groupSessions, cycleGroupingStrategy } from "@/lib/grouping/strategies";
import { ColumnKey, DEFAULT_VISIBLE_COLUMNS } from "./session-columns";
import { usePersistedViewState, type PersistedFieldsConfig } from "@/lib/hooks/usePersistedViewState";
import { useStaleSessionConfig } from "@/lib/hooks/useStaleSessionConfig";
import { ColumnPicker } from "./ColumnPicker";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { MemoryPressureCallout } from "./MemoryPressureCallout";
import { useAppSelector } from "@/lib/store";
import { selectDetectedStatusMap } from "@/lib/store/sessionsSlice";
import { ActionBar } from "@/components/ui/ActionBar";
import { computeRangeIds } from "@/lib/utils/rangeSelect";
import { useInsightsSummary } from "@/lib/hooks/useInsightsService";
import { compareSessionsByCost } from "./sessionCostSort";
import {
  container,
  header,
  headerTop,
  title,
  headerActions,
  selectModeButton,
  selectModeButtonActive,
  filters,
  filterTopRow,
  filterToggle,
  filterToggleActive,
  filterActiveDot,
  filterControls,
  filterControlsOpen,
  searchInput,
  select,
  sortDirButton,
  checkboxLabel,
  categoryTitle,
  collapseToggle,
  empty,
  clearButton,
  newSessionHeaderButton,
} from "./SessionList.css";

interface SessionListProps {
  sessions: Session[];
  onSessionClick?: (session: Session) => void;
  onSessionOpenInNewPane?: (session: Session) => void;
  onDeleteSession?: (sessionId: string) => Promise<void> | void;
  onPauseSession?: (sessionId: string) => void;
  onResumeSession?: (session: Session) => void;
  /** Called for bulk resume to skip the confirmation modal and resume immediately. */
  onDirectResumeSession?: (session: Session) => void;
  onCloneSession?: (sessionId: string) => void;
  onNewWorkspaceSession?: (sessionId: string) => void;
  onRenameSession?: (sessionId: string, newTitle: string) => Promise<boolean>;
  onRestartSession?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onNewSession?: () => void;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onListCheckpoints?: (sessionId: string) => Promise<CheckpointProto[]>;
  onForkFromCheckpoint?: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onToggleAutonomousMode?: (sessionId: string, enabled: boolean) => void;
  onToggleAutoApprove?: (sessionId: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (sessionId: string, message: string) => void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onHibernateSession?: (sessionId: string) => void;
  onResumeHibernatedSession?: (sessionId: string) => void;
  /**
   * Called when the "Show archived" toggle changes to true, so the parent can
   * re-fetch sessions with includeArchived via the session service. Turning the
   * toggle off does not call this — archived sessions are filtered client-side
   * instead, since the server-side default already excludes them going forward.
   */
  onFetchArchivedSessions?: (includeArchived: boolean) => void;
  /** When true, renders the loading skeleton instead of the session list. */
  isLoading?: boolean;
  /** Prefix for localStorage keys, used when multiple instances are rendered (e.g. split view). */
  storageKeyPrefix?: string;
  /** Extra action buttons rendered in the header beside the "+" button. */
  extraHeaderActions?: React.ReactNode;
  /** Display mode: compact single-line rows ("row") or full cards ("card"). Default: "row". */
  viewMode?: "card" | "row";
}

type SortField = 'lastActivity' | 'name' | 'createdAt' | 'updatedAt' | 'tokenCost';
type SortDir = 'asc' | 'desc';

// Stable-callback prop types for SessionRowWrapper.
// Using (session: Session) / (id: string) shapes lets the parent pass
// one useCallback per action instead of one closure per row.
interface SessionRowHandlers {
  onSessionClick?: (session: Session) => void;
  onSessionOpenInNewPane?: (session: Session) => void;
  onDeleteSession?: (id: string) => Promise<void> | void;
  onPauseSession?: (id: string) => void;
  onResumeSession?: (session: Session) => void;
  onCloneSession?: (id: string) => void;
  onNewWorkspaceSession?: (id: string) => void;
  onRestartSession?: (id: string) => Promise<boolean | void>;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onSetRateLimitEnabled?: (id: string, enabled: boolean) => void;
  onToggleAutonomousMode?: (id: string, enabled: boolean) => void;
  onToggleAutoApprove?: (id: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (id: string, message: string) => void;
  onClearConversationState?: (id: string) => Promise<boolean>;
  onHibernateSession?: (id: string) => void;
  onResumeHibernatedSession?: (id: string) => void;
  onUpdateTags?: (id: string, tags: string[]) => void;
  onToggleSession?: (id: string, e: React.MouseEvent) => void;
}

interface SessionRowWrapperProps extends SessionRowHandlers {
  session: Session;
  visibleColumns: ColumnKey[];
  selectMode: boolean;
  isSelected: boolean;
  suppressApprovalSubStatus: boolean;
  staleThresholdMinutes: number;
}

// Memoized wrapper: turns stable per-action handlers into per-session closures
// only once per session identity change. The outer SessionList can pass one
// useCallback per action (e.g. onDeleteSession accepts an id), and this wrapper
// creates the () => onDeleteSession(session.id) closure just once.
const SessionRowWrapper = React.memo(function SessionRowWrapper({
  session,
  visibleColumns,
  selectMode,
  isSelected,
  suppressApprovalSubStatus,
  staleThresholdMinutes,
  onSessionClick,
  onSessionOpenInNewPane,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
  onCloneSession,
  onNewWorkspaceSession,
  onRestartSession,
  onCreateCheckpoint,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onToggleAutoApprove,
  onSteerAutonomousSession,
  onClearConversationState,
  onHibernateSession,
  onResumeHibernatedSession,
  onUpdateTags,
  onToggleSession,
}: SessionRowWrapperProps) {
  const id = session.id;
  return (
    <SessionRow
      session={session}
      onClick={onSessionClick ? () => onSessionClick(session) : undefined}
      onPause={onPauseSession ? () => onPauseSession(id) : undefined}
      onResume={onResumeSession ? () => onResumeSession(session) : undefined}
      onDelete={onDeleteSession ? () => onDeleteSession(id) : undefined}
      onClone={onCloneSession ? () => onCloneSession(id) : undefined}
      onOpenInNewPane={onSessionOpenInNewPane ? () => onSessionOpenInNewPane(session) : undefined}
      onNewWorkspace={onNewWorkspaceSession ? () => onNewWorkspaceSession(id) : undefined}
      onRestart={onRestartSession}
      onCreateCheckpoint={onCreateCheckpoint}
      onSetRateLimitEnabled={onSetRateLimitEnabled}
      onToggleAutonomousMode={onToggleAutonomousMode}
      onToggleAutoApprove={onToggleAutoApprove}
      onSteerAutonomousSession={onSteerAutonomousSession}
      onClearConversationState={onClearConversationState}
      onHibernate={onHibernateSession ? () => onHibernateSession(id) : undefined}
      onResumeFromHibernation={onResumeHibernatedSession ? () => onResumeHibernatedSession(id) : undefined}
      onUpdateTags={onUpdateTags}
      suppressApprovalSubStatus={suppressApprovalSubStatus}
      staleThresholdMinutes={staleThresholdMinutes}
      visibleColumns={visibleColumns}
      selectMode={selectMode}
      isSelected={isSelected}
      onToggleSelect={onToggleSession ? (e) => onToggleSession(id, e) : undefined}
    />
  );
});

// Toggle button rendered inside a group header to collapse/expand its sessions.
// Shared between row-mode and card-mode header render sites.
const CategoryCollapseToggle = React.memo(function CategoryCollapseToggle({
  groupKey,
  displayName,
  collapsed,
  onToggle,
}: {
  groupKey: string;
  displayName: string;
  collapsed: boolean;
  onToggle: (groupKey: string) => void;
}) {
  return (
    <button
      type="button"
      data-testid="category-collapse-toggle"
      className={collapseToggle}
      aria-expanded={!collapsed}
      aria-label={`${collapsed ? "Expand" : "Collapse"} ${displayName} category`}
      onClick={(e) => {
        e.stopPropagation();
        onToggle(groupKey);
      }}
    >
      {collapsed ? <ChevronRight size={16} aria-hidden="true" /> : <ChevronDown size={16} aria-hidden="true" />}
    </button>
  );
});

const BASE_STORAGE_KEYS = {
  SEARCH_QUERY: 'stapler-squad-search-query',
  SELECTED_STATUS: 'stapler-squad-selected-status',
  SELECTED_CATEGORY: 'stapler-squad-selected-category',
  SELECTED_TAG: 'stapler-squad-selected-tag',
  HIDE_PAUSED: 'stapler-squad-hide-paused',
  SHOW_ARCHIVED: 'stapler-squad-show-archived',
  FILTER_NEEDS_APPROVAL: 'stapler-squad-filter-needs-approval',
  GROUPING_STRATEGY: 'stapler-squad-grouping-strategy',
  COLLAPSED_GROUPS: 'stapler-squad-collapsed-groups',
  SORT_FIELD: 'stapler-squad-sort-field',
  SORT_DIR: 'stapler-squad-sort-dir',
  VISIBLE_COLUMNS: 'stapler-squad-visible-columns',
};

interface SessionListPersistedState {
  searchQuery: string;
  selectedStatus: SessionStatus | "all";
  selectedCategory: string | "all";
  selectedTag: string | "all";
  hidePaused: boolean;
  showArchived: boolean;
  filterNeedsApproval: boolean;
  groupingStrategy: GroupingStrategy;
  collapsedGroups: Set<string>;
  sortField: SortField;
  sortDir: SortDir;
  visibleColumns: ColumnKey[];
}

const SORT_FIELDS: SortField[] = ['lastActivity', 'name', 'createdAt', 'updatedAt', 'tokenCost'];
const SORT_DIRS: SortDir[] = ['asc', 'desc'];
const GROUPING_STRATEGY_VALUES = Object.values(GroupingStrategy);

// Builds a PersistedFieldsConfig keyed off BASE_STORAGE_KEYS, prefixed per-instance
// (e.g. split-pane view) so multiple SessionList instances don't collide in localStorage.
function buildPersistedFieldsConfig(prefix = ''): PersistedFieldsConfig<SessionListPersistedState> {
  const k = (base: string) => `${prefix}${base}`;
  return {
    searchQuery: { key: k(BASE_STORAGE_KEYS.SEARCH_QUERY), defaultValue: "" },
    selectedStatus: {
      key: k(BASE_STORAGE_KEYS.SELECTED_STATUS),
      defaultValue: "all",
      isValid: (v) => v === "all" || typeof v === "number",
    },
    selectedCategory: { key: k(BASE_STORAGE_KEYS.SELECTED_CATEGORY), defaultValue: "all" },
    selectedTag: { key: k(BASE_STORAGE_KEYS.SELECTED_TAG), defaultValue: "all" },
    hidePaused: {
      key: k(BASE_STORAGE_KEYS.HIDE_PAUSED),
      defaultValue: false,
      isValid: (v) => typeof v === "boolean",
    },
    // showArchived: when true, re-fetches sessions with includeArchived=true (server-side
    // default excludes archived sessions) and stops client-side filtering them out below.
    showArchived: {
      key: k(BASE_STORAGE_KEYS.SHOW_ARCHIVED),
      defaultValue: false,
      isValid: (v) => typeof v === "boolean",
    },
    // filterNeedsApproval: when true, show only Active sessions with subStatus === NEEDS_APPROVAL.
    filterNeedsApproval: {
      key: k(BASE_STORAGE_KEYS.FILTER_NEEDS_APPROVAL),
      defaultValue: false,
      isValid: (v) => typeof v === "boolean",
    },
    groupingStrategy: {
      key: k(BASE_STORAGE_KEYS.GROUPING_STRATEGY),
      defaultValue: GroupingStrategy.Category,
      isValid: (v) => GROUPING_STRATEGY_VALUES.includes(v as GroupingStrategy),
    },
    // Collapsed group keys — flat set shared across grouping strategies (a key that
    // recurs after switching strategies, e.g. "Backlog", stays collapsed).
    collapsedGroups: {
      key: k(BASE_STORAGE_KEYS.COLLAPSED_GROUPS),
      defaultValue: new Set<string>(),
      serialize: (value) => Array.from(value),
      deserialize: (raw) => new Set(raw as string[]),
      isValid: (v) => v instanceof Set,
    },
    sortField: {
      key: k(BASE_STORAGE_KEYS.SORT_FIELD),
      defaultValue: 'lastActivity',
      isValid: (v) => SORT_FIELDS.includes(v as SortField),
    },
    sortDir: {
      key: k(BASE_STORAGE_KEYS.SORT_DIR),
      defaultValue: 'desc',
      isValid: (v) => SORT_DIRS.includes(v as SortDir),
    },
    visibleColumns: {
      key: k(BASE_STORAGE_KEYS.VISIBLE_COLUMNS),
      defaultValue: DEFAULT_VISIBLE_COLUMNS,
      isValid: (v) => Array.isArray(v),
    },
  };
}

const getTimestampMs = (ts?: { seconds: bigint; nanos: number }): number => {
  if (!ts || ts.seconds === BigInt(0)) return 0;
  return Number(ts.seconds) * 1000;
};

export function SessionList({
  sessions,
  onSessionClick,
  onSessionOpenInNewPane,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
  onDirectResumeSession,
  onCloneSession,
  onNewWorkspaceSession,
  onRenameSession,
  onRestartSession,
  onUpdateTags,
  onNewSession,
  onCreateCheckpoint,
  onListCheckpoints,
  onForkFromCheckpoint,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onToggleAutoApprove,
  onSteerAutonomousSession,
  onClearConversationState,
  onHibernateSession,
  onResumeHibernatedSession,
  onFetchArchivedSessions,
  isLoading = false,
  storageKeyPrefix,
  extraHeaderActions,
  viewMode = "row",
}: SessionListProps) {
  // Review queue items indexed by session ID for badge display on session cards
  const { items: reviewItems } = useReviewQueueContext();
  const reviewItemBySessionId = useMemo(() => {
    const map = new Map(reviewItems.map(item => [item.sessionId, item]));
    return map;
  }, [reviewItems]);

  // Terminal-detected status data from Redux store
  const detectedStatusMap = useAppSelector(selectDetectedStatusMap);

  // Resolved stale-session threshold/notify config, fetched once on mount.
  const staleSessionConfig = useStaleSessionConfig();

  // Stale-session re-render tick: a session can cross the stale threshold purely by
  // clock time passing, with no new session data arriving. Force a re-render every
  // 60s so groupedSessions (below) recomputes and reclassifies it without a page
  // refresh. The setter's argument is discarded — only the re-render matters.
  const [staleRecomputeTick, forceStaleRecompute] = useState(0);
  useEffect(() => {
    const interval = setInterval(() => forceStaleRecompute((n) => n + 1), 60_000);
    return () => clearInterval(interval);
  }, []);

  // clearedSessions: optimistic approval suppression per session (card mode only; row mode uses SubStatusChip suppression)
  const { clearedSessions } = useApprovalsContext();

  // Filters, sort, grouping, and visible columns — persisted across page loads,
  // namespaced per-instance via storageKeyPrefix (e.g. split-pane view).
  const persistedFields = useMemo(() => buildPersistedFieldsConfig(storageKeyPrefix), [storageKeyPrefix]);
  const sessionListViewState = usePersistedViewState<SessionListPersistedState>(persistedFields);
  const {
    searchQuery,
    selectedStatus,
    selectedCategory,
    selectedTag,
    hidePaused,
    showArchived,
    filterNeedsApproval,
    groupingStrategy,
    collapsedGroups,
    sortField,
    sortDir,
    visibleColumns,
  } = sessionListViewState.state;
  const {
    searchQuery: setSearchQuery,
    selectedStatus: setSelectedStatus,
    selectedCategory: setSelectedCategory,
    selectedTag: setSelectedTag,
    hidePaused: setHidePaused,
    showArchived: setShowArchived,
    filterNeedsApproval: setFilterNeedsApproval,
    groupingStrategy: setGroupingStrategy,
    collapsedGroups: setCollapsedGroups,
    sortField: setSortField,
    sortDir: setSortDir,
    visibleColumns: setVisibleColumns,
  } = sessionListViewState.setters;
  const [columnPickerOpen, setColumnPickerOpen] = useState(false);

  // Multi-select state for bulk actions
  const [selectMode, setSelectMode] = useState(false);
  const lastAnchorRef = useRef<string | null>(null);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
  const [bulkFeedback, setBulkFeedback] = useState<string | null>(null);
  const [isBulkTagEditing, setIsBulkTagEditing] = useState(false);
  const bulkTagEditorTriggerRef = useRef<HTMLElement | null>(null);

  // Notification hook for undo toasts
  const { showUndoToast, removeNotification, addNotification } = useNotifications();

  // Pending-delete state: tracks sessions optimistically removed from the list while undo window is open
  const pendingDeleteRef = useRef<{
    ids: Set<string>;
    timer: ReturnType<typeof setTimeout> | null;
    toastId: string;
  } | null>(null);
  const [pendingDeleteIds, setPendingDeleteIds] = useState<Set<string>>(new Set());

  // Mobile filter panel toggle
  const [filtersOpen, setFiltersOpen] = useState(false);

  // S4: Project data for grouping headers and "Group as..." functionality
  const [projects, setProjects] = useState<Project[]>([]);
  const containerRef = useRef<HTMLDivElement>(null);
  const selectButtonRef = useRef<HTMLButtonElement>(null);
  const projectClientRef = useRef(
    createClient(SessionService, getConnectTransport())
  );

  // Fetch projects from API (called on mount and after mutations)
  const fetchProjects = useCallback(async () => {
    try {
      const response = await projectClientRef.current.listProjects({});
      setProjects(response.projects ?? []);
    } catch {
      // Projects are non-critical; ignore fetch errors
    }
  }, []);

  useEffect(() => {
    fetchProjects();
  }, [fetchProjects]);

  // S4-4: Group selected sessions into a project
  const handleGroupAs = useCallback(async (projectName: string) => {
    const sessionIds = Array.from(selectedSessions);
    if (sessionIds.length === 0) return;

    let projectId: string;
    const existing = projects.find((p) => p.name.toLowerCase() === projectName.toLowerCase());
    if (existing) {
      projectId = existing.id;
    } else {
      const created = await projectClientRef.current.createProject({ name: projectName, description: "" });
      projectId = created.project?.id ?? "";
      if (!projectId) return;
    }

    await projectClientRef.current.assignSessionsToProject({ projectId, sessionIds });
    await fetchProjects();
    showFeedback(`${sessionIds.length} session${sessionIds.length !== 1 ? "s" : ""} grouped as "${projectName}"`);
    setSelectedSessions(new Set());
    setSelectMode(false);
  }, [selectedSessions, projects, fetchProjects]);

  // S4-5: Inline rename/delete state for project group headers
  const [renamingProjectId, setRenamingProjectId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameError, setRenameError] = useState<string | null>(null);
  const [deletingProjectId, setDeletingProjectId] = useState<string | null>(null);

  const handleProjectRename = useCallback(async (projectId: string, newName: string) => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    await projectClientRef.current.updateProject({ id: projectId, name: trimmed });
    setRenamingProjectId(null);
    setTimeout(() => {
      document.querySelector<HTMLElement>(`[data-rename-btn="${projectId}"]`)?.focus();
    }, 0);
    await fetchProjects();
  }, [fetchProjects]);

  const handleProjectDelete = useCallback(async (projectId: string) => {
    await projectClientRef.current.deleteProject({ id: projectId });
    setDeletingProjectId(null);
    await fetchProjects();
  }, [fetchProjects]);

  // Re-fetch with includeArchived whenever the toggle changes (including on mount, so a
  // persisted "on" preference re-fetches archived sessions rather than showing a stale
  // client-only filtered view). The server excludes archived sessions by default, so
  // turning the toggle off does not need a re-fetch — filteredSessions below hides them.
  useEffect(() => {
    if (showArchived) {
      onFetchArchivedSessions?.(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showArchived]);

  // Extract unique categories from sessions
  const categories = useMemo(() => {
    const categorySet = new Set<string>();
    sessions.forEach((session) => {
      if (session.category) {
        categorySet.add(session.category);
      }
    });
    return Array.from(categorySet).sort();
  }, [sessions]);

  // Extract unique tags from sessions
  const tags = useMemo(() => {
    const tagSet = new Set<string>();
    sessions.forEach((session) => {
      if (session.tags) {
        session.tags.forEach(tag => tagSet.add(tag));
      }
    });
    return Array.from(tagSet).sort();
  }, [sessions]);

  // Filter sessions based on search query and filters
  const filteredSessions = useMemo(() => {
    return sessions.filter((session) => {
      // Exclude sessions that are pending deletion (optimistic removal)
      if (pendingDeleteIds.has(session.id)) return false;

      // Search filter
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const matchesSearch =
          session.title.toLowerCase().includes(query) ||
          session.path.toLowerCase().includes(query) ||
          session.branch.toLowerCase().includes(query) ||
          (session.category && session.category.toLowerCase().includes(query)) ||
          (session.tags && session.tags.some(tag => tag.toLowerCase().includes(query))) ||
          (session.program && session.program.toLowerCase().includes(query));

        if (!matchesSearch) return false;
      }

      // Status filter
      if (selectedStatus !== "all" && session.status !== selectedStatus) {
        return false;
      }

      // Category filter
      if (selectedCategory !== "all" && session.category !== selectedCategory) {
        return false;
      }

      // Tag filter
      if (selectedTag !== "all") {
        if (!session.tags || !session.tags.includes(selectedTag)) {
          return false;
        }
      }

      // Hide paused filter
      if (hidePaused && session.status === SessionStatus.PAUSED) {
        return false;
      }

      // Needs-approval quick filter — show only Active sessions with subStatus === NEEDS_APPROVAL
      if (filterNeedsApproval && !(session.status === SessionStatus.ACTIVE && (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED))) {
        return false;
      }

      // Archived filter — hidden by default even if a prior includeArchived fetch
      // left archived sessions in the Redux store (e.g. toggle turned back off
      // without a fresh non-archived fetch).
      if (!showArchived && session.archivedAt) {
        return false;
      }

      return true;
    });
  }, [sessions, searchQuery, selectedStatus, selectedCategory, selectedTag, hidePaused, filterNeedsApproval, showArchived, pendingDeleteIds]);

  // AC-2: per-session cost data, joined by session_id, for the "Sort: Cost" option.
  const { summary: insightsSummary } = useInsightsSummary({ includeOrphans: true });
  const costById = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of insightsSummary?.sessions ?? []) {
      if (s.sessionId) m.set(s.sessionId, s.estimatedCostUsd);
    }
    return m;
  }, [insightsSummary]);

  // Sort filtered sessions
  const sortedSessions = useMemo(() => {
    const sorted = [...filteredSessions];
    sorted.sort((a, b) => {
      if (sortField === 'tokenCost') {
        // compareSessionsByCost already applies sortDir internally (to keep
        // unloaded/unpriced rows last in BOTH directions) — return directly,
        // skipping the shared sortDir flip below.
        return compareSessionsByCost(a, b, costById, sortDir);
      }
      let cmp = 0;
      switch (sortField) {
        case 'name':
          cmp = a.title.localeCompare(b.title);
          break;
        case 'createdAt':
          cmp = getTimestampMs(a.createdAt) - getTimestampMs(b.createdAt);
          break;
        case 'updatedAt':
          cmp = getTimestampMs(a.updatedAt) - getTimestampMs(b.updatedAt);
          break;
        case 'lastActivity': {
          const act = (s: Session) => Math.max(
            getTimestampMs(s.lastMeaningfulOutput),
            getTimestampMs(s.lastTerminalUpdate)
          );
          cmp = act(a) - act(b);
          break;
        }
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return sorted;
  }, [filteredSessions, sortField, sortDir, costById]);

  // Epic 4.1: filteredSessionIds — for intersecting selectedSessions with visible sessions
  const filteredSessionIds = useMemo(
    () => new Set(filteredSessions.map(s => s.id)),
    [filteredSessions]
  );

  // Epic 4.1: activeSelection — intersection of selectedSessions with currently filtered sessions
  const activeSelection = useMemo(
    () => new Set([...selectedSessions].filter(id => filteredSessionIds.has(id))),
    [selectedSessions, filteredSessionIds]
  );

  // Derived: whether any filter is active (used for empty-state messaging)
  const hasActiveFilters = !!(searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval);

  // Group sessions by selected strategy. staleRecomputeTick is a dependency purely to
  // force recomputation on the 60s tick above — a session can cross the stale threshold
  // with no change to sortedSessions/groupingStrategy, and this is the only way to pick
  // that up without a page refresh.
  const groupedSessions = useMemo(() => {
    return groupSessions(sortedSessions, groupingStrategy, {
      thresholdMinutes: staleSessionConfig.thresholdMinutes,
    });
  }, [sortedSessions, groupingStrategy, staleSessionConfig.thresholdMinutes, staleRecomputeTick]);

  // Flat item list for row-mode virtualizer: headers and sessions interleaved.
  type FlatItem =
    | { kind: "header"; groupKey: string; displayName: string; groupSessions: Session[]; projectData?: Project; isProjectGrouping: boolean; isUngrouped: boolean }
    | { kind: "session"; session: Session };

  const flatItems = useMemo<FlatItem[]>(() => {
    if (viewMode !== "row") return [];
    const items: FlatItem[] = [];
    const isProjectGrouping = groupingStrategy === GroupingStrategy.Project;
    for (const { groupKey, displayName, sessions: grpSessions } of groupedSessions) {
      const projectData = isProjectGrouping
        ? projects.find((p) => p.id === groupKey || p.name === displayName)
        : undefined;
      const isUngrouped = groupKey === "No Project";
      items.push({ kind: "header", groupKey, displayName, groupSessions: grpSessions, projectData, isProjectGrouping, isUngrouped });
      if (!collapsedGroups.has(groupKey)) {
        for (const s of grpSessions) {
          items.push({ kind: "session", session: s });
        }
      }
    }
    return items;
  }, [groupedSessions, groupingStrategy, projects, viewMode, collapsedGroups]);

  const flatItemsRef = useRef(flatItems);
  flatItemsRef.current = flatItems;

  // Card-mode virtualization data: flat session array and per-group counts for GroupedVirtuoso.
  const { cardFlatSessions, cardGroupCounts } = useMemo(() => {
    if (viewMode !== "card") return { cardFlatSessions: [] as Session[], cardGroupCounts: [] as number[] };
    const flat: Session[] = [];
    const counts: number[] = [];
    // Collapsed groups keep their slot in cardGroupCounts (count 0) rather than being
    // removed — this preserves the groupIndex → groupedSessions[groupIndex] mapping
    // GroupedVirtuoso's groupContent callback relies on, with no index remap needed.
    for (const { groupKey, sessions: grpSessions } of groupedSessions) {
      if (collapsedGroups.has(groupKey)) {
        counts.push(0);
        continue;
      }
      counts.push(grpSessions.length);
      for (const s of grpSessions) flat.push(s);
    }
    return { cardFlatSessions: flat, cardGroupCounts: counts };
  }, [groupedSessions, viewMode, collapsedGroups]);

  const rowVirtualizer = useVirtualizer({
    count: viewMode === "row" ? flatItems.length : 0,
    getScrollElement: () => containerRef.current,
    estimateSize: (i) => (flatItems[i]?.kind === "header" ? 40 : 50),
    overscan: 8,
    measureElement: (el) => el.getBoundingClientRect().height,
  });

  // Handler for cycling grouping strategy (keyboard shortcut 'G')
  const handleCycleGrouping = () => {
    setGroupingStrategy(cycleGroupingStrategy(groupingStrategy));
  };

  const toggleGroupCollapsed = useCallback((groupKey: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupKey)) {
        next.delete(groupKey);
      } else {
        next.add(groupKey);
      }
      return next;
    });
  }, []);

  // Bulk actions handlers
  const handleToggleSelectMode = () => {
    setSelectMode(!selectMode);
    if (selectMode) {
      // Clear selections when exiting select mode
      setSelectedSessions(new Set());
    }
  };

  const feedbackTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const showFeedback = (msg: string, isError = false) => {
    if (feedbackTimerRef.current !== null) clearTimeout(feedbackTimerRef.current);
    setBulkFeedback(msg);
    feedbackTimerRef.current = setTimeout(() => {
      setBulkFeedback(null);
      feedbackTimerRef.current = null;
    }, isError ? 5000 : 3000);
  };

  // Entering selectMode automatically when hovering a card and clicking its checkbox.
  const handleToggleSession = useCallback((sessionId: string, e?: React.MouseEvent) => {
    if (e?.shiftKey && lastAnchorRef.current !== null) {
      const rangeIds = computeRangeIds(lastAnchorRef.current, sessionId, flatItemsRef.current);
      setSelectedSessions(new Set(rangeIds));
    } else {
      setSelectMode(true);
      setSelectedSessions((prev) => {
        const next = new Set(prev);
        if (next.has(sessionId)) {
          next.delete(sessionId);
        } else {
          next.add(sessionId);
        }
        return next;
      });
      lastAnchorRef.current = sessionId;
    }
  }, []);

  const handleSelectAll = useCallback(() => {
    const allSessionIds = new Set(filteredSessions.map(s => s.id));
    setSelectedSessions(allSessionIds);
  }, [filteredSessions]);

  // Stable row-handler callbacks passed to SessionRowWrapper.
  // These accept (session) or (id) so SessionRowWrapper can be memoized —
  // the per-session closures are only recreated inside the wrapper when session identity changes.
  const stableOnSessionClick = useCallback((session: Session) => onSessionClick?.(session), [onSessionClick]);
  const stableOnSessionOpenInNewPane = useCallback((session: Session) => onSessionOpenInNewPane?.(session), [onSessionOpenInNewPane]);
  const stableOnDeleteSession = useCallback((id: string) => onDeleteSession?.(id), [onDeleteSession]);
  const stableOnPauseSession = useCallback((id: string) => onPauseSession?.(id), [onPauseSession]);
  const stableOnResumeSession = useCallback((session: Session) => onResumeSession?.(session), [onResumeSession]);
  const stableOnCloneSession = useCallback((id: string) => onCloneSession?.(id), [onCloneSession]);
  const stableOnNewWorkspaceSession = useCallback((id: string) => onNewWorkspaceSession?.(id), [onNewWorkspaceSession]);
  const stableOnHibernateSession = useCallback((id: string) => onHibernateSession?.(id), [onHibernateSession]);
  const stableOnResumeHibernatedSession = useCallback((id: string) => onResumeHibernatedSession?.(id), [onResumeHibernatedSession]);

  const handleClearSelection = useCallback(() => {
    setSelectedSessions(new Set());
    setSelectMode(false);
    lastAnchorRef.current = null;
    showFeedback("Selection cleared");
    setTimeout(() => selectButtonRef.current?.focus(), 0);
  }, []);

  useEffect(() => {
    if (!selectMode) return;
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const inInput = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
      if ((e.metaKey || e.ctrlKey) && e.key === "a") {
        if (inInput) return;
        e.preventDefault();
        handleSelectAll();
      }
      if (e.key === "Escape") {
        e.preventDefault();
        handleClearSelection();
        e.stopImmediatePropagation();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [selectMode, handleSelectAll, handleClearSelection]);

  // Epic 3.3: Flush pending deletes on unmount — fire RPCs immediately rather than losing them.
  // Note: async in cleanup is fire-and-forget; tab-close data loss is a known limitation.
  useEffect(() => {
    return () => {
      void flushPendingDeletes();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // intentionally empty — runs only on unmount

  const handlePauseSelected = () => {
    if (!onPauseSession) return;
    const ids = Array.from(activeSelection);
    ids.forEach(id => onPauseSession(id));
    showFeedback(`${ids.length} session${ids.length !== 1 ? 's' : ''} paused`);
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleResumeSelected = () => {
    if (!onDirectResumeSession && !onResumeSession) return;
    const ids = Array.from(activeSelection);
    // Bulk resume bypasses the confirmation modal to avoid opening N modals
    ids.forEach(id => {
      const session = sessions.find(s => s.id === id);
      if (session) {
        if (onDirectResumeSession) {
          onDirectResumeSession(session);
        } else {
          onResumeSession?.(session);
        }
      }
    });
    showFeedback(`${ids.length} session${ids.length !== 1 ? 's' : ''} resumed`);
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleStopSelected = handlePauseSelected;

  const flushPendingDeletes = useCallback(async () => {
    if (!pendingDeleteRef.current) return;
    clearTimeout(pendingDeleteRef.current.timer ?? undefined);
    const ids = [...pendingDeleteRef.current.ids];
    const toastId = pendingDeleteRef.current.toastId;
    removeNotification(toastId);
    pendingDeleteRef.current = null;

    if (!onDeleteSession) {
      setPendingDeleteIds(new Set());
      return;
    }

    const results = await Promise.allSettled(ids.map(id => Promise.resolve(onDeleteSession(id))));

    const failed: string[] = [];
    results.forEach((result, i) => {
      if (result.status === "rejected") failed.push(ids[i]);
    });

    // Clear pendingDeleteIds unconditionally — failed sessions reappear via server state
    setPendingDeleteIds(new Set());

    if (failed.length > 0 && failed.length < ids.length) {
      const succeeded = ids.length - failed.length;
      addNotification({
        message: `${succeeded} deleted, ${failed.length} failed — failed sessions are back in the list`,
        notificationType: "error",
        sessionId: "",
        sessionName: "",
      });
    } else if (failed.length === ids.length) {
      addNotification({
        message: `All ${failed.length} delete${failed.length === 1 ? "" : "s"} failed — sessions are back in the list`,
        notificationType: "error",
        sessionId: "",
        sessionName: "",
      });
    }
  }, [onDeleteSession, removeNotification, addNotification]);

  const handleDeleteSelected = useCallback(() => {
    if (!onDeleteSession) return;

    // Step 1: Synchronously flush any existing pending batch to avoid async race.
    // Calling flushPendingDeletes() asynchronously would let its terminal
    // setPendingDeleteIds(new Set()) overwrite the new batch we're about to set.
    if (pendingDeleteRef.current) {
      clearTimeout(pendingDeleteRef.current.timer ?? undefined);
      removeNotification(pendingDeleteRef.current.toastId);
      const prevIds = [...pendingDeleteRef.current.ids];
      pendingDeleteRef.current = null;
      void Promise.allSettled(prevIds.map(id => Promise.resolve(onDeleteSession(id))));
    }

    // Step 2: Capture session IDs for delete
    const ids = Array.from(activeSelection);

    // Step 3: Optimistic removal
    setPendingDeleteIds(new Set(ids));

    // Step 4: Exit select mode
    setSelectedSessions(new Set());
    setSelectMode(false);
    lastAnchorRef.current = null;
    setTimeout(() => selectButtonRef.current?.focus(), 0);

    // Step 5: Start pending delete timer
    let toastId = "";
    toastId = showUndoToast(
      `Deleted ${ids.length} session${ids.length !== 1 ? "s" : ""}`,
      () => {
        // Guard: if the flush timer fired first, pendingDeleteRef is already null.
        if (!pendingDeleteRef.current) return;
        clearTimeout(pendingDeleteRef.current.timer ?? undefined);
        removeNotification(toastId);
        setPendingDeleteIds(new Set());
        pendingDeleteRef.current = null;
      },
      5000,
    );

    const timer = setTimeout(() => {
      void flushPendingDeletes();
    }, 5000);

    pendingDeleteRef.current = {
      ids: new Set(ids),
      timer,
      toastId,
    };
  }, [onDeleteSession, activeSelection, flushPendingDeletes, showUndoToast, removeNotification]);

  const handleBulkAddTag = (triggerEl: HTMLElement) => {
    bulkTagEditorTriggerRef.current = triggerEl;
    setIsBulkTagEditing(true);
  };

  const handleBulkTagSave = (newTags: string[]) => {
    if (newTags.length > 0 && onUpdateTags) {
      const sessionMap = new Map(sessions.map(s => [s.id, s]));
      activeSelection.forEach(id => {
        const session = sessionMap.get(id);
        const merged = Array.from(new Set([...(session?.tags ?? []), ...newTags]));
        onUpdateTags(id, merged);
      });
      showFeedback(`Added ${newTags.length} tag${newTags.length !== 1 ? 's' : ''} to ${activeSelection.size} session${activeSelection.size !== 1 ? 's' : ''}`);
    }
    setIsBulkTagEditing(false);
  };

  return (
    <div ref={containerRef} className={container} data-context="session-list" data-select-mode={selectMode ? "true" : "false"} aria-multiselectable={selectMode ? "true" : undefined}>
      <div className={header}>
        <div className={headerTop}>
          <h2 className={title} aria-live="polite" aria-atomic="true">Sessions ({filteredSessions.length !== sessions.length ? `${filteredSessions.length} of ${sessions.length}` : filteredSessions.length})</h2>
          <div className={headerActions}>
            {extraHeaderActions}
            {viewMode === "row" && (
              <ColumnPicker
                visibleColumns={visibleColumns}
                onChange={setVisibleColumns}
                open={columnPickerOpen}
                onOpenChange={setColumnPickerOpen}
              />
            )}
            <button
              onClick={() => onNewSession?.()}
              className={newSessionHeaderButton}
              aria-label="Create new session"
              aria-keyshortcuts="Control+K"
              title="New session (Ctrl+K)"
            >
              +
            </button>
            <button
              ref={selectButtonRef}
              onClick={handleToggleSelectMode}
              className={`${selectModeButton} ${selectMode ? selectModeButtonActive : ""}`}
              aria-label={selectMode ? "Exit select mode" : "Enter select mode"}
              aria-pressed={selectMode}
            >
              {selectMode ? "Cancel" : "Select"}
            </button>
          </div>
        </div>

        <div className={filters}>
          {/* Search input — always visible */}
          <div className={filterTopRow}>
            <input
              type="text"
              placeholder="Search sessions..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className={searchInput}
              aria-label="Search sessions"
            />
            {/* Filter toggle — only shown on mobile via CSS */}
            <button
              className={`${filterToggle} ${
                selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval
                  ? filterToggleActive
                  : ""
              }`}
              aria-expanded={filtersOpen}
              aria-controls="session-filter-controls"
              aria-label={(() => {
                const activeCount = [selectedStatus !== "all", selectedCategory !== "all", selectedTag !== "all", hidePaused, filterNeedsApproval].filter(Boolean).length;
                return activeCount > 0 ? `Filters (${activeCount} active)` : "Filters";
              })()}
              onClick={() => setFiltersOpen((prev) => !prev)}
            >
              Filters
              {(selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval) && (
                <span className={filterActiveDot} aria-hidden="true" />
              )}
            </button>
          </div>

          {/* Collapsible filter controls */}
          <ActionBar
            scroll
            compact
            gap="sm"
            id="session-filter-controls"
            className={`${filterControls} ${filtersOpen ? filterControlsOpen : ""}`}
          >
            {/* Status filter */}
            <select
              value={selectedStatus}
              onChange={(e) =>
                setSelectedStatus(
                  e.target.value === "all" ? "all" : Number(e.target.value)
                )
              }
              className={select}
              aria-label="Filter by status"
            >
              <option value="all">All Statuses</option>
              <option value={SessionStatus.ACTIVE}>Active</option>
              <option value={SessionStatus.PAUSED}>Paused</option>
              <option value={SessionStatus.STOPPED}>Stopped</option>
              <option value={SessionStatus.CREATING}>Creating</option>
            </select>

            {/* Category filter */}
            <select
              value={selectedCategory}
              onChange={(e) => setSelectedCategory(e.target.value)}
              className={select}
              aria-label="Filter by category"
            >
              <option value="all">All Categories</option>
              {categories.map((cat) => (
                <option key={cat} value={cat}>
                  {cat}
                </option>
              ))}
            </select>

            {/* Tag filter */}
            <select
              value={selectedTag}
              onChange={(e) => setSelectedTag(e.target.value)}
              className={select}
              aria-label="Filter by tag"
            >
              <option value="all">All Tags</option>
              {tags.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>

            {/* Hide paused toggle */}
            <label className={checkboxLabel}>
              <input
                type="checkbox"
                checked={hidePaused}
                onChange={(e) => setHidePaused(e.target.checked)}
                aria-label="Hide paused sessions"
              />
              <span>Hide Paused</span>
            </label>

            {/* Show archived toggle — archived sessions are excluded server-side by
                default; enabling this re-fetches with includeArchived. */}
            <label className={checkboxLabel}>
              <input
                type="checkbox"
                checked={showArchived}
                onChange={(e) => setShowArchived(e.target.checked)}
                aria-label="Show archived sessions"
                data-testid="show-archived-toggle"
              />
              <span>Show Archived</span>
            </label>

            {/* Needs-approval quick filter */}
            <label className={checkboxLabel}>
              <input
                type="checkbox"
                checked={filterNeedsApproval}
                onChange={(e) => setFilterNeedsApproval(e.target.checked)}
                aria-label="Show only sessions needing approval"
              />
              <span>Needs Approval</span>
            </label>

            {/* Grouping strategy selector */}
            <select
              value={groupingStrategy}
              onChange={(e) => setGroupingStrategy(e.target.value as GroupingStrategy)}
              className={select}
              title="Group by (Keyboard: G)"
              aria-label="Group sessions by (keyboard: G)"
            >
              {Object.entries(GroupingStrategyLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>

            {/* Sort field */}
            <select
              value={sortField}
              onChange={(e) => setSortField(e.target.value as SortField)}
              className={select}
              aria-label="Sort sessions by"
            >
              <option value="lastActivity">Sort: Last Activity</option>
              <option value="name">Sort: Name</option>
              <option value="createdAt">Sort: Created</option>
              <option value="updatedAt">Sort: Updated</option>
              <option value="tokenCost">Sort: Cost</option>
            </select>

            {/* Sort direction toggle */}
            <button
              onClick={() => setSortDir(d => d === 'asc' ? 'desc' : 'asc')}
              className={sortDirButton}
              title={sortDir === 'asc' ? 'Ascending — click to sort descending' : 'Descending — click to sort ascending'}
              aria-label={sortDir === 'asc' ? 'Switch to descending sort' : 'Switch to ascending sort'}
            >
              <span aria-hidden="true">{sortDir === 'asc' ? '↑' : '↓'}</span>
            </button>
          </ActionBar>
        </div>
      </div>

      {onHibernateSession && (
        <MemoryPressureCallout
          sessions={filteredSessions}
          onHibernate={onHibernateSession}
        />
      )}

      {/* Persistent aria-live region so bulk-action announcements survive BulkActions unmount */}
      <div id="bulk-feedback-live" role="status" aria-live="polite" aria-atomic="true" aria-label="Action feedback" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clipPath: "inset(50%)", whiteSpace: "nowrap" }}>{bulkFeedback ?? ""}</div>
      {/* Persistent live region for empty-state — always in DOM so NVDA announces on content change */}
      <div id="empty-state-live" role="status" aria-live="polite" aria-atomic="true" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clipPath: "inset(50%)", whiteSpace: "nowrap" }}>{filteredSessions.length === 0 && hasActiveFilters ? "No sessions found" : ""}</div>

      {/* Bulk actions bar — BulkActions renders null when selectedCount === 0 */}
      {selectMode && (
        <BulkActions
          selectedCount={activeSelection.size}
          totalCount={filteredSessions.length}
          onPauseAll={handlePauseSelected}
          onResumeAll={handleResumeSelected}
          onDeleteAll={handleDeleteSelected}
          onAddTagAll={(e) => handleBulkAddTag(e.currentTarget)}
          onSelectAll={handleSelectAll}
          onClearSelection={handleClearSelection}
          feedback={bulkFeedback}
          onGroupAs={handleGroupAs}
        />
      )}

      {/* Bulk tag editor modal */}
      {isBulkTagEditing && (
        <TagEditor
          tags={[]}
          onSave={handleBulkTagSave}
          onCancel={() => setIsBulkTagEditing(false)}
          sessionTitle={`${selectedSessions.size} selected session${selectedSessions.size !== 1 ? 's' : ''}`}
          triggerRef={bulkTagEditorTriggerRef}
        />
      )}

      {/* Session list */}
      {isLoading ? (
        <SessionListSkeleton />
      ) : filteredSessions.length === 0 ? (
        (() => {
          return hasActiveFilters ? (
            <div className={empty} role="region" aria-label="No results">
              <p id="no-sessions-msg">No sessions found</p>
              <button
                className={clearButton}
                aria-describedby="no-sessions-msg"
                onClick={() => {
                  setSearchQuery("");
                  setSelectedStatus("all");
                  setSelectedCategory("all");
                  setSelectedTag("all");
                  setHidePaused(false);
                  setFilterNeedsApproval(false);
                }}
              >
                Clear filters
              </button>
            </div>
          ) : (
            <SessionListEmptyState />
          );
        })()
      ) : viewMode === "row" ? (
        // Row mode: virtualized — only renders visible items (~20 rows at a time).
        <div
          role="list"
          aria-label={`Sessions, ${flatItems.filter(i => i && i.kind === "session").length} items`}
          style={{
            height: rowVirtualizer.getTotalSize(),
            width: "100%",
            position: "relative",
          }}
        >
          {rowVirtualizer.getVirtualItems().map((virtualItem) => {
            const item = flatItems[virtualItem.index];
            if (!item) return null;
            const isSessionItem = item.kind === "session";
            return (
              <div
                key={virtualItem.key}
                role="presentation"
                ref={rowVirtualizer.measureElement}
                data-index={virtualItem.index}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                {item.kind === "header" ? (
                  <div role="listitem">
                  <div
                    role="heading"
                    aria-level={3}
                    className={categoryTitle}
                    style={{ display: "flex", alignItems: "center", gap: "0.5rem", flexWrap: "wrap" }}
                    onClick={(e) => {
                      if (groupingStrategy === GroupingStrategy.None) return;
                      if (e.target !== e.currentTarget) return;
                      toggleGroupCollapsed(item.groupKey);
                    }}
                  >
                    {item.isProjectGrouping && item.projectData && renamingProjectId === item.projectData.id ? (
                      <form
                        aria-label={`Rename project ${item.displayName}`}
                        style={{ display: "flex", gap: "6px", alignItems: "center", flexWrap: "wrap" }}
                        onSubmit={async (e) => { e.preventDefault(); setRenameError(null); try { await handleProjectRename(item.projectData!.id, renameValue); } catch { setRenameError("Failed to rename — try again"); } }}
                      >
                        <input
                          autoFocus
                          type="text"
                          value={renameValue}
                          onChange={(e) => setRenameValue(e.target.value)}
                          onKeyDown={(e) => { if (e.key === "Escape") { const id = item.projectData!.id; setRenamingProjectId(null); setRenameError(null); setTimeout(() => { document.querySelector<HTMLElement>(`[data-rename-btn="${id}"]`)?.focus(); }, 0); } }}
                          aria-label={`Rename project ${item.displayName}`}
                          aria-describedby={renameError ? `row-rename-error-${item.projectData!.id}` : undefined}
                          style={{
                            padding: "2px 6px",
                            border: "1px solid var(--input-focus-border)",
                            borderRadius: "4px",
                            fontSize: "inherit",
                            fontWeight: "inherit",
                            background: "var(--input-background)",
                            color: "var(--text-primary)",
                          }}
                        />
                        <button type="submit" style={{ background: "none", border: "none", cursor: "pointer", color: "var(--success)", fontSize: "1rem" }} title="Save" aria-label="Save project name">✓</button>
                        <button type="button" onClick={() => { const id = item.projectData!.id; setRenamingProjectId(null); setRenameError(null); setTimeout(() => { document.querySelector<HTMLElement>(`[data-rename-btn="${id}"]`)?.focus(); }, 0); }} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "1rem" }} title="Cancel" aria-label="Cancel rename">✕</button>
                        {renameError && <span id={`row-rename-error-${item.projectData!.id}`} role="alert" style={{ color: "var(--error)", fontSize: "0.75rem", width: "100%" }}>{renameError}</span>}
                      </form>
                    ) : (
                      <>
                        {groupingStrategy !== GroupingStrategy.None && (
                          <CategoryCollapseToggle
                            groupKey={item.groupKey}
                            displayName={item.displayName}
                            collapsed={collapsedGroups.has(item.groupKey)}
                            onToggle={toggleGroupCollapsed}
                          />
                        )}
                        <span
                          onClick={() => {
                            if (groupingStrategy === GroupingStrategy.None) return;
                            toggleGroupCollapsed(item.groupKey);
                          }}
                        >
                          {item.displayName} ({item.groupSessions.length})
                        </span>
                        {item.isProjectGrouping && item.projectData && (
                          <>
                            {item.projectData.runningCount > 0 && (
                              <span role="img" aria-label={`${item.projectData.runningCount} running`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--success-bg)", color: "var(--success)", borderRadius: "10px" }}>
                                {item.projectData.runningCount} Running
                              </span>
                            )}
                            {item.projectData.completeCount > 0 && (
                              <span role="img" aria-label={`${item.projectData.completeCount} complete`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--primary)", color: "white", borderRadius: "10px", opacity: 0.85 }}>
                                {item.projectData.completeCount} Complete
                              </span>
                            )}
                            {item.projectData.reviewReadyCount > 0 && (
                              <span role="img" aria-label={`${item.projectData.reviewReadyCount} ready for review`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--warning-bg)", color: "var(--warning)", borderRadius: "10px" }}>
                                {item.projectData.reviewReadyCount} Review
                              </span>
                            )}
                          </>
                        )}
                        {item.isProjectGrouping && item.projectData && !item.isUngrouped && (
                          <span style={{ marginLeft: "auto", display: "flex", gap: "4px", flexShrink: 0 }}>
                            <button
                              type="button"
                              data-rename-btn={item.projectData!.id}
                              onClick={() => { setRenamingProjectId(item.projectData!.id); setRenameValue(item.projectData!.name); setRenameError(null); }}
                              title="Rename project"
                              aria-label={`Rename project ${item.displayName}`}
                              aria-expanded={false}
                              style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "0.875rem", padding: "2px" }}
                            >
                              <span aria-hidden="true">✏️</span>
                            </button>
                            {deletingProjectId === item.projectData.id ? (
                              <span role="group" aria-label="Confirm project deletion" style={{ display: "flex", gap: "4px", alignItems: "center", fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                                <span role="alert" aria-atomic="true">Remove project? {item.groupSessions.length} session{item.groupSessions.length !== 1 ? "s" : ""} will become ungrouped.</span>
                                <button
                                  type="button"
                                  onClick={() => handleProjectDelete(item.projectData!.id)}
                                  aria-label={`Confirm delete project ${item.displayName}`}
                                  style={{ background: "var(--error)", color: "white", border: "none", borderRadius: "4px", cursor: "pointer", padding: "2px 6px", fontSize: "0.75rem" }}
                                >
                                  Delete
                                </button>
                                <button
                                  type="button"
                                  autoFocus
                                  aria-label={`Cancel delete project ${item.displayName}`}
                                  onClick={() => setDeletingProjectId(null)}
                                  style={{ background: "none", border: "1px solid var(--border-color)", borderRadius: "4px", cursor: "pointer", padding: "2px 6px", fontSize: "0.75rem", color: "var(--text-secondary)" }}
                                >
                                  Cancel
                                </button>
                              </span>
                            ) : (
                              <button
                                type="button"
                                onClick={() => setDeletingProjectId(item.projectData!.id)}
                                title="Delete project"
                                aria-label={`Delete project ${item.displayName}`}
                                style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "0.875rem", padding: "2px" }}
                              >
                                <span aria-hidden="true">🗑️</span>
                              </button>
                            )}
                          </span>
                        )}
                      </>
                    )}
                  </div>
                  </div>
                ) : (
                  <div role="listitem">
                  <SessionRowWrapper
                    session={item.session}
                    onSessionClick={stableOnSessionClick}
                    onSessionOpenInNewPane={stableOnSessionOpenInNewPane}
                    onDeleteSession={stableOnDeleteSession}
                    onPauseSession={stableOnPauseSession}
                    onResumeSession={stableOnResumeSession}
                    onCloneSession={stableOnCloneSession}
                    onNewWorkspaceSession={stableOnNewWorkspaceSession}
                    onRestartSession={onRestartSession}
                    onCreateCheckpoint={onCreateCheckpoint}
                    onSetRateLimitEnabled={onSetRateLimitEnabled}
                    onToggleAutonomousMode={onToggleAutonomousMode}
                    onToggleAutoApprove={onToggleAutoApprove}
                    onSteerAutonomousSession={onSteerAutonomousSession}
                    onClearConversationState={onClearConversationState}
                    onHibernateSession={stableOnHibernateSession}
                    onResumeHibernatedSession={stableOnResumeHibernatedSession}
                    onUpdateTags={onUpdateTags}
                    suppressApprovalSubStatus={clearedSessions.has(item.session.id)}
                    staleThresholdMinutes={staleSessionConfig.thresholdMinutes}
                    visibleColumns={visibleColumns}
                    selectMode={selectMode}
                    isSelected={selectedSessions.has(item.session.id)}
                    onToggleSession={handleToggleSession}
                  />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        // Card mode: virtualized with GroupedVirtuoso — only visible cards are rendered.
        <GroupedVirtuoso
          customScrollParent={containerRef.current ?? undefined}
          groupCounts={cardGroupCounts}
          groupContent={(groupIndex) => {
            const grp = groupedSessions[groupIndex];
            if (!grp) return null;
            const { groupKey, displayName, sessions: grpSessions } = grp;
            const isProjectGrouping = groupingStrategy === GroupingStrategy.Project;
            const projectData = isProjectGrouping
              ? projects.find((p) => p.id === groupKey || p.name === displayName)
              : undefined;
            const isUngrouped = groupKey === "No Project";
            return (
              <div
                role="heading"
                aria-level={3}
                className={categoryTitle}
                style={{ display: "flex", alignItems: "center", gap: "0.5rem", flexWrap: "wrap" }}
                onClick={(e) => {
                  if (groupingStrategy === GroupingStrategy.None) return;
                  if (e.target !== e.currentTarget) return;
                  toggleGroupCollapsed(groupKey);
                }}
              >
                {isProjectGrouping && projectData && renamingProjectId === projectData.id ? (
                  <form
                    aria-label={`Rename project ${displayName}`}
                    style={{ display: "flex", gap: "6px", alignItems: "center", flexWrap: "wrap" }}
                    onSubmit={async (e) => { e.preventDefault(); setRenameError(null); try { await handleProjectRename(projectData.id, renameValue); } catch { setRenameError("Failed to rename — try again"); } }}
                  >
                    <input
                      autoFocus
                      type="text"
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Escape") { const id = projectData!.id; setRenamingProjectId(null); setRenameError(null); setTimeout(() => { document.querySelector<HTMLElement>(`[data-rename-btn="${id}"]`)?.focus(); }, 0); } }}
                      aria-label={`Rename project ${displayName}`}
                      aria-describedby={renameError ? `card-rename-error-${projectData!.id}` : undefined}
                      style={{
                        padding: "2px 6px",
                        border: "1px solid var(--input-focus-border)",
                        borderRadius: "4px",
                        fontSize: "inherit",
                        fontWeight: "inherit",
                        background: "var(--input-background)",
                        color: "var(--text-primary)",
                      }}
                    />
                    <button type="submit" style={{ background: "none", border: "none", cursor: "pointer", color: "var(--success)", fontSize: "1rem" }} title="Save" aria-label="Save project name">✓</button>
                    <button type="button" onClick={() => { const id = projectData!.id; setRenamingProjectId(null); setRenameError(null); setTimeout(() => { document.querySelector<HTMLElement>(`[data-rename-btn="${id}"]`)?.focus(); }, 0); }} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "1rem" }} title="Cancel" aria-label="Cancel rename">✕</button>
                    {renameError && <span id={`card-rename-error-${projectData!.id}`} role="alert" style={{ color: "var(--error)", fontSize: "0.75rem", width: "100%" }}>{renameError}</span>}
                  </form>
                ) : (
                  <>
                    {groupingStrategy !== GroupingStrategy.None && (
                      <CategoryCollapseToggle
                        groupKey={groupKey}
                        displayName={displayName}
                        collapsed={collapsedGroups.has(groupKey)}
                        onToggle={toggleGroupCollapsed}
                      />
                    )}
                    <span
                      onClick={() => {
                        if (groupingStrategy === GroupingStrategy.None) return;
                        toggleGroupCollapsed(groupKey);
                      }}
                    >
                      {displayName} ({grpSessions.length})
                    </span>
                    {isProjectGrouping && projectData && (
                      <>
                        {projectData.runningCount > 0 && (
                          <span role="img" aria-label={`${projectData.runningCount} running`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--success-bg)", color: "var(--success)", borderRadius: "10px" }}>
                            {projectData.runningCount} Running
                          </span>
                        )}
                        {projectData.completeCount > 0 && (
                          <span role="img" aria-label={`${projectData.completeCount} complete`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--primary)", color: "white", borderRadius: "10px", opacity: 0.85 }}>
                            {projectData.completeCount} Complete
                          </span>
                        )}
                        {projectData.reviewReadyCount > 0 && (
                          <span role="img" aria-label={`${projectData.reviewReadyCount} ready for review`} style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--warning-bg)", color: "var(--warning)", borderRadius: "10px" }}>
                            {projectData.reviewReadyCount} Review
                          </span>
                        )}
                      </>
                    )}
                    {isProjectGrouping && projectData && !isUngrouped && (
                      <span style={{ marginLeft: "auto", display: "flex", gap: "4px" }}>
                        <button
                          type="button"
                          data-rename-btn={projectData.id}
                          onClick={() => { setRenamingProjectId(projectData.id); setRenameValue(projectData.name); setRenameError(null); }}
                          title="Rename project"
                          aria-label={`Rename project ${displayName}`}
                          aria-expanded={false}
                          style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "0.875rem", padding: "2px" }}
                        >
                          <span aria-hidden="true">✏️</span>
                        </button>
                        {deletingProjectId === projectData.id ? (
                          <span role="group" aria-label="Confirm project deletion" style={{ display: "flex", gap: "4px", alignItems: "center", fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            <span role="alert" aria-atomic="true">Remove project? {grpSessions.length} session{grpSessions.length !== 1 ? "s" : ""} will become ungrouped.</span>
                            <button
                              type="button"
                              onClick={() => handleProjectDelete(projectData.id)}
                              aria-label={`Confirm delete project ${displayName}`}
                              style={{ background: "var(--error)", color: "white", border: "none", borderRadius: "4px", cursor: "pointer", padding: "2px 6px", fontSize: "0.75rem" }}
                            >
                              Delete
                            </button>
                            <button
                              type="button"
                              autoFocus
                              aria-label={`Cancel delete project ${displayName}`}
                              onClick={() => setDeletingProjectId(null)}
                              style={{ background: "none", border: "1px solid var(--border-color)", borderRadius: "4px", cursor: "pointer", padding: "2px 6px", fontSize: "0.75rem", color: "var(--text-secondary)" }}
                            >
                              Cancel
                            </button>
                          </span>
                        ) : (
                          <button
                            type="button"
                            onClick={() => setDeletingProjectId(projectData.id)}
                            title="Delete project"
                            aria-label={`Delete project ${displayName}`}
                            style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "0.875rem", padding: "2px" }}
                          >
                            <span aria-hidden="true">🗑️</span>
                          </button>
                        )}
                      </span>
                    )}
                  </>
                )}
              </div>
            );
          }}
          itemContent={(index) => {
            const session = cardFlatSessions[index];
            if (!session) return null;
            return (
              <div role="listitem" style={{'--card-index': index} as React.CSSProperties}>
                <SessionCard
                  session={session}
                  onClick={() => onSessionClick?.(session)}
                  onOpenInNewPane={onSessionOpenInNewPane ? () => onSessionOpenInNewPane(session) : undefined}
                  onDelete={() => onDeleteSession?.(session.id)}
                  onPause={() => onPauseSession?.(session.id)}
                  onResume={() => onResumeSession?.(session)}
                  onClone={() => onCloneSession?.(session.id)}
                  onNewWorkspace={() => onNewWorkspaceSession?.(session.id)}
                  onRename={onRenameSession}
                  onRestart={onRestartSession}
                  onUpdateTags={onUpdateTags}
                  onCreateCheckpoint={onCreateCheckpoint}
                  onListCheckpoints={onListCheckpoints}
                  onForkFromCheckpoint={onForkFromCheckpoint}
                  onSetRateLimitEnabled={onSetRateLimitEnabled}
                  onToggleAutonomousMode={onToggleAutonomousMode}
                  onToggleAutoApprove={onToggleAutoApprove}
                  onSteerAutonomousSession={onSteerAutonomousSession}
                  onClearConversationState={onClearConversationState}
                  onHibernate={onHibernateSession ? () => onHibernateSession(session.id) : undefined}
                  onResumeFromHibernation={onResumeHibernatedSession ? () => onResumeHibernatedSession(session.id) : undefined}
                  selectMode={selectMode}
                  isSelected={selectedSessions.has(session.id)}
                  onToggleSelect={(e) => handleToggleSession(session.id, e)}
                  reviewItem={reviewItemBySessionId.get(session.id)}
                  staleThresholdMinutes={staleSessionConfig.thresholdMinutes}
                  detectedStatus={detectedStatusMap[session.id]?.detectedStatus}
                  detectedContext={detectedStatusMap[session.id]?.detectedContext}
                  suppressApprovalSubStatus={clearedSessions.has(session.id)}
                />
              </div>
            );
          }}
        />
      )}

    </div>
  );
}
