"use client";

import { useState, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { PathEntry } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

const DEBOUNCE_MS = 150;
const CACHE_MAX = 100;
const CACHE_TTL_MS = 30_000;

// ---------------------------------------------------------------------------
// Module-level LRU cache
// JavaScript Map preserves insertion order; we use that for LRU eviction.
// ---------------------------------------------------------------------------

interface CacheEntry {
  entries: PathEntry[];
  baseDir: string;
  baseDirExists: boolean;
  pathExists: boolean;
  expiresAt: number;
}

const completionCache = new Map<string, CacheEntry>();

/** Clears all cached completions. Exposed for testing. */
export function clearCompletionCache(): void {
  completionCache.clear();
}

function getCached(key: string): CacheEntry | null {
  const entry = completionCache.get(key);
  if (!entry) return null;
  if (Date.now() > entry.expiresAt) {
    completionCache.delete(key);
    return null;
  }
  // Refresh LRU position.
  completionCache.delete(key);
  completionCache.set(key, entry);
  return entry;
}

function setCached(key: string, value: CacheEntry): void {
  if (completionCache.size >= CACHE_MAX) {
    const firstKey = completionCache.keys().next().value;
    if (firstKey !== undefined) {
      completionCache.delete(firstKey);
    }
  }
  completionCache.set(key, value);
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export interface PathCompletionResult {
  entries: PathEntry[];
  baseDir: string;
  baseDirExists: boolean;
  pathExists: boolean;
  isLoading: boolean;
  error: string | null;
}

interface UsePathCompletionsOptions {
  baseUrl?: string;
  /** Set to false to disable fetching (e.g. when input type is not a path). */
  enabled?: boolean;
  directoriesOnly?: boolean;
  maxResults?: number;
}

/**
 * Fetches real-time filesystem path completions for a given path prefix.
 *
 * Three-layer protection against redundant requests:
 *  1. 150ms debounce – coalesces rapid keystrokes.
 *  2. AbortController – cancels the in-flight HTTP request when the prefix changes.
 *  3. Generation counter – discards responses that arrive after a newer request fired.
 *
 * Results are cached in a module-level LRU (100 entries, 30s TTL) to avoid
 * redundant RPCs for burst typing that returns to the same prefix.
 */
export function usePathCompletions(
  pathPrefix: string,
  options: UsePathCompletionsOptions = {}
): PathCompletionResult {
  const {
    enabled = true,
    directoriesOnly = true,
    maxResults = 50,
  } = options;

  const [result, setResult] = useState<PathCompletionResult>({
    entries: [],
    baseDir: "",
    baseDirExists: false,
    pathExists: false,
    isLoading: false,
    error: null,
  });

  const generationRef = useRef(0);

  useEffect(() => {
    if (!enabled || pathPrefix === "") {
      setResult((prev) => ({
        ...prev,
        entries: [],
        baseDirExists: false,
        pathExists: false,
        isLoading: false,
        error: null,
      }));
      return;
    }

    const generation = ++generationRef.current;

    // Serve from cache immediately (no loading state needed).
    const cacheKey = `${pathPrefix}::${directoriesOnly}::${maxResults}`;
    const cached = getCached(cacheKey);
    if (cached) {
      setResult({
        entries: cached.entries,
        baseDir: cached.baseDir,
        baseDirExists: cached.baseDirExists,
        pathExists: cached.pathExists,
        isLoading: false,
        error: null,
      });
      return;
    }

    setResult((prev) => ({ ...prev, isLoading: true, error: null }));

    const abortController = new AbortController();

    const debounceTimer = setTimeout(async () => {
      try {
        const client = createClient(SessionService, getConnectTransport());

        const response = await client.listPathCompletions(
          { pathPrefix, directoriesOnly, maxResults },
          { signal: abortController.signal }
        );

        if (generation !== generationRef.current) return;

        const newResult = {
          entries: response.entries,
          baseDir: response.baseDir,
          baseDirExists: response.baseDirExists,
          pathExists: response.pathExists,
        };

        setCached(cacheKey, {
          ...newResult,
          expiresAt: Date.now() + CACHE_TTL_MS,
        });

        setResult({ ...newResult, isLoading: false, error: null });
      } catch (err) {
        if (generation !== generationRef.current) return;
        if (abortController.signal.aborted) return;

        const message =
          err instanceof Error ? err.message : "Failed to fetch completions";
        setResult((prev) => ({ ...prev, isLoading: false, error: message }));
      }
    }, DEBOUNCE_MS);

    return () => {
      clearTimeout(debounceTimer);
      abortController.abort();
    };
  }, [pathPrefix, enabled, directoriesOnly, maxResults]);

  return result;
}
