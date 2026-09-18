import type { LinkedSession } from "@/lib/hooks/useBacklogService";

/**
 * Closed classification of a LinkedSession's kind. "work" and "review" are
 * Real Sessions (backed by an actual session.Instance/tmux/PTY); the other
 * three are Synthetic Sessions — DB-only rows with no backing Instance, no
 * terminal, nothing to attach to.
 */
export type SessionKind =
  | "work"
  | "review"
  | "headless_diagnostic"
  | "blocked_guardrail"
  | "manual_review_marker";

/**
 * Classifies a LinkedSession's kind from its role and sessionId prefix.
 * Single source of truth for the ad hoc prefix-string checks that used to be
 * scattered across BacklogItemDetail.tsx's Sessions section — fixes the
 * pre-existing dead-link bug where a `manual-review-*` or `diff-error-*`
 * session (role "review", no recognized prefix in the old inline check)
 * fell through to a clickable `<a href="/?session=...">` even though it was
 * never Instance-backed.
 *
 * Check order matters: role "triage" or a "headless-" sessionId prefix wins
 * first, then the two blocked-guardrail prefixes, then the manual-review
 * prefix, then role "review", with "work" as the final fallback.
 */
export function classifySessionKind(session: Pick<LinkedSession, "role" | "sessionId">): SessionKind {
  if (session.role === "triage" || session.sessionId.startsWith("headless-")) {
    return "headless_diagnostic";
  }
  if (session.sessionId.startsWith("review-blocked-") || session.sessionId.startsWith("diff-error-")) {
    return "blocked_guardrail";
  }
  if (session.sessionId.startsWith("manual-review-")) {
    return "manual_review_marker";
  }
  if (session.role === "review") {
    return "review";
  }
  return "work";
}

/**
 * A LinkedSession is steerable iff it is Instance-backed ("work"/"review",
 * per classifySessionKind) and has not ended. Synthetic session kinds
 * (headless triage/review, blocked-guardrail, manual-review-marker) are
 * never steerable — no session.Instance was ever created for them. See
 * ADR-002 (project_plans/backlog-operator-feedback-loop/decisions/).
 */
export function isSteerable(session: Pick<LinkedSession, "role" | "sessionId" | "endedAt">): boolean {
  const kind = classifySessionKind(session);
  return (kind === "work" || kind === "review") && !session.endedAt;
}
