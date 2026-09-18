"use client";
// +feature: backlog-plan-verdict-box

import { useEffect, useRef, useState } from "react";
import type { PlanReviewStatus } from "@/lib/backlog/planReviewStatus";
import * as styles from "./PlanVerdictBox.css";
import { InlineError } from "./InlineError";

export interface PlanVerdictBoxProps {
  status: PlanReviewStatus;
  /** Persisted rejection reason (present only when status === "changes_requested"). */
  rejectionReason?: string;
  /** True once the item has entered a terminal state elsewhere — hides all action affordances. */
  readOnly?: boolean;
  /** True while a reject-plan submission is in flight (drives the Submit button's aria-busy state). */
  actionPending?: boolean;
  onReject?: (reason: string) => Promise<void>;
  onRegenerateWithFeedback?: () => Promise<void>;
}

const STATUS_CONFIG: Record<
  PlanReviewStatus,
  { icon: string; label: string; cardClass: string; iconClass: string; labelClass: string }
> = {
  no_plan: {
    icon: "○",
    label: "No plan yet",
    cardClass: styles.cardNoPlan,
    iconClass: styles.iconNoPlan,
    labelClass: styles.labelNoPlan,
  },
  pending_review: {
    icon: "◌",
    label: "Pending review",
    cardClass: styles.cardPendingReview,
    iconClass: styles.iconPendingReview,
    labelClass: styles.labelPendingReview,
  },
  approved: {
    icon: "✓",
    label: "Plan approved",
    cardClass: styles.cardApproved,
    iconClass: styles.iconApproved,
    labelClass: styles.labelApproved,
  },
  changes_requested: {
    icon: "✎",
    label: "Revisions requested",
    cardClass: styles.cardChangesRequested,
    iconClass: styles.iconChangesRequested,
    labelClass: styles.labelChangesRequested,
  },
  skipped: {
    icon: "⊘",
    label: "Planning skipped",
    cardClass: styles.cardSkipped,
    iconClass: styles.iconSkipped,
    labelClass: styles.labelSkipped,
  },
};

/**
 * Story 4.2.1: read-only status card + reject-with-reason form for the
 * plan-review lifecycle. Modeled on GateVerdictBox's toggle/form/focus
 * pattern. Does NOT render an Approve action — ActionsSection already owns
 * that button (avoid duplicating the same action in two places); this
 * component's own write action is "Request Changes" (and, once rejected,
 * "Regenerate Plan with This Feedback" per ADR-002).
 */
export function PlanVerdictBox({
  status,
  rejectionReason,
  readOnly = false,
  actionPending = false,
  onReject,
  onRegenerateWithFeedback,
}: PlanVerdictBoxProps) {
  const config = STATUS_CONFIG[status];

  const [showReject, setShowReject] = useState(false);
  const [reason, setReason] = useState("");
  const [localPending, setLocalPending] = useState(false);
  const [regeneratePending, setRegeneratePending] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionErrorHeadline, setActionErrorHeadline] = useState("Action failed");

  const isPending = localPending || actionPending;
  const canSubmit = reason.trim().length > 0 && !isPending;

  const toggleRef = useRef<HTMLButtonElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Focus the reject-reason textarea when the form opens — mirrors
  // GateVerdictBox's override-form focus pattern.
  useEffect(() => {
    if (showReject) {
      textareaRef.current?.focus();
    }
  }, [showReject]);

  function handleCancel() {
    setShowReject(false);
    setReason("");
    toggleRef.current?.focus();
  }

  function handleTextareaKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Escape") {
      handleCancel();
    }
  }

  async function handleSubmit() {
    if (!onReject || !canSubmit) return;
    setLocalPending(true);
    try {
      await onReject(reason);
      setShowReject(false);
      setReason("");
    } catch (err) {
      setActionErrorHeadline("Failed to request changes");
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  async function handleRegenerate() {
    if (!onRegenerateWithFeedback) return;
    setRegeneratePending(true);
    try {
      await onRegenerateWithFeedback();
    } catch (err) {
      setActionErrorHeadline("Failed to regenerate plan");
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setRegeneratePending(false);
    }
  }

  return (
    <section
      role="status"
      aria-live="polite"
      aria-atomic="true"
      aria-label="Plan review status"
      className={styles.section}
    >
      <p className={styles.sectionTitle}>Plan Review</p>

      <div className={config.cardClass}>
        <div className={styles.cardHeader}>
          <span className={config.iconClass} aria-hidden="true">
            {config.icon}
          </span>
          <span className={`${styles.label} ${config.labelClass}`}>{config.label}</span>
        </div>

        {status === "changes_requested" && rejectionReason && (
          <p className={styles.reasonText}>{rejectionReason}</p>
        )}
      </div>

      {!readOnly && status === "changes_requested" && onRegenerateWithFeedback && (
        <div className={styles.actions}>
          <button
            className={styles.primaryButton}
            onClick={() => void handleRegenerate()}
            disabled={regeneratePending}
            aria-busy={regeneratePending}
            data-testid="backlog-action-regenerate-plan"
          >
            {regeneratePending ? "Regenerating…" : "Regenerate Plan with This Feedback"}
          </button>
        </div>
      )}

      {actionError && (
        // Dismiss-only: neither reject-plan nor regenerate-plan failure has
        // a wireable retry here (the user re-triggers the action via the
        // still-open form/button below), so offering a "Retry" that just
        // clears state would misrepresent what the button does.
        <InlineError
          type="transient"
          headline={actionErrorHeadline}
          onDismiss={() => setActionError(null)}
          customMessage={actionError}
        />
      )}

      {!readOnly && onReject && (status === "pending_review" || status === "changes_requested") && (
        <div>
          {/* Toggle button stays mounted (never conditionally unmounted) so
              toggleRef remains attached — handleCancel's toggleRef.current
              would be null if this button unmounted while the form was
              open. Mirrors GateVerdictBox's overrideToggle. */}
          <button
            ref={toggleRef}
            className={styles.secondaryButton}
            aria-expanded={showReject}
            onClick={() => setShowReject((prev) => !prev)}
            data-testid="backlog-action-reject-plan"
          >
            Request Changes
          </button>

          {showReject && (
            <div role="form" aria-label="Request changes to plan" className={styles.form}>
              <label htmlFor="plan-reject-reason" className={styles.formLabel}>
                What should change? (required)
              </label>
              <textarea
                id="plan-reject-reason"
                ref={textareaRef}
                data-testid="plan-reject-reason"
                rows={3}
                placeholder="Explain what should be different about this plan…"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                onKeyDown={handleTextareaKeyDown}
                className={styles.formTextarea}
              />
              <div className={styles.formActions}>
                <button className={styles.secondaryButton} onClick={handleCancel}>
                  Cancel
                </button>
                <button
                  className={styles.primaryButton}
                  aria-disabled={!canSubmit}
                  disabled={!canSubmit}
                  aria-busy={isPending}
                  onClick={() => void handleSubmit()}
                  data-testid="backlog-action-reject-plan-submit"
                >
                  {isPending ? "Requesting…" : "Submit"}
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
