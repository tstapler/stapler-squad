"use client";

import { useState, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

const DEBOUNCE_MS = 150;
// Above the server's own ListWorktrees request-scoped timeout so a genuine
// server-side timeout error surfaces before this client-side one fires.
const REQUEST_TIMEOUT_MS = 5_000;

interface UseWorktreeSuggestionsOptions {
  baseUrl?: string;
  enabled?: boolean;
}

/**
 * Fetches git worktrees for the given repository path.
 * Used to populate the "Use Existing Worktree" dropdown in the Omnibar.
 *
 * Mirrors usePathCompletions' debounce + AbortController + timeout pattern so a
 * hung backend request surfaces a bounded error instead of loading forever.
 */
export function useWorktreeSuggestions(
  repoPath: string,
  options: UseWorktreeSuggestionsOptions = {}
) {
  const { enabled = true } = options;
  const [worktrees, setWorktrees] = useState<WorktreeEntry[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const generationRef = useRef(0);

  useEffect(() => {
    if (!enabled || !repoPath.trim()) {
      setWorktrees([]);
      setIsLoading(false);
      setError(null);
      return;
    }

    const generation = ++generationRef.current;
    setIsLoading(true);
    setError(null);

    const abortController = new AbortController();

    const debounceTimer = setTimeout(() => {
      const timeoutTimer = setTimeout(
        () => abortController.abort(),
        REQUEST_TIMEOUT_MS
      );

      const client = createClient(SessionService, getConnectTransport());

      client
        .listWorktrees({ repoPath }, { signal: abortController.signal })
        .then((response) => {
          if (generation !== generationRef.current) return;
          setWorktrees(response.worktrees || []);
          setIsLoading(false);
        })
        .catch((err) => {
          if (generation !== generationRef.current) return;
          if (abortController.signal.aborted) {
            setError("Fetching worktrees timed out");
          } else {
            setError(err instanceof Error ? err.message : "Failed to fetch worktrees");
          }
          setWorktrees([]);
          setIsLoading(false);
        })
        .finally(() => clearTimeout(timeoutTimer));
    }, DEBOUNCE_MS);

    return () => {
      clearTimeout(debounceTimer);
      abortController.abort();
    };
  }, [repoPath, enabled]);

  return { worktrees, isLoading, error };
}
