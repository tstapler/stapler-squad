"use client";
// +feature: review-queue-pr-creation

import { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { useReviewQueueNavigation } from "@/lib/hooks/useReviewQueueNavigation";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { Priority, AttentionReason, ReviewItem, WorkingState } from "@/gen/session/v1/types_pb";
import {
  panel,
  header,
  titleRow,
  title,
  count,
  refreshButton,
  stats,
  stat,
  filters,
  filterGroup,
  filterLabel,
  filterButtons,
  filterButton,
  filterButtonActive,
  items as itemsClass,
  item,
  itemClickable,
  currentItem,
  itemActions,
  itemHeader,
  itemTitle,
  itemBody,
  itemContext,
  commandPreview,
  expiredBadge,
  itemPattern,
  sessionDetails,
  detailRow,
  detailLabel,
  detailValue,
  tags,
  tag,
  itemFooter,
  itemAge,
  diffStats,
  diffAdded,
  diffRemoved,
  loading as loadingClass,
  empty as emptyClass,
  error as errorClass,
  emptySubtext,
  completionState,
  completionIcon,
  retryButton,
  visuallyHidden,
  oldestCallout,
  newItemsBanner,
  filterToggleRow,
  filterToggle,
  filterToggleActive,
  filterClear,
} from "./ReviewQueuePanel.css";
import { Button } from "@/components/ui";

interface ReviewQueuePanelProps {
  onSessionClick?: (sessionId: string) => void;
  onSkipSession?: (sessionId: string) => Promise<void>;
  autoRefresh?: boolean;
  refreshInterval?: number;
  onItemsChange?: (items: ReviewItem[]) => void; // Callback to expose queue items for navigation
  onAcknowledged?: (sessionId: string) => void; // Notifies parent when a session is acknowledged (for auto-advance)
  onRunOneShot?: (sessionId: string, prompt: string) => Promise<{ prUrl?: string; error?: string } | null>; // S3-3
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
const DEFAULT_PR_PROMPT = "Create a pull request for the changes in this session. Use a descriptive title and include a summary of the changes made.";

export function ReviewQueuePanel({
  onSessionClick,
  onSkipSession,
  autoRefresh = true,
  refreshInterval = 5000,
  onItemsChange,
  onAcknowledged,
  onRunOneShot,
}: ReviewQueuePanelProps) {
  // S3-3: PR creation modal state
  const [prModal, setPrModal] = useState<{ sessionId: string; prompt: string } | null>(null);
  const [prRunning, setPrRunning] = useState(false);
  const [prResult, setPrResult] = useState<{ prUrl?: string; error?: string } | null>(null);
  const [priorityFilter, setPriorityFilter] = useState<Priority | undefined>(
    undefined
  );
  const [reasonFilter, setReasonFilter] = useState<AttentionReason | undefined>(
    undefined
  );
  const [isFiltersOpen, setIsFiltersOpen] = useState(false);
  // Track whether queue ever had items so we can show "all done" vs generic empty state
  const [hadItems, setHadItems] = useState(false);

  // Live region announcement text for screen readers
  const [liveAnnouncement, setLiveAnnouncement] = useState('');
  // Prevent announcement on initial mount
  const hasMountedRef = useRef(false);

  const {
    items: allItems,
    totalItems,
    loading,
    error,
    byPriority,
    byReason,
    averageAgeSeconds,
    oldestAgeSeconds,
    refresh,
    acknowledgeSession,
  } = useReviewQueueContext();

  // ─── Snapshot-on-enter pattern ────────────────────────────────────────────
  // Captures the session IDs present when the user enters the queue.
  // New items arriving while reviewing appear in a banner rather than being
  // injected mid-list, preventing queue jumps during triage (Twitter-style).
  const [reviewingIdsSnapshot, setReviewingIdsSnapshot] = useState<Set<string> | null>(null);

  // Initialize snapshot when the queue first loads with items
  useEffect(() => {
    if (reviewingIdsSnapshot === null && allItems.length > 0) {
      setReviewingIdsSnapshot(new Set(allItems.map((item) => item.sessionId)));
    }
  }, [allItems, reviewingIdsSnapshot]);

  // Remove acknowledged/resolved items from snapshot (forward-only — no re-injection)
  useEffect(() => {
    if (reviewingIdsSnapshot === null) return;
    const liveIds = new Set(allItems.map((item) => item.sessionId));
    const pruned = new Set([...reviewingIdsSnapshot].filter((id) => liveIds.has(id)));
    if (pruned.size !== reviewingIdsSnapshot.size) {
      setReviewingIdsSnapshot(pruned);
    }
  }, [allItems, reviewingIdsSnapshot]);

  const refreshSnapshot = useCallback(() => {
    setReviewingIdsSnapshot(new Set(allItems.map((item) => item.sessionId)));
    refresh();
  }, [allItems, refresh]);
  // ─────────────────────────────────────────────────────────────────────────

  // Separate working sessions from waiting sessions for count display.
  const workingCount = useMemo(
    () =>
      allItems.filter(
        (item) =>
          item.workingState === WorkingState.ACTIVE ||
          item.workingState === WorkingState.PROCESSING
      ).length,
    [allItems]
  );
  const stuckCount = useMemo(
    () =>
      allItems.filter(
        (item) => item.workingState === WorkingState.WAITING
      ).length,
    [allItems]
  );

  // Apply client-side filtering to all live items, excluding actively-working sessions
  // so the queue only shows sessions that need user attention.
  const allFilteredItems = useMemo(() => {
    let filtered = allItems.filter(
      (item) =>
        item.workingState !== WorkingState.ACTIVE &&
        item.workingState !== WorkingState.PROCESSING
    );
    if (priorityFilter !== undefined) {
      filtered = filtered.filter((item) => item.priority === priorityFilter);
    }
    if (reasonFilter !== undefined) {
      filtered = filtered.filter((item) => item.reason === reasonFilter);
    }
    return filtered;
  }, [allItems, priorityFilter, reasonFilter]);

  // Items that are in the snapshot (stable ordered list for the main queue)
  const items = useMemo(() => {
    if (reviewingIdsSnapshot === null) return allFilteredItems;
    return allFilteredItems.filter((item) => reviewingIdsSnapshot.has(item.sessionId));
  }, [allFilteredItems, reviewingIdsSnapshot]);

  // New items not yet in snapshot — shown in the refresh banner
  const newItemsCount = useMemo(() => {
    if (reviewingIdsSnapshot === null) return 0;
    return allFilteredItems.filter((item) => !reviewingIdsSnapshot.has(item.sessionId)).length;
  }, [allFilteredItems, reviewingIdsSnapshot]);

  // Approval actions for APPROVAL_PENDING items
  const { approve: approveRequest, deny: denyRequest } = useApprovalsContext();

  // Keyboard navigation
  const { currentIndex, goToNext, goToPrevious } = useReviewQueueNavigation({
    items,
    onNavigate: (item, index) => {
      // Navigate to the selected session
      onSessionClick?.(item.sessionId);
    },
    enableKeyboardShortcuts: true,
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

  // Update live announcement for screen readers when queue changes
  useEffect(() => {
    if (loading) return;
    if (!hasMountedRef.current) {
      hasMountedRef.current = true;
      return; // Skip announcement on initial mount
    }
    if (items.length === 0 && hadItems) {
      setLiveAnnouncement('Queue cleared. All items reviewed.');
    } else if (items.length > 0) {
      setLiveAnnouncement(`${items.length} ${items.length === 1 ? 'item' : 'items'} need attention.`);
    }
  }, [items.length, hadItems, loading]);

  // Format duration in seconds (e.g., averageAgeSeconds, oldestAgeSeconds)
  const formatDuration = (durationSeconds: bigint): string => {
    const duration = Number(durationSeconds);
    if (duration < 0 || duration > 31_536_000) return "Unknown"; // Cap at 1 year; guards clock skew / unit mismatch
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

  const summaryCount = useMemo(() => {
    const parts: string[] = [];
    const reasonEntries: [AttentionReason, string][] = [
      [AttentionReason.APPROVAL_PENDING, "approval"],
      [AttentionReason.INPUT_REQUIRED, "input needed"],
      [AttentionReason.ERROR_STATE, "error"],
      [AttentionReason.IDLE_TIMEOUT, "timed out"],
      [AttentionReason.IDLE, "idle"],
      [AttentionReason.STALE, "stale"],
      [AttentionReason.TASK_COMPLETE, "complete"],
    ];
    for (const [reason, label] of reasonEntries) {
      const count = byReason.get(reason) ?? 0;
      if (count > 0) parts.push(`${count} ${label}${count !== 1 ? "s" : ""}`);
    }
    return parts.join(", ");
  }, [byReason]);

  const activeFilterLabel = useMemo(() => {
    if (priorityFilter !== undefined) return `Filter: ${getPriorityLabel(priorityFilter)}`;
    if (reasonFilter !== undefined) return `Filter: ${getReasonLabel(reasonFilter)}`;
    return "Filter";
  }, [priorityFilter, reasonFilter]);

  const hasActiveFilter = priorityFilter !== undefined || reasonFilter !== undefined;

  if (error) {
    return (
      <div className={errorClass}>
        <p>Failed to load review queue: {error.message}</p>
        <Button onClick={refresh} intent="secondary" size="md">
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className={panel} data-testid="review-queue">
      {/* Screen reader live region for queue count changes */}
      <div aria-live="polite" aria-atomic="true" className={visuallyHidden}>
        {liveAnnouncement}
      </div>
      <div className={header}>
        <div className={titleRow}>
          <h2 className={title}>
            Review Queue{" "}
            {totalItems > 0 && (
              <span className={count} data-testid="review-queue-badge">
                ({totalItems})
              </span>
            )}
          </h2>
          <button
            onClick={refreshSnapshot}
            className={refreshButton}
            disabled={loading}
            aria-label="Refresh review queue"
          >
            {loading ? "⟳" : "↻"}
          </button>
        </div>

        {totalItems > 0 && (
          <div className={stats} data-testid="queue-statistics">
            <span className={stat} data-testid="total-items">
              {summaryCount || `${totalItems} ${totalItems === 1 ? "item" : "items"}`}
            </span>
            {(workingCount > 0 || stuckCount > 0) && (
              <span className={stat} data-testid="working-state-counts">
                {items.filter(i => i.workingState !== WorkingState.WAITING).length} waiting
                {workingCount > 0 && ` · ${workingCount} working`}
                {stuckCount > 0 && ` · ${stuckCount} stuck`}
              </span>
            )}
          </div>
        )}

        {/* Heads-up callout when oldest item is over 5 minutes old */}
        {oldestAgeSeconds > BigInt(300) && (
          <div className={oldestCallout} role="status">
            Oldest item: {formatDuration(oldestAgeSeconds)}
          </div>
        )}

        {/* New-items banner: shows when items arrive after snapshot was taken */}
        {newItemsCount > 0 && (
          <button
            className={newItemsBanner}
            onClick={refreshSnapshot}
            aria-label={`${newItemsCount} new item${newItemsCount !== 1 ? "s" : ""} added. Click to refresh the list.`}
          >
            {newItemsCount} new item{newItemsCount !== 1 ? "s" : ""} added — click to refresh
          </button>
        )}
      </div>

      {totalItems > 0 && (
        <div className={filterToggleRow}>
          <button
            className={`${filterToggle} ${hasActiveFilter ? filterToggleActive : ""}`}
            onClick={() => setIsFiltersOpen((o) => !o)}
            aria-expanded={isFiltersOpen}
            aria-controls="review-queue-filters"
          >
            {activeFilterLabel} {isFiltersOpen ? "▲" : "▼"}
          </button>
          {hasActiveFilter && (
            <button
              className={filterClear}
              onClick={() => { setPriorityFilter(undefined); setReasonFilter(undefined); }}
              aria-label="Clear active filter"
            >
              ✕ Clear
            </button>
          )}
        </div>
      )}

      {isFiltersOpen && (
        <div id="review-queue-filters" className={filters}>
          <div className={filterGroup}>
            <label className={filterLabel}>Priority:</label>
            <div className={filterButtons}>
              <button
                className={`${filterButton} ${priorityFilter === undefined ? filterButtonActive : ""}`}
                onClick={() => handleFilterByPriority(undefined)}
                aria-pressed={priorityFilter === undefined}
              >
                All ({totalItems})
              </button>
              {[Priority.URGENT, Priority.HIGH, Priority.MEDIUM, Priority.LOW].map(
                (priority) => {
                  const priorityCount = byPriority.get(priority) ?? 0;
                  return (
                    <button
                      key={priority}
                      className={`${filterButton} ${priorityFilter === priority ? filterButtonActive : ""}`}
                      onClick={() => handleFilterByPriority(priority)}
                      disabled={priorityCount === 0}
                      aria-pressed={priorityFilter === priority}
                    >
                      {getPriorityLabel(priority)} ({priorityCount})
                    </button>
                  );
                }
              )}
            </div>
          </div>

          <div className={filterGroup}>
            <label className={filterLabel}>Reason:</label>
            <div className={filterButtons}>
              <button
                className={`${filterButton} ${reasonFilter === undefined ? filterButtonActive : ""}`}
                onClick={() => handleFilterByReason(undefined)}
                aria-pressed={reasonFilter === undefined}
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
                const reasonCount = byReason.get(reason) ?? 0;
                return (
                  <button
                    key={reason}
                    className={`${filterButton} ${reasonFilter === reason ? filterButtonActive : ""}`}
                    onClick={() => handleFilterByReason(reason)}
                    disabled={reasonCount === 0}
                    aria-pressed={reasonFilter === reason}
                  >
                    {getReasonLabel(reason)} ({reasonCount})
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      )}

      <div className={itemsClass}>
        {loading && items.length === 0 ? (
          <div className={loadingClass}>Loading review queue...</div>
        ) : items.length === 0 ? (
          hadItems ? (
            <div className={`${emptyClass} ${completionState}`}>
              <p className={completionIcon}>[✓]</p>
              <p>All done! 0 items remaining.</p>
              <p className={emptySubtext}>
                Queue cleared.
              </p>
            </div>
          ) : (
            <div className={emptyClass}>
              <p>No sessions need attention!</p>
              <p className={emptySubtext}>
                All sessions are running smoothly.
              </p>
            </div>
          )
        ) : (
          <>
            {items.map((queueItem, index) => (
              <div
                key={queueItem.sessionId}
                className={item}
                data-testid={index === currentIndex ? "current-item" : "review-item"}
                data-session-id={queueItem.sessionId}
              >
                <div
                  className={`${itemClickable} ${index === currentIndex ? currentItem : ""}`}
                  onClick={() => onSessionClick?.(queueItem.sessionId)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      onSessionClick?.(queueItem.sessionId);
                    }
                  }}
                  role="button"
                  tabIndex={0}
                  data-testid={`review-item-${queueItem.sessionId}`}
                  data-current={index === currentIndex ? "true" : undefined}
                >
                  <div className={itemHeader}>
                    <h3 className={itemTitle}>{queueItem.sessionName}</h3>
                    <ReviewQueueBadge
                      priority={queueItem.priority}
                      reason={queueItem.reason}
                      compact={true}
                    />
                  </div>
                  <div className={itemBody}>
                    <ReviewQueueBadge
                      priority={queueItem.priority}
                      reason={queueItem.reason}
                      compact={false}
                    />
                    {queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (
                      <p className={itemContext}>{queueItem.context}</p>
                    )}
                    {queueItem.patternName && (
                      <span className={itemPattern}>
                        Pattern: {queueItem.patternName}
                      </span>
                    )}
                    {queueItem.metadata?.["pending_approval_id"] && (
                      <>
                        {(queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]) && (
                          <pre className={commandPreview}>
                            {queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]}
                          </pre>
                        )}
                        {queueItem.metadata["cwd"] && (
                          <div className={detailRow}>
                            <span className={detailLabel}>Directory:</span>
                            <span className={detailValue}>{queueItem.metadata["cwd"]}</span>
                          </div>
                        )}
                        {queueItem.metadata["orphaned"] === "true" && (
                          <span className={expiredBadge}>Expired</span>
                        )}
                      </>
                    )}
                    {/* Session details */}
                    <div className={sessionDetails}>
                      <div className={detailRow}>
                        <span className={detailLabel}>Program:</span>
                        <span className={detailValue}>{queueItem.program}</span>
                      </div>
                      <div className={detailRow}>
                        <span className={detailLabel}>Branch:</span>
                        <span className={detailValue}>{queueItem.branch}</span>
                      </div>
                      <div className={detailRow}>
                        <span className={detailLabel}>Path:</span>
                        <span className={detailValue} title={queueItem.path}>{queueItem.path}</span>
                      </div>
                      {queueItem.tags && queueItem.tags.length > 0 && (
                        <div className={detailRow}>
                          <span className={detailLabel}>Tags:</span>
                          <div className={tags}>
                            {queueItem.tags.map((t, idx) => (
                              <span key={idx} className={tag}>{t}</span>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className={itemFooter}>
                    <span className={itemAge}>
                      Last Activity: {formatTimestamp(queueItem.lastActivity?.seconds ?? BigInt(0))}{" "}
                      ago
                    </span>
                    {queueItem.diffStats && (queueItem.diffStats.added > 0 || queueItem.diffStats.removed > 0) && (
                      <span className={diffStats}>
                        <span className={diffAdded}>+{queueItem.diffStats.added}</span>
                        <span className={diffRemoved}>-{queueItem.diffStats.removed}</span>
                      </span>
                    )}
                  </div>
                </div>
                <div className={itemActions} style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  {queueItem.metadata?.["pending_approval_id"] && (
                    <>
                      <Button
                        intent="primary"
                        size="lg"
                        onClick={(e) => {
                          e.stopPropagation();
                          approveRequest(queueItem.metadata!["pending_approval_id"]).finally(() => {
                            acknowledgeSession(queueItem.sessionId);
                            onAcknowledged?.(queueItem.sessionId);
                          });
                        }}
                        title="Approve this tool-use request"
                        aria-label="Approve"
                        data-testid={`approve-${queueItem.sessionId}`}
                      >
                        ✓ Approve
                      </Button>
                      <Button
                        intent="danger"
                        size="lg"
                        onClick={(e) => {
                          e.stopPropagation();
                          denyRequest(queueItem.metadata!["pending_approval_id"]).finally(() => {
                            acknowledgeSession(queueItem.sessionId);
                            onAcknowledged?.(queueItem.sessionId);
                          });
                        }}
                        title="Deny this tool-use request"
                        aria-label="Deny"
                        data-testid={`deny-${queueItem.sessionId}`}
                      >
                        ✗ Deny
                      </Button>
                    </>
                  )}
                  {/* Skip button: only shown for non-approval items.
                      Approval items already have explicit ✓ Approve / ✗ Deny buttons above. */}
                  {!queueItem.metadata?.["pending_approval_id"] && (
                    <Button
                      intent="ghost"
                      size="md"
                      onClick={(e) => {
                        e.stopPropagation();
                        if (onSkipSession) {
                          onSkipSession(queueItem.sessionId);
                        } else {
                          acknowledgeSession(queueItem.sessionId);
                        }
                        onAcknowledged?.(queueItem.sessionId);
                      }}
                      title="Acknowledge session (remove from queue)"
                      aria-label="Acknowledge session"
                      data-testid={`acknowledge-${queueItem.sessionId}`}
                    >
                      ⏭ Skip
                    </Button>
                  )}
                  {/* S3-3: Create PR button — only for TASK_COMPLETE items without an existing PR URL */}
                  {queueItem.reason === AttentionReason.TASK_COMPLETE &&
                    !queueItem.githubPrUrl &&
                    onRunOneShot && (
                      <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
                        {queueItem.branchDivergedFromBase && (
                          <span
                            style={{
                              fontSize: "0.75rem",
                              padding: "2px 6px",
                              background: "var(--warning-bg)",
                              color: "var(--warning)",
                              borderRadius: "4px",
                              whiteSpace: "nowrap",
                            }}
                          >
                            ⚠ Diverged from main
                          </span>
                        )}
                        <Button
                          intent="primary"
                          size="md"
                          onClick={(e) => {
                            e.stopPropagation();
                            setPrResult(null);
                            setPrModal({ sessionId: queueItem.sessionId, prompt: DEFAULT_PR_PROMPT });
                          }}
                          title="Create a pull request for this session"
                          aria-label="Create PR"
                          data-testid={`create-pr-${queueItem.sessionId}`}
                        >
                          🔀 Create PR
                        </Button>
                      </div>
                    )}
                </div>
              </div>
            ))}
            {!loading && <div data-testid="review-queue-loaded" aria-hidden="true" />}
          </>
        )}
      </div>

      {/* S3-3: Create PR confirmation modal */}
      {prModal && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "var(--overlay-background)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 1000,
          }}
          onClick={() => {
            if (!prRunning) {
              setPrModal(null);
              setPrResult(null);
            }
          }}
        >
          <div
            style={{
              background: "var(--modal-background)",
              border: "1px solid var(--modal-border)",
              borderRadius: "8px",
              padding: "1.5rem",
              maxWidth: "520px",
              width: "90%",
              display: "flex",
              flexDirection: "column",
              gap: "1rem",
            }}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Create Pull Request"
          >
            <h3 style={{ margin: 0, fontSize: "1.125rem", fontWeight: 600, color: "var(--text-primary)" }}>
              Create Pull Request
            </h3>
            <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--text-secondary)" }}>
              Review and edit the prompt that will be used to create the PR. This may take up to 30 seconds.
            </p>
            <textarea
              value={prModal.prompt}
              onChange={(e) => setPrModal((m) => m ? { ...m, prompt: e.target.value } : null)}
              disabled={prRunning}
              rows={5}
              style={{
                padding: "0.625rem 0.875rem",
                border: "1px solid var(--modal-border)",
                borderRadius: "6px",
                fontSize: "0.875rem",
                resize: "vertical",
                background: "var(--input-background)",
                color: "var(--text-primary)",
                fontFamily: "inherit",
              }}
            />
            {prRunning && (
              <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--text-secondary)", fontStyle: "italic" }}>
                ⏳ Creating PR, this may take up to 30 seconds…
              </p>
            )}
            {prResult?.prUrl && (
              <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--success)" }}>
                ✓ PR created:{" "}
                <a href={prResult.prUrl} target="_blank" rel="noopener noreferrer" style={{ color: "var(--primary)" }}>
                  {prResult.prUrl}
                </a>
              </p>
            )}
            {prResult?.error && (
              <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--error)" }}>
                ✗ {prResult.error}
              </p>
            )}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: "0.75rem" }}>
              <Button
                intent="secondary"
                size="md"
                onClick={() => { setPrModal(null); setPrResult(null); }}
                disabled={prRunning}
              >
                {prResult?.prUrl ? "Close" : "Cancel"}
              </Button>
              {!prResult?.prUrl && (
                <Button
                  intent="primary"
                  size="md"
                  disabled={prRunning || !prModal.prompt.trim()}
                  onClick={async () => {
                    if (!onRunOneShot) return;
                    setPrRunning(true);
                    setPrResult(null);
                    try {
                      const result = await onRunOneShot(prModal.sessionId, prModal.prompt);
                      if (result?.prUrl) {
                        setPrResult({ prUrl: result.prUrl });
                      } else {
                        setPrResult({ error: result?.error || "No PR URL found in output. The command may have failed." });
                      }
                    } catch (err) {
                      setPrResult({ error: err instanceof Error ? err.message : "An unexpected error occurred." });
                    } finally {
                      setPrRunning(false);
                    }
                  }}
                >
                  {prRunning ? "Creating…" : "Run"}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
