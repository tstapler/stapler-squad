"use client";

import * as styles from "./RetryBadge.css";

interface RetryBadgeProps {
  retryAttempt: number;
  retryMaxAttempts: number;
  compact?: boolean;
}

/**
 * Compact "🔁 N/max" badge shown once a session has consumed at least one
 * automated retry attempt (session-retry-backoff, AC4). Renders nothing at
 * attempt 0 — the majority case (healthy sessions) should see zero visual
 * noise, per research/ux.md.
 */
export function RetryBadge({ retryAttempt, retryMaxAttempts, compact = false }: RetryBadgeProps) {
  if (retryAttempt <= 0) {
    return null;
  }

  const isFinalAttempt = retryMaxAttempts > 0 && retryAttempt >= retryMaxAttempts;
  const toneClass = isFinalAttempt ? styles.warning : styles.neutral;
  const label = `Retry attempt ${retryAttempt} of ${retryMaxAttempts}`;

  if (compact) {
    return (
      <span
        className={`${styles.badge} ${toneClass}`}
        role="img"
        aria-label={label}
        title={label}
      >
        <span aria-hidden="true">🔁</span>
        {retryAttempt}/{retryMaxAttempts}
      </span>
    );
  }

  return (
    <span className={`${styles.badge} ${toneClass}`} role="img" aria-label={label}>
      <span aria-hidden="true">🔁</span>
      {label}
    </span>
  );
}
