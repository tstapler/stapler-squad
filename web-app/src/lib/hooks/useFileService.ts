"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { ListFilesResponse, GetFileContentResponse, SearchFilesResponse } from "@/gen/session/v1/session_pb";
import type { FileNode } from "@/gen/session/v1/types_pb";

export type { FileNode, ListFilesResponse, GetFileContentResponse, SearchFilesResponse };

// ---- File content cache ----

interface FileContentCacheEntry {
  data: GetFileContentResponse;
  timestamp: number;
}

const fileContentCache = new Map<string, FileContentCacheEntry>();
const FILE_CONTENT_CACHE_TTL_MS = 30_000;

/** Drops a cached read so the next fetch hits the server (reload after an on-disk change). */
export function invalidateFileContent(sessionId: string, filePath: string): void {
  fileContentCache.delete(`${sessionId}:${filePath}`);
}

interface UseGetFileContentResult {
  data: GetFileContentResponse | null;
  loading: boolean;
  error: string | null;
}

/**
 * createFileClient creates a ConnectRPC client for the SessionService.
 * Extracted as a helper so both hooks share the same pattern.
 */
function createFileClient(_baseUrl: string) {
  return createClient(SessionService, getConnectTransport());
}

/**
 * fetchDirectoryFiles is a standalone async function for listing files.
 * Components can call this directly or via the hook.
 */
export async function fetchDirectoryFiles(
  sessionId: string,
  path: string,
  includeIgnored: boolean,
  baseUrl: string
): Promise<ListFilesResponse> {
  const client = createFileClient(baseUrl);
  return await client.listFiles({
    sessionId,
    path: path || ".",
    includeIgnored,
  });
}

/**
 * searchFiles calls the SearchFiles RPC and returns matching files with full paths.
 */
export async function searchFiles(
  sessionId: string,
  query: string,
  includeIgnored: boolean,
  baseUrl: string
): Promise<SearchFilesResponse> {
  const client = createFileClient(baseUrl);
  return await client.searchFiles({
    sessionId,
    query,
    includeIgnored,
    maxResults: 500,
  });
}

/**
 * fetchFileContent is a standalone async function for fetching file content.
 */
export async function fetchFileContent(
  sessionId: string,
  path: string,
  baseUrl: string
): Promise<GetFileContentResponse> {
  const client = createFileClient(baseUrl);
  return await client.getFileContent({ sessionId, path });
}

/**
 * useGetFileContent fetches the content of a file whenever filePath changes.
 * Returns null data while loading or if filePath is null.
 */
export function useGetFileContent(
  sessionId: string,
  filePath: string | null,
  baseUrl: string,
  /** Bump to refetch the same file; pair it with `invalidateFileContent`. */
  reloadKey = 0
): UseGetFileContentResult {
  // Tagged with its source so a render for a new path never sees the previous file's content (the effect below
  // only replaces it after that render commits).
  const [loaded, setLoaded] = useState<{ key: string; data: GetFileContentResponse } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Track which path we're fetching to avoid stale updates.
  const requestIdRef = useRef(0);
  const setData = useCallback(
    (next: GetFileContentResponse | null, key = "") => setLoaded(next ? { key, data: next } : null),
    []
  );

  useEffect(() => {
    // Bump on every run, including the early returns, so an in-flight fetch for the previous file goes stale.
    const requestId = ++requestIdRef.current;
    if (!sessionId || !filePath) {
      setData(null);
      setLoading(false);
      setError(null);
      return;
    }

    // Check module-level cache first – avoids re-fetching on tab re-entry.
    const cacheKey = `${sessionId}:${filePath}`;
    const cached = fileContentCache.get(cacheKey);
    if (cached && Date.now() - cached.timestamp < FILE_CONTENT_CACHE_TTL_MS) {
      setData(cached.data, cacheKey);
      setLoading(false);
      setError(null);
      return;
    }

    setLoading(true);
    setError(null);
    setData(null);

    fetchFileContent(sessionId, filePath, baseUrl)
      .then((response) => {
        if (requestId === requestIdRef.current) {
          fileContentCache.set(cacheKey, { data: response, timestamp: Date.now() });
          setData(response, cacheKey);
        }
      })
      .catch((err) => {
        if (requestId === requestIdRef.current) {
          setError(err instanceof Error ? err.message : "Failed to load file content");
          console.error("Error fetching file content:", err);
        }
      })
      .finally(() => {
        if (requestId === requestIdRef.current) {
          setLoading(false);
        }
      });
  }, [sessionId, filePath, baseUrl, reloadKey, setData]);

  const data = loaded?.key === `${sessionId}:${filePath}` ? loaded.data : null;
  return { data, loading, error };
}
