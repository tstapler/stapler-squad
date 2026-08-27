"use client";

import type { MouseEvent } from "react";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { GateVerdictBox } from "../GateVerdictBox";
import * as styles from "../BacklogItemDetail.css";

export interface ReviewingSectionProps {
  item: BacklogItem;
  /** Current work session — same value as every other call site (Story 1.1.2, D3). */
  workSession: LinkedSession | undefined;
  actionLoading: string | null;
  defaultExpanded: boolean;
  onViewChanges: (e: MouseEvent<HTMLElement>) => void;
  onGateApprove: () => Promise<void>;
  onGateReopen: (feedback: string) => Promise<void>;
  onGateOverride: (reason: string) => Promise<void>;
  onGateSkip: () => Promise<void>;
  onReReview: () => Promise<void>;
  /**
   * Task 5.3.1c (backlog-event-driven-updates): true once an
   * ArchivedEvent/RemovedEvent has arrived for this item — GateVerdictBox is
   * rendered in its read-only historical-record mode (no Approve/Reopen/
   * Override/Skip Gate/Re-review affordances) once set.
   */
  readOnly?: boolean;
}

/**
 * The "Reviewing" work-session context box + GateVerdictBox — only rendered
 * while `item.status === "review"` (guard preserved verbatim from the
 * pre-extraction inline block, Story 3.1.2 Task 3.1.2b). Wrapped in a
 * Collapsible, default-expanded only while the item is actually in review.
 */
export function ReviewingSection({
  item,
  workSession,
  actionLoading,
  defaultExpanded,
  onViewChanges,
  onGateApprove,
  onGateReopen,
  onGateOverride,
  onGateSkip,
  onReReview,
  readOnly = false,
}: ReviewingSectionProps) {
  const activeReviewSession = [...item.linkedSessions]
    .reverse()
    .find(
      (s) =>
        s.role === "review" &&
        !s.endedAt &&
        !s.sessionId.startsWith("headless-") &&
        !s.sessionId.startsWith("review-blocked-")
    );

  return (
    <CollapsibleSection sectionKey="reviewing" title="Reviewing" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <div className={styles.reviewContextBox}>
          <div className={styles.reviewContextInfo}>
            {workSession ? (
              <>
                <span className={styles.reviewContextLabel}>Work session</span>
                <a
                  className={styles.reviewContextSessionId}
                  href={`/?session=${workSession.sessionId}`}
                  title="Open in terminal"
                >
                  {workSession.sessionId}
                </a>
                {workSession.endedAt && (
                  <span className={styles.reviewContextDate}>
                    Completed {new Date(workSession.endedAt).toLocaleString()}
                  </span>
                )}
              </>
            ) : (
              <span className={styles.reviewContextLabel}>No work session found</span>
            )}
            {activeReviewSession && (
              <>
                <span className={styles.reviewContextLabel}>Review session</span>
                <a
                  className={styles.reviewContextSessionId}
                  href={`/?session=${activeReviewSession.sessionId}`}
                  title="Open review session in terminal"
                >
                  {activeReviewSession.sessionId}
                </a>
              </>
            )}
          </div>
          {workSession && (
            <button
              className={styles.viewChangesButton}
              onClick={onViewChanges}
              data-testid="backlog-review-view-changes"
            >
              View Changes ↗
            </button>
          )}
        </div>
      </div>

      <div className={styles.section}>
        {readOnly ? (
          <GateVerdictBox
            readOnly
            verdict={item.gateVerdict ?? "PENDING"}
            summary={item.gateVerdictSummary || "Review in progress"}
            criteria={item.gateCriteria}
            elapsedSeconds={undefined}
          />
        ) : (
          <GateVerdictBox
            verdict={item.gateVerdict ?? "PENDING"}
            summary={item.gateVerdictSummary || "Review in progress"}
            criteria={item.gateCriteria}
            elapsedSeconds={undefined}
            onApprove={onGateApprove}
            onReopen={onGateReopen}
            onOverride={onGateOverride}
            onSkipGate={onGateSkip}
            onReReview={onReReview}
            actionPending={actionLoading !== null}
          />
        )}
      </div>
    </CollapsibleSection>
  );
}
