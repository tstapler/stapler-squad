"use client";

import { useCallback, useMemo, useState } from "react";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { prAttention } from "@/lib/unfinished/prAttention";
import { type PRSortBy } from "@/lib/unfinished/prOrdering";

export type FilterStatus =
  | "all"
  | "ci-failing"
  | "changes-requested"
  | "with-session"
  | "draft"
  | "needs-attention";

export type SortBy = PRSortBy;

export const DEFAULT_FILTER: FilterStatus = "all";
export const DEFAULT_SORT: SortBy = "attention-first";

/** Filters and searches only; ordering is `orderPRs`' job so the list can freeze it. */
export function applyFilter(prs: UserPR[], filter: FilterStatus, search: string): UserPR[] {
  let result = prs;

  if (search.trim()) {
    const q = search.trim().toLowerCase();
    result = result.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.headRef.toLowerCase().includes(q) ||
        `${p.owner}/${p.repo}`.toLowerCase().includes(q) ||
        String(p.number).includes(q)
    );
  }

  switch (filter) {
    case "ci-failing":
      result = result.filter(
        (p) => p.checkConclusion === "failure" || p.checkConclusion === "error"
      );
      break;
    case "changes-requested":
      result = result.filter((p) => p.changesReqCount > 0);
      break;
    case "with-session":
      result = result.filter((p) => p.sessionIds.length > 0 || p.linkedSessions.length > 0);
      break;
    case "draft":
      result = result.filter((p) => p.isDraft);
      break;
    case "needs-attention":
      result = result.filter((p) => prAttention(p).needsAttention);
      break;
  }
  return result;
}

export interface PRListFilters {
  filterStatus: FilterStatus;
  sortBy: SortBy;
  searchQuery: string;
  setFilterStatus: (f: FilterStatus) => void;
  setSortBy: (s: SortBy) => void;
  setSearchQuery: (q: string) => void;
  /** Resets filter, sort and search to their defaults. */
  clear: () => void;
  /** Any of filter, sort or search differs from its default. */
  isActive: boolean;
  apply: (prs: UserPR[]) => UserPR[];
}

/**
 * PR filter/sort/search state, owned by the page (not the PRs panel) so it survives tab
 * switches. Deliberately component state, not URL state: lost on reload and not shareable.
 */
export function usePRListFilters(): PRListFilters {
  const [filterStatus, setFilterStatus] = useState<FilterStatus>(DEFAULT_FILTER);
  const [sortBy, setSortBy] = useState<SortBy>(DEFAULT_SORT);
  const [searchQuery, setSearchQuery] = useState("");
  const clear = useCallback(() => {
    setFilterStatus(DEFAULT_FILTER);
    setSortBy(DEFAULT_SORT);
    setSearchQuery("");
  }, []);
  return useMemo(
    () => ({
      filterStatus,
      sortBy,
      searchQuery,
      setFilterStatus,
      setSortBy,
      setSearchQuery,
      clear,
      isActive:
        filterStatus !== DEFAULT_FILTER || sortBy !== DEFAULT_SORT || searchQuery.trim() !== "",
      apply: (prs) => applyFilter(prs, filterStatus, searchQuery),
    }),
    [filterStatus, sortBy, searchQuery, clear]
  );
}
