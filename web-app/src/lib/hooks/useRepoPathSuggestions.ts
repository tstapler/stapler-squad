"use client";

import { useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAbortableEffect } from "@/lib/hooks/useAbortableEffect";

const DEBOUNCE_MS = 150;

export interface RepoPathWorktreeInfo {
  rootPath: string;
  isMain: boolean;
  branch: string;
}

/** Strips a trailing slash so a candidate path matches its ListWorktrees entry regardless of how it was typed/stored. */
function normalizePath(p: string): string {
  return p.length > 1 ? p.replace(/\/+$/, "") : p;
}

/** Merges one candidate path's resolved worktree family into `cache`. Returns true if anything new was added. */
function mergeWorktreeFamily(
  cache: Map<string, RepoPathWorktreeInfo>,
  worktrees: WorktreeEntry[]
): boolean {
  const main = worktrees.find((w) => w.isMain);
  if (!main) return false;

  const rootPath = normalizePath(main.path);
  let changed = false;
  for (const w of worktrees) {
    const wp = normalizePath(w.path);
    if (!cache.has(wp)) {
      cache.set(wp, { rootPath, isMain: w.isMain, branch: w.branch });
      changed = true;
    }
  }
  return changed;
}

/** Resolves the still-unresolved candidates and merges any newly-discovered families into `cache`. Returns true if the cache changed. */
async function resolveUnresolvedPaths(
  candidates: string[],
  cache: Map<string, RepoPathWorktreeInfo>,
  signal: AbortSignal
): Promise<boolean> {
  const unresolved = Array.from(new Set(candidates.map(normalizePath))).filter(
    (p) => p && !cache.has(p)
  );
  if (unresolved.length === 0) return false;

  const client = createClient(SessionService, getConnectTransport());
  const results = await Promise.allSettled(
    unresolved.map((repoPath) => client.listWorktrees({ repoPath }, { signal }))
  );
  if (signal.aborted) return false;

  let changed = false;
  for (const result of results) {
    if (result.status !== "fulfilled") continue;
    if (mergeWorktreeFamily(cache, result.value.worktrees || [])) {
      changed = true;
    }
  }
  return changed;
}

/**
 * Resolves each candidate path's git-worktree family (root + siblings) via
 * SessionService.ListWorktrees, so callers can group a churny worktree path
 * under its primary repo checkout instead of treating every path as an
 * independent, unranked entry.
 *
 * Debounces on the candidate set (mirrors useWorktreeSuggestions), then
 * fetches via useAbortableEffect so a fast-changing candidate list (the user
 * still typing) cancels stale in-flight requests instead of piling them up.
 * A repo's worktree topology doesn't change mid-session, so resolved paths
 * are cached for the component's lifetime rather than re-fetched.
 *
 * An RPC failure or timeout for a given path just leaves it out of the
 * returned map — callers treat an absent entry as "not a worktree" and fall
 * back to today's plain, ungrouped behavior rather than surfacing an error.
 *
 * The returned `resolutions` map is mutated in place (not replaced) as new
 * results land, so its reference never changes — the returned `version`
 * counter is what actually changes, and is what a caller should list in a
 * useMemo dependency array instead of `resolutions` itself.
 */
export interface RepoPathSuggestions {
  resolutions: Map<string, RepoPathWorktreeInfo>;
  version: number;
}

export function useRepoPathSuggestions(paths: string[]): RepoPathSuggestions {
  const [version, setVersion] = useState(0);
  const cacheRef = useRef(new Map<string, RepoPathWorktreeInfo>());
  const [debounced, setDebounced] = useState<string[]>([]);
  const key = paths.join("\n");

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(paths), DEBOUNCE_MS);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  const debouncedKey = debounced.join("\n");

  useAbortableEffect(
    async (signal) => {
      const changed = await resolveUnresolvedPaths(debounced, cacheRef.current, signal);
      if (changed) setVersion((v) => v + 1);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [debouncedKey]
  );

  return { resolutions: cacheRef.current, version };
}
