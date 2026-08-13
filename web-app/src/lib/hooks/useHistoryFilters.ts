"use client";

import { useState, useEffect, useMemo, useCallback } from "react";
import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { isWithinDateFilter } from "@/lib/utils/timestamp";
import type { DateFilter } from "@/lib/utils/timestamp";

// ============================================================================
// Types and Constants
// ============================================================================

export type SortField = "updated" | "created" | "messages" | "name";
export type SortOrder = "asc" | "desc";
export type SearchMode = "metadata" | "fulltext";
export type { DateFilter };

export enum HistoryGroupingStrategy {
  None = "none",
  Date = "date",
  Project = "project",
  Model = "model",
}

export const GroupingStrategyLabels: Record<HistoryGroupingStrategy, string> = {
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
// Storage Helpers
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

// ============================================================================
// Hook Return Types
// ============================================================================

export interface HistoryFilterState {
  searchQuery: string;
  branchFilter: string;
  selectedModel: string;
  dateFilter: DateFilter;
  sortField: SortField;
  sortOrder: SortOrder;
  groupingStrategy: HistoryGroupingStrategy;
  searchMode: SearchMode;
  isHydrated: boolean;
}

export interface HistoryFilterSetters {
  setSearchQuery: (value: string) => void;
  setBranchFilter: (value: string) => void;
  setSelectedModel: (value: string) => void;
  setDateFilter: (value: DateFilter) => void;
  setSortField: (value: SortField) => void;
  setSortOrder: (value: SortOrder) => void;
  setGroupingStrategy: (value: HistoryGroupingStrategy) => void;
  setSearchMode: (value: SearchMode) => void;
}

export interface HistoryFilterDerived {
  uniqueModels: string[];
  filteredEntries: ClaudeHistoryEntry[];
  hasActiveFilters: boolean;
}

export interface HistoryFilterActions {
  clearFilters: () => void;
  cycleGroupingStrategy: () => void;
}

export interface UseHistoryFiltersReturn {
  filterState: HistoryFilterState;
  setters: HistoryFilterSetters;
  derived: HistoryFilterDerived;
  actions: HistoryFilterActions;
}

// ============================================================================
// Hook
// ============================================================================

export function useHistoryFilters(entries: ClaudeHistoryEntry[]): UseHistoryFiltersReturn {
  // Filter state (persisted) - use defaults initially to avoid hydration mismatch
  const [searchQuery, setSearchQuery] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [selectedModel, setSelectedModel] = useState<string>("all");
  const [dateFilter, setDateFilter] = useState<DateFilter>("all");
  const [sortField, setSortField] = useState<SortField>("updated");
  const [sortOrder, setSortOrder] = useState<SortOrder>("desc");
  const [groupingStrategy, setGroupingStrategy] = useState<HistoryGroupingStrategy>(HistoryGroupingStrategy.Date);
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

  // Persist filter preferences
  useEffect(() => { saveToStorage(STORAGE_KEYS.SEARCH_QUERY, searchQuery); }, [searchQuery]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SELECTED_MODEL, selectedModel); }, [selectedModel]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.DATE_FILTER, dateFilter); }, [dateFilter]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SORT_FIELD, sortField); }, [sortField]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SORT_ORDER, sortOrder); }, [sortOrder]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.GROUPING_STRATEGY, groupingStrategy); }, [groupingStrategy]);
  useEffect(() => { saveToStorage(STORAGE_KEYS.SEARCH_MODE, searchMode); }, [searchMode]);

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
      // Branch filter
      if (branchFilter) {
        const b = (entry.branch || "").toLowerCase();
        if (!b.includes(branchFilter.toLowerCase())) return false;
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

  // Check if any filters are active
  const hasActiveFilters = !!(searchQuery || branchFilter || selectedModel !== "all" || dateFilter !== "all");

  // Actions
  const clearFilters = useCallback(() => {
    setSearchQuery("");
    setBranchFilter("");
    setSelectedModel("all");
    setDateFilter("all");
  }, []);

  const cycleGroupingStrategy = useCallback(() => {
    const strategies = Object.values(HistoryGroupingStrategy);
    const currentIndex = strategies.indexOf(groupingStrategy);
    const nextIndex = (currentIndex + 1) % strategies.length;
    setGroupingStrategy(strategies[nextIndex]);
  }, [groupingStrategy]);

  return {
    filterState: {
      searchQuery,
      branchFilter,
      selectedModel,
      dateFilter,
      sortField,
      sortOrder,
      groupingStrategy,
      searchMode,
      isHydrated,
    },
    setters: {
      setSearchQuery,
      setBranchFilter,
      setSelectedModel,
      setDateFilter,
      setSortField,
      setSortOrder,
      setGroupingStrategy,
      setSearchMode,
    },
    derived: {
      uniqueModels,
      filteredEntries,
      hasActiveFilters,
    },
    actions: {
      clearFilters,
      cycleGroupingStrategy,
    },
  };
}
