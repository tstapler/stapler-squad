"use client";

import { useState, useMemo, useEffect } from "react";
import Link from "next/link";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { SessionCard } from "./SessionCard";
import { BulkActions } from "./BulkActions";
import { GroupingStrategy, GroupingStrategyLabels, groupSessions, cycleGroupingStrategy } from "@/lib/grouping/strategies";
import styles from "./SessionList.module.css";

interface SessionListProps {
  sessions: Session[];
  onSessionClick?: (session: Session) => void;
  onDeleteSession?: (sessionId: string) => void;
  onPauseSession?: (sessionId: string) => void;
  onResumeSession?: (sessionId: string) => void;
  onDuplicateSession?: (sessionId: string) => void;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
}

// Local storage keys for persisting UI preferences
const STORAGE_KEYS = {
  SEARCH_QUERY: 'claude-squad-search-query',
  SELECTED_STATUS: 'claude-squad-selected-status',
  SELECTED_CATEGORY: 'claude-squad-selected-category',
  SELECTED_TAG: 'claude-squad-selected-tag',
  HIDE_PAUSED: 'claude-squad-hide-paused',
  GROUPING_STRATEGY: 'claude-squad-grouping-strategy',
};

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

export function SessionList({
  sessions,
  onSessionClick,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
  onDuplicateSession,
  onUpdateTags,
}: SessionListProps) {
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

  // Multi-select state for bulk actions
  const [selectMode, setSelectMode] = useState(false);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());

  // Persist filter preferences to local storage whenever they change
  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SEARCH_QUERY, searchQuery);
  }, [searchQuery]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_STATUS, selectedStatus);
  }, [selectedStatus]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_CATEGORY, selectedCategory);
  }, [selectedCategory]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.SELECTED_TAG, selectedTag);
  }, [selectedTag]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.HIDE_PAUSED, hidePaused);
  }, [hidePaused]);

  useEffect(() => {
    saveToStorage(STORAGE_KEYS.GROUPING_STRATEGY, groupingStrategy);
  }, [groupingStrategy]);

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
          (session.tags && session.tags.some(tag => tag.toLowerCase().includes(query)));

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

  // Group sessions by selected strategy
  const groupedSessions = useMemo(() => {
    return groupSessions(filteredSessions, groupingStrategy);
  }, [filteredSessions, groupingStrategy]);

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

  const handleToggleSession = (sessionId: string) => {
    const newSelected = new Set(selectedSessions);
    if (newSelected.has(sessionId)) {
      newSelected.delete(sessionId);
    } else {
      newSelected.add(sessionId);
    }
    setSelectedSessions(newSelected);
  };

  const handleSelectAll = () => {
    const allSessionIds = new Set(filteredSessions.map(s => s.id));
    setSelectedSessions(allSessionIds);
  };

  const handleClearSelection = () => {
    setSelectedSessions(new Set());
  };

  const handlePauseSelected = () => {
    if (!onPauseSession) return;
    selectedSessions.forEach(id => onPauseSession(id));
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleResumeSelected = () => {
    if (!onResumeSession) return;
    selectedSessions.forEach(id => onResumeSession(id));
    setSelectedSessions(new Set());
    setSelectMode(false);
  };

  const handleDeleteSelected = () => {
    if (!onDeleteSession) return;
    if (window.confirm(`Are you sure you want to delete ${selectedSessions.size} session(s)?`)) {
      selectedSessions.forEach(id => onDeleteSession(id));
      setSelectedSessions(new Set());
      setSelectMode(false);
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerTop}>
          <h2 className={styles.title}>Sessions ({filteredSessions.length})</h2>
          <div className={styles.headerActions}>
            <button
              onClick={handleToggleSelectMode}
              className={`${styles.selectModeButton} ${selectMode ? styles.active : ""}`}
              aria-label={selectMode ? "Exit select mode" : "Enter select mode"}
            >
              {selectMode ? "Cancel" : "Select"}
            </button>
            <Link href="/sessions/new" className={styles.newSessionButton}>
              <span className={styles.newSessionIcon}>+</span>
              New Session
            </Link>
          </div>
        </div>

        <div className={styles.filters}>
          {/* Search input */}
          <input
            type="text"
            placeholder="Search sessions..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className={styles.searchInput}
          />

          {/* Status filter */}
          <select
            value={selectedStatus}
            onChange={(e) =>
              setSelectedStatus(
                e.target.value === "all" ? "all" : Number(e.target.value)
              )
            }
            className={styles.select}
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
            className={styles.select}
          >
            <option value="all">All Categories</option>
            {categories.map((category) => (
              <option key={category} value={category}>
                {category}
              </option>
            ))}
          </select>

          {/* Tag filter */}
          <select
            value={selectedTag}
            onChange={(e) => setSelectedTag(e.target.value)}
            className={styles.select}
          >
            <option value="all">All Tags</option>
            {tags.map((tag) => (
              <option key={tag} value={tag}>
                {tag}
              </option>
            ))}
          </select>

          {/* Hide paused toggle */}
          <label className={styles.checkbox}>
            <input
              type="checkbox"
              checked={hidePaused}
              onChange={(e) => setHidePaused(e.target.checked)}
            />
            <span>Hide Paused</span>
          </label>

          {/* Grouping strategy selector */}
          <select
            value={groupingStrategy}
            onChange={(e) => setGroupingStrategy(e.target.value as GroupingStrategy)}
            className={styles.select}
            title="Group by (Keyboard: G)"
          >
            {Object.entries(GroupingStrategyLabels).map(([value, label]) => (
              <option key={value} value={value}>
                Group by: {label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Bulk actions bar */}
      {selectMode && selectedSessions.size > 0 && (
        <BulkActions
          selectedCount={selectedSessions.size}
          totalCount={filteredSessions.length}
          onPauseAll={handlePauseSelected}
          onResumeAll={handleResumeSelected}
          onDeleteAll={handleDeleteSelected}
          onSelectAll={handleSelectAll}
          onClearSelection={handleClearSelection}
        />
      )}

      {/* Session list */}
      {filteredSessions.length === 0 ? (
        <div className={styles.empty}>
          <p>{searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused
            ? "No sessions found"
            : "No sessions yet"
          }</p>
          {searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused ? (
            <button
              className={styles.clearButton}
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
            <div className={styles.emptyActions}>
              <p className={styles.emptyHint}>
                Get started by creating your first AI coding session
              </p>
              <Link href="/sessions/new" className={styles.newSessionButtonLarge}>
                <span className={styles.newSessionIcon}>+</span>
                Create Your First Session
              </Link>
            </div>
          )}
        </div>
      ) : (
        <div className={styles.sessionList}>
          {groupedSessions.map(({ groupKey, displayName, sessions: groupSessions }) => (
            <div key={groupKey} className={styles.categoryGroup}>
              <h3 className={styles.categoryTitle}>
                {displayName} ({groupSessions.length})
              </h3>
              <div className={styles.categoryContent}>
                {groupSessions.map((session) => (
                  <SessionCard
                    key={session.id}
                    session={session}
                    onClick={() => onSessionClick?.(session)}
                    onDelete={() => onDeleteSession?.(session.id)}
                    onPause={() => onPauseSession?.(session.id)}
                    onResume={() => onResumeSession?.(session.id)}
                    onDuplicate={() => onDuplicateSession?.(session.id)}
                    onUpdateTags={onUpdateTags}
                    selectMode={selectMode}
                    isSelected={selectedSessions.has(session.id)}
                    onToggleSelect={() => handleToggleSession(session.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
