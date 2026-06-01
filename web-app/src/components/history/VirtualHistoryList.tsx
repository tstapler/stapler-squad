"use client";

import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useCallback, useEffect, useRef, useState } from "react";
import { HistoryGroupingStrategy } from "@/lib/hooks/useHistoryFilters";
import type { HistoryGroup } from "@/lib/hooks/useHistoryGrouping";
import { HistoryEntryCard } from "./HistoryEntryCard";
import * as styles from "./HistoryGroupView.css";

type VirtualItem =
  | { type: "header"; groupKey: string; displayName: string; count: number }
  | { type: "card"; entry: ClaudeHistoryEntry; flatIndex: number };

interface VirtualHistoryListProps {
  groupedEntries: HistoryGroup[];
  flatEntries: ClaudeHistoryEntry[];
  selectedEntry: ClaudeHistoryEntry | null;
  enrichedEntry?: ClaudeHistoryEntry | null;
  loading: boolean;
  entriesCount: number;
  filteredCount: number;
  hasActiveFilters: boolean;
  groupingStrategy: HistoryGroupingStrategy;
  hasNextPage: boolean;
  loadingMore: boolean;
  onSelectEntry: (entry: ClaudeHistoryEntry, index: number) => void;
  onClearFilters: () => void;
  onLoadMore: () => void;
  fetchMessages: (id: string) => Promise<ClaudeMessage[]>;
  /** Ref used by the parent's keyboard handler to call scrollToIndex */
  virtualizerRef?: React.MutableRefObject<{ scrollToIndex: (i: number) => void } | null>;
}

export function VirtualHistoryList({
  groupedEntries,
  flatEntries,
  selectedEntry,
  enrichedEntry,
  loading,
  entriesCount,
  filteredCount,
  hasActiveFilters,
  groupingStrategy,
  hasNextPage,
  loadingMore,
  onSelectEntry,
  onClearFilters,
  onLoadMore,
  fetchMessages,
  virtualizerRef,
}: VirtualHistoryListProps) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const scrollRef = useRef<HTMLDivElement>(null);
  // Guard: prevent scroll-anchor correction during expand animation.
  const adjustRef = useRef(true);

  const handleToggleExpand = useCallback((id: string) => {
    adjustRef.current = false;
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); } else { next.add(id); }
      return next;
    });
    requestAnimationFrame(() => { adjustRef.current = true; });
  }, []);

  // Build flat virtual items array from groups.
  const items: VirtualItem[] = [];
  for (const { groupKey, displayName, entries } of groupedEntries) {
    if (groupingStrategy !== HistoryGroupingStrategy.None) {
      items.push({ type: "header", groupKey, displayName, count: entries.length });
    }
    for (const entry of entries) {
      items.push({ type: "card", entry, flatIndex: flatEntries.indexOf(entry) });
    }
  }
  // Sentinel item for load-more trigger.
  if (hasNextPage) items.push({ type: "header", groupKey: "__sentinel__", displayName: "", count: 0 });

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 88,
    overscan: 8,
    measureElement: (el) => el.getBoundingClientRect().height,
  });

  // Expose scrollToIndex so the parent keyboard handler can drive it.
  useEffect(() => {
    if (virtualizerRef) {
      virtualizerRef.current = { scrollToIndex: (i) => virtualizer.scrollToIndex(i, { align: "auto" }) };
    }
  }, [virtualizer, virtualizerRef]);

  // Infinite scroll: observe the sentinel item.
  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!sentinelRef.current || !hasNextPage || loadingMore) return;
    const observer = new IntersectionObserver(
      (entries) => { if (entries[0]?.isIntersecting) onLoadMore(); },
      { root: scrollRef.current, threshold: 0 }
    );
    observer.observe(sentinelRef.current);
    return () => observer.disconnect();
  }, [hasNextPage, loadingMore, onLoadMore]);

  if (loading) {
    return (
      <div className={styles.loadingContainer}>
        <div className="spinner" />
        <div className={styles.loadingTitle}>Loading Claude History...</div>
        <div className="text-muted" style={{ fontSize: "14px" }}>
          {entriesCount === 0 ? "This may take a few moments on first load..." : "Refreshing..."}
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
              <button onClick={onClearFilters} className={styles.linkButton}>clear all filters</button>
            </p>
          </>
        ) : entriesCount === 0 ? (
          <>
            <div className={styles.emptyStateIcon}>📚</div>
            <h3 className={styles.emptyStateTitle}>No conversation history yet</h3>
            <p className="text-muted">Start a Claude session to see history here.</p>
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
    <div ref={scrollRef} style={{ overflowY: "auto", height: "100%", maxHeight: "calc(var(--viewport-height, 100dvh) - 280px)" }}>
      <div style={{ height: `${virtualizer.getTotalSize()}px`, position: "relative" }}>
        {virtualizer.getVirtualItems().map((vItem) => {
          const item = items[vItem.index];
          return (
            <div
              key={vItem.key}
              data-index={vItem.index}
              ref={virtualizer.measureElement}
              style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${vItem.start}px)` }}
            >
              {item.type === "header" ? (
                item.groupKey === "__sentinel__" ? (
                  <div ref={sentinelRef} style={{ height: "1px" }} aria-hidden="true">
                    {loadingMore && <div className="text-muted" style={{ textAlign: "center", padding: "8px", fontSize: "13px" }}>Loading more…</div>}
                  </div>
                ) : (
                  <h3 className={styles.categoryTitle} style={{ padding: "8px 0 4px" }}>
                    {item.displayName} ({item.count})
                  </h3>
                )
              ) : (
                <div style={{ paddingBottom: "6px" }}>
                  <HistoryEntryCard
                    entry={item.entry}
                    isSelected={selectedEntry?.id === item.entry.id}
                    enrichedEntry={selectedEntry?.id === item.entry.id ? enrichedEntry : undefined}
                    isExpanded={expandedIds.has(item.entry.id)}
                    onToggleExpand={handleToggleExpand}
                    onSelect={() => onSelectEntry(item.entry, item.flatIndex)}
                    fetchMessages={fetchMessages}
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
