import { useMemo } from "react";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

/**
 * The single source of truth for "the current work session" — the most
 * recent `role === "work"` LinkedSession on an item, or `undefined` if none
 * exists. Extracted verbatim from BacklogItemDetail.tsx's 4 independent
 * inline re-derivations (D3) so they can never drift out of sync.
 */
export function getLatestWorkSession(
  item: Pick<BacklogItem, "linkedSessions"> | null | undefined
): LinkedSession | undefined {
  return [...(item?.linkedSessions ?? [])].reverse().find((s) => s.role === "work");
}

/** Memoized wrapper around getLatestWorkSession, keyed on item.linkedSessions. */
export function useCurrentWorkSession(
  item: Pick<BacklogItem, "linkedSessions"> | null | undefined
): LinkedSession | undefined {
  return useMemo(() => getLatestWorkSession(item), [item]);
}
