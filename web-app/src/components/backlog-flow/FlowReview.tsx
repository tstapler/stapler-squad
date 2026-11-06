"use client";

import type { FlowReviewSection, FlowTrendsSection } from "@/gen/session/v1/backlog_flow_pb";
import { DeadlockTable } from "./DeadlockTable";
import { DrillButton } from "./DrillButton";
import { FlowRatio } from "./FlowRatio";
import { FlowVerdictMix } from "./FlowVerdictMix";
import * as styles from "./flow.css";

interface FlowReviewProps {
  review: FlowReviewSection;
  /** Estimated codes of the review section. */
  estimated: readonly string[];
  trends?: FlowTrendsSection;
  onDrill: (key: string) => void;
}

const DEADLOCK_ESTIMATED = "deadlock_candidates:heuristic";

const classCount = (review: FlowReviewSection, name: string): number => review.classes.find((row) => row.class === name)?.total?.value ?? 0;

function ExcludedNote({ review }: { review: FlowReviewSection }) {
  return (
    <span className={styles.muted}>
      {classCount(review, "no_commits")} &apos;no committed changes&apos; and {classCount(review, "self_check_failed")} self-check failures excluded
      from adjusted; {classCount(review, "reviewer_degraded")} reviewer-degraded results and {classCount(review, "auto_verified")} auto-verified
      passes also excluded
    </span>
  );
}

/** Adjusted FAIL leads, raw FAIL follows; the streak table keeps its own state. */
export function FlowReview({ review, estimated, trends, onDrill }: FlowReviewProps) {
  const noVerdicts = (review.total?.value ?? 0) === 0;
  return (
    <div className={styles.block}>
      {noVerdicts ? (
        <p>No review verdicts in this window</p>
      ) : (
        <div className={styles.block} data-testid="flow-review-rates">
          <span className={styles.row}>
            <span>Adjusted FAIL</span>{" "}
            <FlowRatio ratio={review.adjustedFail} onDrill={onDrill} /> <span>Raw</span>{" "}
            <FlowRatio ratio={review.rawFail} onDrill={onDrill} />{" "}
            {review.override && (
              <span>
                Human overrides{" "}
                <DrillButton drillKey={review.override.drillKey} onDrill={onDrill}>
                  {review.override.num}
                </DrillButton>
              </span>
            )}
          </span>
          {review.unverifiable && review.unverifiable.den > 0 && (
            <span className={styles.row} data-testid="flow-review-unverifiable">
              <span>UNVERIFIABLE</span> <FlowRatio ratio={review.unverifiable} onDrill={onDrill} />
            </span>
          )}
          <ExcludedNote review={review} />
        </div>
      )}
      <FlowVerdictMix trends={trends} />
      {review.deadlocks && (
        <DeadlockTable page={review.deadlocks} weakFilter={review.weakFilter} estimated={estimated.includes(DEADLOCK_ESTIMATED)} />
      )}
    </div>
  );
}
