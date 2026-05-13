"use client";
// +feature: backlog:triage-loading-indicator

import * as styles from "./TriageLoadingIndicator.css";

interface TriageLoadingIndicatorProps {
  elapsedSeconds: number; // updated externally via setInterval
  context: "item" | "list";
  onCancel: () => void;
  compact?: boolean; // true = pill form (list context)
}

function getLabel(context: "item" | "list", elapsedSeconds: number): string {
  if (context === "item") {
    if (elapsedSeconds < 60) return "Thinking about acceptance criteria...";
    return "Still thinking — up to 3 min";
  }
  // list / compact context
  if (elapsedSeconds < 60) return "Thinking...";
  return "Still working — up to 3 min";
}

export function TriageLoadingIndicator({
  elapsedSeconds,
  context,
  onCancel,
  compact = false,
}: TriageLoadingIndicatorProps) {
  if (elapsedSeconds >= 180) {
    return null;
  }

  const ariaElapsed = Math.floor(elapsedSeconds / 30) * 30;
  const ariaLabel = `Triage in progress, ${ariaElapsed} seconds elapsed`;
  const labelText = getLabel(context, elapsedSeconds);

  return (
    <div
      className={compact ? styles.compactContainer : styles.container}
      role="status"
      aria-live="polite"
      aria-label={ariaLabel}
    >
      <span className={styles.spinnerHidden} aria-hidden="true" />
      <span className={styles.label}>{labelText}</span>
      <span className={styles.elapsed}>{elapsedSeconds}s</span>
      {compact ? (
        <button
          aria-label="Cancel triage"
          onClick={onCancel}
          className={styles.cancelButtonCompact}
        >
          ×
        </button>
      ) : (
        <button
          aria-label="Cancel triage"
          onClick={onCancel}
          className={styles.cancelButton}
        >
          Stop
        </button>
      )}
    </div>
  );
}
