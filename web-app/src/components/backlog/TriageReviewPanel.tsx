"use client";

import { useState, useEffect, useCallback } from "react";
import { createPortal } from "react-dom";
import type { BacklogItem, TriageResult, TriageTask, AcCriterion } from "@/lib/hooks/useBacklogService";
import { TriageDiffSection } from "./TriageDiffSection";
import { TriageErrorBanner } from "./TriageErrorBanner";
import { TriageRelatedWorkSection } from "./TriageRelatedWorkSection";
import * as styles from "./TriageReviewPanel.css";

const DISMISSED_KEY = (id: string) => `triage-panel-dismissed-${id}`;

function isDismissed(itemId: string): boolean {
  if (typeof window === "undefined") return false;
  return Boolean(localStorage.getItem(DISMISSED_KEY(itemId)));
}

function setDismissed(itemId: string) {
  if (typeof window !== "undefined") {
    localStorage.setItem(DISMISSED_KEY(itemId), "1");
  }
}

interface TriageReviewPanelBaseProps {
  item: BacklogItem;
  triageResult: TriageResult;
}

/**
 * Story 4.1.2 (Structured Diagnostic): renders this panel as a read-only
 * historical record for a Headless Diagnostic Session — Apply/Skip/Refine
 * buttons and the dismiss ("Skip ×") button are omitted from the DOM (not
 * disabled — absent), and dismissal is a no-op (a historical record
 * shouldn't be dismissible). All informational content (summary,
 * suggestions, task list) still renders. There is no write-mode callback to
 * fabricate a stand-in for: the readOnly variant simply has none of them.
 */
export interface TriageReviewPanelReadOnlyProps extends TriageReviewPanelBaseProps {
  readOnly: true;
}

export interface TriageReviewPanelWriteProps extends TriageReviewPanelBaseProps {
  readOnly?: false;
  /** Called when the user clicks Apply — parent is responsible for the actual update + transition. */
  onApply: (preApplyCriteria: AcCriterion[]) => Promise<void>;
  /** Called when the user clicks Undo in the toast — parent reverts AC and status. */
  onUndoApply?: (preApplyCriteria: AcCriterion[]) => Promise<void>;
  onSkip: () => void;
  /** Called when the user submits feedback to refine this triage result. */
  onRefine?: (feedback: string) => Promise<void>;
}

export type TriageReviewPanelProps = TriageReviewPanelReadOnlyProps | TriageReviewPanelWriteProps;

function isReadOnlyProps(props: TriageReviewPanelProps): props is TriageReviewPanelReadOnlyProps {
  return props.readOnly === true;
}

/**
 * TriageReviewPanel — inline triage diff/review panel inside BacklogItemDetail.
 * Shows when triageStatus === "completed" AND item.status === "idea" AND not dismissed.
 *
 * Per UX spec Section 3.1 and Section 7.2.
 */
export function TriageReviewPanel(props: TriageReviewPanelProps) {
  const { item, triageResult } = props;
  const readOnly = isReadOnlyProps(props);
  const onApply = isReadOnlyProps(props) ? undefined : props.onApply;
  const onUndoApply = isReadOnlyProps(props) ? undefined : props.onUndoApply;
  const onSkip = isReadOnlyProps(props) ? undefined : props.onSkip;
  const onRefine = isReadOnlyProps(props) ? undefined : props.onRefine;
  // readOnly panels ignore the interactive dismissed flag entirely — it's
  // keyed by item.id, which a readOnly historical-record render (Story
  // 4.1.2) shares with the live interactive panel for the same item, so
  // respecting it here would incorrectly hide a headless diagnostic
  // session's record just because the user once dismissed the live prompt.
  // A historical record shouldn't be dismissible in the first place, so
  // isDismissed/setDismissed become no-ops in this mode.
  const [dismissed, setDismissedState] = useState(() => (readOnly ? false : isDismissed(item.id)));
  const [applyState, setApplyState] = useState<"idle" | "applying" | "error">("idle");
  const [applyError, setApplyError] = useState<string | undefined>();
  const [showUndoToast, setShowUndoToast] = useState(false);
  const [preApplyCriteria, setPreApplyCriteria] = useState<AcCriterion[] | undefined>();
  const [isMounted, setIsMounted] = useState(false);
  const [showRefineForm, setShowRefineForm] = useState(false);
  const [refineFeedback, setRefineFeedback] = useState("");
  const [refineState, setRefineState] = useState<"idle" | "submitting" | "error">("idle");
  const [refineError, setRefineError] = useState<string | undefined>();

  useEffect(() => {
    setIsMounted(true);
  }, []);

  // Re-check dismissed state when item changes (skipped in readOnly mode).
  useEffect(() => {
    if (readOnly) return;
    setDismissedState(isDismissed(item.id));
  }, [item.id, readOnly]);

  const handleDismiss = useCallback(() => {
    if (readOnly || !onSkip) return;
    setDismissed(item.id);
    setDismissedState(true);
    onSkip();
  }, [item.id, onSkip, readOnly]);

  const handleApply = useCallback(async () => {
    if (!onApply) return;
    // Cache pre-apply criteria for undo
    const cached = [...item.acCriteria];
    setPreApplyCriteria(cached);
    setApplyState("applying");
    setApplyError(undefined);
    try {
      await onApply(cached);
      setDismissed(item.id);
      setDismissedState(true);
      setShowUndoToast(true);
      // Auto-dismiss undo toast after 7s
      setTimeout(() => setShowUndoToast(false), 7000);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setApplyError(msg || "Failed to apply suggestions. The item may have been updated by another process. Reload and try again.");
      setApplyState("error");
    }
  }, [item.acCriteria, item.id, onApply]);

  const handleRefineSubmit = useCallback(async () => {
    const feedback = refineFeedback.trim();
    if (!feedback || !onRefine) return;
    setRefineState("submitting");
    setRefineError(undefined);
    try {
      await onRefine(feedback);
      setShowRefineForm(false);
      setRefineFeedback("");
      setRefineState("idle");
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setRefineError(msg || "Failed to submit feedback. Please try again.");
      setRefineState("error");
    }
  }, [refineFeedback, onRefine]);

  // Undo toast rendered via portal so it appears at the bottom of the viewport.
  // Built BEFORE the dismissed guard so the toast persists after the panel hides itself.
  const undoToast = showUndoToast && preApplyCriteria && isMounted ? (
    createPortal(
      <div className={styles.undoToastOverlay} role="status" data-testid="triage-undo-toast">
        <span>Triage applied — item is now ready.</span>
        <button
          type="button"
          className={styles.undoButton}
          onClick={() => {
            setShowUndoToast(false);
            if (onUndoApply && preApplyCriteria) {
              void onUndoApply(preApplyCriteria);
            }
          }}
          data-testid="triage-undo-button"
        >
          Undo
        </button>
      </div>,
      document.body
    )
  ) : null;

  if (dismissed) return undoToast;

  const acSuggestions = triageResult.suggestions.filter((s) => s.rationale !== "question");
  const hasSuggestions = acSuggestions.length > 0;
  const hasTasks = (triageResult.tasks?.length ?? 0) > 0;
  const isApplying = applyState === "applying";
  const isRefining = refineState === "submitting";

  return (
    <>
      <section
        className={styles.panel}
        aria-live="polite"
        data-testid="triage-review-panel"
      >
        {/* Panel header */}
        <div className={styles.panelHeader}>
          <h3 className={styles.heading}>
            Triage Ready
            {(triageResult.iteration ?? 1) > 1 && (
              <span className={styles.iterationBadge}> · Iteration {triageResult.iteration}</span>
            )}
          </h3>
          {!readOnly && (
            <button
              type="button"
              className={styles.dismissButton}
              onClick={handleDismiss}
              aria-label="Dismiss triage review"
              data-testid="triage-dismiss-button"
              disabled={isApplying}
            >
              Skip ×
            </button>
          )}
        </div>

        {/* Error banner */}
        {applyState === "error" && applyError && (
          <TriageErrorBanner
            message={applyError}
            onReload={() => {
              setApplyState("idle");
              setApplyError(undefined);
            }}
            onSkip={handleDismiss}
          />
        )}

        {/* Summary */}
        <div className={styles.summarySection}>
          <p className={styles.sectionLabel}>Summary</p>
          <p className={styles.summaryText}>{triageResult.summary}</p>
        </div>

        {!readOnly && (
          <>
            <hr className={styles.divider} aria-hidden="true" />
            <TriageRelatedWorkSection itemTitle={item.title} repoPath={item.repoPath} />
          </>
        )}

        {hasSuggestions && (
          <>
            <hr className={styles.divider} aria-hidden="true" />
            <div>
              <p className={styles.sectionLabel}>Suggested Acceptance Criteria</p>
              <TriageDiffSection
                currentCriteria={item.acCriteria}
                suggestedSuggestions={triageResult.suggestions}
              />
            </div>
          </>
        )}

        {!hasSuggestions && (
          <p className={styles.noSuggestionsText}>No AC changes suggested. You can mark this item ready manually.</p>
        )}

        {hasTasks && (
          <>
            <hr className={styles.divider} aria-hidden="true" />
            <div>
              <p className={styles.sectionLabel}>Implementation plan</p>
              <ul className={styles.taskList} data-testid="triage-task-list">
                {(triageResult.tasks ?? []).map((task: TriageTask, i: number) => (
                  <li key={i} className={styles.taskItem}>
                    <span className={styles.taskBullet} aria-hidden="true">•</span>
                    <span className={styles.taskText}>{task.text}</span>
                    {task.estimate && (
                      <span className={styles.taskEstimateBadge}>{task.estimate}</span>
                    )}
                    {task.category && (
                      <span className={styles.taskCategoryBadge}>{task.category}</span>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          </>
        )}

        {/* Actions — omitted entirely in readOnly mode (Story 4.1.2), not
            just disabled, so a historical record never implies an
            action is still possible. */}
        {!readOnly && (
          <div className={styles.actions}>
            {hasSuggestions ? (
              <button
                type="button"
                className={styles.applyButton}
                onClick={handleApply}
                disabled={isApplying}
                aria-label="Apply triage suggestions — replaces acceptance criteria and marks item ready"
                aria-busy={isApplying}
                data-testid="triage-apply-button"
              >
                {isApplying ? "Applying…" : "Apply suggestions"}
              </button>
            ) : (
              <button
                type="button"
                className={styles.applyButton}
                onClick={handleApply}
                disabled={isApplying}
                aria-busy={isApplying}
                data-testid="triage-mark-ready-button"
              >
                {isApplying ? "Applying…" : "Mark ready"}
              </button>
            )}
            <button
              type="button"
              className={styles.skipButton}
              onClick={handleDismiss}
              disabled={isApplying}
              data-testid="triage-skip-button"
            >
              Skip — review later
            </button>
            {onRefine && !showRefineForm && (
              <button
                type="button"
                className={styles.skipButton}
                onClick={() => setShowRefineForm(true)}
                disabled={isApplying}
                data-testid="triage-refine-toggle-button"
              >
                Not quite — give feedback
              </button>
            )}
          </div>
        )}

        {/* Refine with feedback */}
        {!readOnly && onRefine && showRefineForm && (
          <div className={styles.refineForm} role="form" aria-label="Refine triage with feedback">
            {refineState === "error" && refineError && (
              <TriageErrorBanner
                message={refineError}
                onReload={() => {
                  setRefineState("idle");
                  setRefineError(undefined);
                }}
                onSkip={() => setShowRefineForm(false)}
              />
            )}
            <label htmlFor="triage-refine-feedback" className={styles.refineLabel}>
              What should change?
            </label>
            <textarea
              id="triage-refine-feedback"
              rows={3}
              placeholder="e.g. missed the mobile case, re-check the auth approach"
              value={refineFeedback}
              onChange={(e) => setRefineFeedback(e.target.value)}
              className={styles.refineTextarea}
              data-testid="triage-refine-textarea"
              disabled={isRefining}
            />
            <div className={styles.actions}>
              <button
                type="button"
                className={styles.applyButton}
                onClick={() => void handleRefineSubmit()}
                disabled={isRefining || !refineFeedback.trim()}
                aria-busy={isRefining}
                data-testid="triage-refine-submit-button"
              >
                {isRefining ? "Refining…" : "Refine triage"}
              </button>
              <button
                type="button"
                className={styles.skipButton}
                onClick={() => {
                  setShowRefineForm(false);
                  setRefineFeedback("");
                }}
                disabled={isRefining}
                data-testid="triage-refine-cancel-button"
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </section>
      {undoToast}
    </>
  );
}
