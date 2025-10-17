"use client";

import { useState } from "react";
import { useReviewQueue } from "@/lib/hooks/useReviewQueue";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { Priority, AttentionReason } from "@/gen/session/v1/types_pb";
import styles from "./ReviewQueuePanel.module.css";

interface ReviewQueuePanelProps {
  onSessionClick?: (sessionId: string) => void;
  onSkipSession?: (sessionId: string) => Promise<void>;
  autoRefresh?: boolean;
  refreshInterval?: number;
}

/**
 * ReviewQueuePanel displays all sessions that need user attention.
 *
 * Shows items sorted by priority with filtering capabilities.
 * Uses hybrid push/poll strategy for real-time updates:
 * - WebSocket push notifications for immediate session status changes
 * - 30-second fallback polling to catch any missed events
 *
 * @example
 * ```tsx
 * <ReviewQueuePanel
 *   onSessionClick={(id) => navigateToSession(id)}
 *   autoRefresh={true}
 *   refreshInterval={5000}
 * />
 * ```
 */
export function ReviewQueuePanel({
  onSessionClick,
  onSkipSession,
  autoRefresh = true,
  refreshInterval = 5000,
}: ReviewQueuePanelProps) {
  const [priorityFilter, setPriorityFilter] = useState<Priority | undefined>(
    undefined
  );
  const [reasonFilter, setReasonFilter] = useState<AttentionReason | undefined>(
    undefined
  );

  const {
    items,
    totalItems,
    loading,
    error,
    byPriority,
    byReason,
    averageAgeSeconds,
    oldestAgeSeconds,
    refresh,
  } = useReviewQueue({
    // Enable hybrid push/poll by default
    useWebSocketPush: true,
    fallbackPollInterval: 30000, // 30-second fallback polling
    priorityFilter,
    reasonFilter,
    // Legacy polling options (only used if useWebSocketPush is disabled)
    autoRefresh: false, // Disable legacy polling in favor of WebSocket push
    refreshInterval,
  });

  const formatAge = (timestampSeconds: bigint): string => {
    const timestamp = Number(timestampSeconds);
    const now = Math.floor(Date.now() / 1000); // Current time in seconds
    const age = now - timestamp; // Duration in seconds

    if (age < 60) return `${age}s`;
    if (age < 3600) return `${Math.floor(age / 60)}m`;
    if (age < 86400) return `${Math.floor(age / 3600)}h`;
    return `${Math.floor(age / 86400)}d`;
  };

  const getPriorityLabel = (priority: Priority): string => {
    switch (priority) {
      case Priority.URGENT:
        return "Urgent";
      case Priority.HIGH:
        return "High";
      case Priority.MEDIUM:
        return "Medium";
      case Priority.LOW:
        return "Low";
      default:
        return "All";
    }
  };

  const getReasonLabel = (reason: AttentionReason): string => {
    switch (reason) {
      case AttentionReason.APPROVAL_PENDING:
        return "Approval";
      case AttentionReason.INPUT_REQUIRED:
        return "Input";
      case AttentionReason.ERROR_STATE:
        return "Error";
      case AttentionReason.IDLE_TIMEOUT:
        return "Idle";
      case AttentionReason.TASK_COMPLETE:
        return "Complete";
      default:
        return "All";
    }
  };

  const handleFilterByPriority = (priority: Priority | undefined) => {
    setPriorityFilter(priority);
    setReasonFilter(undefined); // Clear reason filter when changing priority
  };

  const handleFilterByReason = (reason: AttentionReason | undefined) => {
    setReasonFilter(reason);
    setPriorityFilter(undefined); // Clear priority filter when changing reason
  };

  if (error) {
    return (
      <div className={styles.error}>
        <p>Failed to load review queue: {error.message}</p>
        <button onClick={refresh} className={styles.retryButton}>
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h2 className={styles.title}>
            Review Queue{" "}
            {totalItems > 0 && (
              <span className={styles.count}>({totalItems})</span>
            )}
          </h2>
          <button
            onClick={refresh}
            className={styles.refreshButton}
            disabled={loading}
            aria-label="Refresh review queue"
          >
            {loading ? "⟳" : "↻"}
          </button>
        </div>

        {totalItems > 0 && (
          <div className={styles.stats}>
            <span className={styles.stat}>
              Avg Age: {formatAge(averageAgeSeconds)}
            </span>
            {oldestAgeSeconds > BigInt(0) && (
              <span className={styles.stat}>
                Oldest: {formatAge(oldestAgeSeconds)}
              </span>
            )}
          </div>
        )}
      </div>

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label className={styles.filterLabel}>Priority:</label>
          <div className={styles.filterButtons}>
            <button
              className={`${styles.filterButton} ${priorityFilter === undefined ? styles.active : ""}`}
              onClick={() => handleFilterByPriority(undefined)}
            >
              All ({totalItems})
            </button>
            {[Priority.URGENT, Priority.HIGH, Priority.MEDIUM, Priority.LOW].map(
              (priority) => {
                const count = byPriority.get(priority) ?? 0;
                return (
                  <button
                    key={priority}
                    className={`${styles.filterButton} ${priorityFilter === priority ? styles.active : ""}`}
                    onClick={() => handleFilterByPriority(priority)}
                    disabled={count === 0}
                  >
                    {getPriorityLabel(priority)} ({count})
                  </button>
                );
              }
            )}
          </div>
        </div>

        <div className={styles.filterGroup}>
          <label className={styles.filterLabel}>Reason:</label>
          <div className={styles.filterButtons}>
            <button
              className={`${styles.filterButton} ${reasonFilter === undefined ? styles.active : ""}`}
              onClick={() => handleFilterByReason(undefined)}
            >
              All ({totalItems})
            </button>
            {[
              AttentionReason.APPROVAL_PENDING,
              AttentionReason.INPUT_REQUIRED,
              AttentionReason.ERROR_STATE,
              AttentionReason.IDLE_TIMEOUT,
              AttentionReason.TASK_COMPLETE,
            ].map((reason) => {
              const count = byReason.get(reason) ?? 0;
              return (
                <button
                  key={reason}
                  className={`${styles.filterButton} ${reasonFilter === reason ? styles.active : ""}`}
                  onClick={() => handleFilterByReason(reason)}
                  disabled={count === 0}
                >
                  {getReasonLabel(reason)} ({count})
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div className={styles.items}>
        {loading && items.length === 0 ? (
          <div className={styles.loading}>Loading review queue...</div>
        ) : items.length === 0 ? (
          <div className={styles.empty}>
            <p>🎉 No sessions need attention!</p>
            <p className={styles.emptySubtext}>
              All sessions are running smoothly.
            </p>
          </div>
        ) : (
          items.map((item) => (
            <div key={item.sessionId} className={styles.item}>
              <div
                className={styles.itemClickable}
                onClick={() => onSessionClick?.(item.sessionId)}
              >
                <div className={styles.itemHeader}>
                  <h3 className={styles.itemTitle}>{item.sessionName}</h3>
                  <ReviewQueueBadge
                    priority={item.priority}
                    reason={item.reason}
                    compact={true}
                  />
                </div>
                <div className={styles.itemBody}>
                  <ReviewQueueBadge
                    priority={item.priority}
                    reason={item.reason}
                    compact={false}
                  />
                  {item.context && (
                    <p className={styles.itemContext}>{item.context}</p>
                  )}
                  {item.patternName && (
                    <span className={styles.itemPattern}>
                      Pattern: {item.patternName}
                    </span>
                  )}
                </div>
                <div className={styles.itemFooter}>
                  <span className={styles.itemAge}>
                    Detected: {formatAge(item.detectedAt?.seconds ?? BigInt(0))}{" "}
                    ago
                  </span>
                </div>
              </div>
              {onSkipSession && (
                <div className={styles.itemActions}>
                  <button
                    className={styles.skipButton}
                    onClick={(e) => {
                      e.stopPropagation();
                      onSkipSession(item.sessionId);
                    }}
                    title="Skip session (hide until next update)"
                    aria-label="Skip session"
                  >
                    ⏭
                  </button>
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
