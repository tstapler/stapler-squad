"use client";

import { useState, useEffect } from "react";
import { useReviewQueue } from "@/lib/hooks/useReviewQueue";
import { useReviewQueueNavigation } from "@/lib/hooks/useReviewQueueNavigation";
import { useReviewQueueNotifications } from "@/lib/hooks/useReviewQueueNotifications";
import { useApprovals } from "@/lib/hooks/useApprovals";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { Priority, AttentionReason, ReviewItem } from "@/gen/session/v1/types_pb";
import { NotificationSound } from "@/lib/utils/notifications";
import styles from "./ReviewQueuePanel.module.css";

interface ReviewQueuePanelProps {
  onSessionClick?: (sessionId: string) => void;
  onSkipSession?: (sessionId: string) => Promise<void>;
  autoRefresh?: boolean;
  refreshInterval?: number;
  onItemsChange?: (items: ReviewItem[]) => void; // Callback to expose queue items for navigation
  onAcknowledged?: (sessionId: string) => void; // Notifies parent when a session is acknowledged (for auto-advance)
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
  onItemsChange,
  onAcknowledged,
}: ReviewQueuePanelProps) {
  const [priorityFilter, setPriorityFilter] = useState<Priority | undefined>(
    undefined
  );
  const [reasonFilter, setReasonFilter] = useState<AttentionReason | undefined>(
    undefined
  );
  // Track whether queue ever had items so we can show "all done" vs generic empty state
  const [hadItems, setHadItems] = useState(false);

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
    acknowledgeSession,
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

  // Approval actions for APPROVAL_PENDING items
  const { approve: approveRequest, deny: denyRequest } = useApprovals();

  // Keyboard navigation
  const { currentIndex, goToNext, goToPrevious } = useReviewQueueNavigation({
    items,
    onNavigate: (item, index) => {
      // Navigate to the selected session
      onSessionClick?.(item.sessionId);
    },
    enableKeyboardShortcuts: true,
  });

  // Play notification sound when new items are added to the queue
  useReviewQueueNotifications(items, {
    enabled: true,
    soundType: NotificationSound.DING,
    showBrowserNotification: true,
    showToastNotification: true,
    notificationTitle: "Session Needs Attention",
    onNavigateToSession: (sessionId) => {
      // Open the session when clicking "View Session" in toast
      onSessionClick?.(sessionId);
    },
    onAcknowledge: (sessionId) => {
      // When user clicks "Dismiss" in toast, acknowledge the session
      // This prevents re-notification for the grace period
      acknowledgeSession(sessionId);
    },
  });

  // Notify parent component when queue items change (for navigation)
  useEffect(() => {
    if (onItemsChange) {
      onItemsChange(items);
    }
  }, [items, onItemsChange]);

  // Track if queue ever had items (for "all done" vs generic empty state)
  useEffect(() => {
    if (items.length > 0) {
      setHadItems(true);
    }
  }, [items.length]);

  // Format duration in seconds (e.g., averageAgeSeconds, oldestAgeSeconds)
  const formatDuration = (durationSeconds: bigint): string => {
    const duration = Number(durationSeconds);
    if (duration < 60) return `${duration}s`;
    if (duration < 3600) return `${Math.floor(duration / 60)}m`;
    if (duration < 86400) return `${Math.floor(duration / 3600)}h`;
    return `${Math.floor(duration / 86400)}d`;
  };

  // Format timestamp (seconds since epoch) as "time ago"
  const formatTimestamp = (timestampSeconds: bigint): string => {
    const timestamp = Number(timestampSeconds);
    if (timestamp === 0) return "never";

    const now = Math.floor(Date.now() / 1000);
    const age = now - timestamp;

    if (age < 0) return "in the future"; // Clock skew protection
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
      case AttentionReason.IDLE:
        return "Idle";
      case AttentionReason.TASK_COMPLETE:
        return "Complete";
      case AttentionReason.STALE:
        return "Stale";
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
    <div className={styles.panel} data-testid="review-queue">
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h2 className={styles.title}>
            Review Queue{" "}
            {totalItems > 0 && (
              <span className={styles.count} data-testid="review-queue-badge">
                ({totalItems})
              </span>
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
          <div className={styles.stats} data-testid="queue-statistics">
            <span className={styles.stat} data-testid="total-items">
              {totalItems} {totalItems === 1 ? "item" : "items"} need attention
            </span>
            <span className={styles.stat}>
              Avg age: {formatDuration(averageAgeSeconds)}
            </span>
            {oldestAgeSeconds > BigInt(0) && (
              <span className={styles.stat}>
                Oldest: {formatDuration(oldestAgeSeconds)}
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
              AttentionReason.IDLE,
              AttentionReason.STALE,
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
          hadItems ? (
            <div className={`${styles.empty} ${styles.completionState}`}>
              <p className={styles.completionIcon}>[✓]</p>
              <p>All done! 0 items remaining.</p>
              <p className={styles.emptySubtext}>
                Queue cleared.
              </p>
            </div>
          ) : (
            <div className={styles.empty}>
              <p>No sessions need attention!</p>
              <p className={styles.emptySubtext}>
                All sessions are running smoothly.
              </p>
            </div>
          )
        ) : (
          <>
            {items.map((item, index) => (
              <div
                key={item.sessionId}
                className={styles.item}
                data-testid={index === currentIndex ? "current-item" : "review-item"}
                data-session-id={item.sessionId}
              >
                <div
                  className={`${styles.itemClickable} ${index === currentIndex ? styles.currentItem : ""}`}
                  onClick={() => onSessionClick?.(item.sessionId)}
                  data-testid={`review-item-${item.sessionId}`}
                  data-current={index === currentIndex ? "true" : undefined}
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
                    {/* Session details */}
                    <div className={styles.sessionDetails}>
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Program:</span>
                        <span className={styles.detailValue}>{item.program}</span>
                      </div>
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Branch:</span>
                        <span className={styles.detailValue}>{item.branch}</span>
                      </div>
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Path:</span>
                        <span className={styles.detailValue} title={item.path}>{item.path}</span>
                      </div>
                      {item.tags && item.tags.length > 0 && (
                        <div className={styles.detailRow}>
                          <span className={styles.detailLabel}>Tags:</span>
                          <div className={styles.tags}>
                            {item.tags.map((tag, idx) => (
                              <span key={idx} className={styles.tag}>{tag}</span>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className={styles.itemFooter}>
                    <span className={styles.itemAge}>
                      Last Activity: {formatTimestamp(item.lastActivity?.seconds ?? BigInt(0))}{" "}
                      ago
                    </span>
                    {item.diffStats && (item.diffStats.added > 0 || item.diffStats.removed > 0) && (
                      <span className={styles.diffStats}>
                        <span className={styles.diffAdded}>+{item.diffStats.added}</span>
                        <span className={styles.diffRemoved}>-{item.diffStats.removed}</span>
                      </span>
                    )}
                  </div>
                </div>
                <div className={styles.itemActions}>
                  {item.metadata?.["pending_approval_id"] && (
                    <>
                      <button
                        className={styles.approveButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          approveRequest(item.metadata!["pending_approval_id"]);
                        }}
                        title="Approve this tool-use request"
                        aria-label="Approve"
                        data-testid={`approve-${item.sessionId}`}
                      >
                        ✓
                      </button>
                      <button
                        className={styles.denyButton}
                        onClick={(e) => {
                          e.stopPropagation();
                          denyRequest(item.metadata!["pending_approval_id"]);
                        }}
                        title="Deny this tool-use request"
                        aria-label="Deny"
                        data-testid={`deny-${item.sessionId}`}
                      >
                        ✗
                      </button>
                    </>
                  )}
                  <button
                    className={styles.skipButton}
                    onClick={(e) => {
                      e.stopPropagation();
                      if (onSkipSession) {
                        onSkipSession(item.sessionId);
                      } else {
                        acknowledgeSession(item.sessionId);
                      }
                      onAcknowledged?.(item.sessionId);
                    }}
                    title="Acknowledge session (remove from queue)"
                    aria-label="Acknowledge session"
                    data-testid={`acknowledge-${item.sessionId}`}
                  >
                    ⏭
                  </button>
                </div>
              </div>
            ))}
            {!loading && <div data-testid="review-queue-loaded" aria-hidden="true" />}
          </>
        )}
      </div>
    </div>
  );
}
