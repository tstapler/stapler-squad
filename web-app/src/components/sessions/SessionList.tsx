"use client";

import React, { useState, useMemo, useEffect, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService, Project } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
import { AppLink } from "@/components/ui/AppLink";
import { Session, SessionStatus, CheckpointProto } from "@/gen/session/v1/types_pb";
import { SessionCard } from "./SessionCard";
import { BulkActions } from "./BulkActions";
import { TagEditor } from "./TagEditor";
import { GroupingStrategy, GroupingStrategyLabels, groupSessions, cycleGroupingStrategy } from "@/lib/grouping/strategies";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useAppSelector } from "@/lib/store";
import { selectDetectedStatusMap } from "@/lib/store/sessionsSlice";
import { ActionBar } from "@/components/ui/ActionBar";
import { Modal, ModalContent, ModalTitle, ModalFooter } from "@/components/ui/Modal";
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
  sessionList,
  categoryGroup,
  categoryTitle,
  categoryContent,
  empty,
  clearButton,
  emptyActions,
  emptyHint,
  newSessionButtonLarge,
  newSessionIcon,
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
  onRunOneShot?: (sessionId: string) => Promise<void>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  /** Prefix for localStorage keys, used when multiple instances are rendered (e.g. split view). */
  storageKeyPrefix?: string;
  /** Extra action buttons rendered in the header beside the "+" button. */
  extraHeaderActions?: React.ReactNode;
}

type SortField = 'lastActivity' | 'name' | 'createdAt' | 'updatedAt';
type SortDir = 'asc' | 'desc';

const BASE_STORAGE_KEYS = {
  SEARCH_QUERY: 'stapler-squad-search-query',
  SELECTED_STATUS: 'stapler-squad-selected-status',
  SELECTED_CATEGORY: 'stapler-squad-selected-category',
  SELECTED_TAG: 'stapler-squad-selected-tag',
  HIDE_PAUSED: 'stapler-squad-hide-paused',
  GROUPING_STRATEGY: 'stapler-squad-grouping-strategy',
  SORT_FIELD: 'stapler-squad-sort-field',
  SORT_DIR: 'stapler-squad-sort-dir',
};

function makeStorageKeys(prefix = '') {
  if (!prefix) return BASE_STORAGE_KEYS;
  return Object.fromEntries(
    Object.entries(BASE_STORAGE_KEYS).map(([k, v]) => [k, `${prefix}${v}`])
  ) as typeof BASE_STORAGE_KEYS;
}

// Helper functions for local storage operations
const loadFromStorage = <T,>(key: string, defaultValue: T): T => {
  if (typeof window === 'undefined') return defaultValue;
  try {
    const item = window.localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch (error) {
    console.warn(`Failed to load ${key} from localStorage:`, error);
    return defaultValue;
  }
};

const saveToStorage = <T,>(key: string, value: T): void => {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch (error) {
    console.warn(`Failed to save ${key} to localStorage:`, error);
  }
};

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
  onRunOneShot,
  onSetRateLimitEnabled,
  onClearConversationState,
  storageKeyPrefix,
  extraHeaderActions,
}: SessionListProps) {
  // Stable storage key set — only recomputed when storageKeyPrefix changes
  const STORAGE_KEYS = useMemo(() => makeStorageKeys(storageKeyPrefix), [storageKeyPrefix]);
  // Review queue items indexed by session ID for badge display on session cards
  const { items: reviewItems } = useReviewQueueContext();
  const reviewItemBySessionId = useMemo(() => {
    const map = new Map(reviewItems.map(item => [item.sessionId, item]));
    return map;
  }, [reviewItems]);

  // Terminal-detected status data from Redux store
  const detectedStatusMap = useAppSelector(selectDetectedStatusMap);

  // Initialize state from local storage
  const [searchQuery, setSearchQuery] = useState(() => loadFromStorage(STORAGE_KEYS.SEARCH_QUERY, ""));
  const [selectedStatus, setSelectedStatus] = useState<SessionStatus | "all">(() =>
    loadFromStorage(STORAGE_KEYS.SELECTED_STATUS, "all")
  );
  const [selectedCategory, setSelectedCategory] = useState<string | "all">(() =>
    loadFromStorage(STORAGE_KEYS.SELECTED_CATEGORY, "all")
  );
  const [selectedTag, setSelectedTag] = useState<string | "all">(() =>
    loadFromStorage(STORAGE_KEYS.SELECTED_TAG, "all")
  );
  const [hidePaused, setHidePaused] = useState(() =>
    loadFromStorage(STORAGE_KEYS.HIDE_PAUSED, false)
  );
  const [groupingStrategy, setGroupingStrategy] = useState<GroupingStrategy>(() =>
    loadFromStorage(STORAGE_KEYS.GROUPING_STRATEGY, GroupingStrategy.Category)
  );
  const [sortField, setSortField] = useState<SortField>(() =>
    loadFromStorage(STORAGE_KEYS.SORT_FIELD, 'lastActivity')
  );
  const [sortDir, setSortDir] = useState<SortDir>(() =>
    loadFromStorage(STORAGE_KEYS.SORT_DIR, 'desc')
  );

  // Multi-select state for bulk actions
  const [selectMode, setSelectMode] = useState(false);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
  const [bulkFeedback, setBulkFeedback] = useState<string | null>(null);
  const [isBulkTagEditing, setIsBulkTagEditing] = useState(false);
  const [showBulkDeleteConfirm, setShowBulkDeleteConfirm] = useState(false);

  // Mobile filter panel toggle
  const [filtersOpen, setFiltersOpen] = useState(false);

  // S4: Project data for grouping headers and "Group as..." functionality
  const [projects, setProjects] = useState<Project[]>([]);
  const projectClientRef = useRef(
    createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() }))
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
  const [deletingProjectId, setDeletingProjectId] = useState<string | null>(null);

  const handleProjectRename = useCallback(async (projectId: string, newName: string) => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    await projectClientRef.current.updateProject({ id: projectId, name: trimmed });
    setRenamingProjectId(null);
    await fetchProjects();
  }, [fetchProjects]);

  const handleProjectDelete = useCallback(async (projectId: string) => {
    await projectClientRef.current.deleteProject({ id: projectId });
    setDeletingProjectId(null);
    await fetchProjects();
  }, [fetchProjects]);

  // Persist filter preferences to local storage whenever they change
  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SEARCH_QUERY, searchQuery);
  }, [STORAGE_KEYS, searchQuery]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_STATUS, selectedStatus);
  }, [STORAGE_KEYS, selectedStatus]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_CATEGORY, selectedCategory);
  }, [STORAGE_KEYS, selectedCategory]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_TAG, selectedTag);
  }, [STORAGE_KEYS, selectedTag]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.HIDE_PAUSED, hidePaused);
  }, [STORAGE_KEYS, hidePaused]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.GROUPING_STRATEGY, groupingStrategy);
  }, [STORAGE_KEYS, groupingStrategy]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SORT_FIELD, sortField);
  }, [STORAGE_KEYS, sortField]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SORT_DIR, sortDir);
  }, [STORAGE_KEYS, sortDir]);

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

      return true;
    });
  }, [sessions, searchQuery, selectedStatus, selectedCategory, selectedTag, hidePaused]);

  // Sort filtered sessions
  const sortedSessions = useMemo(() => {
    const sorted = [...filteredSessions];
    sorted.sort((a, b) => {
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
  }, [filteredSessions, sortField, sortDir]);

  // Group sessions by selected strategy
  const groupedSessions = useMemo(() => {
    return groupSessions(sortedSessions, groupingStrategy);
  }, [sortedSessions, groupingStrategy]);

  // Handler for cycling grouping strategy (keyboard shortcut 'G')
  const handleCycleGrouping = () => {
    setGroupingStrategy(cycleGroupingStrategy(groupingStrategy));
  };

  // Bulk actions handlers
  const handleToggleSelectMode = () => {
    setSelectMode(!selectMode);
    if (selectMode) {
      // Clear selections when exiting select mode
      setSelectedSessions(new Set());
    }
  };

  const showFeedback = (msg: string) => {
    setBulkFeedback(msg);
    setTimeout(() => setBulkFeedback(null), 3000);
  };

  // Entering selectMode automatically when hovering a card and clicking its checkbox.
  const handleToggleSession = useCallback((sessionId: string) => {
    setSelectMode(true);
    setSelectedSessions((prev) => {
      const newSelected = new Set(prev);
      if (newSelected.has(sessionId)) {
        newSelected.delete(sessionId);
      } else {
        newSelected.add(sessionId);
      }
      return newSelected;
    });
  }, []);

  const handleSelectAll = () => {
    const allSessionIds = new Set(filteredSessions.map(s => s.id));
    setSelectedSessions(allSessionIds);
  };

  const handleClearSelection = () => {
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handlePauseSelected = () => {
    if (!onPauseSession) return;
    const ids = Array.from(selectedSessions);
    ids.forEach(id => onPauseSession(id));
    showFeedback(`${ids.length} session${ids.length !== 1 ? 's' : ''} paused`);
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleResumeSelected = () => {
    if (!onDirectResumeSession && !onResumeSession) return;
    const ids = Array.from(selectedSessions);
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

  const handleStopSelected = () => {
    if (!onPauseSession) return;
    const ids = Array.from(selectedSessions);
    ids.forEach(id => onPauseSession(id));
    showFeedback(`${ids.length} session${ids.length !== 1 ? 's' : ''} stopped`);
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleDeleteSelected = () => {
    if (!onDeleteSession) return;
    setShowBulkDeleteConfirm(true);
  };

  const handleConfirmBulkDelete = async () => {
    if (!onDeleteSession) return;
    const ids = Array.from(selectedSessions);
    const results = await Promise.allSettled(
      ids.map(id => Promise.resolve(onDeleteSession(id)))
    );
    const succeeded = results.filter(r => r.status === 'fulfilled').length;
    const failed = results.filter(r => r.status === 'rejected').length;
    const failedIds = new Set(ids.filter((_, i) => results[i].status === 'rejected'));
    if (failed > 0) {
      showFeedback(`${succeeded} deleted, ${failed} failed — failed sessions remain selected`);
      setSelectedSessions(failedIds);
    } else {
      showFeedback(`${succeeded} session${succeeded !== 1 ? 's' : ''} deleted`);
      setSelectedSessions(new Set());
      setSelectMode(false);
    }
  };

  const handleBulkAddTag = () => {
    setIsBulkTagEditing(true);
  };

  const handleBulkTagSave = (newTags: string[]) => {
    if (newTags.length > 0 && onUpdateTags) {
      selectedSessions.forEach(id => {
        const session = sessions.find(s => s.id === id);
        const merged = Array.from(new Set([...(session?.tags ?? []), ...newTags]));
        onUpdateTags(id, merged);
      });
      showFeedback(`Added ${newTags.length} tag${newTags.length !== 1 ? 's' : ''} to ${selectedSessions.size} session${selectedSessions.size !== 1 ? 's' : ''}`);
    }
    setIsBulkTagEditing(false);
  };

  return (
    <div className={container} data-context="session-list">
      <div className={header}>
        <div className={headerTop}>
          <h2 className={title}>Sessions ({filteredSessions.length})</h2>
          <div className={headerActions}>
            {extraHeaderActions}
            <button
              onClick={() => onNewSession?.()}
              className={newSessionHeaderButton}
              aria-label="Create new session (Ctrl+K)"
              title="Create new session (Ctrl+K)"
            >
              +
            </button>
            <button
              onClick={handleToggleSelectMode}
              className={`${selectModeButton} ${selectMode ? selectModeButtonActive : ""}`}
              aria-label={selectMode ? "Exit select mode" : "Enter select mode"}
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
                selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused
                  ? filterToggleActive
                  : ""
              }`}
              aria-expanded={filtersOpen}
              aria-controls="session-filter-controls"
              onClick={() => setFiltersOpen((prev) => !prev)}
            >
              Filters
              {(selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused) && (
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
              <option value={SessionStatus.RUNNING}>Running</option>
              <option value={SessionStatus.READY}>Ready</option>
              <option value={SessionStatus.PAUSED}>Paused</option>
              <option value={SessionStatus.LOADING}>Loading</option>
              <option value={SessionStatus.NEEDS_APPROVAL}>
                Needs Approval
              </option>
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

            {/* Grouping strategy selector */}
            <select
              value={groupingStrategy}
              onChange={(e) => setGroupingStrategy(e.target.value as GroupingStrategy)}
              className={select}
              title="Group by (Keyboard: G)"
              aria-label="Group sessions by"
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
            </select>

            {/* Sort direction toggle */}
            <button
              onClick={() => setSortDir(d => d === 'asc' ? 'desc' : 'asc')}
              className={sortDirButton}
              title={sortDir === 'asc' ? 'Ascending — click to sort descending' : 'Descending — click to sort ascending'}
              aria-label={`Sort direction: ${sortDir === 'asc' ? 'ascending' : 'descending'}`}
            >
              {sortDir === 'asc' ? '↑' : '↓'}
            </button>
          </ActionBar>
        </div>
      </div>

      {/* Bulk actions bar — BulkActions renders null when selectedCount === 0 */}
      {selectMode && (
        <BulkActions
          selectedCount={selectedSessions.size}
          totalCount={filteredSessions.length}
          onPauseAll={handlePauseSelected}
          onResumeAll={handleResumeSelected}
          onStopAll={handleStopSelected}
          onDeleteAll={handleDeleteSelected}
          onAddTagAll={handleBulkAddTag}
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
        />
      )}

      {/* Session list */}
      {filteredSessions.length === 0 ? (
        <div className={empty}>
          <p>{searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused
            ? "No sessions found"
            : "No sessions yet"
          }</p>
          {searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused ? (
            <button
              className={clearButton}
              onClick={() => {
                setSearchQuery("");
                setSelectedStatus("all");
                setSelectedCategory("all");
                setSelectedTag("all");
                setHidePaused(false);
              }}
            >
              Clear filters
            </button>
          ) : (
            <div className={emptyActions}>
              <p className={emptyHint}>
                Get started by creating your first AI coding session
              </p>
              <button
                className={newSessionButtonLarge}
                onClick={() => onNewSession?.()}
              >
                <span className={newSessionIcon}>+</span>
                Create Your First Session
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className={sessionList}>
          {groupedSessions.map(({ groupKey, displayName, sessions: groupSessions }) => {
            // S4-5: Enhanced project group headers when GroupByProject is active
            const isProjectGrouping = groupingStrategy === GroupingStrategy.Project;
            const projectData = isProjectGrouping
              ? projects.find((p) => p.id === groupKey || p.name === displayName)
              : undefined;
            const isUngrouped = groupKey === "No Project";

            return (
            <div key={groupKey} className={categoryGroup}>
              <h3 className={categoryTitle} style={{ display: "flex", alignItems: "center", gap: "0.5rem", flexWrap: "wrap" }}>
                {/* Inline rename input for project groups */}
                {isProjectGrouping && projectData && renamingProjectId === projectData.id ? (
                  <form
                    style={{ display: "flex", gap: "6px", alignItems: "center" }}
                    onSubmit={(e) => { e.preventDefault(); handleProjectRename(projectData.id, renameValue); }}
                  >
                    <input
                      autoFocus
                      type="text"
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Escape") setRenamingProjectId(null); }}
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
                    <button type="submit" style={{ background: "none", border: "none", cursor: "pointer", color: "var(--success)", fontSize: "1rem" }} title="Save">✓</button>
                    <button type="button" onClick={() => setRenamingProjectId(null)} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "1rem" }} title="Cancel">✕</button>
                  </form>
                ) : (
                  <>
                    <span>{displayName} ({groupSessions.length})</span>
                    {/* Project stats pills */}
                    {isProjectGrouping && projectData && (
                      <>
                        {projectData.runningCount > 0 && (
                          <span style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--success-bg)", color: "var(--success)", borderRadius: "10px" }}>
                            {projectData.runningCount} Running
                          </span>
                        )}
                        {projectData.completeCount > 0 && (
                          <span style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--primary)", color: "white", borderRadius: "10px", opacity: 0.85 }}>
                            {projectData.completeCount} Complete
                          </span>
                        )}
                        {projectData.reviewReadyCount > 0 && (
                          <span style={{ fontSize: "0.75rem", padding: "1px 6px", background: "var(--warning-bg)", color: "var(--warning)", borderRadius: "10px" }}>
                            {projectData.reviewReadyCount} Review
                          </span>
                        )}
                      </>
                    )}
                    {/* Inline rename/delete actions for project groups */}
                    {isProjectGrouping && projectData && !isUngrouped && (
                      <span style={{ marginLeft: "auto", display: "flex", gap: "4px" }}>
                        <button
                          type="button"
                          onClick={() => { setRenamingProjectId(projectData.id); setRenameValue(projectData.name); }}
                          title="Rename project"
                          aria-label={`Rename project ${displayName}`}
                          style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-secondary)", fontSize: "0.875rem", padding: "2px" }}
                        >
                          ✏️
                        </button>
                        {deletingProjectId === projectData.id ? (
                          <span style={{ display: "flex", gap: "4px", alignItems: "center", fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Remove project? {groupSessions.length} session{groupSessions.length !== 1 ? "s" : ""} will become ungrouped.
                            <button
                              type="button"
                              onClick={() => handleProjectDelete(projectData.id)}
                              style={{ background: "var(--error)", color: "white", border: "none", borderRadius: "4px", cursor: "pointer", padding: "2px 6px", fontSize: "0.75rem" }}
                            >
                              Delete
                            </button>
                            <button
                              type="button"
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
                            🗑️
                          </button>
                        )}
                      </span>
                    )}
                  </>
                )}
              </h3>
              <div className={categoryContent}>
                {groupSessions.map((session, index) => (
                  <div key={session.id} style={{'--card-index': index} as React.CSSProperties}>
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
                      onRunOneShot={onRunOneShot}
                      onSetRateLimitEnabled={onSetRateLimitEnabled}
                      onClearConversationState={onClearConversationState}
                      selectMode={selectMode}
                      isSelected={selectedSessions.has(session.id)}
                      onToggleSelect={() => handleToggleSession(session.id)}
                      reviewItem={reviewItemBySessionId.get(session.id)}
                      detectedStatus={detectedStatusMap[session.id]?.detectedStatus}
                      detectedContext={detectedStatusMap[session.id]?.detectedContext}
                    />
                  </div>
                ))}
              </div>
            </div>
          );
          })}
        </div>
      )}

      <Modal open={showBulkDeleteConfirm} onOpenChange={setShowBulkDeleteConfirm}>
        <ModalContent fallbackTitle="Confirm delete">
          <ModalTitle>Delete {selectedSessions.size} session{selectedSessions.size !== 1 ? 's' : ''}?</ModalTitle>
          <p style={{ color: 'var(--text-secondary)', marginBottom: '1rem', fontSize: '0.875rem' }}>
            This will permanently delete {selectedSessions.size} selected session{selectedSessions.size !== 1 ? 's' : ''}. This cannot be undone.
          </p>
          <ModalFooter>
            <button
              style={{ padding: '0.5rem 1rem', border: '1px solid var(--border-color)', borderRadius: '6px', background: 'var(--card-background)', color: 'var(--text-primary)', cursor: 'pointer', fontSize: '0.875rem' }}
              onClick={() => setShowBulkDeleteConfirm(false)}
            >
              Cancel
            </button>
            <button
              style={{ padding: '0.5rem 1rem', border: 'none', borderRadius: '6px', background: 'var(--error)', color: 'white', cursor: 'pointer', fontSize: '0.875rem', fontWeight: 600 }}
              onClick={() => { setShowBulkDeleteConfirm(false); handleConfirmBulkDelete(); }}
            >
              Delete {selectedSessions.size} session{selectedSessions.size !== 1 ? 's' : ''}
            </button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
