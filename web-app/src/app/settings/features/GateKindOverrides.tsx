"use client";

import { useEffect, useMemo, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  flagDescription,
  overrides,
  overridesSummary,
  overrideRow,
  overrideLabel,
  overrideSegments,
  overrideSegment,
  overrideSegmentActive,
  resetButton,
} from "./page.css";

/**
 * The closed set of hidden kinds an override may name, mirroring
 * deliverygate.ScopableKinds. kind:triage is absent on purpose: Spike 1.3h
 * found no producer that creates a triage-tagged hidden session.
 */
export const GATE_OVERRIDE_KINDS = ["review", "diagnose", "other"] as const;

export type OverrideMode = "inherit" | "on" | "off";

export const OVERRIDE_POLL_MS = 30_000;

const MODES: ReadonlyArray<{ mode: OverrideMode; label: string }> = [
  { mode: "inherit", label: "Inherit" },
  { mode: "on", label: "On" },
  { mode: "off", label: "Off" },
];

export function modeOf(scopes: Record<string, boolean> | undefined, kind: string): OverrideMode {
  const v = scopes?.[`kind:${kind}`];
  return v === undefined ? "inherit" : v ? "on" : "off";
}

interface GateKindOverridesProps {
  /** Explicit per-scope values as the server read them back. */
  scopes?: Record<string, boolean>;
  onChange: (scope: string, mode: OverrideMode) => void;
  onReset: () => void;
  /** Test seam; defaults to GetDeliveryGateStats.events_by_kind_24h. */
  fetchCounts?: () => Promise<Record<string, number>>;
  pollMs?: number;
}

/**
 * Per-session-kind overrides of hidden_session_gate: Inherit / On / Off per
 * reachable kind with the last-24h count of events resolved to that kind (so a
 * dead scope is visible), and a global "Reset to default".
 */
export function GateKindOverrides({
  scopes,
  onChange,
  onReset,
  fetchCounts,
  pollMs = OVERRIDE_POLL_MS,
}: GateKindOverridesProps) {
  const [counts, setCounts] = useState<Record<string, number> | null>(null);
  const defaultFetch = useMemo(() => {
    const client = createClient(SessionService, getConnectTransport());
    return async () => {
      const res = await client.getDeliveryGateStats({});
      return Object.fromEntries(Object.entries(res.eventsByKind24h).map(([k, v]) => [k, Number(v)]));
    };
  }, []);
  const load = fetchCounts ?? defaultFetch;

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const c = await load();
        if (!cancelled) setCounts(c);
      } catch {
        if (!cancelled) setCounts(null);
      }
    };
    void tick();
    const id = setInterval(() => void tick(), pollMs);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [load, pollMs]);

  return (
    <details className={overrides} data-testid="gate-kind-overrides">
      <summary className={overridesSummary}>Overrides by session kind</summary>
      {GATE_OVERRIDE_KINDS.map((kind) => {
        const current = modeOf(scopes, kind);
        const count = counts ? (counts[kind] ?? 0) : null;
        return (
          <div key={kind} className={overrideRow} data-testid={`gate-override-${kind}`}>
            <span className={overrideLabel}>
              {kind}
              <span data-testid={`gate-override-count-${kind}`}>
                {count === null ? "" : ` (${count} events in the last 24h)`}
              </span>
            </span>
            <div className={overrideSegments} role="radiogroup" aria-label={`Gate override for ${kind} sessions`}>
              {MODES.map(({ mode, label }) => (
                <button
                  key={mode}
                  type="button"
                  role="radio"
                  aria-checked={current === mode}
                  className={`${overrideSegment} ${current === mode ? overrideSegmentActive : ""}`}
                  onClick={() => current !== mode && onChange(`kind:${kind}`, mode)}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        );
      })}
      <div className={flagDescription}>
        Events from sessions that cannot be resolved to a kind, and events that are not from a session, use the
        global value.
      </div>
      <button type="button" className={resetButton} onClick={onReset} data-testid="gate-reset-default">
        Reset to default
      </button>
    </details>
  );
}
