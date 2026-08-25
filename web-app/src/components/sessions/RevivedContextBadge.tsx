"use client";

import { ReviveOutcome, Session } from "@/gen/session/v1/types_pb";
import { icon, wrapper } from "./RevivedContextBadge.css";

interface RevivedContextBadgeProps {
  session: Session;
}

// hasLostContext is the single source of truth for "this session's last
// restart lost its conversation history" — shared with SessionRow's aria-label
// extension so the two surfaces can never drift on the condition.
export function hasLostContext(session: Session): boolean {
  return session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY;
}

// RevivedContextBadge renders only when the session's tmux pane restarted and
// its previous conversation history could not be recovered from disk
// (session-revive-uuid-loss AC3) — a durable, at-a-glance signal that survives
// the toast notification auto-closing.
export function RevivedContextBadge({ session }: RevivedContextBadgeProps) {
  if (!hasLostContext(session)) {
    return null;
  }
  return (
    <span
      className={wrapper}
      role="status"
      aria-live="polite"
      aria-label="This session lost its previous conversation and started fresh"
      title="This session lost its previous conversation and started fresh"
      data-testid="revived-context-badge"
    >
      <span className={icon} aria-hidden="true">⚠</span>
      Context lost
    </span>
  );
}
