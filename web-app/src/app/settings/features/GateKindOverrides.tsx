"use client";

import { useRef } from "react";
import { loadGateStats, useGateStats } from "./useGateStats";
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

function countsOf(raw: { eventsByKind24h?: Record<string, bigint | number> } | null): Record<string, number> | null {
  if (!raw?.eventsByKind24h) return null;
  return Object.fromEntries(Object.entries(raw.eventsByKind24h).map(([k, v]) => [k, Number(v)]));
}

const MODES: ReadonlyArray<{ mode: OverrideMode; label: string }> = [
  { mode: "inherit", label: "Inherit" },
  { mode: "on", label: "On" },
  { mode: "off", label: "Off" },
];

/** Effective value and where it comes from: "On - override" or "Off - from global". */
export function effectiveText(scopes: Record<string, boolean> | undefined, kind: string, globalEnabled: boolean): string {
  const override = scopes?.[`kind:${kind}`];
  if (override !== undefined) return `${override ? "On" : "Off"} - override`;
  return `${globalEnabled ? "On" : "Off"} - from global`;
}

export function modeOf(scopes: Record<string, boolean> | undefined, kind: string): OverrideMode {
  const v = scopes?.[`kind:${kind}`];
  return v === undefined ? "inherit" : v ? "on" : "off";
}

interface GateKindOverridesProps {
  /** Explicit per-scope values as the server read them back. */
  scopes?: Record<string, boolean>;
  /** The global value, so a kind without an override can say what it inherits (FG-6). */
  globalEnabled?: boolean;
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
  globalEnabled,
  onChange,
  onReset,
  fetchCounts,
  pollMs = OVERRIDE_POLL_MS,
}: GateKindOverridesProps) {
  // Without a seam this reads the same shared poller as GateStatusLine (one request per tick).
  const raw = useGateStats<unknown>((fetchCounts ?? loadGateStats) as () => Promise<unknown>, pollMs);
  const counts = fetchCounts ? (raw as Record<string, number> | null) : countsOf(raw as Parameters<typeof countsOf>[0]);
  const radioRefs = useRef<Record<string, Array<HTMLButtonElement | null>>>({});

  const onRadioKey = (e: React.KeyboardEvent, kind: string, index: number, current: OverrideMode) => {
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (step === 0) return;
    e.preventDefault();
    const next = (index + step + MODES.length) % MODES.length;
    radioRefs.current[kind]?.[next]?.focus();
    if (MODES[next].mode !== current) onChange(`kind:${kind}`, MODES[next].mode);
  };

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
              {globalEnabled !== undefined && (
                <span data-testid={`gate-override-effective-${kind}`}> {effectiveText(scopes, kind, globalEnabled)}</span>
              )}
            </span>
            <div className={overrideSegments} role="radiogroup" aria-label={`Gate override for ${kind} sessions`}>
              {MODES.map(({ mode, label }, index) => (
                <button
                  key={mode}
                  ref={(el) => {
                    (radioRefs.current[kind] ??= [])[index] = el;
                  }}
                  type="button"
                  role="radio"
                  aria-checked={current === mode}
                  tabIndex={current === mode ? 0 : -1}
                  onKeyDown={(e) => onRadioKey(e, kind, index, current)}
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
