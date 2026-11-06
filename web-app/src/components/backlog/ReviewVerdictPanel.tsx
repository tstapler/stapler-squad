"use client";

// +feature: backlog-review-results-panel

import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { RawJsonDisclosure } from "./RawJsonDisclosure";
import * as styles from "./ResultPanel.css";

export const REVIEW_RESULTS_PANEL_FLAG = "backlog-review-results-panel";

export interface ReviewVerdictPanelProps {
  /** Undefined hides the panel (no review yet); `loading` shows a skeleton instead. */
  verdict: LinkedSession["reviewVerdict"] | undefined;
  loading?: boolean;
}

/** Verdict of the most recent review session that has one (linkedSessions is oldest-first). */
export function latestReviewVerdict(sessions: LinkedSession[]): LinkedSession["reviewVerdict"] | undefined {
  return sessions.filter((s) => s.role === "review" && s.reviewVerdict).at(-1)?.reviewVerdict;
}

function outcomeBadge(outcome: string | undefined): string {
  const o = (outcome ?? "").toUpperCase();
  if (o === "PASS") return styles.badgePass;
  if (o === "FAIL") return styles.badgeFail;
  return styles.badgeWarn;
}

export function ReviewVerdictPanel({ verdict, loading }: ReviewVerdictPanelProps) {
  const enabled = useFeatureFlag(REVIEW_RESULTS_PANEL_FLAG);
  if (!enabled) return null;

  if (loading) {
    return (
      <section className={styles.container} aria-busy="true" aria-label="Review Results" data-testid="review-verdict-skeleton">
        <div className={styles.skeletonShort} />
        <div className={styles.skeletonLine} />
      </section>
    );
  }
  if (!verdict) return null;

  if (!Array.isArray(verdict.perCriterion ?? [])) {
    return (
      <section className={styles.container} aria-label="Review Results" data-testid="review-verdict-panel">
        <h3 className={styles.title}>Review Results</h3>
        <p className={styles.mutedText}>Review result could not be displayed.</p>
        <RawJsonDisclosure value={verdict} testId="review-raw-json" />
      </section>
    );
  }

  const criteria = verdict.perCriterion ?? [];
  return (
    <section className={styles.container} aria-label="Review Results" data-testid="review-verdict-panel">
      <div className={styles.header}>
        <h3 className={styles.title}>Review Results</h3>
        <span className={outcomeBadge(verdict.overallOutcome)} data-testid="review-overall">
          {verdict.overallOutcome ?? "PENDING"}
        </span>
      </div>
      {verdict.summary && <p>{verdict.summary}</p>}
      {criteria.length > 0 && (
        <ul className={styles.list} aria-label="Per-criterion results">
          {criteria.map((c) => (
            <li key={c.criterionIndex} className={styles.row}>
              <span className={styles.rowText}>Criterion {c.criterionIndex + 1}</span>
              <span className={outcomeBadge(c.outcome)}>{c.outcome}</span>
              {c.evidence && <span className={styles.evidence}>{c.evidence}</span>}
            </li>
          ))}
        </ul>
      )}
      <RawJsonDisclosure value={verdict} testId="review-raw-json" />
    </section>
  );
}
