"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { createClient, type Client } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import AnsiToHtml from "ansi-to-html";
import { SessionService } from "@/gen/session/v1/session_pb";

interface SnapshotCacheEntry {
  html: string;
  isEmpty: boolean;
  timestamp: number;
}

// Module-level cache shared across all card instances with the same sessionId.
// Prevents thundering-herd on mount when the session list renders 20+ cards at once.
const snapshotCache = new Map<string, SnapshotCacheEntry>();
const CACHE_TTL_MS = 5_000;
// Randomize each card's refresh cadence by up to ±20% so N cards' timers don't
// stay phase-locked and fire their refetch in the same synchronized burst forever.
const CACHE_TTL_JITTER_MS = CACHE_TTL_MS * 0.2;
const LAST_N_LINES = 20;

function getCached(sessionId: string): SnapshotCacheEntry | null {
  const entry = snapshotCache.get(sessionId);
  if (entry && Date.now() - entry.timestamp < CACHE_TTL_MS) return entry;
  return null;
}

// Shared client, built once and reused by every card — this hook is
// instantiated per visible session card, polling every 5s.
let sharedClient: Client<typeof SessionService> | null = null;

function getSharedClient(): Client<typeof SessionService> {
  if (!sharedClient) {
    sharedClient = createClient(SessionService, getConnectTransport());
  }
  return sharedClient;
}

// Singleton converter — ansi-to-html's Convert holds no per-call state, so
// constructing a fresh instance (plus a dynamic require()) on every render
// was pure overhead at polling scale.
const ansiConverter = new AnsiToHtml({ escapeXML: true });

/** Render ANSI escape sequences as HTML spans. Falls back to plain text on error. */
function renderAnsi(raw: string): string {
  try {
    return ansiConverter.toHtml(raw);
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
        const client = getSharedClient();
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

  const fetchRef = useRef(fetch);
  fetchRef.current = fetch;

  useEffect(() => {
    if (!enabled) return;

    let timeoutId: ReturnType<typeof setTimeout>;
    const scheduleNext = () => {
      // Jitter each card's next tick independently so N cards mounted in the
      // same render burst don't stay phase-locked and refetch in lockstep forever.
      const jitter = (Math.random() * 2 - 1) * CACHE_TTL_JITTER_MS;
      timeoutId = setTimeout(() => {
        fetchRef.current(true);
        scheduleNext();
      }, CACHE_TTL_MS + jitter);
    };

    fetch();
    scheduleNext();
    return () => clearTimeout(timeoutId);
  }, [fetch, enabled]);

  return { html, isEmpty, loading, error, refetch: () => fetch(true) };
}
