"use client";

import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";
import { InlineNotice } from "@/components/common/InlineNotice";
import { getAvailableActions } from "@/lib/backlog/itemActions";
import { derivePlanReviewStatus } from "@/lib/backlog/planReviewStatus";
import * as styles from "../BacklogItemDetail.css";
import { ActionButtonLabel } from "./ActionButtonLabel";

export interface ActionsSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  latestWorkSession: LinkedSession | undefined;
  showManualReview: boolean;
  manualReviewOutcome: string;
  manualReviewSummary: string;
  onAction: (action: string) => void;
  onManualReviewOutcomeChange: (value: string) => void;
  onManualReviewSummaryChange: (value: string) => void;
  onManualReviewSubmit: () => void;
  onManualReviewCancel: () => void;
  /**
   * Task 5.3.1c (backlog-event-driven-updates): set once an
   * ArchivedEvent/RemovedEvent arrives for this item via BacklogItemDetail's
   * item-scoped terminal-state watch. Every mutating action (including
   * Delete) is replaced by a single informational InlineNotice — once an
   * item is archived/removed elsewhere there is nothing left to act on.
   */
  terminalState: "archived" | "removed" | null;
}

/**
 * The full status-conditional action-button block, including the inline
 * manual-review form — extracted verbatim from BacklogItemDetail.tsx
 * (Story 3.1.3, Task 3.1.3b). Always visible (primary content, not
 * progressive disclosure) — no Collapsible wrapper. Every data-testid is
 * preserved exactly: manual-review-form, manual-review-outcome,
 * manual-review-summary, manual-review-submit, backlog-action-ship-pr,
 * backlog-action-override-done, backlog-action-re-review,
 * backlog-action-manual-review, backlog-action-restart-session, and the
 * rest of the per-status action buttons.
 */
export function ActionsSection({
  item,
  actionLoading,
  latestWorkSession,
  showManualReview,
  manualReviewOutcome,
  manualReviewSummary,
  onAction,
  onManualReviewOutcomeChange,
  onManualReviewSummaryChange,
  onManualReviewSubmit,
  onManualReviewCancel,
  terminalState,
}: ActionsSectionProps) {
  // getAvailableActions (web-app/src/lib/backlog/itemActions.ts) is the single
  // source of truth for which actions this item's current status + gate flags
  // expose — see its doc comment for why this replaced a scattered set of
  // per-status JSX conditionals, some of which (approve_plan) used to key off
  // incidental data (planArtifactsPath) instead of the real gate condition
  // (docs/tasks/backlog-feature-improvement.md's 2026-08-03 entry, item
  // be676dab). Everything below this line is transient per-click UI state
  // (actionLoading, AC-criteria-empty tooltips, etc.) that stays local.
  const { actions } = getAvailableActions(item);

  // Autonomous mode does its own planning — no plan-approval gate needed,
  // just the ready-status check `actions.has("spawn_session_autonomous")`
  // already encodes.
  const canRunAutonomously = actions.has("spawn_session_autonomous");
  // Task 4.3.1c: reuse the single derivePlanReviewStatus source of truth
  // (also used by PlanVerdictBox) instead of re-deriving the raw
  // skipPlanning/planApproved check here — pure internal-logic refactor,
  // behaviorally identical for every case that mattered before (a rejected
  // plan was never approved, so it already blocked spawn; adding
  // "changes_requested" as its own state doesn't loosen or tighten the gate).
  const planStatus = derivePlanReviewStatus(item);
  const canSpawnSession =
    actions.has("spawn_session") && (planStatus === "skipped" || planStatus === "approved");

  // Self-service "Ship PR" action: only makes sense for an item sitting in
  // review with no PR yet — the exact gap this closes (see
  // docs/tasks/backlog-feature-improvement.md, 2026-07-18 update). All AC
  // criteria must be complete before shipping; a gate verdict of PASS is
  // encouraged (via the button's title) but not required — same
  // human-override philosophy as the existing "Override → Done" action.
  const acAllComplete =
    item.acCriteria.length > 0 && item.acCriteria.every((c) => c.status === "done");

  return (
    <div className={styles.section}>
      <h3 className={styles.sectionTitle}>Actions</h3>
      <div className={styles.actionsPanel} role="group" aria-label="Item actions">
        {terminalState ? (
          <>
            <InlineNotice
              message={
                terminalState === "archived"
                  ? "This item was archived elsewhere."
                  : "This item was removed elsewhere."
              }
              data-testid="backlog-detail-terminal-notice"
            />
            {terminalState === "archived" && (
              <button
                className={styles.actionButton}
                onClick={() => onAction("unarchive")}
                disabled={actionLoading !== null}
                aria-busy={actionLoading === "unarchive"}
                title="Restores the item to the Idea column. Its git worktree, if any, was deleted at archive time and cannot be recreated."
                data-testid="backlog-action-unarchive"
              >
                <ActionButtonLabel pending={actionLoading === "unarchive"} label="Unarchive" />
              </button>
            )}
          </>
        ) : (
          <>
        {actions.has("mark_ready") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("mark_ready")}
            disabled={actionLoading !== null || item.acCriteria.length === 0}
            aria-disabled={item.acCriteria.length === 0}
            aria-busy={actionLoading === "mark_ready"}
            title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
            data-testid="backlog-action-mark-ready"
          >
            <ActionButtonLabel pending={actionLoading === "mark_ready"} label="Mark Ready" />
          </button>
        )}

        {actions.has("trigger_triage") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("trigger_triage")}
            disabled={actionLoading !== null || !item.repoPath}
            aria-disabled={!item.repoPath}
            aria-busy={actionLoading === "trigger_triage"}
            title={!item.repoPath ? "Set repository path first" : undefined}
            data-testid="backlog-action-trigger-triage"
          >
            <ActionButtonLabel pending={actionLoading === "trigger_triage"} label="Trigger Triage" />
          </button>
        )}

        {actions.has("spawn_session") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("spawn_session")}
            disabled={actionLoading !== null || !canSpawnSession}
            aria-disabled={!canSpawnSession}
            aria-busy={actionLoading === "spawn_session"}
            title={
              !canSpawnSession
                ? "Approve the plan or enable skip_planning to spawn a session"
                : undefined
            }
            data-testid="backlog-action-spawn-session"
          >
            <ActionButtonLabel pending={actionLoading === "spawn_session"} label="Spawn Session" />
          </button>
        )}

        {actions.has("spawn_session_autonomous") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("spawn_session_autonomous")}
            disabled={actionLoading !== null || !canRunAutonomously}
            aria-disabled={!canRunAutonomously}
            aria-busy={actionLoading === "spawn_session_autonomous"}
            title={
              !canRunAutonomously
                ? "Item must be in Ready status to run autonomously"
                : "Run the agent without human approval for tool calls"
            }
            data-testid="backlog-action-run-autonomously"
          >
            <ActionButtonLabel pending={actionLoading === "spawn_session_autonomous"} label="Run Autonomously" />
          </button>
        )}

        {/* Approve Plan / Retry Triage: mutually exclusive, both driven by
            getAvailableActions' isGatedOnPlanApproval + hasPlan derivation,
            never by planArtifactsPath presence alone (see itemActions.ts —
            docs/tasks/backlog-feature-improvement.md's 2026-08-03 entry). A
            gated item with no usable plan gets an explicit retry affordance
            instead of the Approve Plan button silently disappearing. */}
        {actions.has("approve_plan") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("approve_plan")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "approve_plan"}
            title={
              item.status === "queued"
                ? "Queued items can't be dequeued until their plan is approved (or skip_planning is set)."
                : undefined
            }
            data-testid="backlog-action-approve-plan"
          >
            <ActionButtonLabel pending={actionLoading === "approve_plan"} label="Approve Plan" />
          </button>
        )}
        {actions.has("retry_triage") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("retry_triage")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "retry_triage"}
            title="This item's most recent triage session ended without producing a usable plan — retry to generate one."
            data-testid="backlog-action-retry-triage"
          >
            <ActionButtonLabel pending={actionLoading === "retry_triage"} label="Retry Triage" />
          </button>
        )}

        {actions.has("view_session") && (
          <a
            className={styles.actionButton}
            href={`/?session=${(latestWorkSession ?? item.linkedSessions[item.linkedSessions.length - 1]).sessionId}`}
            data-testid="backlog-action-view-session"
          >
            View Session
          </a>
        )}

        {actions.has("ship_pr") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("ship_pr")}
            disabled={actionLoading !== null || !acAllComplete}
            aria-disabled={!acAllComplete}
            aria-busy={actionLoading === "ship_pr"}
            title={
              !acAllComplete
                ? "All acceptance criteria must be complete before shipping a PR."
                : "Ask the agent to push the branch and open a pull request for this item."
            }
            data-testid="backlog-action-ship-pr"
          >
            <ActionButtonLabel pending={actionLoading === "ship_pr"} label="🚀 Ship PR" />
          </button>
        )}

        {actions.has("override_done") && (
          <button
            className={`${styles.actionButton} ${styles.actionButtonDanger}`}
            onClick={() => onAction("override_done")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "override_done"}
            data-testid="backlog-action-override-done"
          >
            <ActionButtonLabel pending={actionLoading === "override_done"} label="Override → Done" />
          </button>
        )}

        {actions.has("re_review") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("re_review")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "re_review"}
            data-testid="backlog-action-re-review"
          >
            <ActionButtonLabel pending={actionLoading === "re_review"} label="Re-review" />
          </button>
        )}

        {actions.has("manual_review") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("manual_review")}
            disabled={actionLoading !== null}
            data-testid="backlog-action-manual-review"
          >
            Submit Review
          </button>
        )}

        {actions.has("restart_session") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("restart_session")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "restart_session"}
            title={
              item.status === "review"
                ? "Stop the review session and restart work from scratch in a fresh git worktree"
                : "Stop the current session and re-spawn it in a fresh git worktree"
            }
            data-testid="backlog-action-restart-session"
          >
            <ActionButtonLabel pending={actionLoading === "restart_session"} label="Restart" />
          </button>
        )}

        {showManualReview && item.status === "review" && (
          <div className={styles.manualReviewForm} data-testid="manual-review-form">
            <h4 className={styles.manualReviewTitle}>Submit Review</h4>
            <div className={styles.manualReviewRow}>
              <label className={styles.manualReviewLabel}>Verdict</label>
              <select
                className={styles.manualReviewSelect}
                value={manualReviewOutcome}
                onChange={(e) => onManualReviewOutcomeChange(e.target.value)}
                data-testid="manual-review-outcome"
              >
                <option value="PASS">PASS — meets all criteria</option>
                <option value="FAIL">FAIL — does not meet criteria</option>
                <option value="PARTIAL">PARTIAL — partially meets criteria</option>
                <option value="UNVERIFIABLE">UNVERIFIABLE — cannot verify</option>
              </select>
            </div>
            <div className={styles.manualReviewRow}>
              <label className={styles.manualReviewLabel}>Summary</label>
              <textarea
                className={styles.manualReviewTextarea}
                placeholder="Describe your findings…"
                value={manualReviewSummary}
                onChange={(e) => onManualReviewSummaryChange(e.target.value)}
                rows={4}
                data-testid="manual-review-summary"
              />
            </div>
            <div className={styles.manualReviewActions}>
              <button
                className={styles.actionButton}
                disabled={!manualReviewSummary.trim() || actionLoading !== null}
                aria-busy={actionLoading === "manual_review_submit"}
                onClick={onManualReviewSubmit}
                data-testid="manual-review-submit"
              >
                <ActionButtonLabel pending={actionLoading === "manual_review_submit"} label="Submit" />
              </button>
              <button
                className={styles.actionButtonSecondary}
                onClick={onManualReviewCancel}
                data-testid="manual-review-cancel"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {actions.has("archive") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("archive")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "archive"}
            data-testid="backlog-action-archive"
          >
            <ActionButtonLabel pending={actionLoading === "archive"} label="Archive" />
          </button>
        )}
        {actions.has("reopen") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("reopen")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "reopen"}
            data-testid="backlog-action-reopen"
          >
            <ActionButtonLabel pending={actionLoading === "reopen"} label="Re-open to Review" />
          </button>
        )}
        {actions.has("unarchive") && (
          <button
            className={styles.actionButton}
            onClick={() => onAction("unarchive")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "unarchive"}
            title="Restores the item to the Idea column. Its git worktree, if any, was deleted at archive time and cannot be recreated."
            data-testid="backlog-action-unarchive"
          >
            <ActionButtonLabel pending={actionLoading === "unarchive"} label="Unarchive" />
          </button>
        )}

        {/* Backward transitions — visible whenever there's an earlier stage to return to */}
        {actions.has("send_back_idea") && (
          <button
            className={`${styles.actionButton} ${styles.actionButtonSecondary}`}
            onClick={() => onAction("send_back_idea")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "send_back_idea"}
            title="Reset to Idea and clear plan approval so triage can re-run"
            data-testid="backlog-action-send-back-idea"
          >
            <ActionButtonLabel pending={actionLoading === "send_back_idea"} label="↩ Return to Triage" />
          </button>
        )}
        {actions.has("send_back_ready") && (
          <button
            className={`${styles.actionButton} ${styles.actionButtonSecondary}`}
            onClick={() => onAction("send_back_ready")}
            disabled={actionLoading !== null}
            aria-busy={actionLoading === "send_back_ready"}
            title="Move back to Ready to re-spawn without full re-triage"
            data-testid="backlog-action-send-back-ready"
          >
            <ActionButtonLabel pending={actionLoading === "send_back_ready"} label="↩ Back to Ready" />
          </button>
        )}

        <button
          className={styles.actionButtonDanger}
          onClick={() => onAction("delete")}
          disabled={actionLoading !== null}
          aria-busy={actionLoading === "delete"}
          data-testid="backlog-action-delete"
        >
          <ActionButtonLabel pending={actionLoading === "delete"} label="Delete" />
        </button>
          </>
        )}
      </div>
    </div>
  );
}
