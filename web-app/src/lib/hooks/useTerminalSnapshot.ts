"use client";

import { useState, useEffect, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";

interface SnapshotCacheEntry {
  html: string;
  isEmpty: boolean;
  timestamp: number;
}

// Module-level cache shared across all card instances with the same sessionId.
// Prevents thundering-herd on mount when the session list renders 20+ cards at once.
const snapshotCache = new Map<string, SnapshotCacheEntry>();
const CACHE_TTL_MS = 5_000;
const LAST_N_LINES = 20;

function getCached(sessionId: string): SnapshotCacheEntry | null {
  const entry = snapshotCache.get(sessionId);
  if (entry && Date.now() - entry.timestamp < CACHE_TTL_MS) return entry;
  return null;
}

/** Render ANSI escape sequences as HTML spans. Falls back to plain text on error. */
function renderAnsi(raw: string): string {
  try {
    // Dynamically require to avoid server-side import issues (ansi-to-html is browser-only).
    const Convert = require("ansi-to-html");
    const convert = new Convert({ escapeXML: true });
    return convert.toHtml(raw);
  } catch {
    // Plain text fallback — escape HTML entities to prevent XSS
    return raw
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }
}

export interface TerminalSnapshotResult {
  html: string;
  isEmpty: boolean;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

/**
 * Fetches and caches the last N lines of terminal output for a session card preview.
 * - Does not block card render (async on mount)
 * - 5-second TTL shared across all cards for the same session
 * - ANSI escape codes rendered as HTML; plain text fallback on failure
 */
export function useTerminalSnapshot(
  sessionId: string,
  enabled = true
): TerminalSnapshotResult {
  const hit = getCached(sessionId);
  const [html, setHtml] = useState<string>(hit?.html ?? "");
  const [isEmpty, setIsEmpty] = useState<boolean>(hit?.isEmpty ?? false);
  const [loading, setLoading] = useState<boolean>(!hit && enabled);
  const [error, setError] = useState<string | null>(null);

  const fetch = useCallback(
    async (skipCache = false) => {
      if (!enabled) return;

      if (!skipCache) {
        const cached = getCached(sessionId);
        if (cached) {
          setHtml(cached.html);
          setIsEmpty(cached.isEmpty);
          setLoading(false);
          return;
        }
      }

      setLoading(true);
      setError(null);

      try {
        const client = createClient(
          SessionService,
          createConnectTransport({
            baseUrl: getApiBaseUrl(),
            interceptors: [createAuthInterceptor()],
          })
        );
        const response = await client.getTerminalSnapshot({
          sessionId,
          lastNLines: LAST_N_LINES,
        });

        const renderedHtml = response.isEmpty ? "" : renderAnsi(response.content);
        const entry: SnapshotCacheEntry = {
          html: renderedHtml,
          isEmpty: response.isEmpty,
          timestamp: Date.now(),
        };
        snapshotCache.set(sessionId, entry);
        setHtml(renderedHtml);
        setIsEmpty(response.isEmpty);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load snapshot");
      } finally {
        setLoading(false);
      }
    },
    [sessionId, enabled]
  );

  useEffect(() => {
    if (!enabled) return;
    fetch();
    const interval = setInterval(() => fetch(true), CACHE_TTL_MS);
    return () => clearInterval(interval);
  }, [fetch, enabled]);

  return { html, isEmpty, loading, error, refetch: () => fetch(true) };
}
