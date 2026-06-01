"use client";

import { RefObject } from "react";
import { HistorySearchInput } from "@/components/history/HistorySearchInput";
import {
  GroupingStrategyLabels,
  HistoryGroupingStrategy,
} from "@/lib/hooks/useHistoryFilters";
import type {
  SortField,
  DateFilter,
  SearchMode,
} from "@/lib/hooks/useHistoryFilters";
import type { useHistoryFullTextSearch } from "@/lib/hooks/useHistoryFullTextSearch";
import { ActionBar } from "@/components/ui/ActionBar";
import * as styles from "./HistoryFilterBar.css";

// ============================================================================
// Types
// ============================================================================

interface HistoryFilterBarProps {
  // Filter state
  searchQuery: string;
  branchFilter: string;
  selectedModel: string;
  dateFilter: DateFilter;
  sortField: SortField;
  sortOrder: "asc" | "desc";
  groupingStrategy: HistoryGroupingStrategy;
  searchMode: SearchMode;

  // Setters
  setSearchQuery: (value: string) => void;
  setBranchFilter: (value: string) => void;
  setSelectedModel: (value: string) => void;
  setDateFilter: (value: DateFilter) => void;
  setSortField: (value: SortField) => void;
  setSortOrder: (value: "asc" | "desc") => void;
  setGroupingStrategy: (value: HistoryGroupingStrategy) => void;
  setSearchMode: (value: SearchMode) => void;

  // Derived
  uniqueModels: string[];
  hasActiveFilters: boolean;

  // Search
  searching: boolean;
  onSearch: () => void;
  onClearFilters: () => void;
  searchInputRef: RefObject<HTMLInputElement | null>;

  // Full-text search
  fullTextSearch: ReturnType<typeof useHistoryFullTextSearch>;
}

// ============================================================================
// Component
// ============================================================================

export function HistoryFilterBar({
  searchQuery,
  branchFilter,
  selectedModel,
  dateFilter,
  sortField,
  sortOrder,
  groupingStrategy,
  searchMode,
  setSearchQuery,
  setBranchFilter,
  setSelectedModel,
  setDateFilter,
  setSortField,
  setSortOrder,
  setGroupingStrategy,
  setSearchMode,
  uniqueModels,
  hasActiveFilters,
  searching,
  onSearch,
  onClearFilters,
  searchInputRef,
  fullTextSearch,
}: HistoryFilterBarProps) {
  return (
    <div className={styles.filterBar}>
      {/* Search Mode Toggle */}
      <div className={styles.searchModeToggle}>
        <button
          className={`${styles.searchModeButton} ${searchMode === "metadata" ? styles.searchModeButtonActive : ""}`}
          onClick={() => setSearchMode("metadata")}
          title="Search by name, project, model"
        >
          📋 Metadata
        </button>
        <button
          className={`${styles.searchModeButton} ${searchMode === "fulltext" ? styles.searchModeButtonActive : ""}`}
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
            onKeyDown={(e) => e.key === "Enter" && onSearch()}
            className={styles.searchInput}
          />
          <button
            onClick={onSearch}
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
              onClick={onClearFilters}
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
      <ActionBar scroll compact gap="sm" className={styles.filters}>
        <input
          type="text"
          placeholder="⎇ Branch…"
          value={branchFilter}
          onChange={(e) => setBranchFilter(e.target.value)}
          className={styles.select}
          style={{ minWidth: "100px" }}
          aria-label="Filter by branch"
        />
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
      </ActionBar>
    </div>
  );
}
