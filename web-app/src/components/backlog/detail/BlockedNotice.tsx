"use client";

import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import * as styles from "./BlockedNotice.css";

/**
 * The three kinds of synthetic session BlockedNotice handles:
 *  - "blocked_guardrail" / "manual_review_marker" are the two documented
 *    SessionKinds this notice was designed for (ux.md Surface 4 & 5).
 *  - "missing_diagnostic_data" is the defensive fallback SessionDiagnosticPanel
 *    routes to when a session classifies as "headless_diagnostic" but has
 *    neither triageResult nor reviewVerdict populated (malformed/partial
 *    data) — rather than rendering nothing and reproducing the original
 *    inert-row bug for a new edge case.
 */
export type BlockedNoticeKind = "blocked_guardrail" | "manual_review_marker" | "missing_diagnostic_data";

const KIND_CONFIG: Record<BlockedNoticeKind, { icon: string; label: string; fallbackText: string }> = {
  blocked_guardrail: {
    icon: "🚫",
    label: "Blocked before starting",
    fallbackText: "No summary recorded.",
  },
  manual_review_marker: {
    icon: "✍️",
    label: "Manual review",
    fallbackText: "No summary recorded.",
  },
  missing_diagnostic_data: {
    icon: "⚠️",
    label: "No diagnostic data",
    fallbackText: "No diagnostic data recorded.",
  },
};

export interface BlockedNoticeProps {
  kind: BlockedNoticeKind;
  session: Pick<LinkedSession, "reviewVerdict">;
}

/**
 * Blocked-Before-Start Notice (ux.md Surface 4 & 5): the plain-text
 * explanation shown for a Blocked Guardrail or Manual Review Marker
 * synthetic session — there was never a session to open, so this never
 * renders an "open session" affordance of any kind (Story 4.1.3).
 *
 * `role="status"`, not `role="log"`: a single current-state announcement,
 * not a scrolling transcript — no raw transcript exists for these DB-only
 * rows (plan.md's "Structured Diagnostic" glossary entry).
 *
 * Text is rendered verbatim from the backend. `session.reviewVerdict.summary`
 * for a "blocked_guardrail" kind comes from exactly two backend code paths
 * (see classifySessionKind in sessionKind.ts, and session/review_gate.go):
 *
 *  - "review-blocked-*": built from RunPreGateSecurityCheck's error string
 *    (session/review_gate.go ~line 228). Confirmed safe by Story 4.1.1's
 *    security review (plan.md's Unresolved Question #1) and covered by
 *    TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff
 *    in session/backlog_review_test.go — the error string only ever contains
 *    a fixed pattern name, never the matched secret or diff content.
 *  - "diff-error-*": built from GetGitDiffRef's wrapped command error
 *    (session/review_gate.go ~line 191, session/backlog_review.go's
 *    GetGitDiffRef). Currently benign — the wrapped error is `exec.Cmd`'s
 *    own Error() string (e.g. "exit status 128") plus the range arg and
 *    directory path, never command stderr or diff content, per
 *    TestGetGitDiffRefError_should_NeverEmbedCommandStderr_When_DiffCommandFails
 *    in session/backlog_review_test.go. This path has NOT had the same
 *    security review as RunPreGateSecurityCheck — if GetGitDiffRef's error
 *    wrapping is ever changed to include cmd.Stderr, that regression test
 *    is what would catch it before it reaches this UI surface.
 */
export function BlockedNotice({ kind, session }: BlockedNoticeProps) {
  const config = KIND_CONFIG[kind];
  const summary = session.reviewVerdict?.summary;

  return (
    <div className={styles.notice} role="status" data-testid="blocked-notice">
      <p className={styles.label}>
        <span aria-hidden="true">{config.icon}</span> {config.label}
      </p>
      <p className={styles.summaryText}>{summary || config.fallbackText}</p>
    </div>
  );
}
