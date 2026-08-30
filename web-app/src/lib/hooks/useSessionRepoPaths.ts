"use client";

import { useMemo } from "react";
import { useAppSelector } from "@/lib/store";
import { selectActiveSessionsSortedByUpdatedAt } from "@/lib/store/sessionsSlice";

/** Returns deduplicated list of repo root paths from all known sessions. */
export function useSessionRepoPaths(): string[] {
  const sessions = useAppSelector(selectActiveSessionsSortedByUpdatedAt);
  return useMemo(() => {
    const seen = new Set<string>();
    const paths: string[] = [];
    for (const s of sessions) {
      const p = s.path;
      if (p && !seen.has(p)) {
        seen.add(p);
        paths.push(p);
      }
    }
    return paths;
  }, [sessions]);
}
