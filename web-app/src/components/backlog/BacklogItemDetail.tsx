"use client";
// +feature: backlog:item-detail

import { useState, useEffect, useCallback } from "react";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { AcCriteriaList } from "./AcCriteriaList";
import { GateVerdictBox } from "./GateVerdictBox";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";
import * as styles from "./BacklogItemDetail.css";

interface BacklogItemDetailProps {
  itemId: string;
  onClose?: () => void;
}

const STATUS_LABELS: Record<BacklogItemStatus, string> = {
  idea: "Idea",
  ready: "Ready",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
  archived: "Archived",
};

const STATUS_CLASS: Record<BacklogItemStatus, string> = {
  idea: styles.statusIdea,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const PRIORITY_LABELS: Record<number, string> = { 1: "P1", 2: "P2", 3: "P3", 4: "P4", 5: "P5" };

function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function BacklogItemDetail({ itemId, onClose }: BacklogItemDetailProps) {
  const service = useBacklogService();
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState(false);

  // Notes inline editing
  const [editingNotes, setEditingNotes] = useState(false);
  const [notesValue, setNotesValue] = useState("");

  // Triage progress tracking
  const [triageElapsedSeconds, setTriageElapsedSeconds] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await service.getBacklogItem(itemId);
      if (!result) {
        setError("Item not found.");
      } else {
        setItem(result);
        setNotesValue(result.notes ?? "");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load item.");
    } finally {
      setLoading(false);
    }
  }, [itemId, service]);

  useEffect(() => {
    void load();
  }, [load]);

  // Track triage progress: increment elapsed time while triageStatus === "running"
  useEffect(() => {
    if (item?.triageStatus !== "running") {
      return;
    }

    const interval = setInterval(() => {
      setTriageElapsedSeconds((prev) => prev + 1);
    }, 1000);

    return () => clearInterval(interval);
  }, [item?.triageStatus]);

  // Reset triage timer when status changes away from triage
  useEffect(() => {
    if (item?.triageStatus !== "running") {
      setTriageElapsedSeconds(0);
    }
  }, [item?.triageStatus]);

  const handleAction = useCallback(
    async (action: string) => {
      if (!item) return;
      setActionLoading(true);
      try {
        switch (action) {
          case "mark_ready":
            await service.transitionStatus(item.id, "ready");
            break;
          case "trigger_triage":
            await service.triggerTriage(item.id);
            break;
          case "spawn_session":
            await service.spawnSessionFromItem(item.id);
            break;
          case "approve_plan":
            await service.approvePlan(item.id);
            break;
          case "override_done": {
            const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
            if (reviewSession) {
              await service.overrideVerdict(reviewSession.entityId, "Manual override to done", "done");
            } else {
              setError("No review session found — cannot override verdict.");
              return;
            }
            break;
          }
          case "re_review":
            await service.triggerReReview(item.id);
            break;
          case "archive":
            await service.archiveBacklogItem(item.id);
            break;
          case "reopen":
            await service.transitionStatus(item.id, "review");
            break;
          default:
            break;
        }
        await load();
      } catch (e) {
        setError(e instanceof Error ? e.message : "Action failed.");
      } finally {
        setActionLoading(false);
      }
    },
    [item, service, load]
  );

  const handleSaveNotes = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const updated = await service.updateBacklogItem(item.id, { notes: notesValue });
      if (updated) setItem(updated);
      setEditingNotes(false);
    } finally {
      setActionLoading(false);
    }
  }, [item, notesValue, service]);

  const handleCancelTriage = useCallback(async () => {
    // TODO: implement cancel triage RPC call (if backend supports it)
    // For now, just reload the item to reflect the current state
    await load();
  }, [load]);

  const handleGateApprove = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const ok = await service.transitionStatus(item.id, "done");
      if (!ok) {
        setError(service.lastError?.message ?? "Failed to approve — please try again.");
        return;
      }
      await load();
    } finally {
      setActionLoading(false);
    }
  }, [item, service, load]);

  const handleGateReopen = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      await service.transitionStatus(item.id, "in_progress");
      await load();
    } finally {
      setActionLoading(false);
    }
  }, [item, service, load]);

  const handleGateOverride = useCallback(
    async (reason: string) => {
      if (!item) return;
      setActionLoading(true);
      try {
        const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
        if (!reviewSession) {
          setError("No review session found — cannot override verdict.");
          return;
        }
        await service.overrideVerdict(reviewSession.entityId, reason, "done");
        await load();
      } finally {
        setActionLoading(false);
      }
    },
    [item, service, load]
  );

  const handleGateSkip = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
      if (reviewSession) {
        await service.overrideVerdict(reviewSession.entityId, "Gate skipped by user", "done");
      } else {
        // No review session yet — direct transition (item.skipReviewGate path)
        const ok = await service.transitionStatus(item.id, "done");
        if (!ok) {
          setError(service.lastError?.message ?? "Failed to skip gate — please try again.");
          return;
        }
      }
      await load();
    } finally {
      setActionLoading(false);
    }
  }, [item, service, load]);

  if (loading) {
    return (
      <div className={styles.container} data-testid="backlog-item-detail">
        <div className={styles.loadingState} role="status" aria-label="Loading backlog item">
          Loading…
        </div>
      </div>
    );
  }

  if (error || !item) {
    return (
      <div className={styles.container} data-testid="backlog-item-detail">
        <div className={styles.errorState} role="alert">
          {error ?? "Item not found."}
        </div>
      </div>
    );
  }

  const canSpawnSession =
    item.status === "ready" &&
    (item.skipPlanning || item.planApproved);

  return (
    <article
      className={styles.container}
      data-testid="backlog-item-detail"
      aria-label={`Backlog item: ${item.title}`}
    >
      <div className={styles.scrollArea}>
        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerRow}>
            <div className={styles.titleGroup}>
              <h2 className={styles.itemTitle}>{item.title}</h2>
              <div className={styles.metaRow}>
                <span
                  className={`${styles.statusBadge} ${STATUS_CLASS[item.status]}`}
                  aria-label={`Status: ${STATUS_LABELS[item.status]}`}
                >
                  {STATUS_LABELS[item.status]}
                </span>
                <span
                  className={styles.priorityBadge}
                  aria-label={`Priority: ${PRIORITY_LABELS[item.priority] ?? "Unknown"}`}
                >
                  {PRIORITY_LABELS[item.priority] ?? "P?"}
                </span>
                {item.createdAt && (
                  <span className={styles.dateMeta}>
                    Created {formatDate(item.createdAt)}
                  </span>
                )}
                {item.updatedAt && (
                  <span className={styles.dateMeta}>
                    · Updated {formatDate(item.updatedAt)}
                  </span>
                )}
              </div>
            </div>
            {onClose && (
              <button
                className={styles.closeButton}
                onClick={onClose}
                aria-label="Close item detail"
                data-testid="backlog-detail-close"
              >
                ×
              </button>
            )}
          </div>
        </div>

        {/* Triage Progress Indicator */}
        {(item.status === "idea" || item.status === "ready") && item.triageStatus === "running" && (
          <div className={styles.section}>
            <TriageLoadingIndicator
              elapsedSeconds={triageElapsedSeconds}
              context="item"
              onCancel={handleCancelTriage}
              compact={false}
            />
          </div>
        )}

        {/* Gate Verdict */}
        {item.status === "review" && (
          <div className={styles.section}>
            <GateVerdictBox
              verdict={item.gateVerdict ?? "PENDING"}
              summary={item.gateVerdictSummary || "Review in progress"}
              criteria={item.gateCriteria}
              elapsedSeconds={undefined}
              onApprove={handleGateApprove}
              onReopen={handleGateReopen}
              onOverride={handleGateOverride}
              onSkipGate={handleGateSkip}
              actionPending={actionLoading}
            />
          </div>
        )}

        {/* Description */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Description</h3>
          {item.description ? (
            <p className={styles.description}>{item.description}</p>
          ) : (
            <p className={styles.emptyText}>No description.</p>
          )}
        </div>

        {/* Acceptance Criteria */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>
            Acceptance Criteria ({item.acCriteria.filter((c) => c.status === "done").length}/{item.acCriteria.length})
          </h3>
          <AcCriteriaList criteria={item.acCriteria} />
        </div>

        {/* Actions */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Actions</h3>
          <div className={styles.actionsPanel} role="group" aria-label="Item actions">
            {item.status === "idea" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("mark_ready")}
                  disabled={actionLoading || item.acCriteria.length === 0}
                  aria-disabled={item.acCriteria.length === 0}
                  title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
                  data-testid="backlog-action-mark-ready"
                >
                  Mark Ready
                </button>
              </>
            )}

            {item.status === "ready" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("trigger_triage")}
                  disabled={actionLoading}
                  data-testid="backlog-action-trigger-triage"
                >
                  Trigger Triage
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("spawn_session")}
                  disabled={actionLoading || !canSpawnSession}
                  aria-disabled={!canSpawnSession}
                  title={
                    !canSpawnSession
                      ? "Approve the plan or enable skip_planning to spawn a session"
                      : undefined
                  }
                  data-testid="backlog-action-spawn-session"
                >
                  Spawn Session
                </button>
                {item.planArtifactsPath && (
                  <button
                    className={styles.actionButton}
                    onClick={() => handleAction("approve_plan")}
                    disabled={actionLoading}
                    data-testid="backlog-action-approve-plan"
                  >
                    Approve Plan
                  </button>
                )}
              </>
            )}

            {item.status === "in_progress" && item.linkedSessions.length > 0 && (
              <a
                className={styles.actionButton}
                href={`/?session=${item.linkedSessions[item.linkedSessions.length - 1].sessionId}`}
                data-testid="backlog-action-view-session"
              >
                View Session
              </a>
            )}

            {item.status === "review" && (
              <>
                <button
                  className={`${styles.actionButton} ${styles.actionButtonDanger}`}
                  onClick={() => handleAction("override_done")}
                  disabled={actionLoading}
                  data-testid="backlog-action-override-done"
                >
                  Override → Done
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("re_review")}
                  disabled={actionLoading}
                  data-testid="backlog-action-re-review"
                >
                  Re-review
                </button>
              </>
            )}

            {item.status === "done" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("archive")}
                  disabled={actionLoading}
                  data-testid="backlog-action-archive"
                >
                  Archive
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("reopen")}
                  disabled={actionLoading}
                  data-testid="backlog-action-reopen"
                >
                  Re-open to Review
                </button>
              </>
            )}
          </div>
        </div>

        {/* Plan Artifacts Path */}
        {item.planArtifactsPath && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Plan Artifacts</h3>
            <code className={styles.artifactsPath}>{item.planArtifactsPath}</code>
          </div>
        )}

        {/* Linked Sessions */}
        {item.linkedSessions.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Sessions ({item.linkedSessions.length})</h3>
            <div className={styles.sessionList} role="list" aria-label="Linked sessions">
              {item.linkedSessions.map((s) => (
                <div key={s.sessionId} className={styles.sessionRow} role="listitem">
                  <span className={styles.sessionId} title={s.sessionId}>
                    {s.sessionId}
                  </span>
                  <span className={styles.sessionRole}>{s.role}</span>
                  {s.startedAt && (
                    <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Notes */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Notes</h3>
          {editingNotes ? (
            <>
              <textarea
                className={styles.notesTextarea}
                value={notesValue}
                onChange={(e) => setNotesValue(e.target.value)}
                aria-label="Notes"
                data-testid="backlog-notes-textarea"
              />
              <div className={styles.inlineEditActions}>
                <button
                  className={styles.saveNotesButton}
                  onClick={handleSaveNotes}
                  disabled={actionLoading}
                  data-testid="backlog-notes-save"
                >
                  Save
                </button>
                <button
                  className={styles.cancelNotesButton}
                  onClick={() => {
                    setNotesValue(item.notes ?? "");
                    setEditingNotes(false);
                  }}
                  data-testid="backlog-notes-cancel"
                >
                  Cancel
                </button>
              </div>
            </>
          ) : (
            <p
              className={item.notes ? styles.description : styles.emptyText}
              onClick={() => setEditingNotes(true)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") setEditingNotes(true);
              }}
              aria-label="Click to edit notes"
              data-testid="backlog-notes-display"
            >
              {item.notes ?? "Click to add notes…"}
            </p>
          )}
        </div>
      </div>
    </article>
  );
}
