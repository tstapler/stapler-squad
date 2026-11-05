"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { GitHubBadge } from "@/components/shared/GitHubBadge";
import * as styles from "../BacklogItemDetail.css";
import { ActionButtonLabel } from "./ActionButtonLabel";
import { classifySessionKind } from "@/lib/backlog/sessionKind";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import { useItemPullRequests } from "@/lib/hooks/useItemPullRequests";
import type { ItemPullRequestActions } from "@/lib/hooks/useItemPullRequestActions";
import { ItemPullRequestList } from "./ItemPullRequestList";

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
  /** Open stuck reasons for this item (primary + others); drives the PR status line. */
  stuckReasons?: readonly StuckReason[];
  /** Item-level exits for the multi-PR list: "Stop tracking" and "Track a PR by URL". */
  prActions?: ItemPullRequestActions;
}

/** PR status line derived from open stuck reasons; falls back to the neutral "waiting for merge". */
export function prStatusLabel(stuckReasons: readonly StuckReason[] = []): string {
  if (stuckReasons.includes(StuckReason.PR_NEEDS_FIX)) {
    return "needs fixes (failing checks, conflicts or review feedback)";
  }
  if (stuckReasons.includes(StuckReason.PR_READY_UNMERGED)) {
    return "ready to merge";
  }
  return "waiting for merge";
}

/**
 * "Pull Request" block. With `session_multi_pr` reader:backlog on and PRs listed it is the "k of N merged"
 * list for EVERY item status, done and archived included (the exit from a cleanup hold must exist there);
 * otherwise the legacy single-PR block, only while `pr_pending` (Story 3.1.2 Task 3.1.2c). This is the
 * sole data source for the PR URL/number text (D4): VersionControlSection opts its own PR link out
 * whenever this section renders.
 */
export function PullRequestSection(props: PullRequestSectionProps) {
  const list = useItemPullRequests(props.item);
  if (list) return <ItemPullRequestsBlock {...props} pullRequests={list} />;
  return props.item.status === "pr_pending" ? <LegacyPullRequestBlock {...props} /> : null;
}

function MarkDoneButton({ onMarkDone, actionLoading, readOnly }: Pick<PullRequestSectionProps, "onMarkDone" | "actionLoading" | "readOnly">) {
  return (
    <button
      className={styles.actionButton}
      onClick={onMarkDone}
      disabled={actionLoading !== null || readOnly}
      aria-busy={actionLoading === "mark_done"}
      title="Mark done manually (if PR already merged)"
      data-testid="backlog-action-mark-done"
    >{/* analytics-exempt */}
      <ActionButtonLabel pending={actionLoading === "mark_done"} label="Mark Done" />
    </button>
  );
}

function ItemPullRequestsBlock({ item, actionLoading, onMarkDone, readOnly = false, prActions, pullRequests }: PullRequestSectionProps & { pullRequests: NonNullable<ReturnType<typeof useItemPullRequests>> }) {
  return (
    <CollapsibleSection sectionKey="pull-request" title="Pull Requests" defaultExpanded={true}>
      <div className={styles.section}>
        <ItemPullRequestList pullRequests={pullRequests} holds={item.prHolds ?? []} actions={prActions} readOnly={readOnly} />
        {item.status === "pr_pending" && <MarkDoneButton onMarkDone={onMarkDone} actionLoading={actionLoading} readOnly={readOnly} />}
      </div>
    </CollapsibleSection>
  );
}

function LegacyPullRequestBlock({ item, actionLoading, onMarkDone, readOnly = false, stuckReasons }: PullRequestSectionProps) {
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
                  PR #{item.prNumber} — {prStatusLabel(stuckReasons)}
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
          <MarkDoneButton onMarkDone={onMarkDone} actionLoading={actionLoading} readOnly={readOnly} />
        </div>
      </div>
    </CollapsibleSection>
  );
}
