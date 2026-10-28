import { getAllLeaves } from "@/lib/pane/paneUtils";
import type { NamedWindow, WindowId } from "./windowTypes";

/** Windows kept mounted while hidden: the current one plus the two most recent. */
export const MAX_RETAINED_WINDOWS = 3;

const sessionIdsOf = (w: NamedWindow): Set<string> =>
  new Set(
    getAllLeaves(w.paneState.root)
      .map((leaf) => leaf.sessionId)
      .filter((id): id is string => !!id),
  );

/**
 * Next retained-window list, oldest first and ending with `currentId`. Drops windows that no longer exist,
 * evicts any retained window sharing a session with the current one (a session is docked once, so one host
 * element never moves between anchors), then evicts the oldest until at most `max` remain.
 * Returns `prev` itself when nothing changed.
 */
export function nextRetainedWindows(
  prev: readonly WindowId[],
  currentId: WindowId,
  windows: readonly NamedWindow[],
  max: number = MAX_RETAINED_WINDOWS,
): readonly WindowId[] {
  const byId = new Map(windows.map((w) => [w.id, w]));
  const current = byId.get(currentId);
  if (!current) return prev;
  const currentSessions = sessionIdsOf(current);
  const survivors = prev.filter((id) => {
    if (id === currentId || !byId.has(id)) return false;
    const other = sessionIdsOf(byId.get(id) as NamedWindow);
    return ![...other].some((s) => currentSessions.has(s));
  });
  const next = [...survivors, currentId].slice(-max);
  const unchanged = next.length === prev.length && next.every((id, i) => id === prev[i]);
  return unchanged ? prev : next;
}
