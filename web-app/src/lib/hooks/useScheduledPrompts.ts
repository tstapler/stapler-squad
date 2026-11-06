"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { PeerHelpInfo, WakeTriggerInfo } from "@/gen/session/v1/wake_trigger_pb";
import { listPeerHelpRequests, listWakeTriggers } from "@/lib/api/wakeTriggerApi";

export const SCHEDULED_POLL_MS = 30_000;

export interface ScheduledPromptsState {
  triggers: readonly WakeTriggerInfo[];
  peers: readonly PeerHelpInfo[];
  /** The server's kill-switch state: false means agent_wake_triggers is off and nothing fires. */
  enabled: boolean;
  /** A first fetch is in flight and nothing has loaded yet. */
  loading: boolean;
  /** The latest fetch failed; the lists keep the last good data. */
  failed: boolean;
  lastUpdatedAt: number | null;
  /** Re-reads now; resolves to the server's kill-switch state, or null when the read failed. */
  refresh: () => Promise<boolean | null>;
}

/**
 * Polls ListWakeTriggers and ListPeerHelpRequests together, once at once and then every
 * `intervalMs`, only while `active` (the Background segment is showing).
 */
export function useScheduledPrompts(active: boolean, intervalMs = SCHEDULED_POLL_MS): ScheduledPromptsState {
  const [triggers, setTriggers] = useState<readonly WakeTriggerInfo[]>([]);
  const [peers, setPeers] = useState<readonly PeerHelpInfo[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const loadedOnce = useRef(false);

  const fetchOnce = useCallback(async (): Promise<boolean | null> => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    if (!loadedOnce.current) setLoading(true);
    try {
      const [wakes, help] = await Promise.all([
        listWakeTriggers(controller.signal),
        listPeerHelpRequests(controller.signal),
      ]);
      if (controller.signal.aborted) return null;
      setTriggers(wakes.triggers);
      setEnabled(wakes.enabled);
      setPeers(help.requests);
      setFailed(false);
      setLastUpdatedAt(Date.now());
      loadedOnce.current = true;
      return wakes.enabled;
    } catch {
      if (!controller.signal.aborted) setFailed(true);
      return null;
    } finally {
      if (controllerRef.current === controller) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!active) return;
    void fetchOnce();
    const timer = setInterval(() => void fetchOnce(), intervalMs);
    return () => {
      clearInterval(timer);
      controllerRef.current?.abort();
    };
  }, [active, fetchOnce, intervalMs]);

  return { triggers, peers, enabled, loading, failed, lastUpdatedAt, refresh: fetchOnce };
}
