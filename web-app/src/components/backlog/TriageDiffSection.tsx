"use client";

import type { AcCriterion, TriageSuggestion } from "@/lib/hooks/useBacklogService";
import * as styles from "./TriageDiffSection.css";

interface TriageDiffSectionProps {
  currentCriteria: AcCriterion[];
  /** All suggestions from triage result — questions (rationale === "question") are filtered out */
  suggestedSuggestions: TriageSuggestion[];
}

/**
 * TriageDiffSection — two-column diff showing current AC vs suggested AC.
 * Filters out suggestions with rationale === "question" (those render in a
 * separate "Triage Questions" section below the diff).
 * Uses text comparison for simple set diff (v1 — no LCS needed).
 */
export function TriageDiffSection({ currentCriteria, suggestedSuggestions }: TriageDiffSectionProps) {
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
          {questionSuggestions.map((q, i) => (
            <div key={i} className={styles.questionItem}>
              {q.text}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
