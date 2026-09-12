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
export function normalizePath(p: string): string {
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

/**
 * Resolves the still-unresolved candidates and merges each one into `cache`
 * as soon as its own request settles, calling `onChange` per merge — so one
 * slow candidate doesn't hold up surfacing the others that already resolved.
 */
async function resolveUnresolvedPaths(
  candidates: string[],
  cache: Map<string, RepoPathWorktreeInfo>,
  signal: AbortSignal,
  onChange: () => void
): Promise<void> {
  const unresolved = Array.from(new Set(candidates.map(normalizePath))).filter(
    (p) => p && !cache.has(p)
  );
  if (unresolved.length === 0) return;

  const client = createClient(SessionService, getConnectTransport());
  await Promise.all(
    unresolved.map(async (repoPath) => {
      let worktrees: WorktreeEntry[];
      try {
        const result = await client.listWorktrees({ repoPath }, { signal });
        worktrees = result.worktrees || [];
      } catch {
        return;
      }
      if (signal.aborted) return;
      if (mergeWorktreeFamily(cache, worktrees)) {
        onChange();
      }
    })
  );
}

/**
 * `resolutions` is mutated in place (not replaced) as results land, so its
 * reference never changes — `version` is the signal that actually changes,
 * and is what a caller should list in a useMemo dependency array instead.
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
    (signal) =>
      resolveUnresolvedPaths(debounced, cacheRef.current, signal, () =>
        setVersion((v) => v + 1)
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [debouncedKey]
  );

  return { resolutions: cacheRef.current, version };
}
