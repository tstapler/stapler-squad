// Sort/group-by-repository helpers for the Backlog table view.

import type { BacklogItem } from "@/lib/hooks/useBacklogService";

/** Sentinel bucket label for items with no repo_path set. Sorts last, mirrors
 * the "No Path" convention in web-app/src/lib/grouping/strategies.ts. */
export const NO_REPOSITORY = "No Repository";

export function compareByRepoPath(a: Pick<BacklogItem, "repoPath">, b: Pick<BacklogItem, "repoPath">): number {
  return (a.repoPath ?? "").localeCompare(b.repoPath ?? "");
}

export interface RepoGroup {
  groupKey: string;
  displayName: string;
  items: BacklogItem[];
}

/** Groups items by repo_path. Items retain the relative order they arrived in
 * (callers should sort first), so sort-then-group composes as expected. */
export function groupByRepoPath(items: BacklogItem[]): RepoGroup[] {
  const grouped = new Map<string, BacklogItem[]>();
  for (const item of items) {
    const key = item.repoPath?.trim() || NO_REPOSITORY;
    const bucket = grouped.get(key);
    if (bucket) {
      bucket.push(item);
    } else {
      grouped.set(key, [item]);
    }
  }

  const keys = Array.from(grouped.keys()).sort((a, b) => {
    if (a === NO_REPOSITORY) return 1;
    if (b === NO_REPOSITORY) return -1;
    return a.localeCompare(b, undefined, { sensitivity: "base" });
  });

  return keys.map((key) => ({
    groupKey: key,
    displayName: key,
    items: grouped.get(key) ?? [],
  }));
}
