"use client";

import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";
import { InlineNotice } from "@/components/common/InlineNotice";
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
  // Pure derivations of `item`, moved in from BacklogItemDetail.tsx (code
  // review follow-up) — nothing outside this component consumed them, so
  // there was no reason to compute them in the parent and thread them
  // through as props.
  const canSpawnSession =
    item.status === "ready" &&
    (item.skipPlanning || item.planApproved);
  // Autonomous mode does its own planning — no plan-approval gate needed.
  const canRunAutonomously = item.status === "ready";

  // Self-service "Ship PR" action: only makes sense for an item sitting in
  // review with no PR yet — the exact gap this closes (see
  // docs/tasks/backlog-feature-improvement.md, 2026-07-18 update). All AC
  // criteria must be complete before shipping; a gate verdict of PASS is
  // encouraged (via the button's title) but not required — same
  // human-override philosophy as the existing "Override → Done" action.
  const acAllComplete =
    item.acCriteria.length > 0 && item.acCriteria.every((c) => c.status === "done");
  const canShipPR = item.status === "review" && !item.prUrl;

  return (
    <div className={styles.section}>
      <h3 className={styles.sectionTitle}>Actions</h3>
      <div className={styles.actionsPanel} role="group" aria-label="Item actions">
        {terminalState ? (
          <InlineNotice
            message={
              terminalState === "archived"
                ? "This item was archived elsewhere."
                : "This item was removed elsewhere."
            }
            data-testid="backlog-detail-terminal-notice"
          />
        ) : (
          <>
        {item.status === "idea" && (
          <>
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
          </>
        )}

        {item.status === "ready" && (
          <>
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
            {item.planArtifactsPath && (
              <button
                className={styles.actionButton}
                onClick={() => onAction("approve_plan")}
                disabled={actionLoading !== null}
                aria-busy={actionLoading === "approve_plan"}
                data-testid="backlog-action-approve-plan"
              >
                <ActionButtonLabel pending={actionLoading === "approve_plan"} label="Approve Plan" />
              </button>
            )}
          </>
        )}

        {item.status === "in_progress" && item.linkedSessions.length > 0 && (
          <>
            <a
              className={styles.actionButton}
              href={`/?session=${(latestWorkSession ?? item.linkedSessions[item.linkedSessions.length - 1]).sessionId}`}
              data-testid="backlog-action-view-session"
            >
              View Session
            </a>
            <button
              className={styles.actionButton}
              onClick={() => onAction("restart_session")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "restart_session"}
              title="Stop the current session and re-spawn it in a fresh git worktree"
              data-testid="backlog-action-restart-session"
            >
              <ActionButtonLabel pending={actionLoading === "restart_session"} label="Restart" />
            </button>
          </>
        )}

        {item.status === "review" && (
          <>
            {canShipPR && (
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
            <button
              className={`${styles.actionButton} ${styles.actionButtonDanger}`}
              onClick={() => onAction("override_done")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "override_done"}
              data-testid="backlog-action-override-done"
            >
              <ActionButtonLabel pending={actionLoading === "override_done"} label="Override → Done" />
            </button>
            <button
              className={styles.actionButton}
              onClick={() => onAction("re_review")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "re_review"}
              data-testid="backlog-action-re-review"
            >
              <ActionButtonLabel pending={actionLoading === "re_review"} label="Re-review" />
            </button>
            <button
              className={styles.actionButton}
              onClick={() => onAction("manual_review")}
              disabled={actionLoading !== null}
              data-testid="backlog-action-manual-review"
            >
              Submit Review
            </button>
            <button
              className={styles.actionButton}
              onClick={() => onAction("restart_session")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "restart_session"}
              title="Stop the review session and restart work from scratch in a fresh git worktree"
              data-testid="backlog-action-restart-session"
            >
              <ActionButtonLabel pending={actionLoading === "restart_session"} label="Restart" />
            </button>
          </>
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

        {item.status === "done" && (
          <>
            <button
              className={styles.actionButton}
              onClick={() => onAction("archive")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "archive"}
              data-testid="backlog-action-archive"
            >
              <ActionButtonLabel pending={actionLoading === "archive"} label="Archive" />
            </button>
            <button
              className={styles.actionButton}
              onClick={() => onAction("reopen")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "reopen"}
              data-testid="backlog-action-reopen"
            >
              <ActionButtonLabel pending={actionLoading === "reopen"} label="Re-open to Review" />
            </button>
          </>
        )}

        {/* Backward transitions — visible whenever there's an earlier stage to return to */}
        {["refining", "ready", "in_progress", "review", "pr_pending", "done"].includes(item.status) && (
          <>
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
            {["in_progress", "review", "pr_pending", "done"].includes(item.status) && (
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
          </>
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
