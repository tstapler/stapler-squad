"use client";

import * as styles from "../BacklogItemDetail.css";

/**
 * Renders a button's label, swapping in a spinner + "Running…" while
 * `pending`. Shared by ActionsSection, PullRequestSection, and NotesSection
 * — each had copy-pasted this same helper after the Epic 3 extraction from
 * BacklogItemDetail.tsx.
 */
export function ActionButtonLabel({ pending, label }: { pending: boolean; label: string }) {
  if (!pending) return <>{label}</>;
  return (
    <>
      <span className={styles.buttonSpinner} aria-hidden="true" />
      Running…
    </>
  );
}
