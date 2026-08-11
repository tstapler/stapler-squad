"use client";

import { useMemo } from "react";
import Fuse, { type IFuseOptions } from "fuse.js";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";

export interface SessionSearchResult {
  session: Session;
  score: number; // 0.0 = perfect match, 1.0 = no match (Fuse.js convention)
  matchedFields: string[]; // which fields contributed to the match
}

// Fuse.js config: multi-field weighted search
// title:0.5 > branch:0.3 > path:0.15 > tags:0.05
// threshold:0.4 — allow moderate fuzziness; tighten if too many false positives
// minMatchCharLength:1 — single character queries return results
const FUSE_OPTIONS: IFuseOptions<Session> = {
  keys: [
    { name: "title", weight: 0.5 },
    { name: "branch", weight: 0.3 },
    { name: "path", weight: 0.15 },
    { name: "tags", weight: 0.05 },
  ],
  includeScore: true,
  includeMatches: true,
  threshold: 0.4,
  minMatchCharLength: 1,
  ignoreLocation: true, // don't penalize matches far from string start
};

/**
 * Client-side fuzzy session search using Fuse.js.
 * Returns ranked results for display in the Omnibar session section.
 * Empty query returns empty array (empty state shows recents instead).
 *
 * Filters out UNSPECIFIED sessions (status=0). All other statuses
 * (RUNNING, READY, LOADING, PAUSED, NEEDS_APPROVAL) are included.
 */
export function useSessionSearch(query: string): SessionSearchResult[] {
  const sessions = useAppSelector(selectAllSessions);

  // Filter to active sessions only (exclude UNSPECIFIED)
  const activeSessions = useMemo(
    () => sessions.filter((s) => s.status !== SessionStatus.UNSPECIFIED),
    [sessions]
  );

  // Rebuild Fuse index when active session list changes
  const fuse = useMemo(
    () => new Fuse(activeSessions, FUSE_OPTIONS),
    [activeSessions]
  );

  return useMemo(() => {
    const trimmed = query.trim();
    if (!trimmed) return [];

    const results = fuse.search(trimmed, { limit: 8 });

    return results.map((r) => ({
      session: r.item,
      score: r.score ?? 1.0,
      matchedFields: r.matches?.map((m) => m.key ?? "") ?? [],
    }));
  }, [query, fuse]);
}
