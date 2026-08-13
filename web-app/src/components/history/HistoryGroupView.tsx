"use client";

import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { useState, useCallback } from "react";
import { HistoryGroupingStrategy } from "@/lib/hooks/useHistoryFilters";
import type { HistoryGroup } from "@/lib/hooks/useHistoryGrouping";
import { HistoryEntryCard } from "./HistoryEntryCard";
import * as styles from "./HistoryGroupView.css";

interface HistoryGroupViewProps {
  groupedEntries: HistoryGroup[];
  flatEntries: ClaudeHistoryEntry[];
  selectedEntry: ClaudeHistoryEntry | null;
  enrichedEntry?: ClaudeHistoryEntry | null;
  loading: boolean;
  entriesCount: number;
  filteredCount: number;
  hasActiveFilters: boolean;
  groupingStrategy: HistoryGroupingStrategy;
  onSelectEntry: (entry: ClaudeHistoryEntry, index: number) => void;
  onClearFilters: () => void;
  fetchMessages: (id: string) => Promise<ClaudeMessage[]>;
}

export function HistoryGroupView({
  groupedEntries,
  flatEntries,
  selectedEntry,
  enrichedEntry,
  loading,
  entriesCount,
  filteredCount,
  hasActiveFilters,
  groupingStrategy,
  onSelectEntry,
  onClearFilters,
  fetchMessages,
}: HistoryGroupViewProps) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); } else { next.add(id); }
      return next;
    });
  }, []);

  if (loading) {
    return (
      <div className={styles.loadingContainer}>
        <div className="spinner" />
        <div className={styles.loadingTitle}>Loading Claude History...</div>
        <div className="text-muted" style={{ fontSize: "14px" }}>
          {entriesCount === 0
            ? "This may take a few moments on first load..."
            : "Refreshing..."}
        </div>
      </div>
    );
  }

  if (filteredCount === 0) {
    return (
      <div className={styles.emptyStateContainer}>
        {hasActiveFilters ? (
          <>
            <div className={styles.emptyStateIcon}>🔍</div>
            <h3 className={styles.emptyStateTitle}>No results found</h3>
            <p className="text-muted">
              Try adjusting your filters or{" "}
              <button onClick={onClearFilters} className={styles.linkButton}>
                clear all filters
              </button>
            </p>
          </>
        ) : entriesCount === 0 ? (
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
            <p className="text-muted">Adjust your filters to see more results.</p>
          </>
        )}
      </div>
    );
  }

  return (
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
                <HistoryEntryCard
                  key={entry.id}
                  entry={entry}
                  isSelected={isSelected}
                  enrichedEntry={isSelected ? enrichedEntry : undefined}
                  isExpanded={expandedIds.has(entry.id)}
                  onToggleExpand={handleToggleExpand}
                  onSelect={() => onSelectEntry(entry, entryIndex)}
                  fetchMessages={fetchMessages}
                />
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
