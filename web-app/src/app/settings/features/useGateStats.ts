"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { GateStatsLike } from "./GateStatusLine";

export const GATE_STATS_POLL_MS = 30_000;

type Load<T> = () => Promise<T>;

interface Poller {
  listeners: Set<(value: unknown) => void>;
  last: { value: unknown } | undefined;
  timer: ReturnType<typeof setInterval>;
}

// One poller per load function: every component on the page that reads the same
// stats shares one interval and one in-flight request instead of polling separately.
const pollers = new Map<Load<unknown>, Poller>();

/** The production source; a module-level function so all callers share its poller. */
export const loadGateStats: Load<GateStatsLike & { eventsByKind24h: Record<string, bigint | number> }> = async () => {
  const client = createClient(SessionService, getConnectTransport());
  return client.getDeliveryGateStats({});
};

function subscribe(load: Load<unknown>, pollMs: number, listener: (value: unknown) => void): () => void {
  let poller = pollers.get(load);
  if (!poller) {
    const created: Poller = { listeners: new Set(), last: undefined, timer: undefined as never };
    const tick = async () => {
      let value: unknown = null;
      try {
        value = await load();
      } catch {
        // A failed poll renders as "no data"; the next tick retries.
      }
      created.last = { value };
      created.listeners.forEach((l) => l(value));
    };
    created.timer = setInterval(() => void tick(), pollMs);
    pollers.set(load, created);
    void tick();
    poller = created;
  }
  poller.listeners.add(listener);
  if (poller.last) listener(poller.last.value);
  return () => {
    poller.listeners.delete(listener);
    if (poller.listeners.size === 0) {
      clearInterval(poller.timer);
      pollers.delete(load);
    }
  };
}

/** Latest result of `load`, polled every `pollMs` while any subscriber is mounted; null before the first result or after a failure. */
export function useGateStats<T>(load: Load<T>, pollMs: number = GATE_STATS_POLL_MS): T | null {
  const [value, setValue] = useState<T | null>(null);
  useEffect(() => subscribe(load, pollMs, (v) => setValue(v as T | null)), [load, pollMs]);
  return value;
}
