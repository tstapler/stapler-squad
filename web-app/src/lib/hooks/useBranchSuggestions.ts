"use client";

import { useState, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { AsyncResult } from "@/lib/types/asyncResult";

interface BranchSuggestionsOptions {
  repositoryPath?: string;
  baseUrl?: string;
}

interface BranchSuggestionsResult extends AsyncResult {
  suggestions: string[];
  /** @deprecated Use `loading` instead (AsyncResult-compatible field). */
  isLoading: boolean;
}

/**
 * Hook to provide git branch suggestions from the real git refs of the selected repo.
 * Calls ListBranches RPC when repositoryPath changes. Cancels in-flight requests on path change.
 * Returns { suggestions, loading, error } — implements AsyncResult.
 */
export function useBranchSuggestions(options: BranchSuggestionsOptions = {}): BranchSuggestionsResult {
  const { repositoryPath } = options;
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  useEffect(() => {
    // Cancel any in-flight request from a previous repositoryPath.
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    if (!repositoryPath) {
      setSuggestions([]);
      setLoading(false);
      setError(null);
      return;
    }

    const controller = new AbortController();
    abortControllerRef.current = controller;

    // Create client once per effect invocation (not per retry).
    const client = createClient(SessionService, getConnectTransport());

    const fetchBranches = async () => {
      setLoading(true);
      setError(null);
      setSuggestions([]);

      try {
        const response = await client.listBranches(
          { repoPath: repositoryPath },
          { signal: controller.signal }
        );

        if (!controller.signal.aborted) {
          setSuggestions(response.branches ?? []);
        }
      } catch (err: unknown) {
        if (controller.signal.aborted) {
          return; // Request was cancelled — ignore
        }
        console.error("Failed to fetch branch suggestions:", err);
        setError(err instanceof Error ? err : new Error(String(err)));
        setSuggestions([]);
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      }
    };

    fetchBranches();

    return () => {
      controller.abort();
    };
  }, [repositoryPath]);

  return { suggestions, loading, error, isLoading: loading };
}
