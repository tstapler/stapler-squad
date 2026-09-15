"use client";
// +feature: backlog:send-back-feedback

import { useEffect, useRef, useState } from "react";
import * as styles from "./SendBackFeedbackBox.css";
import { InlineError } from "../InlineError";
import { InlineNotice } from "@/components/common/InlineNotice";
import { deriveSendBackErrorCopy, type SendBackErrorCopy } from "./SendBackError";

export interface SendBackFeedbackBoxProps {
  visible: boolean;
  activeWorkSessionCount: number;
  /** True while another action is in flight elsewhere in ActionsSection — disables the toggle. */
  disabled?: boolean;
  /** True while THIS box's own submit is in flight (drives aria-busy). */
  actionPending?: boolean;
  onSubmit: (feedback: string) => Promise<void>;
}

// Renders null only when the action is ineligible (visible=false) AND
// there's no pending actionError AND no submit (incl. a Retry) is in
// flight — so a partial-failure error set just before a parent re-render
// still gets shown even after `visible` flips false, and a Retry launched
// from a `visible=false`/no-error state stays rendered for the duration of
// its own pending phase instead of blanking. See the early-return guard below.
export function SendBackFeedbackBox({
  visible,
  activeWorkSessionCount,
  disabled = false,
  actionPending = false,
  onSubmit,
}: SendBackFeedbackBoxProps) {
  const [showForm, setShowForm] = useState(false);
  const [feedback, setFeedback] = useState("");
  const [localPending, setLocalPending] = useState(false);
  const [actionError, setActionError] = useState<SendBackErrorCopy | null>(null);

  const isPending = localPending || actionPending;
  const canSubmit = feedback.trim().length > 0 && !isPending;

  const toggleRef = useRef<HTMLButtonElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Mirrors BacklogItemDetail.tsx's mountedRef convention: guards the four
  // setState calls below handleSubmit's `await onSubmit(...)` against firing
  // after this box has unmounted mid-request.
  const isMountedRef = useRef(true);
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (showForm) textareaRef.current?.focus();
  }, [showForm]);

  // Stays mounted while `isPending` too (not just while there's an
  // actionError): the row-4 Retry clears actionError before onSubmit
  // resolves, while `visible` can still be false at that instant — without
  // this, the guard would unmount the in-flight Retry UI.
  if (!visible && !actionError && !isPending) return null;

  function handleCancel() {
    // Gated on isPending, matching Submit's existing gating: without this,
    // a click or Escape mid-submit visually aborts the form while the
    // in-flight request keeps running unseen.
    if (isPending) return;
    setShowForm(false);
    setFeedback("");
    setActionError(null); // lets the component unmount on its next render once !visible (see the guard above)
    toggleRef.current?.focus();
  }

  function handleTextareaKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Escape") handleCancel();
  }

  // Retry re-invokes the exact same submit path: handleSubmit already reads
  // the current `feedback` state and delegates to `onSubmit`, which reads
  // the parent's current item status — no separate retry codepath needed.
  function handleRetry() {
    void handleSubmit();
  }

  async function handleSubmit() {
    if (!canSubmit) return;
    setLocalPending(true);
    // Clear any stale error from a prior attempt before this one starts —
    // load-bearing for Retry, which calls handleSubmit directly.
    setActionError(null);
    try {
      await onSubmit(feedback);
      if (!isMountedRef.current) return;
      setShowForm(false);
      setFeedback("");
    } catch (err) {
      if (isMountedRef.current) setActionError(deriveSendBackErrorCopy(err));
      console.error(err);
    } finally {
      if (isMountedRef.current) setLocalPending(false);
    }
  }

  return (
    <div className={styles.section}>
      {visible && (
        <button
          ref={toggleRef}
          className={styles.secondaryButton}
          aria-expanded={showForm}
          disabled={disabled || isPending}
          onClick={() => setShowForm((prev) => !prev)}
          data-testid="backlog-action-send-back-feedback"
        >
          ↩ Send back for re-planning
        </button>
      )}

      {showForm && (
        <div role="form" aria-label="Send back for re-planning" className={styles.form}>
          {activeWorkSessionCount > 0 && (
            <InlineNotice
              message="This item has an active session — submitting this will stop it, and its work may be superseded once re-planning completes."
              data-testid="send-back-active-session-notice"
            />
          )}
          <label htmlFor="send-back-feedback" className={styles.formLabel}>
            What should change? (required)
          </label>
          <textarea
            id="send-back-feedback"
            ref={textareaRef}
            data-testid="send-back-feedback-textarea"
            rows={3}
            placeholder="e.g. missed the mobile case, re-check the auth approach"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            onKeyDown={handleTextareaKeyDown}
            className={styles.formTextarea}
          />
          {actionError && (
            <InlineError
              type="transient"
              headline={actionError.headline}
              onDismiss={() => setActionError(null)}
              customMessage={actionError.message}
              // Wired only for the retryable=true (idea-landing) case — the
              // other branches never set retryable, preserving ux.md's
              // "no misleading retry" discipline for those.
              onRetry={actionError.retryable ? handleRetry : undefined}
              retryAriaLabel="Retry send-back with this feedback"
            />
          )}
          <div className={styles.formActions}>
            <button
              className={styles.secondaryButton}
              onClick={handleCancel}
              disabled={isPending}
              aria-disabled={isPending}
            >
              Cancel
            </button>
            <button
              className={styles.submitButton}
              aria-disabled={!canSubmit}
              disabled={!canSubmit}
              aria-busy={isPending}
              onClick={() => void handleSubmit()}
              data-testid="backlog-action-send-back-feedback-submit"
            >
              {isPending ? "Sending…" : "Submit"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
