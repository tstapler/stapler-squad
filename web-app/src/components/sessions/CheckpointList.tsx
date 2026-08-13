"use client";

import { useState } from "react";
import { CheckpointProto } from "@/gen/session/v1/types_pb";
import * as styles from "./CheckpointList.css";

const MAX_VISIBLE = 10;

interface CheckpointListProps {
  sessionId: string;
  checkpoints: CheckpointProto[];
  onDelete?: (checkpointId: string) => void;
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

export function CheckpointList({ sessionId: _sessionId, checkpoints, onDelete }: CheckpointListProps) {
  const [showAll, setShowAll] = useState(false);

  // Sort newest-first by timestamp
  const sorted = [...checkpoints].sort((a, b) => {
    const aTs = a.timestamp ? Number(a.timestamp.seconds) : 0;
    const bTs = b.timestamp ? Number(b.timestamp.seconds) : 0;
    return bTs - aTs;
  });

  const visible = showAll ? sorted : sorted.slice(0, MAX_VISIBLE);
  const hiddenCount = sorted.length - MAX_VISIBLE;

  if (sorted.length === 0) {
    return (
      <div className={styles.container}>
        <p className={styles.emptyState}>No checkpoints yet</p>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <ul className={styles.list} aria-label="Session checkpoints">
        {visible.map((cp) => (
          <li key={cp.id} className={styles.item}>
            <div className={styles.itemInfo}>
              <span className={styles.itemLabel} title={cp.label}>
                {cp.label || "Unnamed checkpoint"}
              </span>
              <div className={styles.itemMeta}>
                <span className={styles.timestamp}>{formatRelativeTime(cp.timestamp)}</span>
                {cp.gitCommitSha && (
                  <span className={styles.pill} title={`Git commit: ${cp.gitCommitSha}`}>
                    {cp.gitCommitSha.slice(0, 7)}
                  </span>
                )}
                {cp.claudeConvUuid && (
                  <span className={styles.pill} title={`Conversation: ${cp.claudeConvUuid}`}>
                    {cp.claudeConvUuid.slice(0, 8)}
                  </span>
                )}
              </div>
            </div>
            {onDelete && (
              <button
                className={styles.deleteButton}
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete(cp.id);
                }}
                title={`Delete checkpoint "${cp.label}"`}
                aria-label={`Delete checkpoint ${cp.label}`}
                type="button"
              >
                ×
              </button>
            )}
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
