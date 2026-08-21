"use client";

import * as styles from "./TriageErrorBanner.css";

interface TriageErrorBannerProps {
  message: string;
  onReload: () => void;
  onSkip: () => void;
}

/**
 * TriageErrorBanner — inline error state for the TriageReviewPanel.
 * Renders above the diff so context is preserved during error recovery.
 * Uses role="alert" so screen readers announce it immediately.
 */
export function TriageErrorBanner({ message, onReload, onSkip }: TriageErrorBannerProps) {
  return (
    <div role="alert" className={styles.banner} data-testid="triage-error-banner">
      <p className={styles.message}>{message}</p>
      <div className={styles.actions}>
        <button
          type="button"
          className={styles.reloadButton}
          onClick={onReload}
          data-testid="triage-error-reload"
        >
          Reload item
        </button>
        <button
          type="button"
          className={styles.skipButton}
          onClick={onSkip}
          data-testid="triage-error-skip"
        >
          Skip without applying
        </button>
      </div>
    </div>
  );
}
