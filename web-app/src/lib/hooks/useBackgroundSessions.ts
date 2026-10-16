"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { SessionService, ListSessionsRequestSchema } from "@/gen/session/v1/session_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { HiddenSessionRef, HiddenSessionSummary } from "@/lib/utils/backgroundActivity";
import { knownHiddenSessions, rememberHiddenSession } from "@/lib/utils/hiddenSessionRegistry";

export const BACKGROUND_POLL_MS = 30_000;

export interface BackgroundSessionsState {
  sessions: HiddenSessionSummary[];
  /** Hidden sessions seen earlier in this tab that the latest list no longer returns. */
  departed: HiddenSessionRef[];
  /** A first fetch is in flight and nothing has loaded yet. */
  loading: boolean;
  /** The most recent fetch failed; `sessions` keeps the last good data. */
  failed: boolean;
  lastUpdatedAt: number | null;
  refresh: () => void;
}

function stateOf(status: SessionStatus): HiddenSessionSummary["state"] {
  if (status === SessionStatus.ACTIVE || status === SessionStatus.READY) return "running";
  if (status === SessionStatus.STOPPED) return "stopped";
  return "other";
}

/**
 * Polls `ListSessions{hidden_only:true}`: once immediately and then every 30s, only while
 * `enabled` (the Background segment is showing). Disabled means no timer and no request.
 */
export function useBackgroundSessions(enabled: boolean, intervalMs = BACKGROUND_POLL_MS): BackgroundSessionsState {
  const [sessions, setSessions] = useState<HiddenSessionSummary[]>([]);
  const [departed, setDeparted] = useState<HiddenSessionRef[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const loadedOnce = useRef(false);

  const fetchOnce = useCallback(async () => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    if (!loadedOnce.current) setLoading(true);
    try {
      const client = createClient(SessionService, getConnectTransport());
      const res = await client.listSessions(create(ListSessionsRequestSchema, { hiddenOnly: true }), {
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      const next: HiddenSessionSummary[] = res.sessions.map((s) => ({
        id: s.id,
        title: s.title,
        state: stateOf(s.status),
        updatedAtMs: s.updatedAt ? Number(s.updatedAt.seconds) * 1000 : 0,
      }));
      next.forEach(rememberHiddenSession);
      const live = new Set(next.flatMap((s) => [s.id, s.title]));
      setSessions(next);
      setDeparted(knownHiddenSessions().filter((k) => !live.has(k.id) && !live.has(k.title)));
      setFailed(false);
      setLastUpdatedAt(Date.now());
      loadedOnce.current = true;
    } catch {
      if (!controller.signal.aborted) setFailed(true);
    } finally {
      if (controllerRef.current === controller) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void fetchOnce();
    const timer = setInterval(() => void fetchOnce(), intervalMs);
    return () => {
      clearInterval(timer);
      controllerRef.current?.abort();
    };
  }, [enabled, intervalMs, fetchOnce]);

  const refresh = useCallback(() => void fetchOnce(), [fetchOnce]);

  return { sessions, departed, loading, failed, lastUpdatedAt, refresh };
}
