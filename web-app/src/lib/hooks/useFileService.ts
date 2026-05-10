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
  baseUrl: string
): UseGetFileContentResult {
  const [data, setData] = useState<GetFileContentResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Track which path we're fetching to avoid stale updates.
  const requestIdRef = useRef(0);

  useEffect(() => {
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
      setData(cached.data);
      setLoading(false);
      setError(null);
      return;
    }

    const requestId = ++requestIdRef.current;
    setLoading(true);
    setError(null);
    setData(null);

    fetchFileContent(sessionId, filePath, baseUrl)
      .then((response) => {
        if (requestId === requestIdRef.current) {
          fileContentCache.set(cacheKey, { data: response, timestamp: Date.now() });
          setData(response);
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
  }, [sessionId, filePath, baseUrl]);

  return { data, loading, error };
}
