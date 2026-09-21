"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { GitHubBadge } from "@/components/shared/GitHubBadge";
import * as styles from "../BacklogItemDetail.css";
import { ActionButtonLabel } from "./ActionButtonLabel";
import { classifySessionKind } from "@/lib/backlog/sessionKind";

export interface PullRequestSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  onMarkDone: () => void;
  /**
   * Task 5.3.1c (backlog-event-driven-updates): true once an
   * ArchivedEvent/RemovedEvent has arrived for this item — disables "Mark
   * Done" the same way the Actions panel's own mutating buttons are
   * replaced entirely once the item is no longer live.
   */
  readOnly?: boolean;
}

/**
 * "Pull Request" block — only rendered while `item.status === "pr_pending"`
 * (guard preserved verbatim, Story 3.1.2 Task 3.1.2c). Only rendered when
 * relevant, so always-expanded by default is correct here. This is the sole
 * data source for the PR URL/number text (D4) — VersionControlSection
 * (Story 3.1.4) opts its own VcsWidget out of the duplicate PR link via
 * `showPrLink={false}` when this section is also rendering.
 */
export function PullRequestSection({ item, actionLoading, onMarkDone, readOnly = false }: PullRequestSectionProps) {
  // Story 3.3.2, Task 3.3.2b: linkedSessions is in creation order (same
  // convention BacklogItemDetail.tsx's Jules dispatch-gate branch prefill
  // relies on). The newest linked session isn't necessarily the one that
  // opened this PR -- a review-gate session links *after* the work session
  // it reviews (see julesDispatchGate.ts's skipReviewGate flow), so `.at(-1)`
  // can return a "review" session and misattribute (or drop) the badge.
  // Only sessionKind "work" (classifySessionKind) actually produces a PR --
  // filter to those before taking the last one.
  const producingSession = item.linkedSessions.filter((s) => classifySessionKind(s) === "work").at(-1);
  const producedByJules = producingSession?.role === "jules_work";

  return (
    <CollapsibleSection sectionKey="pull-request" title="Pull Request" defaultExpanded={true}>
      <div className={styles.section}>
        <div className={styles.reviewContextBox}>
          <div className={styles.reviewContextInfo}>
            {item.prUrl ? (
              <>
                <span className={styles.reviewContextLabel}>
                  PR #{item.prNumber} — waiting for merge
                </span>
                <div className={styles.sessionRowMain}>
                  <GitHubBadge prNumber={item.prNumber} prUrl={item.prUrl} />
                  {producedByJules && (
                    <span className={styles.julesProvenanceMarker} aria-label="Opened by Jules">
                      <span aria-hidden="true">☁</span> Jules
                    </span>
                  )}
                </div>
              </>
            ) : (
              <span className={styles.reviewContextLabel}>
                PR pending — no URL recorded yet
              </span>
            )}
          </div>
          <button
            className={styles.actionButton}
            onClick={onMarkDone}
            disabled={actionLoading !== null || readOnly}
            aria-busy={actionLoading === "mark_done"}
            title="Mark done manually (if PR already merged)"
            data-testid="backlog-action-mark-done"
          >
            <ActionButtonLabel pending={actionLoading === "mark_done"} label="Mark Done" />
          </button>
        </div>
      </div>
    </CollapsibleSection>
  );
}
