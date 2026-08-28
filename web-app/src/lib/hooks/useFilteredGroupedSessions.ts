import { useMemo } from "react";
import { type Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { groupSessions, type GroupingStrategy, type GroupedSessions } from "@/lib/grouping/strategies";
import { compareSessionsByCost } from "@/components/sessions/sessionCostSort";

export type SortField = "lastActivity" | "name" | "createdAt" | "updatedAt" | "tokenCost";
export type SortDir = "asc" | "desc";

export interface UseFilteredGroupedSessionsParams {
  sessions: Session[];
  searchQuery: string;
  selectedStatus: SessionStatus | "all";
  selectedCategory: string | "all";
  selectedTag: string | "all";
  hidePaused: boolean;
  showArchived: boolean;
  filterNeedsApproval: boolean;
  /** Optimistically-removed session IDs, excluded from the filtered result. */
  pendingDeleteIds: Set<string>;
  sortField: SortField;
  sortDir: SortDir;
  /** Estimated cost by session ID, used only when sortField === "tokenCost". */
  costById: Map<string, number>;
  groupingStrategy: GroupingStrategy;
  /** Minutes of inactivity after which a session is considered stale, for the "Stale" grouping strategy. */
  staleThresholdMinutes: number;
  /** Bumped on a timer to force stale-session reclassification with no other input change. */
  staleRecomputeTick: number;
}

export interface UseFilteredGroupedSessionsResult {
  filteredSessions: Session[];
  sortedSessions: Session[];
  groupedSessions: GroupedSessions[];
  filteredSessionIds: Set<string>;
}

const getTimestampMs = (ts?: { seconds: bigint; nanos: number }): number => {
  if (!ts || ts.seconds === BigInt(0)) return 0;
  return Number(ts.seconds) * 1000;
};

/**
 * Shared filter -> sort -> group pipeline for session collections. Extracted from
 * SessionList so SessionBoard can reuse identical filtering/search/grouping behavior.
 */
export function useFilteredGroupedSessions({
  sessions,
  searchQuery,
  selectedStatus,
  selectedCategory,
  selectedTag,
  hidePaused,
  showArchived,
  filterNeedsApproval,
  pendingDeleteIds,
  sortField,
  sortDir,
  costById,
  groupingStrategy,
  staleThresholdMinutes,
  staleRecomputeTick,
}: UseFilteredGroupedSessionsParams): UseFilteredGroupedSessionsResult {
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

  const sortedSessions = useMemo(() => {
    const sorted = [...filteredSessions];
    sorted.sort((a, b) => {
      if (sortField === "tokenCost") {
        // compareSessionsByCost already applies sortDir internally (to keep
        // unloaded/unpriced rows last in BOTH directions) — return directly,
        // skipping the shared sortDir flip below.
        return compareSessionsByCost(a, b, costById, sortDir);
      }
      let cmp = 0;
      switch (sortField) {
        case "name":
          cmp = a.title.localeCompare(b.title);
          break;
        case "createdAt":
          cmp = getTimestampMs(a.createdAt) - getTimestampMs(b.createdAt);
          break;
        case "updatedAt":
          cmp = getTimestampMs(a.updatedAt) - getTimestampMs(b.updatedAt);
          break;
        case "lastActivity": {
          const act = (s: Session) => Math.max(
            getTimestampMs(s.lastMeaningfulOutput),
            getTimestampMs(s.lastTerminalUpdate)
          );
          cmp = act(a) - act(b);
          break;
        }
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
    return sorted;
  }, [filteredSessions, sortField, sortDir, costById]);

  const filteredSessionIds = useMemo(
    () => new Set(filteredSessions.map(s => s.id)),
    [filteredSessions]
  );

  // staleRecomputeTick is a dependency purely to force recomputation on the periodic
  // tick a caller may drive — a session can cross the stale threshold with no change
  // to sortedSessions/groupingStrategy, and this is the only way to pick that up.
  const groupedSessionsResult = useMemo(() => {
    return groupSessions(sortedSessions, groupingStrategy, {
      thresholdMinutes: staleThresholdMinutes,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sortedSessions, groupingStrategy, staleThresholdMinutes, staleRecomputeTick]);

  return {
    filteredSessions,
    sortedSessions,
    groupedSessions: groupedSessionsResult,
    filteredSessionIds,
  };
}
