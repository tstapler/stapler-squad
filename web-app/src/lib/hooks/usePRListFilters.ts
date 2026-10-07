"use client";

import { useMemo, useState } from "react";
import { type UserPR } from "@/gen/session/v1/types_pb";

export type FilterStatus =
  | "all"
  | "ci-failing"
  | "changes-requested"
  | "with-session"
  | "draft";

export type SortBy = "updated-desc" | "updated-asc" | "repo" | "ci-status";


export function applyFilterSort(
  prs: UserPR[],
  filter: FilterStatus,
  sort: SortBy,
  search: string
): UserPR[] {
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
      result = result.filter((p) => p.sessionIds.length > 0);
      break;
    case "draft":
      result = result.filter((p) => p.isDraft);
      break;
  }

  const sorted = [...result];
  switch (sort) {
    case "updated-desc":
      sorted.sort(
        (a, b) =>
          Number(b.updatedAt?.seconds ?? 0n) - Number(a.updatedAt?.seconds ?? 0n)
      );
      break;
    case "updated-asc":
      sorted.sort(
        (a, b) =>
          Number(a.updatedAt?.seconds ?? 0n) - Number(b.updatedAt?.seconds ?? 0n)
      );
      break;
    case "repo":
      sorted.sort((a, b) =>
        `${a.owner}/${a.repo}`.localeCompare(`${b.owner}/${b.repo}`)
      );
      break;
    case "ci-status": {
      const rank = (p: UserPR) => {
        if (p.checkConclusion === "failure" || p.checkConclusion === "error") return 0;
        if (p.changesReqCount > 0) return 1;
        if (p.checkConclusion === "success") return 3;
        return 2;
      };
      sorted.sort((a, b) => rank(a) - rank(b));
      break;
    }
  }
  return sorted;
}

export interface PRListFilters {
  filterStatus: FilterStatus;
  sortBy: SortBy;
  searchQuery: string;
  setFilterStatus: (f: FilterStatus) => void;
  setSortBy: (s: SortBy) => void;
  setSearchQuery: (q: string) => void;
  apply: (prs: UserPR[]) => UserPR[];
}

/**
 * PR filter/sort/search state, owned by the page (not the PRs panel) so it survives tab
 * switches. Deliberately component state, not URL state: lost on reload and not shareable.
 */
export function usePRListFilters(): PRListFilters {
  const [filterStatus, setFilterStatus] = useState<FilterStatus>("all");
  const [sortBy, setSortBy] = useState<SortBy>("updated-desc");
  const [searchQuery, setSearchQuery] = useState("");
  return useMemo(
    () => ({
      filterStatus,
      sortBy,
      searchQuery,
      setFilterStatus,
      setSortBy,
      setSearchQuery,
      apply: (prs) => applyFilterSort(prs, filterStatus, sortBy, searchQuery),
    }),
    [filterStatus, sortBy, searchQuery]
  );
}
