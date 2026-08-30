"use client";

import { useState, useMemo, useCallback } from "react";
import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { isWithinDateFilter } from "@/lib/utils/timestamp";
import type { DateFilter } from "@/lib/utils/timestamp";
import { usePersistedViewState, type PersistedFieldsConfig } from "@/lib/hooks/usePersistedViewState";

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

// Persisted fields — keys unchanged from the hand-rolled implementation this
// hook used to have, so pre-migration user preferences aren't lost.
// branchFilter intentionally has no entry here: it was never persisted.
interface PersistedHistoryFilters {
  searchQuery: string;
  selectedModel: string;
  dateFilter: DateFilter;
  sortField: SortField;
  sortOrder: SortOrder;
  groupingStrategy: HistoryGroupingStrategy;
  searchMode: SearchMode;
}

const DATE_FILTERS: DateFilter[] = ["all", "today", "week", "month"];
const SORT_FIELDS: SortField[] = ["updated", "created", "messages", "name"];
const SORT_ORDERS: SortOrder[] = ["asc", "desc"];
const SEARCH_MODES: SearchMode[] = ["metadata", "fulltext"];

const HISTORY_FILTER_FIELDS: PersistedFieldsConfig<PersistedHistoryFilters> = {
  searchQuery: { key: "claude-history-search-query", defaultValue: "" },
  selectedModel: { key: "claude-history-selected-model", defaultValue: "all" },
  dateFilter: {
    key: "claude-history-date-filter",
    defaultValue: "all",
    isValid: (v) => DATE_FILTERS.includes(v as DateFilter),
  },
  sortField: {
    key: "claude-history-sort-field",
    defaultValue: "updated",
    isValid: (v) => SORT_FIELDS.includes(v as SortField),
  },
  sortOrder: {
    key: "claude-history-sort-order",
    defaultValue: "desc",
    isValid: (v) => SORT_ORDERS.includes(v as SortOrder),
  },
  groupingStrategy: {
    key: "claude-history-grouping-strategy",
    defaultValue: HistoryGroupingStrategy.Date,
    isValid: (v) => (Object.values(HistoryGroupingStrategy) as string[]).includes(v as string),
  },
  searchMode: {
    key: "claude-history-search-mode",
    defaultValue: "metadata",
    isValid: (v) => SEARCH_MODES.includes(v as SearchMode),
  },
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
  // branchFilter is intentionally not persisted, same as before migration.
  const [branchFilter, setBranchFilter] = useState("");

  const { state, setters, isHydrated } = usePersistedViewState<PersistedHistoryFilters>(HISTORY_FILTER_FIELDS);
  const { searchQuery, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode } = state;
  const {
    searchQuery: setSearchQuery,
    selectedModel: setSelectedModel,
    dateFilter: setDateFilter,
    sortField: setSortField,
    sortOrder: setSortOrder,
    groupingStrategy: setGroupingStrategy,
    searchMode: setSearchMode,
  } = setters;

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
