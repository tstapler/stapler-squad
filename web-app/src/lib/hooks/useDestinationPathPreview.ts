"use client";

import { useState, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

const DEBOUNCE_MS = 300;
const REQUEST_TIMEOUT_MS = 3_000;

export interface DestinationPathPreviewParams {
  mode: "github_url" | "new_worktree";
  input: string;
  repoPath?: string;
  sessionName?: string;
}

interface UseDestinationPathPreviewOptions {
  enabled?: boolean;
}

interface DestinationPathPreviewResult {
  path: string | null;
  isExact: boolean;
  isLoading: boolean;
  error: string | null;
}

/**
 * Fetches a live preview of where a session's checkout/worktree would be created,
 * without performing any git or filesystem mutation. Mirrors useWorktreeSuggestions'
 * debounce + AbortController + generation-counter pattern so stale responses (from a
 * fast-typing user) never clobber a newer one.
 */
export function useDestinationPathPreview(
  params: DestinationPathPreviewParams | null,
  options: UseDestinationPathPreviewOptions = {}
): DestinationPathPreviewResult {
  const { enabled = true } = options;
  const [path, setPath] = useState<string | null>(null);
  const [isExact, setIsExact] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const generationRef = useRef(0);

  const mode = params?.mode;
  const input = params?.input ?? "";
  const repoPath = params?.repoPath ?? "";
  const sessionName = params?.sessionName ?? "";

  useEffect(() => {
    const paramsMissing =
      !mode ||
      (mode === "github_url" && !input.trim()) ||
      (mode === "new_worktree" && (!repoPath.trim() || !sessionName.trim()));

    if (!enabled || paramsMissing) {
      setPath(null);
      setIsExact(false);
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
        .previewDestinationPath(
          { input, mode, repoPath, sessionName },
          { signal: abortController.signal }
        )
        .then((response) => {
          if (generation !== generationRef.current) return;
          if (response.unresolvedReason) {
            setPath(null);
            setIsExact(false);
          } else {
            setPath(response.path);
            setIsExact(response.isExact);
          }
          setIsLoading(false);
        })
        .catch((err) => {
          if (generation !== generationRef.current) return;
          if (abortController.signal.aborted) {
            setError("Destination path preview timed out");
          } else {
            setError(
              err instanceof Error ? err.message : "Failed to preview destination path"
            );
          }
          setPath(null);
          setIsExact(false);
          setIsLoading(false);
        })
        .finally(() => clearTimeout(timeoutTimer));
    }, DEBOUNCE_MS);

    return () => {
      clearTimeout(debounceTimer);
      abortController.abort();
    };
  }, [mode, input, repoPath, sessionName, enabled]);

  return { path, isExact, isLoading, error };
}
