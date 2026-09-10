"use client";

import { useState } from "react";
import type { RetryAttemptRecord } from "@/gen/session/v1/types_pb";
import * as styles from "./CheckpointList.css";

const MAX_VISIBLE = 10;

interface RetryHistoryListProps {
  /** Absent for a session predating this feature or not yet retried. */
  history?: RetryAttemptRecord[];
  /** Loading skeleton state — distinct from an empty history (AC10). */
  isLoading?: boolean;
}

function formatRelativeTime(timestamp?: { seconds: bigint; nanos: number }): string {
  if (!timestamp || timestamp.seconds === BigInt(0)) return "Unknown time";
  const now = Date.now();
  const date = new Date(Number(timestamp.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);

  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

/**
 * Retry attempt history for a session (session-retry-backoff, AC5). Clones
 * CheckpointList's shape verbatim (newest-first, MAX_VISIBLE + show-all
 * toggle, formatRelativeTime, empty state).
 */
export function RetryHistoryList({ history = [], isLoading = false }: RetryHistoryListProps) {
  const [showAll, setShowAll] = useState(false);

  if (isLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>Retry History</div>
        <p className={styles.emptyState} aria-hidden="true">
          Loading retry history…
        </p>
      </div>
    );
  }

  // Newest first — the proto field is append-ordered (oldest first).
  const sorted = [...history].sort((a, b) => b.attempt - a.attempt);

  if (sorted.length === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.header}>Retry History</div>
        <p className={styles.emptyState}>No retries yet</p>
      </div>
    );
  }

  const visible = showAll ? sorted : sorted.slice(0, MAX_VISIBLE);
  const hiddenCount = sorted.length - MAX_VISIBLE;

  return (
    <div className={styles.container}>
      <div className={styles.header}>Retry History</div>
      <ul className={styles.list} aria-label="Retry attempt history">
        {visible.map((rec) => (
          <li key={rec.attempt} className={styles.item}>
            <div className={styles.itemInfo}>
              <span className={styles.itemLabel}>Attempt {rec.attempt}</span>
              <div className={styles.itemMeta}>
                <span className={styles.timestamp}>{formatRelativeTime(rec.timestamp)}</span>
                <span className={styles.pill} title={`Failure reason: ${rec.reason}`}>
                  {rec.reason}
                </span>
              </div>
            </div>
          </li>
        ))}
      </ul>
      {!showAll && hiddenCount > 0 && (
        <button
          className={styles.showMoreButton}
          onClick={(e) => {
            e.stopPropagation();
            setShowAll(true);
          }}
          type="button"
        >
          Show all ({sorted.length})
        </button>
      )}
    </div>
  );
}
