"use client";

import { useEffect, useMemo, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";

/** The slice of GetDeliveryGateStatsResponse the status line reads. */
export interface GateStatsLike {
  sinceProcessStart: ReadonlyArray<{ counter: string; count: bigint | number }>;
  buckets: ReadonlyArray<{
    hourStart?: Timestamp;
    counters: ReadonlyArray<{ counter: string; count: bigint | number }>;
  }>;
}

const DAY_MS = 24 * 60 * 60 * 1000;
export const GATE_STATS_POLL_MS = 30_000;

/** Buckets are hourly, so "last 24h" means buckets that started within 24h. */
function last24hCount(stats: GateStatsLike, counter: string, now: Date): number {
  const cutoff = now.getTime() - DAY_MS;
  let total = 0;
  for (const b of stats.buckets) {
    if (!b.hourStart || timestampDate(b.hourStart).getTime() < cutoff) continue;
    for (const c of b.counters) {
      if (c.counter === counter) total += Number(c.count);
    }
  }
  return total;
}

export function summarizeGateStats(stats: GateStatsLike, now: Date): string {
  const wouldSuppress = last24hCount(stats, "would_suppress", now);
  const unresolved = last24hCount(stats, "unresolved", now);
  const unversioned = stats.sinceProcessStart
    .filter((c) => c.counter === "rpc_unversioned")
    .reduce((n, c) => n + Number(c.count), 0);
  return (
    `Shadow: ${wouldSuppress} hidden events would have been suppressed in the last 24h; ` +
    `${unresolved} unresolved fail-open; ${unversioned} unversioned ssq-notify`
  );
}

interface GateStatusLineProps {
  className?: string;
  /** Test seam; defaults to the SessionService RPC. */
  fetchStats?: () => Promise<GateStatsLike>;
  pollMs?: number;
}

/**
 * The standing reminder under hidden_session_gate: reads GetDeliveryGateStats
 * while the page is open and stops polling when it unmounts. Failures render
 * nothing (the flag row is still usable).
 */
export function GateStatusLine({ className, fetchStats, pollMs = GATE_STATS_POLL_MS }: GateStatusLineProps) {
  const [text, setText] = useState<string | null>(null);
  const defaultFetch = useMemo(() => {
    const client = createClient(SessionService, getConnectTransport());
    return () => client.getDeliveryGateStats({});
  }, []);
  const load = fetchStats ?? defaultFetch;

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const stats = await load();
        if (!cancelled) setText(summarizeGateStats(stats, new Date()));
      } catch {
        if (!cancelled) setText(null);
      }
    };
    void tick();
    const id = setInterval(() => void tick(), pollMs);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [load, pollMs]);

  if (!text) return null;
  return (
    <div className={className} data-testid="gate-status-line">
      {text}
    </div>
  );
}
