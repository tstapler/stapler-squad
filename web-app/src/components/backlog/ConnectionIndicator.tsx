"use client";
// +feature: backlog:connection-indicator

import type { BacklogConnectionState } from "@/lib/hooks/useWatchBacklogItems";
import { dots, label, wrapper } from "./ConnectionIndicator.css";

interface ConnectionIndicatorProps {
  connectionState: BacklogConnectionState;
}

// Epic 6.2 (Task 6.2.1b) — small persistent connection-state affordance,
// mounted wherever useWatchBacklogItems is active (ux.md §5, plan.md Domain
// Glossary "ConnectionIndicator"). Deliberately a distinct component from
// web-app/src/components/layout/ConnectionIndicator.tsx: that one is
// WatchSessions-specific (reads sessionsSlice via Redux, offers a
// click-to-reconnect affordance and only has 3 states). This one is a pure
// presentational display driven entirely by the connectionState prop
// useWatchBacklogItems already returns — no store coupling of its own, and
// no reconnect action, since the backlog hook self-heals via its own
// exponential-backoff/backstop timers (plan.md Task 5.4.1a's discovery pass
// confirmed there is no existing session-level indicator to reuse instead).
//
// ux.md §5 UX AC #17 asks for "exactly one of three distinguishable states"
// (connected/reconnecting/degraded) — written before Story 4.2.3 (pre-mortem
// P1 #1: idle-staleness backstop) added a 5th BacklogConnectionState value,
// "stale". Per this epic's task brief, "stale" intentionally gets a visually
// distinct treatment from "reconnecting" rather than being silently folded
// into it: the whole point of the pre-mortem fix was making a
// self-healing-but-currently-not-updating state visible to the user, not
// hiding it behind the same label/animation as an actively-retrying stream.
// "connecting" (the hook's initial pre-first-connect state) shares
// "reconnecting"'s spinner treatment — there is no meaningful difference for
// the user between "hasn't connected yet" and "lost connection, retrying."
const STATE_LABEL: Record<BacklogConnectionState, string> = {
  connecting: "Connecting…",
  live: "Live",
  reconnecting: "Reconnecting…",
  stale: "Stale — reconnecting…",
  polling: "Polling (every 30s)",
};

const STATE_ARIA_LABEL: Record<BacklogConnectionState, string> = {
  connecting: "Connecting to live backlog updates",
  live: "Live — backlog data is up to date",
  reconnecting: "Reconnecting to live backlog updates",
  stale: "Live backlog updates stalled — reconnecting",
  polling: "Live backlog updates unavailable — refreshing every 30 seconds",
};

export function ConnectionIndicator({ connectionState }: ConnectionIndicatorProps) {
  return (
    <div
      className={wrapper}
      role="status"
      aria-live="polite"
      aria-atomic="true"
      aria-label={STATE_ARIA_LABEL[connectionState]}
      data-testid="connection-indicator"
    >
      <span className={dots[connectionState]} aria-hidden="true" />
      <span className={label}>{STATE_LABEL[connectionState]}</span>
    </div>
  );
}
