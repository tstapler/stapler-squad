"use client";
// +feature: backlog:send-back-feedback

import { useEffect, useRef, useState } from "react";
import * as styles from "./SendBackFeedbackBox.css";
import { InlineError } from "../InlineError";
import { InlineNotice } from "@/components/common/InlineNotice";
import { getErrorMessage } from "@/lib/utils/connectError"; // matches BacklogItemDetail.tsx:33's existing import
import { SendBackError } from "./SendBackError"; // Task 2.2.1a — carries failedAt + itemStatusAfterFailure

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
// (Task 2.2.1a's load()) still gets shown even after `visible` flips
// false, and a Retry launched from a `visible=false`/no-error state stays
// rendered for the duration of its own pending phase instead of blanking.
// See the early-return guard below.
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
  // headline/message pair, not a bare string: BLOCKER 5.2's fix needs distinct
  // copy for "transitionStatus itself failed" vs. "it succeeded but a later
  // call failed" (ux.md Surface 7), so a single string can no longer carry both
  // the headline and the body. `retryable` (iteration 3 repair pass, BLOCKER B)
  // is true only for the "landed at idea" case (Surface 7 row 4) — idea→ready
  // is itself a valid transition (session/domain/backlog.go's validTransitions),
  // so unlike the row 2/3 cases (where resubmitting would replay
  // transitionStatus against an already-"ready" item and fail), retrying from
  // "idea" is a genuine, correct recovery path, not a misleading affordance.
  const [actionError, setActionError] = useState<{ headline: string; message: string; retryable?: boolean } | null>(
    null
  );

  const isPending = localPending || actionPending;
  const canSubmit = feedback.trim().length > 0 && !isPending;

  const toggleRef = useRef<HTMLButtonElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (showForm) textareaRef.current?.focus();
  }, [showForm]);

  // Stay mounted even when `visible` goes false, as long as there's an
  // unresolved actionError to show: handleSendBackWithFeedback's catch
  // block (Task 2.2.1a) calls load() before re-throwing, which re-fetches
  // the item and can flip `visible` to false (its new status — "ready" or
  // "idea" — is outside CAN_SEND_BACK_READY) before Surface 7's error copy
  // ever has a chance to render. Because actionError is local useState,
  // it survives that parent re-render as long as this component instance
  // stays mounted — which is exactly what this guard preserves (fixes a
  // BLOCKER a fresh UX triad-lens review found in the iteration 2
  // plan-repair pass; see ux.md Surface 7's mechanism note).
  //
  // Also stay mounted while `isPending` is true (iteration 4 repair pass,
  // BLOCKER): the row-4 Retry (handleRetry -> handleSubmit) sets
  // localPending true AND clears actionError to null in the same call,
  // before onSubmit resolves. At that instant `visible` is still false
  // (the item's status is still "idea", outside CAN_SEND_BACK_READY, until
  // the retry's own transitionStatus->rejectPlan->triggerTriage chain
  // settles) and actionError is now null too — so without the isPending
  // check, this guard would return null for the whole retry, unmounting
  // the toggle, form, and "Sending..."/aria-busy UI and re-mounting only
  // once the promise settles. isPending is already computed above (line
  // 601), so this adds no new state, and once pending finishes with the
  // error cleared and visible still false, the guard again correctly
  // returns null (no stuck-mounted-forever regression).
  if (!visible && !actionError && !isPending) return null;

  function handleCancel() {
    // Gated on isPending, matching Submit's existing gating: without this,
    // a click or Escape mid-submit visually aborts the form while the
    // in-flight transitionStatus->rejectPlan->triggerTriage chain keeps
    // running unseen, dropping its eventual success toast or error.
    if (isPending) return;
    setShowForm(false);
    setFeedback("");
    setActionError(null); // lets the component unmount on its next render once !visible (see the guard above)
    toggleRef.current?.focus();
  }

  function handleTextareaKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Escape") handleCancel();
  }

  // BLOCKER B fix (iteration 3 repair pass): re-invokes the exact same
  // submit path — handleSubmit already reads the CURRENT `feedback` state
  // (untouched by this failure) and delegates to `onSubmit`
  // (handleSendBackWithFeedback in the parent), which itself reads the
  // parent's current `item.status`/`item.updatedAtRaw` — "idea" by this
  // point, not the originally-assumed source status — so no separate retry
  // codepath is needed; only a rendering hook to reach it. See Task 2.2.1a's
  // note on why the parent's closure is already current.
  function handleRetry() {
    void handleSubmit();
  }

  async function handleSubmit() {
    if (!canSubmit) return;
    setLocalPending(true);
    // Clear any stale error from a prior attempt before this one starts —
    // load-bearing for Retry (below), which calls handleSubmit directly:
    // without this, a successful retry would leave the old actionError
    // sitting in state (harmless today since showForm/visible both gate its
    // rendering, but not future-proof) and a *second* failed retry would
    // briefly show the previous attempt's copy before the new branch runs.
    setActionError(null);
    try {
      await onSubmit(feedback);
      setShowForm(false);
      setFeedback("");
    } catch (err) {
      // BLOCKER 5.2 fix: distinguish "transitionStatus itself failed" (nothing
      // changed server-side — ux.md Surface 6) from "it succeeded but a later
      // call failed" (ux.md Surface 7) using the SendBackError tag
      // handleSendBackWithFeedback (Task 2.2.1a) throws. A plain rejection with
      // no SendBackError wrapper (e.g. a unit test calling onSubmit directly)
      // falls back to the generic Surface 6 copy.
      if (err instanceof SendBackError && err.failedAt !== "transition") {
        // Pre-mortem P1 #1: triggerTriage's own internal ready→idea CAS
        // (backlog_service_trigger_triage.go:249-260) can commit before a
        // later step in that same call fails, so the item may now be at
        // "idea" rather than "ready" — in which case neither PlanVerdictBox
        // card (ready-only) is the right pointer.
        if (err.itemStatusAfterFailure === "idea") {
          // BLOCKER B fix (iteration 3 repair pass): the old copy here told
          // the operator to "close this form and re-run Send back for
          // re-planning from there" — but SendBackFeedbackBox's own
          // `visible` prop can never be true at "idea" (CAN_SEND_BACK_READY
          // excludes it), so that pointed at an affordance that cannot
          // render. It also claimed "resubmitting this box now will fail,"
          // which is wrong: idea→ready is itself a valid transition
          // (session/domain/backlog.go's validTransitions), so replaying
          // this exact chain from "idea" works. The feedback text is still
          // sitting in this component's own `feedback` state (never
          // cleared on this failure path) — offer a real Retry instead of
          // a dead pointer.
          setActionError({
            headline: "Sent back, but retriage didn't start",
            message:
              'The item moved back to "idea" before retriage could start. ' +
              "Your feedback is still in this box — click Retry to send it " +
              'back to "ready" and start retriage again.',
            retryable: true,
          });
        } else if (err.failedAt === "reject") {
          setActionError({
            headline: "Sent back, but retriage didn't start",
            message:
              'The item already moved to "ready." Close this form and use ' +
              'the "Request Changes" button below to record your feedback ' +
              "and retry — do not resubmit this box.",
          });
        } else {
          // BLOCKER A fix (iteration 3 repair pass): the old copy pointed at
          // the plain "Trigger Triage" button. That button
          // (ActionsSection.tsx) calls triggerTriage(item.id) with NO
          // feedback argument, and the backend only ever reads feedback from
          // the RPC request — never from the stored PlanRejectionReason
          // field rejectPlan (call 2) already persisted — so following the
          // old instruction silently discarded the operator's feedback. At
          // this point rejectPlan succeeded, so PlanVerdictBox is showing
          // its changes_requested card with a "Regenerate Plan with This
          // Feedback" button (PlanVerdictBox.tsx) that explicitly passes
          // item.planRejectionReason to triggerTriage — that's the correct,
          // feedback-preserving affordance to name here.
          setActionError({
            headline: "Sent back, but retriage didn't start",
            message:
              'The item already moved to "ready" and your feedback was ' +
              'recorded. Close this form and use the "Regenerate Plan ' +
              'with This Feedback" button below to retry — do not ' +
              "resubmit this box.",
          });
        }
      } else {
        setActionError({
          headline: "Failed to send back",
          message: getErrorMessage(err, "Failed to send back."),
        });
      }
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  return (
    <div className={styles.section}>
      {visible && (
        <button
          ref={toggleRef}
          className={styles.toggleButton}
          aria-expanded={showForm}
          disabled={disabled}
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
              // BLOCKER B fix: reuses InlineError's existing onRetry
              // affordance (already built for exactly this "retry is real,
              // not misleading" case, per its own doc comment) — wired only
              // for the retryable=true (idea-landing) case. The row 2/3
              // branches above never set retryable, so they keep getting no
              // onRetry, preserving ux.md's existing "no misleading retry"
              // discipline for those.
              onRetry={actionError.retryable ? handleRetry : undefined}
              retryAriaLabel="Retry send-back with this feedback"
            />
          )}
          <div className={styles.formActions}>
            <button
              className={styles.toggleButton}
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
