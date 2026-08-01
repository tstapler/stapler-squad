"use client";
// +feature: backlog-plan-verdict-box

import { useEffect, useRef, useState } from "react";
import type { PlanReviewStatus } from "@/lib/backlog/planReviewStatus";
import * as styles from "./PlanVerdictBox.css";

export interface PlanVerdictBoxProps {
  status: PlanReviewStatus;
  rejectionReason?: string;
  readOnly?: boolean;
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
    label: "Changes requested",
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

export function PlanVerdictBox({
  status,
  rejectionReason,
  readOnly = false,
  actionPending = false,
  onReject,
  onRegenerateWithFeedback,
}: PlanVerdictBoxProps) {
  const [showRejectForm, setShowRejectForm] = useState(false);
  const [reason, setReason] = useState("");
  const [localPending, setLocalPending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);

  const isPending = localPending || actionPending;
  const config = STATUS_CONFIG[status];

  useEffect(() => {
    if (showRejectForm) {
      textareaRef.current?.focus();
    }
  }, [showRejectForm]);

  async function handleRejectSubmit() {
    if (!onReject || reason.trim() === "") return;
    setLocalPending(true);
    try {
      await onReject(reason.trim());
      setShowRejectForm(false);
      setReason("");
    } catch (err) {
      // Caller (BacklogItemDetail's handleRejectPlan) already surfaces a
      // toast and re-throws so callers can react — swallow it here rather
      // than letting it become an unhandled rejection from the fire-and-
      // forget onClick, and leave the form open so the user can retry.
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  async function handleRegenerate() {
    if (!onRegenerateWithFeedback) return;
    setLocalPending(true);
    try {
      await onRegenerateWithFeedback();
    } catch (err) {
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  return (
    <section role="status" aria-live="polite" aria-atomic="true" aria-label="Plan review status" className={styles.section}>
      <div className={config.cardClass}>
        <div className={styles.header}>
          <span className={config.iconClass} aria-hidden="true">
            {config.icon}
          </span>
          <span className={`${styles.label} ${config.labelClass}`}>{config.label}</span>
        </div>

        {status === "changes_requested" && rejectionReason && (
          <p className={styles.reasonText} data-testid="plan-rejection-reason">
            {rejectionReason}
          </p>
        )}
      </div>

      {!readOnly && status === "changes_requested" && onRegenerateWithFeedback && (
        <div className={styles.actions}>
          <button
            className={styles.primaryButton}
            disabled={isPending}
            onClick={() => void handleRegenerate()}
            data-testid="backlog-action-regenerate-plan"
          >
            Regenerate Plan with This Feedback
          </button>
        </div>
      )}

      {!readOnly && (status === "pending_review" || status === "approved") && onReject && (
        <div className={styles.actions}>
          <button
            ref={toggleRef}
            className={styles.secondaryButton}
            disabled={isPending}
            aria-expanded={showRejectForm}
            onClick={() => setShowRejectForm((prev) => !prev)}
            data-testid="backlog-action-reject-plan"
          >
            Request Changes
          </button>
        </div>
      )}

      {!readOnly && showRejectForm && (
        <div role="form" aria-label="Request changes" className={styles.form}>
          <label htmlFor="plan-reject-reason" className={styles.formLabel}>
            What should change? (required)
          </label>
          <textarea
            id="plan-reject-reason"
            ref={textareaRef}
            rows={3}
            placeholder="Explain what's missing or needs to change..."
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            className={styles.formTextarea}
            data-testid="plan-reject-reason"
          />
          <div className={styles.formActions}>
            <button
              className={styles.secondaryButton}
              onClick={() => {
                setShowRejectForm(false);
                setReason("");
                toggleRef.current?.focus();
              }}
            >
              Cancel
            </button>
            <button
              className={styles.primaryButton}
              aria-disabled={isPending || reason.trim() === ""}
              disabled={isPending || reason.trim() === ""}
              onClick={() => void handleRejectSubmit()}
              data-testid="backlog-action-reject-plan-submit"
            >
              Submit
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
