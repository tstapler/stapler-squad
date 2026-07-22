"use client";

import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";
import { classifySessionKind } from "@/lib/backlog/sessionKind";
import { TriageReviewPanel } from "../TriageReviewPanel";
import { GateVerdictBox } from "../GateVerdictBox";
import { BlockedNotice } from "./BlockedNotice";
import * as styles from "./SessionDiagnosticPanel.css";

export interface SessionDiagnosticPanelProps {
  session: LinkedSession;
  item: BacklogItem;
}

// readOnly panels never invoke these — required only to satisfy the
// underlying components' prop contracts (Task 4.1.2d).
const noopSync = () => {};
const noopAsync = async () => {};
const noopAsyncWithArg = async (_arg: string) => {};

function summarizeTriage(triageResult: NonNullable<LinkedSession["triageResult"]>): string {
  const suggestionCount = triageResult.suggestions.filter((s) => s.rationale !== "question").length;
  return `Triage completed — ${suggestionCount} suggestion${suggestionCount === 1 ? "" : "s"}`;
}

function summarizeReReview(reviewVerdict: NonNullable<LinkedSession["reviewVerdict"]>): string {
  return `Re-review completed — ${reviewVerdict.overallOutcome ?? "UNVERIFIABLE"}`;
}

/**
 * Dispatches a Synthetic Session (Story 1.1.3's classifySessionKind) to the
 * correct read-only presentation — never a raw transcript/log viewer, per
 * plan.md's "Structured Diagnostic" glossary entry: no scrollback exists for
 * these DB-only rows, only already-fetched structured JSON.
 *
 *  - "headless_diagnostic" with `triageResult` populated → TriageReviewPanel
 *    readOnly (a `headless-triage-*` row).
 *  - "headless_diagnostic" with `reviewVerdict` populated → GateVerdictBox
 *    readOnly (a `headless-re-review-*` row).
 *  - "headless_diagnostic" with NEITHER populated (malformed/partial data) →
 *    falls back to BlockedNotice rather than rendering nothing, so this
 *    edge case can't reproduce the original inert-row bug.
 *  - "blocked_guardrail" / "manual_review_marker" → BlockedNotice (ux.md
 *    Surface 4 & 5) — identical treatment, distinct icon/label per kind.
 */
export function SessionDiagnosticPanel({ session, item }: SessionDiagnosticPanelProps) {
  const kind = classifySessionKind(session);

  if (kind === "headless_diagnostic") {
    if (session.triageResult) {
      return (
        <div className={styles.panel} data-testid="session-diagnostic-panel">
          <p role="status" className={styles.stateSummary}>
            {summarizeTriage(session.triageResult)}
          </p>
          <TriageReviewPanel
            item={item}
            triageResult={session.triageResult}
            readOnly
            onApply={noopAsync}
            onSkip={noopSync}
          />
        </div>
      );
    }

    if (session.reviewVerdict) {
      const { reviewVerdict } = session;
      return (
        <div className={styles.panel} data-testid="session-diagnostic-panel">
          <p role="status" className={styles.stateSummary}>
            {summarizeReReview(reviewVerdict)}
          </p>
          <GateVerdictBox
            verdict={reviewVerdict.overallOutcome ?? "UNVERIFIABLE"}
            summary={reviewVerdict.summary || "No summary recorded."}
            criteria={reviewVerdict.perCriterion?.map((c) => ({
              label: `#${c.criterionIndex} ${c.outcome}${c.evidence ? ` — ${c.evidence}` : ""}`,
              passed: c.outcome === "PASS",
            }))}
            readOnly
            onApprove={noopAsync}
            onReopen={noopAsyncWithArg}
            onOverride={noopAsyncWithArg}
            onSkipGate={noopAsync}
          />
        </div>
      );
    }

    // Defensive edge case (architecture review finding): a headless_diagnostic
    // row with neither field populated must not render nothing — fall back to
    // the Blocked-Before-Start treatment instead.
    return <BlockedNotice kind="missing_diagnostic_data" session={session} />;
  }

  if (kind === "blocked_guardrail" || kind === "manual_review_marker") {
    return <BlockedNotice kind={kind} session={session} />;
  }

  // Defensive: SessionDiagnosticPanel is only meant to be used for Synthetic
  // Sessions. If a caller ever passes a "work"/"review" (Real Session) by
  // mistake, fail safe with the same Blocked-Before-Start treatment rather
  // than rendering nothing.
  return <BlockedNotice kind="missing_diagnostic_data" session={session} />;
}
