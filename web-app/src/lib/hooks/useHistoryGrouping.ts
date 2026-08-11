"use client";

import { useMemo } from "react";
import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { getDateGroup } from "@/lib/utils/timestamp";
import { HistoryGroupingStrategy } from "@/lib/hooks/useHistoryFilters";

// ============================================================================
// Types
// ============================================================================

export interface HistoryGroup {
  groupKey: string;
  displayName: string;
  entries: ClaudeHistoryEntry[];
}

// ============================================================================
// Hook
// ============================================================================

export function useHistoryGrouping(
  filteredEntries: ClaudeHistoryEntry[],
  groupingStrategy: HistoryGroupingStrategy,
): { groupedEntries: HistoryGroup[]; flatEntries: ClaudeHistoryEntry[] } {
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

  return { groupedEntries, flatEntries };
}
