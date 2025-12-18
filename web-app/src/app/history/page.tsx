"use client";

import { useState, useEffect, useRef, useMemo, useCallback } from "react";
import { useRouter } from "next/navigation";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import { HistorySearchInput, HistorySearchResults } from "@/components/history";
import { useHistoryFullTextSearch, SearchResultItem } from "@/lib/hooks/useHistoryFullTextSearch";
import styles from "./history.module.css";

// ============================================================================
// Types and Constants
// ============================================================================

type SortField = "updated" | "created" | "messages" | "name";
type SortOrder = "asc" | "desc";
type DateFilter = "all" | "today" | "week" | "month";
type SearchMode = "metadata" | "fulltext";

enum HistoryGroupingStrategy {
  None = "none",
  Date = "date",
  Project = "project",
  Model = "model",
}

const GroupingStrategyLabels: Record<HistoryGroupingStrategy, string> = {
  [HistoryGroupingStrategy.None]: "No Grouping",
  [HistoryGroupingStrategy.Date]: "Date",
  [HistoryGroupingStrategy.Project]: "Project",
  [HistoryGroupingStrategy.Model]: "Model",
};

// Local storage keys
const STORAGE_KEYS = {
  SEARCH_QUERY: 'claude-history-search-query',
  SELECTED_MODEL: 'claude-history-selected-model',
  DATE_FILTER: 'claude-history-date-filter',
  SORT_FIELD: 'claude-history-sort-field',
  SORT_ORDER: 'claude-history-sort-order',
  GROUPING_STRATEGY: 'claude-history-grouping-strategy',
  SEARCH_MODE: 'claude-history-search-mode',
};

// ============================================================================
// Utility Functions
// ============================================================================

const loadFromStorage = <T,>(key: string, defaultValue: T): T => {
  if (typeof window === 'undefined') return defaultValue;
  try {
    const item = window.localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch {
    return defaultValue;
  }
};

const saveToStorage = <T,>(key: string, value: T): void => {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore storage errors
  }
};

const formatTimeAgo = (timestamp: any): string => {
  if (!timestamp) return "N/A";
  const now = Date.now();
  const date = new Date(Number(timestamp.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);

  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
  return date.toLocaleDateString();
};

const formatDate = (timestamp: any): string => {
  if (!timestamp) return "N/A";
  const date = new Date(Number(timestamp.seconds) * 1000);
  return date.toLocaleString();
};

const truncateMiddle = (str: string, maxLength: number): string => {
  if (str.length <= maxLength) return str;
  const ellipsis = "...";
  const charsToShow = maxLength - ellipsis.length;
  const frontChars = Math.ceil(charsToShow / 2);
  const backChars = Math.floor(charsToShow / 2);
  return str.substring(0, frontChars) + ellipsis + str.substring(str.length - backChars);
};

const getDateGroup = (timestamp: any): string => {
  if (!timestamp) return "Unknown";
  const date = new Date(Number(timestamp.seconds) * 1000);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);
  const weekAgo = new Date(today.getTime() - 7 * 86400000);
  const monthAgo = new Date(today.getTime() - 30 * 86400000);

  if (date >= today) return "Today";
  if (date >= yesterday) return "Yesterday";
  if (date >= weekAgo) return "This Week";
  if (date >= monthAgo) return "This Month";
  return "Older";
};

const isWithinDateFilter = (timestamp: any, filter: DateFilter): boolean => {
  if (filter === "all" || !timestamp) return true;
  const date = new Date(Number(timestamp.seconds) * 1000);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());

  switch (filter) {
    case "today":
      return date >= today;
    case "week":
      return date >= new Date(today.getTime() - 7 * 86400000);
    case "month":
      return date >= new Date(today.getTime() - 30 * 86400000);
    default:
      return true;
  }
};

// ============================================================================
// Main Component
// ============================================================================

export default function HistoryBrowserPage() {
  // Router for navigation after session creation
  const router = useRouter();

  // Core state
  const [entries, setEntries] = useState<ClaudeHistoryEntry[]>([]);
  const [selectedEntry, setSelectedEntry] = useState<ClaudeHistoryEntry | null>(null);
  const [selectedIndex, setSelectedIndex] = useState<number>(-1);
  const [messages, setMessages] = useState<ClaudeMessage[]>([]);
  const [showMessages, setShowMessages] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [previewMessages, setPreviewMessages] = useState<ClaudeMessage[]>([]);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resuming, setResuming] = useState(false);

  // Filter state (persisted) - use defaults initially to avoid hydration mismatch
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedModel, setSelectedModel] = useState<string>("all");
  const [dateFilter, setDateFilter] = useState<DateFilter>("all");
  const [sortField, setSortField] = useState<SortField>("updated");
  const [sortOrder, setSortOrder] = useState<SortOrder>("desc");
  const [groupingStrategy, setGroupingStrategy] = useState<HistoryGroupingStrategy>(HistoryGroupingStrategy.Date);

  // Message search state
  const [messageSearchQuery, setMessageSearchQuery] = useState("");

  // Full-text search mode state
  const [searchMode, setSearchMode] = useState<SearchMode>("metadata");

  // Hydration flag to track when client-side code has run
  const [isHydrated, setIsHydrated] = useState(false);

  // Load persisted state from localStorage after hydration (client-side only)
  useEffect(() => {
    setSearchQuery(loadFromStorage(STORAGE_KEYS.SEARCH_QUERY, ""));
    setSelectedModel(loadFromStorage(STORAGE_KEYS.SELECTED_MODEL, "all"));
    setDateFilter(loadFromStorage(STORAGE_KEYS.DATE_FILTER, "all"));
    setSortField(loadFromStorage(STORAGE_KEYS.SORT_FIELD, "updated"));
    setSortOrder(loadFromStorage(STORAGE_KEYS.SORT_ORDER, "desc"));
    setGroupingStrategy(loadFromStorage(STORAGE_KEYS.GROUPING_STRATEGY, HistoryGroupingStrategy.Date));
    setSearchMode(loadFromStorage(STORAGE_KEYS.SEARCH_MODE, "metadata"));
    setIsHydrated(true);
  }, []);

  // Full-text search hook
  const fullTextSearch = useHistoryFullTextSearch({
    debounceMs: 300,
    autoSearch: true,
  });

  // Refs
  const clientRef = useRef<any>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const entryListRef = useRef<HTMLDivElement>(null);
  const modalRef = useRef<HTMLDivElement>(null);

  // Persist filter preferences
  useEffect(() => { saveToStorage(STORAGE_KEYS.SEARCH_QUERY, searchQuery); }, [searchQuery]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SELECTED_MODEL, selectedModel); }, [selectedModel]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.DATE_FILTER, dateFilter); }, [dateFilter]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SORT_FIELD, sortField); }, [sortField]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SORT_ORDER, sortOrder); }, [sortOrder]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.GROUPING_STRATEGY, groupingStrategy); }, [groupingStrategy]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SEARCH_MODE, searchMode); }, [searchMode]);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
    });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Load history on mount
  useEffect(() => {
    loadHistory();
  }, []);

  // ============================================================================
  // Data Loading
  // ============================================================================

  const loadHistory = useCallback(async (query?: string) => {
    if (!clientRef.current) return;

    try {
      setLoading(true);
      setError(null);
      const response = await clientRef.current.listClaudeHistory({
        limit: 500, // Load more entries for better filtering
        searchQuery: query,
      });
      setEntries(response.entries);
    } catch (err) {
      setError(`Failed to load history: ${err}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadEntryDetail = useCallback(async (id: string) => {
    if (!clientRef.current) return;

    try {
      setError(null);
      setLoadingPreview(true);
      setPreviewMessages([]);

      // Load entry details and preview messages in parallel
      const [detailResponse, messagesResponse] = await Promise.all([
        clientRef.current.getClaudeHistoryDetail({ id }),
        clientRef.current.getClaudeHistoryMessages({ id, limit: 5 }),
      ]);

      if (detailResponse.entry) {
        setSelectedEntry(detailResponse.entry);
      }

      // Show the most recent messages (reverse to show oldest first in preview)
      if (messagesResponse.messages) {
        // Messages come newest-first, reverse for chronological display
        setPreviewMessages([...messagesResponse.messages].reverse());
      }
    } catch (err) {
      setError(`Failed to load entry details: ${err}`);
    } finally {
      setLoadingPreview(false);
    }
  }, []);

  const handleSearch = useCallback(async () => {
    setSearching(true);
    try {
      await loadHistory(searchQuery || undefined);
    } finally {
      setSearching(false);
    }
  }, [searchQuery, loadHistory]);

  const loadMessages = useCallback(async (id: string) => {
    if (!clientRef.current) return;

    try {
      setLoadingMessages(true);
      setError(null);
      const response = await clientRef.current.getClaudeHistoryMessages({ id });
      setMessages(response.messages);
      setShowMessages(true);
      setMessageSearchQuery("");
    } catch (err) {
      setError(`Failed to load messages: ${err}`);
    } finally {
      setLoadingMessages(false);
    }
  }, []);

  // ============================================================================
  // Derived Data
  // ============================================================================

  // Extract unique models for filter dropdown
  const uniqueModels = useMemo(() => {
    const modelSet = new Set<string>();
    entries.forEach(entry => {
      if (entry.model) modelSet.add(entry.model);
    });
    return Array.from(modelSet).sort();
  }, [entries]);

  // Filter and sort entries
  const filteredEntries = useMemo(() => {
    let result = entries.filter(entry => {
      // Model filter
      if (selectedModel !== "all" && entry.model !== selectedModel) {
        return false;
      }
      // Date filter
      if (!isWithinDateFilter(entry.updatedAt, dateFilter)) {
        return false;
      }
      // Search filter (client-side for immediate feedback)
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const matchesSearch =
          entry.name.toLowerCase().includes(query) ||
          (entry.project && entry.project.toLowerCase().includes(query)) ||
          (entry.model && entry.model.toLowerCase().includes(query));
        if (!matchesSearch) return false;
      }
      return true;
    });

    // Sort
    result.sort((a, b) => {
      let comparison = 0;
      switch (sortField) {
        case "updated":
          comparison = Number(b.updatedAt?.seconds || 0) - Number(a.updatedAt?.seconds || 0);
          break;
        case "created":
          comparison = Number(b.createdAt?.seconds || 0) - Number(a.createdAt?.seconds || 0);
          break;
        case "messages":
          comparison = b.messageCount - a.messageCount;
          break;
        case "name":
          comparison = a.name.localeCompare(b.name);
          break;
      }
      return sortOrder === "desc" ? comparison : -comparison;
    });

    return result;
  }, [entries, selectedModel, dateFilter, searchQuery, sortField, sortOrder]);

  // Group entries
  const groupedEntries = useMemo(() => {
    if (groupingStrategy === HistoryGroupingStrategy.None) {
      return [{ groupKey: "all", displayName: "All Entries", entries: filteredEntries }];
    }

    const groups = new Map<string, ClaudeHistoryEntry[]>();

    filteredEntries.forEach(entry => {
      let groupKey: string;
      switch (groupingStrategy) {
        case HistoryGroupingStrategy.Date:
          groupKey = getDateGroup(entry.updatedAt);
          break;
        case HistoryGroupingStrategy.Project:
          groupKey = entry.project || "No Project";
          break;
        case HistoryGroupingStrategy.Model:
          groupKey = entry.model || "Unknown Model";
          break;
        default:
          groupKey = "all";
      }

      if (!groups.has(groupKey)) {
        groups.set(groupKey, []);
      }
      groups.get(groupKey)!.push(entry);
    });

    // Sort groups (Date groups have specific order)
    const dateOrder = ["Today", "Yesterday", "This Week", "This Month", "Older", "Unknown"];
    const sortedGroups = Array.from(groups.entries()).sort(([a], [b]) => {
      if (groupingStrategy === HistoryGroupingStrategy.Date) {
        return dateOrder.indexOf(a) - dateOrder.indexOf(b);
      }
      return a.localeCompare(b);
    });

    return sortedGroups.map(([key, entries]) => ({
      groupKey: key,
      displayName: key,
      entries,
    }));
  }, [filteredEntries, groupingStrategy]);

  // Flatten for keyboard navigation
  const flatEntries = useMemo(() => {
    return groupedEntries.flatMap(g => g.entries);
  }, [groupedEntries]);

  // Filter messages in modal
  const filteredMessages = useMemo(() => {
    if (!messageSearchQuery) return messages;
    const query = messageSearchQuery.toLowerCase();
    return messages.filter(msg =>
      msg.content.toLowerCase().includes(query)
    );
  }, [messages, messageSearchQuery]);

  // Check if any filters are active
  const hasActiveFilters = searchQuery || selectedModel !== "all" || dateFilter !== "all";

  // ============================================================================
  // Event Handlers
  // ============================================================================

  const selectEntry = useCallback((entry: ClaudeHistoryEntry, index: number) => {
    setSelectedIndex(index);
    loadEntryDetail(entry.id);
  }, [loadEntryDetail]);

  const clearFilters = useCallback(() => {
    setSearchQuery("");
    setSelectedModel("all");
    setDateFilter("all");
  }, []);

  const cycleGroupingStrategy = useCallback(() => {
    const strategies = Object.values(HistoryGroupingStrategy);
    const currentIndex = strategies.indexOf(groupingStrategy);
    const nextIndex = (currentIndex + 1) % strategies.length;
    setGroupingStrategy(strategies[nextIndex]);
  }, [groupingStrategy]);

  // Handle clicking on a full-text search result
  const handleSearchResultClick = useCallback((result: SearchResultItem) => {
    // Find the entry in the loaded entries or load it directly
    const existingEntry = entries.find(e => e.id === result.sessionId);
    if (existingEntry) {
      const index = flatEntries.indexOf(existingEntry);
      setSelectedIndex(index >= 0 ? index : 0);
      loadEntryDetail(existingEntry.id);
    } else {
      // Entry not in current list, load it directly
      loadEntryDetail(result.sessionId);
    }
    // Switch back to metadata mode to show the entry list
    setSearchMode("metadata");
  }, [entries, flatEntries, loadEntryDetail]);

  const handleCopyId = useCallback(async (id: string) => {
    try {
      await navigator.clipboard.writeText(id);
      // Could add a toast notification here
    } catch (err) {
      console.error("Failed to copy ID:", err);
    }
  }, []);

  const handleExportEntry = useCallback(async (entry: ClaudeHistoryEntry) => {
    if (!clientRef.current) return;
    try {
      const response = await clientRef.current.getClaudeHistoryMessages({ id: entry.id });
      const exportData = {
        name: entry.name,
        id: entry.id,
        project: entry.project,
        model: entry.model,
        messageCount: entry.messageCount,
        createdAt: entry.createdAt ? new Date(Number(entry.createdAt.seconds) * 1000).toISOString() : null,
        updatedAt: entry.updatedAt ? new Date(Number(entry.updatedAt.seconds) * 1000).toISOString() : null,
        messages: response.messages.map((msg: ClaudeMessage) => ({
          role: msg.role,
          content: msg.content,
          timestamp: msg.timestamp ? new Date(Number(msg.timestamp.seconds) * 1000).toISOString() : null,
          model: msg.model,
        })),
      };

      const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${entry.name.replace(/[^a-z0-9]/gi, "_")}_${entry.id.substring(0, 8)}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(`Failed to export: ${err}`);
    }
  }, []);

  const handleResumeSession = useCallback(async (entry: ClaudeHistoryEntry) => {
    if (!clientRef.current || !entry.project) {
      setError("Cannot resume: Project path is required");
      return;
    }

    try {
      setResuming(true);
      setError(null);

      // Generate a session title from the history entry
      const sessionTitle = `Resumed: ${entry.name}`.substring(0, 50);

      // Create a new session with the resume_id set to the history entry ID
      const response = await clientRef.current.createSession({
        title: sessionTitle,
        path: entry.project,
        resumeId: entry.id,
        category: "Resumed",
      });

      if (response.session) {
        // Navigate to the sessions page to see the new session
        router.push("/");
      }
    } catch (err) {
      setError(`Failed to resume session: ${err}`);
    } finally {
      setResuming(false);
    }
  }, [router]);

  // ============================================================================
  // Keyboard Navigation
  // ============================================================================

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't handle if in input (except for specific keys)
      const isInInput = document.activeElement?.tagName === "INPUT" ||
                        document.activeElement?.tagName === "TEXTAREA";

      // Modal-specific shortcuts
      if (showMessages) {
        if (e.key === "Escape") {
          e.preventDefault();
          setShowMessages(false);
          return;
        }
        return; // Don't process other shortcuts when modal is open
      }

      // Search shortcut (works even when in input via Escape)
      if (e.key === "/" && !isInInput) {
        e.preventDefault();
        searchInputRef.current?.focus();
        return;
      }

      // Escape clears search or selection
      if (e.key === "Escape") {
        if (isInInput) {
          (document.activeElement as HTMLElement)?.blur();
          return;
        }
        if (searchQuery) {
          setSearchQuery("");
          return;
        }
      }

      // Skip other shortcuts if in input
      if (isInInput) return;

      // Navigation
      if (e.key === "ArrowDown" || e.key === "j") {
        e.preventDefault();
        const newIndex = Math.min(selectedIndex + 1, flatEntries.length - 1);
        if (newIndex >= 0 && flatEntries[newIndex]) {
          selectEntry(flatEntries[newIndex], newIndex);
        }
        return;
      }

      if (e.key === "ArrowUp" || e.key === "k") {
        e.preventDefault();
        const newIndex = Math.max(selectedIndex - 1, 0);
        if (flatEntries[newIndex]) {
          selectEntry(flatEntries[newIndex], newIndex);
        }
        return;
      }

      // Enter to view messages
      if (e.key === "Enter" && selectedEntry) {
        e.preventDefault();
        loadMessages(selectedEntry.id);
        return;
      }

      // G to cycle grouping
      if (e.key === "g" || e.key === "G") {
        e.preventDefault();
        cycleGroupingStrategy();
        return;
      }

      // ? for help (could show shortcuts modal)
      if (e.key === "?") {
        e.preventDefault();
        // Could implement help modal
        return;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [showMessages, searchQuery, selectedIndex, selectedEntry, flatEntries, selectEntry, loadMessages, cycleGroupingStrategy]);

  // Focus trap for modal
  useEffect(() => {
    if (showMessages && modalRef.current) {
      const focusableElements = modalRef.current.querySelectorAll(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      const firstElement = focusableElements[0] as HTMLElement;
      const lastElement = focusableElements[focusableElements.length - 1] as HTMLElement;

      const handleTabKey = (e: KeyboardEvent) => {
        if (e.key !== "Tab") return;

        if (e.shiftKey) {
          if (document.activeElement === firstElement) {
            e.preventDefault();
            lastElement?.focus();
          }
        } else {
          if (document.activeElement === lastElement) {
            e.preventDefault();
            firstElement?.focus();
          }
        }
      };

      firstElement?.focus();
      window.addEventListener("keydown", handleTabKey);
      return () => window.removeEventListener("keydown", handleTabKey);
    }
  }, [showMessages]);

  // ============================================================================
  // Render
  // ============================================================================

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>
          📚 Claude History Browser
        </h1>
        <div className={styles.groupingIndicator}>
          📊 {GroupingStrategyLabels[groupingStrategy]}
          <span className={styles.shortcutHint}>(Press G to cycle)</span>
        </div>
      </div>

      {/* Error Banner with Retry */}
      {error && (
        <div className={styles.errorBanner}>
          <div className={styles.errorContent}>
            <span className={styles.errorIcon}>⚠️</span>
            <div>
              <div className={styles.errorTitle}>Error</div>
              <div className="text-muted">{error}</div>
            </div>
          </div>
          <button
            onClick={() => loadHistory(searchQuery || undefined)}
            className="btn btn-secondary btn-sm"
          >
            Retry
          </button>
          <button
            onClick={() => setError(null)}
            className="btn btn-ghost btn-sm"
            aria-label="Dismiss error"
          >
            ✕
          </button>
        </div>
      )}

      {/* Filter Bar */}
      <div className={styles.filterBar}>
        {/* Search Mode Toggle */}
        <div className={styles.searchModeToggle}>
          <button
            className={`${styles.searchModeButton} ${searchMode === "metadata" ? styles.active : ""}`}
            onClick={() => setSearchMode("metadata")}
            title="Search by name, project, model"
          >
            📋 Metadata
          </button>
          <button
            className={`${styles.searchModeButton} ${searchMode === "fulltext" ? styles.active : ""}`}
            onClick={() => setSearchMode("fulltext")}
            title="Search full conversation content"
          >
            🔍 Full-Text
          </button>
        </div>

        {/* Search - Conditional based on mode */}
        {searchMode === "metadata" ? (
          <div className={styles.searchContainer}>
            <input
              ref={searchInputRef}
              type="text"
              placeholder="Search history... (Press /)"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSearch()}
              className={styles.searchInput}
            />
            <button
              onClick={handleSearch}
              disabled={searching}
              className={`btn btn-primary ${styles.searchButton}`}
            >
              {searching ? (
                <>
                  <span className={styles.spinnerSmall} />
                  Searching...
                </>
              ) : (
                "Search"
              )}
            </button>
            {(searchQuery || hasActiveFilters) && (
              <button
                onClick={clearFilters}
                className="btn btn-secondary"
              >
                Clear
              </button>
            )}
          </div>
        ) : (
          <div className={styles.searchContainer}>
            <HistorySearchInput
              value={fullTextSearch.query}
              onChange={fullTextSearch.setQuery}
              onSubmit={(value: string) => fullTextSearch.search({ query: value })}
              loading={fullTextSearch.loading}
              placeholder="Search conversation content..."
              className={styles.fullTextSearchInput}
            />
          </div>
        )}

        {/* Filters */}
        <div className={styles.filters}>
          <select
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            className={styles.select}
          >
            <option value="all">All Models</option>
            {uniqueModels.map(model => (
              <option key={model} value={model}>{model}</option>
            ))}
          </select>

          <select
            value={dateFilter}
            onChange={(e) => setDateFilter(e.target.value as DateFilter)}
            className={styles.select}
          >
            <option value="all">All Time</option>
            <option value="today">Today</option>
            <option value="week">This Week</option>
            <option value="month">This Month</option>
          </select>

          <select
            value={sortField}
            onChange={(e) => setSortField(e.target.value as SortField)}
            className={styles.select}
          >
            <option value="updated">Sort: Last Updated</option>
            <option value="created">Sort: Created Date</option>
            <option value="messages">Sort: Message Count</option>
            <option value="name">Sort: Name</option>
          </select>

          <button
            onClick={() => setSortOrder(sortOrder === "asc" ? "desc" : "asc")}
            className={styles.sortOrderButton}
            aria-label={`Sort ${sortOrder === "asc" ? "descending" : "ascending"}`}
            title={sortOrder === "asc" ? "Ascending" : "Descending"}
          >
            {sortOrder === "asc" ? "↑" : "↓"}
          </button>

          <select
            value={groupingStrategy}
            onChange={(e) => setGroupingStrategy(e.target.value as HistoryGroupingStrategy)}
            className={styles.select}
            title="Group by (Keyboard: G)"
          >
            {Object.entries(GroupingStrategyLabels).map(([value, label]) => (
              <option key={value} value={value}>Group: {label}</option>
            ))}
          </select>
        </div>
      </div>

      <div className={styles.content}>
        {/* Entry List or Full-Text Search Results */}
        <div className={styles.entryList} ref={entryListRef}>
          {searchMode === "fulltext" ? (
            <>
              <h2 className={styles.sectionTitle}>
                Full-Text Search
              </h2>
              <HistorySearchResults
                results={fullTextSearch.results}
                totalMatches={fullTextSearch.totalMatches}
                queryTimeMs={fullTextSearch.queryTimeMs}
                hasMore={fullTextSearch.hasMore}
                loading={fullTextSearch.loading}
                error={fullTextSearch.error}
                query={fullTextSearch.query}
                onResultClick={handleSearchResultClick}
                onLoadMore={fullTextSearch.loadMore}
              />
            </>
          ) : (
            <>
              <h2 className={styles.sectionTitle}>
                History ({filteredEntries.length} of {entries.length} entries)
              </h2>

              {loading ? (
            <div className={styles.loadingContainer}>
              <div className="spinner" />
              <div className={styles.loadingTitle}>Loading Claude History...</div>
              <div className="text-muted" style={{ fontSize: "14px" }}>
                {entries.length === 0
                  ? "This may take a few moments on first load..."
                  : "Refreshing..."}
              </div>
            </div>
          ) : filteredEntries.length === 0 ? (
            // Empty State
            <div className={styles.emptyStateContainer}>
              {hasActiveFilters ? (
                <>
                  <div className={styles.emptyStateIcon}>🔍</div>
                  <h3 className={styles.emptyStateTitle}>No results found</h3>
                  <p className="text-muted">
                    Try adjusting your filters or{" "}
                    <button onClick={clearFilters} className={styles.linkButton}>
                      clear all filters
                    </button>
                  </p>
                </>
              ) : entries.length === 0 ? (
                <>
                  <div className={styles.emptyStateIcon}>📚</div>
                  <h3 className={styles.emptyStateTitle}>No conversation history yet</h3>
                  <p className="text-muted">
                    Your Claude conversation history will appear here once you start using sessions.
                  </p>
                </>
              ) : (
                <>
                  <div className={styles.emptyStateIcon}>📭</div>
                  <h3 className={styles.emptyStateTitle}>No entries match your criteria</h3>
                  <p className="text-muted">
                    Adjust your filters to see more results.
                  </p>
                </>
              )}
            </div>
          ) : (
            // Grouped Entry List
            <div className={styles.entryCards}>
              {groupedEntries.map(({ groupKey, displayName, entries: groupEntries }) => (
                <div key={groupKey} className={styles.categoryGroup}>
                  {groupingStrategy !== HistoryGroupingStrategy.None && (
                    <h3 className={styles.categoryTitle}>
                      {displayName} ({groupEntries.length})
                    </h3>
                  )}
                  <div className={styles.categoryContent}>
                    {groupEntries.map((entry) => {
                      const entryIndex = flatEntries.indexOf(entry);
                      const isSelected = selectedEntry?.id === entry.id;
                      return (
                        <div
                          key={entry.id}
                          onClick={() => selectEntry(entry, entryIndex)}
                          className={`${styles.entryCard} ${isSelected ? styles.selected : ""}`}
                          role="button"
                          tabIndex={0}
                          aria-selected={isSelected}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              selectEntry(entry, entryIndex);
                            }
                          }}
                        >
                          <div className={styles.entryHeader}>
                            <div className={styles.entryName}>{entry.name}</div>
                            <div className={styles.entryTime}>{formatTimeAgo(entry.updatedAt)}</div>
                          </div>
                          <div className={styles.entryMeta}>
                            <span className={styles.entryModel}>{entry.model}</span>
                            <span className={styles.entryDivider}>•</span>
                            <span className={styles.entryMessages}>
                              {entry.messageCount} {entry.messageCount === 1 ? "message" : "messages"}
                            </span>
                          </div>
                          {entry.project && (
                            <div className={styles.entryProject} title={entry.project}>
                              📁 {truncateMiddle(entry.project, 50)}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
            </>
          )}
        </div>

        {/* Detail Panel */}
        <div className={styles.detailPanel}>
          {selectedEntry ? (
            <div>
              <h2 className={styles.sectionTitle}>Entry Details</h2>
              <div className={styles.detailFields}>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Name:</div>
                  <div className="text-primary">{selectedEntry.name}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>ID:</div>
                  <div className={styles.idField}>
                    <code className="text-muted">{selectedEntry.id.substring(0, 8)}...</code>
                    <button
                      onClick={() => handleCopyId(selectedEntry.id)}
                      className={styles.copyButton}
                      title="Copy full ID"
                    >
                      📋
                    </button>
                  </div>
                </div>
                {selectedEntry.project && (
                  <div className={styles.detailField}>
                    <div className={styles.fieldLabel}>Project:</div>
                    <div className={styles.projectPath} title={selectedEntry.project}>
                      {selectedEntry.project}
                    </div>
                  </div>
                )}
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Model:</div>
                  <div className="text-primary">{selectedEntry.model}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Message Count:</div>
                  <div className="text-primary">{selectedEntry.messageCount}</div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Created:</div>
                  <div className="text-secondary" style={{ fontSize: "13px" }}>
                    {formatDate(selectedEntry.createdAt)}
                  </div>
                </div>
                <div className={styles.detailField}>
                  <div className={styles.fieldLabel}>Last Updated:</div>
                  <div className="text-secondary" style={{ fontSize: "13px" }}>
                    {formatDate(selectedEntry.updatedAt)}
                  </div>
                </div>

                {/* Message Preview */}
                <div className={styles.messagePreview}>
                  <div className={styles.previewHeader}>
                    <span className={styles.fieldLabel}>Recent Messages</span>
                    {loadingPreview && <span className="text-muted" style={{ fontSize: "12px" }}>Loading...</span>}
                  </div>
                  {previewMessages.length > 0 ? (
                    <div className={styles.previewMessages}>
                      {previewMessages.map((msg, idx) => (
                        <div
                          key={idx}
                          className={`${styles.previewMessage} ${msg.role === "user" ? styles.userMessage : styles.assistantMessage}`}
                        >
                          <div className={styles.previewRole}>
                            {msg.role === "user" ? "👤" : "🤖"}
                          </div>
                          <div className={styles.previewContent}>
                            {msg.content.length > 200
                              ? msg.content.substring(0, 200) + "..."
                              : msg.content}
                          </div>
                        </div>
                      ))}
                      {selectedEntry.messageCount > 5 && (
                        <button
                          onClick={() => loadMessages(selectedEntry.id)}
                          className={styles.viewMoreButton}
                        >
                          View all {selectedEntry.messageCount} messages →
                        </button>
                      )}
                    </div>
                  ) : !loadingPreview ? (
                    <div className="text-muted" style={{ fontSize: "12px", fontStyle: "italic" }}>
                      No messages available
                    </div>
                  ) : null}
                </div>

                {/* Action Buttons */}
                <div className={styles.detailActions}>
                  <button
                    onClick={() => handleResumeSession(selectedEntry)}
                    disabled={resuming || !selectedEntry.project}
                    className="btn btn-primary"
                    title={selectedEntry.project ? "Start a new session resuming this conversation" : "Cannot resume: No project path"}
                  >
                    {resuming ? "Starting..." : "▶️ Resume Session"}
                  </button>
                  <button
                    onClick={() => loadMessages(selectedEntry.id)}
                    disabled={loadingMessages}
                    className="btn btn-secondary"
                  >
                    {loadingMessages ? "Loading..." : "💬 View Messages"}
                  </button>
                  <button
                    onClick={() => handleExportEntry(selectedEntry)}
                    className="btn btn-secondary"
                    title="Export conversation as JSON"
                  >
                    📥 Export
                  </button>
                  <button
                    onClick={() => handleCopyId(selectedEntry.id)}
                    className="btn btn-secondary"
                    title="Copy conversation ID"
                  >
                    📋 Copy ID
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <div className={styles.emptyState}>
              <div className={styles.emptyStateIcon}>👆</div>
              <p>Select an entry to view details</p>
              <p className="text-muted" style={{ fontSize: "12px", marginTop: "10px" }}>
                Use ↑↓ or j/k to navigate
              </p>
            </div>
          )}
        </div>
      </div>

      {/* Keyboard Shortcuts Hint */}
      <div className={styles.keyboardHints}>
        <span><kbd>/</kbd> Search</span>
        <span><kbd>↑↓</kbd> Navigate</span>
        <span><kbd>Enter</kbd> View Messages</span>
        <span><kbd>G</kbd> Cycle Grouping</span>
        <span><kbd>Esc</kbd> Clear/Close</span>
      </div>

      {/* Messages Modal */}
      {showMessages && (
        <div
          className={styles.modalOverlay}
          onClick={() => setShowMessages(false)}
          role="dialog"
          aria-modal="true"
          aria-labelledby="messages-modal-title"
        >
          <div
            ref={modalRef}
            className={styles.modal}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div className={styles.modalHeader}>
              <h2 id="messages-modal-title" className={styles.modalTitle}>
                Conversation Messages ({filteredMessages.length}
                {messageSearchQuery && ` of ${messages.length}`})
              </h2>
              <div className={styles.messageSearchContainer}>
                <input
                  type="text"
                  placeholder="Search in messages..."
                  value={messageSearchQuery}
                  onChange={(e) => setMessageSearchQuery(e.target.value)}
                  className={styles.messageSearchInput}
                />
                {messageSearchQuery && (
                  <button
                    onClick={() => setMessageSearchQuery("")}
                    className="btn btn-ghost btn-sm"
                  >
                    Clear
                  </button>
                )}
              </div>
              <button
                onClick={() => setShowMessages(false)}
                className={styles.modalCloseButton}
                aria-label="Close messages dialog"
              >
                ✕
              </button>
            </div>

            {/* Messages List */}
            <div className={styles.modalContent}>
              {filteredMessages.length === 0 && messageSearchQuery ? (
                <div className={styles.emptyStateContainer}>
                  <div className={styles.emptyStateIcon}>🔍</div>
                  <h3 className={styles.emptyStateTitle}>No messages match "{messageSearchQuery}"</h3>
                  <button
                    onClick={() => setMessageSearchQuery("")}
                    className={styles.linkButton}
                  >
                    Clear search
                  </button>
                </div>
              ) : (
                filteredMessages.map((msg, idx) => (
                  <div
                    key={idx}
                    className={msg.role === "user" ? styles.messageUser : styles.messageAssistant}
                  >
                    <div className={styles.messageHeader}>
                      <div
                        style={{
                          fontWeight: "600",
                          color: msg.role === "user" ? "var(--primary)" : "var(--text-secondary)",
                          textTransform: "capitalize",
                        }}
                      >
                        {msg.role}
                      </div>
                      <div className="text-muted" style={{ fontSize: "12px" }}>
                        {formatDate(msg.timestamp)}
                      </div>
                    </div>
                    <div className={styles.messageContent}>
                      {msg.content}
                    </div>
                    {msg.model && (
                      <div
                        className="text-muted"
                        style={{
                          fontSize: "11px",
                          marginTop: "8px",
                          fontFamily: "monospace",
                        }}
                      >
                        Model: {msg.model}
                      </div>
                    )}
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
