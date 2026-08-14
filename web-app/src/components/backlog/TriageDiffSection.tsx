"use client";
// +feature: backlog-triage-question-answer

import { useEffect, useRef, useState } from "react";
import type { AcCriterion, TriageSuggestion } from "@/lib/hooks/useBacklogService";
import { composeQuestionAnswerFeedback } from "@/lib/backlog/composeQuestionAnswerFeedback";
import { InlineError } from "./InlineError";
import * as styles from "./TriageDiffSection.css";

interface TriageDiffSectionProps {
  currentCriteria: AcCriterion[];
  suggestedSuggestions: TriageSuggestion[];
  /** Called with a composed "Q:.../A:..." feedback string when the operator submits an answer to one question. Absent in read-only historical renders. */
  onAnswerQuestion?: (feedback: string) => Promise<void>;
}

/**
 * TriageDiffSection — two-column diff showing current AC vs suggested AC.
 * Filters out suggestions with rationale === "question" (those render in a
 * separate "Triage Questions" section below the diff).
 * Uses text comparison for simple set diff (v1 — no LCS needed).
 */
export function TriageDiffSection({ currentCriteria, suggestedSuggestions, onAnswerQuestion }: TriageDiffSectionProps) {
  // Filter out question suggestions
  const acSuggestions = suggestedSuggestions.filter((s) => s.rationale !== "question");
  const questionSuggestions = suggestedSuggestions.filter((s) => s.rationale === "question");

  const currentTexts = new Set(currentCriteria.map((c) => c.text.trim()));
  const suggestedTexts = new Set(acSuggestions.map((s) => s.text.trim()));

  // Added: in suggested but not in current
  const added = acSuggestions.filter((s) => !currentTexts.has(s.text.trim()));
  // Removed: in current but not in suggested
  const removed = currentCriteria.filter((c) => !suggestedTexts.has(c.text.trim()));
  // Retained: in both
  const retained = currentCriteria.filter((c) => suggestedTexts.has(c.text.trim()));

  // Per-question answer form state (Task 1.1.1c). Keyed by the question's
  // index within questionSuggestions — no stable question ID exists.
  const [openIndex, setOpenIndex] = useState<number | null>(null);
  const [answerDrafts, setAnswerDrafts] = useState<Record<number, string>>({});
  const [answeredIndices, setAnsweredIndices] = useState<Set<number>>(new Set());
  const [submittingIndex, setSubmittingIndex] = useState<number | null>(null);
  const [errorIndex, setErrorIndex] = useState<number | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | undefined>();
  // Set right before closing a form (Cancel/Escape/submit-success) so the
  // effect below can return focus to the toggle button only after the DOM
  // has re-rendered with the toggle visible again (it's `hidden` while its
  // form is open, so focusing it synchronously in the click handler — before
  // that re-render commits — would silently no-op).
  const [focusToggleIndex, setFocusToggleIndex] = useState<number | null>(null);

  const toggleRefs = useRef<Record<number, HTMLButtonElement | null>>({});
  const textareaRefs = useRef<Record<number, HTMLTextAreaElement | null>>({});

  useEffect(() => {
    if (openIndex !== null) {
      textareaRefs.current[openIndex]?.focus();
    }
  }, [openIndex]);

  useEffect(() => {
    if (focusToggleIndex !== null) {
      toggleRefs.current[focusToggleIndex]?.focus();
      setFocusToggleIndex(null);
    }
  }, [focusToggleIndex, openIndex]);

  const handleCancel = (i: number) => {
    setOpenIndex(null);
    setErrorIndex(null);
    setErrorMessage(undefined);
    setFocusToggleIndex(i);
  };

  const handleSubmit = async (i: number, questionText: string) => {
    const draft = (answerDrafts[i] ?? "").trim();
    if (!draft || submittingIndex === i || !onAnswerQuestion) return;
    setSubmittingIndex(i);
    setErrorIndex(null);
    setErrorMessage(undefined);
    try {
      const composed = composeQuestionAnswerFeedback(questionText, draft);
      await onAnswerQuestion(composed);
      // Keep the draft in state (rather than clearing it) — the
      // "✓ Answered: {draft}" marker below needs the submitted text to
      // display. Closing the form (openIndex -> null) already removes the
      // editable textarea, which is what "clears the draft" means in
      // practice: no stale editable input left behind.
      setAnsweredIndices((prev) => new Set(prev).add(i));
      setOpenIndex(null);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setErrorIndex(i);
      setErrorMessage(msg || "Failed to submit answer. Please try again.");
    } finally {
      setSubmittingIndex(null);
    }
  };

  return (
    <div>
      <div className={styles.diffContainer}>
        {/* Current AC column */}
        <div className={styles.diffColumn}>
          <div className={styles.columnHeader}>
            Current ({currentCriteria.length} {currentCriteria.length === 1 ? "criterion" : "criteria"})
          </div>
          {currentCriteria.length === 0 ? (
            <span className={styles.emptyState}>(none)</span>
          ) : (
            <>
              {retained.map((c) => (
                <div key={c.index} className={styles.addedItem}>
                  <span className={styles.diffPrefix} aria-hidden="true"> </span>
                  <span>{c.text}</span>
                </div>
              ))}
              {removed.map((c) => (
                <div key={c.index} className={styles.removedItem} aria-label={`Removed: ${c.text}`}>
                  <span className={styles.diffPrefix} aria-hidden="true">−</span>
                  <span>{c.text}</span>
                </div>
              ))}
            </>
          )}
        </div>

        {/* Suggested AC column */}
        <div className={styles.diffColumn}>
          <div className={styles.columnHeader}>
            Suggested ({acSuggestions.length} {acSuggestions.length === 1 ? "criterion" : "criteria"})
          </div>
          {acSuggestions.length === 0 ? (
            <span className={styles.emptyState}>(no changes suggested)</span>
          ) : (
            acSuggestions.map((s, i) => {
              const isNew = !currentTexts.has(s.text.trim());
              return isNew ? (
                <div key={i} className={styles.addedItem} aria-label={`Added: ${s.text}`}>
                  <span className={styles.diffPrefix} aria-hidden="true">+</span>
                  <span>{s.text}</span>
                </div>
              ) : (
                <div key={i} className={styles.addedItem}>
                  <span className={styles.diffPrefix} aria-hidden="true"> </span>
                  <span>{s.text}</span>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* Questions section (R7-lite) */}
      {questionSuggestions.length > 0 && (
        <div className={styles.questionsSection}>
          <h4 className={styles.questionsHeading}>Triage Questions</h4>
          {questionSuggestions.map((q, i) => {
            const isOpen = openIndex === i;
            const isAnswered = answeredIndices.has(i);
            const isSubmitting = submittingIndex === i;
            const draft = answerDrafts[i] ?? "";

            return (
              <div key={i} className={styles.questionRow}>
                <div className={styles.questionItem}>{q.text}</div>

                {isAnswered ? (
                  <div role="status" aria-live="polite" className={styles.answeredMarker}>
                    ✓ Answered: {draft}
                  </div>
                ) : (
                  onAnswerQuestion && (
                    <>
                      {/* Always rendered (even while open) so the Cancel/Escape
                          handlers below have a stable ref to return focus to —
                          unmounting it while open would null out the ref. */}
                      <button
                        type="button"
                        ref={(el) => {
                          toggleRefs.current[i] = el;
                        }}
                        aria-expanded={isOpen}
                        aria-controls={`triage-question-answer-input-${i}`}
                        data-testid={`triage-question-answer-toggle-${i}`}
                        className={styles.answerToggle}
                        onClick={() => setOpenIndex(isOpen ? null : i)}
                        hidden={isOpen}
                      >
                        Answer ▸
                      </button>

                      {isOpen && (
                        <div className={styles.answerForm} role="form" aria-label={`Answer: ${q.text}`}>
                          {errorIndex === i && errorMessage && (
                            // Real retry: re-attempts submit with the same
                            // draft (still in answerDrafts, untouched on
                            // failure). Dismiss closes the form entirely,
                            // matching the prior "Skip without applying"
                            // behavior (discards the in-progress answer).
                            <InlineError
                              type="transient"
                              headline="Failed to submit answer"
                              retryAriaLabel="Retry submitting answer"
                              customMessage={errorMessage}
                              onRetry={() => void handleSubmit(i, q.text)}
                              onDismiss={() => handleCancel(i)}
                            />
                          )}
                          <textarea
                            id={`triage-question-answer-input-${i}`}
                            data-testid={`triage-question-answer-input-${i}`}
                            ref={(el) => {
                              textareaRefs.current[i] = el;
                            }}
                            rows={3}
                            className={styles.answerTextarea}
                            value={draft}
                            disabled={isSubmitting}
                            onChange={(e) =>
                              setAnswerDrafts((prev) => ({ ...prev, [i]: e.target.value }))
                            }
                            onKeyDown={(e) => {
                              if (e.key === "Escape") {
                                handleCancel(i);
                              }
                            }}
                          />
                          <div className={styles.answerActions}>
                            <button
                              type="button"
                              className={styles.answerSubmitButton}
                              data-testid={`triage-question-answer-submit-${i}`}
                              aria-disabled={!draft.trim() || isSubmitting}
                              aria-busy={isSubmitting}
                              disabled={!draft.trim() || isSubmitting}
                              onClick={() => {
                                if (!draft.trim() || isSubmitting) return;
                                void handleSubmit(i, q.text);
                              }}
                            >
                              {isSubmitting ? "Submitting…" : "Submit"}
                            </button>
                            <button
                              type="button"
                              className={styles.answerCancelButton}
                              data-testid={`triage-question-answer-cancel-${i}`}
                              disabled={isSubmitting}
                              onClick={() => handleCancel(i)}
                            >
                              Cancel
                            </button>
                          </div>
                        </div>
                      )}
                    </>
                  )
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
