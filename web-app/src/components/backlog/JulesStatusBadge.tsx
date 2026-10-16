"use client";
// +feature: jules-status-badge

import { useEffect, useRef, useState } from "react";
import {
  badge,
  phaseQueued,
  phaseRunning,
  phaseNeedsReview,
  phaseDone,
  phaseFailed,
  phaseReconnectRequired,
  icon,
  secondaryText,
  updateKeyLink,
  webLink,
  ariaLiveRegion,
} from "./JulesStatusBadge.css";

// JulesStatusBadge shows a Jules cloud session's own reported status
// (Story 3.3.1) — the only signal available for work that runs on Google's
// infrastructure, not stapler-squad's tmux/PTY layer. Structure copied from
// RemoteConnectionIndicator.tsx: null-until-known, a persistent
// aria-live="polite" region for routine transitions, and a separate
// role="alert" region reserved for the one state that should interrupt
// (Failed).
export type JulesSessionPhase =
  | "queued"
  | "running"
  | "needs-review"
  | "done"
  | "failed"
  | "reconnect-required";

const PHASE_LABEL: Record<JulesSessionPhase, string> = {
  queued: "Jules: Queued",
  running: "Jules: Running",
  "needs-review": "Jules: Needs Review",
  done: "Jules: Done",
  failed: "Jules: Failed",
  "reconnect-required": "Jules: Reconnect required",
};

// Aria-hidden glyph per phase — distinguishes phases by shape/content, not
// color alone (ux.md §4's "no color-only signal" requirement, plan.md Task
// 3.3.1a/validation.md's "never color-alone" criterion).
const PHASE_ICON: Record<JulesSessionPhase, string> = {
  queued: "⏳",
  running: "↻",
  "needs-review": "👁",
  done: "✓",
  failed: "✕",
  "reconnect-required": "🔑",
};

const PHASE_CLASS: Record<JulesSessionPhase, string> = {
  queued: phaseQueued,
  running: phaseRunning,
  "needs-review": phaseNeedsReview,
  done: phaseDone,
  failed: phaseFailed,
  "reconnect-required": phaseReconnectRequired,
};

// Polite announcement text for every phase except "failed", which is
// announced through the separate role="alert" region instead (assertive) —
// mirrors RemoteConnectionIndicator's POLITE_ANNOUNCE/ALERT_ANNOUNCE split.
const POLITE_ANNOUNCE: Partial<Record<JulesSessionPhase, string>> = {
  queued: "Jules session queued",
  running: "Jules session running",
  "needs-review": "Jules session needs review",
  done: "Jules session done",
  "reconnect-required": "Jules: Reconnect required — update your API key",
};

const ALERT_ANNOUNCE = "Jules session failed";

// Staleness text (ux.md §4.2 step 5) only makes sense while the session is
// still open and actively being polled — a poller hiccup must never be
// confused with the terminal Done/Failed states or the account-wide
// Reconnect-required condition, which already has its own explanation.
const STALE_PHASES: ReadonlySet<JulesSessionPhase> = new Set(["queued", "running", "needs-review"]);

function minutesAgo(lastPolledAt: Date | string): number {
  const ts = typeof lastPolledAt === "string" ? new Date(lastPolledAt) : lastPolledAt;
  return Math.max(0, Math.floor((Date.now() - ts.getTime()) / 60000));
}

export interface JulesStatusBadgeProps {
  /** Undefined until a real phase value is known — renders nothing (§4.2 step 1). */
  phase: JulesSessionPhase | undefined;
  /** Timestamp of the most recent successful (or attempted) poll tick. */
  lastPolledAt?: Date | string;
  /** False when the poller is stale (unreachable/rate-limited) but the last known phase is preserved. */
  pollHealthy?: boolean;
  /** Link to the session on jules.google.com — the escape hatch to deeper diagnostics (ux.md §4.2 step 3). */
  julesWebUrl?: string;
}

export function JulesStatusBadge({ phase, lastPolledAt, pollHealthy = true, julesWebUrl }: JulesStatusBadgeProps) {
  const prevPhaseRef = useRef<JulesSessionPhase | undefined>(phase);
  const [politeAnnouncement, setPoliteAnnouncement] = useState("");

  useEffect(() => {
    if (prevPhaseRef.current !== phase) {
      const text = phase ? POLITE_ANNOUNCE[phase] : undefined;
      if (text) setPoliteAnnouncement(text);
      prevPhaseRef.current = phase;
    }
  }, [phase]);

  // No real state known yet — never render a neutral/optimistic placeholder
  // chip (validation.md's "nothing renders before a real state is known").
  if (!phase) {
    return null;
  }

  const label = PHASE_LABEL[phase];
  const showStale = !pollHealthy && !!lastPolledAt && STALE_PHASES.has(phase);

  return (
    <>
      <div className={ariaLiveRegion} role="status" aria-live="polite" aria-atomic="true">
        {politeAnnouncement}
      </div>
      <span className={`${badge} ${PHASE_CLASS[phase]}`} role="img" aria-label={label} data-testid="jules-status-badge">
        <span className={icon} aria-hidden="true" data-testid="jules-status-icon">
          {PHASE_ICON[phase]}
        </span>
        <span>{label}</span>
      </span>
      {showStale && lastPolledAt && (
        <span className={secondaryText} data-testid="jules-status-stale">
          {`Last updated ${minutesAgo(lastPolledAt)}m ago, retrying…`}
        </span>
      )}
      {phase === "reconnect-required" && (
        <a className={updateKeyLink} href="/settings/jules">
          Update key
        </a>
      )}
      {/* Visually hidden — fires an assertive (role="alert") announcement
          distinct from the polite region above, only for the one state that
          should interrupt the user (ux.md §4.2 step 4). */}
      {phase === "failed" && (
        <span role="alert" className={ariaLiveRegion}>
          {ALERT_ANNOUNCE}
        </span>
      )}
      {julesWebUrl && (
        <a className={webLink} href={julesWebUrl} target="_blank" rel="noreferrer">
          View this session on jules.google.com
        </a>
      )}
    </>
  );
}
