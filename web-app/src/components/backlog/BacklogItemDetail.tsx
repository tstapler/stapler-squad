"use client";
// +feature: backlog:item-detail

import { useState, useEffect, useCallback } from "react";
import type { BacklogItem, AcCriterion, BacklogItemInput } from "@/lib/hooks/useBacklogService";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { getStatusLabel } from "@/lib/backlog/status";
import { BacklogItemForm } from "./BacklogItemForm";
import { AcCriteriaList } from "./AcCriteriaList";
import { SessionMonitor } from "./SessionMonitor";
import { GateVerdictBox } from "./GateVerdictBox";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";
import { TriageReviewPanel } from "./TriageReviewPanel";
import * as styles from "./BacklogItemDetail.css";

interface BacklogItemDetailProps {
  itemId: string;
  onClose?: () => void;
}

const STATUS_CLASS: Record<string, string> = {
  idea: styles.statusIdea,
  refining: styles.statusRefining,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const getStatusClass = (s: string): string => STATUS_CLASS[s] ?? styles.statusArchived;

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
  const {
    getBacklogItem,
    transitionStatus,
    triggerTriage,
    spawnSessionFromItem,
    approvePlan,
    overrideVerdict,
    triggerReReview,
    archiveBacklogItem,
    updateBacklogItem,
    lastError,
  } = useBacklogService();
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);

  // Notes inline editing
  const [editingNotes, setEditingNotes] = useState(false);
  const [notesValue, setNotesValue] = useState("");

  // Triage progress tracking
  const [triageElapsedSeconds, setTriageElapsedSeconds] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await getBacklogItem(itemId);
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
  }, [itemId, getBacklogItem]);

  useEffect(() => {
    void load();
  }, [load]);

  // Poll for updated item data while triage is running so triage-review-panel appears automatically.
  useEffect(() => {
    if (item?.triageStatus !== "running") return;
    const interval = setInterval(() => { void load(); }, 5_000);
    return () => clearInterval(interval);
  }, [item?.triageStatus, load]);

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
            await transitionStatus(item.id, "ready");
            break;
          case "trigger_triage":
            await triggerTriage(item.id);
            break;
          case "spawn_session":
            await spawnSessionFromItem(item.id);
            break;
          case "spawn_session_autonomous":
            await spawnSessionFromItem(item.id, { autonomous: true });
            break;
          case "approve_plan":
            await approvePlan(item.id);
            break;
          case "override_done": {
            const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
            if (reviewSession) {
              await overrideVerdict(reviewSession.entityId, "Manual override to done", "done");
            } else {
              setError("No review session found — cannot override verdict.");
              return;
            }
            break;
          }
          case "re_review":
            await triggerReReview(item.id);
            break;
          case "archive":
            await archiveBacklogItem(item.id);
            break;
          case "reopen":
            await transitionStatus(item.id, "review");
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
    [item, transitionStatus, triggerTriage, spawnSessionFromItem, approvePlan, overrideVerdict, triggerReReview, archiveBacklogItem, load]
  );

  const handleSaveNotes = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const updated = await updateBacklogItem(item.id, { notes: notesValue });
      if (updated) setItem(updated);
      setEditingNotes(false);
    } finally {
      setActionLoading(false);
    }
  }, [item, notesValue, updateBacklogItem]);

  const handleUpdateItem = useCallback(
    async (data: BacklogItemInput) => {
      if (!item) return;
      const updated = await updateBacklogItem(item.id, data);
      if (updated) {
        setItem(updated);
        setNotesValue(updated.notes ?? "");
      }
      setEditMode(false);
    },
    [item, updateBacklogItem]
  );

  const handleCancelTriage = useCallback(async () => {
    // TODO: implement cancel triage RPC call (if backend supports it)
    // For now, just reload the item to reflect the current state
    await load();
  }, [load]);

  const handleApplyTriageSuggestions = useCallback(
    async (preApplyCriteria: AcCriterion[]) => {
      if (!item) return;
      // Step 1: Update AC with suggestions
      const acSuggestions = item.triageResult?.suggestions.filter((s) => s.rationale !== "question") ?? [];
      const newAcCriteria: AcCriterion[] = acSuggestions.map((s, i) => ({
        index: i,
        text: s.text,
        status: "pending" as const,
      }));
      const updated = await updateBacklogItem(item.id, { acCriteria: newAcCriteria });
      if (!updated) {
        throw new Error("Failed to apply suggestions — item may have been modified by another process. Reload and try again.");
      }
      // Step 2: Transition to ready
      const transitioned = await transitionStatus(item.id, "ready", "idea");
      if (!transitioned) {
        throw new Error("Failed to mark item ready — please try again.");
      }
      await load();
      // Store pre-apply criteria for undo (returned via throw on error, used by panel)
      void preApplyCriteria; // captured in panel for undo
    },
    [item, updateBacklogItem, transitionStatus, load]
  );

  const handleUndoTriageSuggestions = useCallback(
    async (preApplyCriteria: AcCriterion[]) => {
      if (!item) return;
      // Revert AC to the pre-apply snapshot
      const updated = await updateBacklogItem(item.id, { acCriteria: preApplyCriteria });
      if (!updated) {
        throw new Error("Failed to undo — item may have been modified. Reload and try again.");
      }
      // Revert status back to idea
      await transitionStatus(item.id, "idea");
      await load();
    },
    [item, updateBacklogItem, transitionStatus, load]
  );

  const handleGateApprove = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const ok = await transitionStatus(item.id, "done");
      if (!ok) {
        setError(lastError?.message ?? "Failed to approve — please try again.");
        return;
      }
      await load();
    } finally {
      setActionLoading(false);
    }
  }, [item, transitionStatus, lastError, load]);

  const handleGateReopen = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      await transitionStatus(item.id, "in_progress");
      await load();
    } finally {
      setActionLoading(false);
    }
  }, [item, transitionStatus, load]);

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
        await overrideVerdict(reviewSession.entityId, reason, "done");
        await load();
      } catch (e) {
        setError(e instanceof Error ? e.message : "Override failed.");
      } finally {
        setActionLoading(false);
      }
    },
    [item, overrideVerdict, load]
  );

  const handleGateSkip = useCallback(async () => {
    if (!item) return;
    setActionLoading(true);
    try {
      const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
      if (reviewSession) {
        await overrideVerdict(reviewSession.entityId, "Gate skipped by user", "done");
      } else {
        // No review session yet — direct transition (item.skipReviewGate path)
        const ok = await transitionStatus(item.id, "done");
        if (!ok) {
          setError(lastError?.message ?? "Failed to skip gate — please try again.");
          return;
        }
      }
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Skip gate failed.");
    } finally {
      setActionLoading(false);
    }
  }, [item, overrideVerdict, transitionStatus, lastError, load]);

  if (loading) {
    return (
      <article className={styles.container} data-testid="backlog-item-detail">
        {onClose && (
          <div className={styles.header}>
            <div className={styles.headerRow}>
              <span />
              <div className={styles.headerActions}>
                <button className={styles.closeButton} onClick={onClose} aria-label="Close">×</button>
              </div>
            </div>
          </div>
        )}
        <div className={styles.loadingState} role="status" aria-label="Loading backlog item">
          Loading…
        </div>
      </article>
    );
  }

  if (!item) {
    return (
      <article className={styles.container} data-testid="backlog-item-detail">
        {onClose && (
          <div className={styles.header}>
            <div className={styles.headerRow}>
              <span />
              <div className={styles.headerActions}>
                <button className={styles.closeButton} onClick={onClose} aria-label="Close">×</button>
              </div>
            </div>
          </div>
        )}
        <div className={styles.errorState} role="alert">
          {error ?? "Item not found."}
        </div>
      </article>
    );
  }

  const canSpawnSession =
    item.status === "ready" &&
    (item.skipPlanning || item.planApproved);

  if (editMode) {
    return (
      <article
        className={styles.container}
        data-testid="backlog-item-detail"
        aria-label={`Edit backlog item: ${item.title}`}
      >
        <div className={styles.header}>
          <div className={styles.headerRow}>
            <div className={styles.titleGroup}>
              <h2 className={styles.itemTitle}>{item.title}</h2>
            </div>
            <div className={styles.headerActions}>
              <button
                className={styles.closeButton}
                onClick={() => setEditMode(false)}
                aria-label="Cancel editing"
                data-testid="backlog-detail-cancel-edit"
              >
                ×
              </button>
            </div>
          </div>
        </div>
        <div className={styles.scrollArea}>
          <BacklogItemForm
            initialValues={item}
            onSubmit={handleUpdateItem}
            onCancel={() => setEditMode(false)}
          />
        </div>
      </article>
    );
  }

  return (
    <article
      className={styles.container}
      data-testid="backlog-item-detail"
      aria-label={`Backlog item: ${item.title}`}
    >
      {/* Sticky header — always visible regardless of scroll */}
      <div className={styles.header}>
        <div className={styles.headerRow}>
          <div className={styles.titleGroup}>
            <h2 className={styles.itemTitle}>{item.title}</h2>
            <div className={styles.metaRow}>
              <span
                className={`${styles.statusBadge} ${getStatusClass(item.status)}`}
                aria-label={`Status: ${getStatusLabel(item.status)}`}
              >
                {getStatusLabel(item.status)}
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
          <div className={styles.headerActions}>
            <button
              className={styles.editButton}
              onClick={() => setEditMode(true)}
              aria-label="Edit item"
              data-testid="backlog-detail-edit"
            >
              Edit
            </button>
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
      </div>

      <div className={styles.scrollArea}>
        {/* Inline action error banner */}
        {error && (
          <div className={styles.errorBanner} role="alert">
            <span>{error}</span>
            <button
              className={styles.errorBannerDismiss}
              onClick={() => setError(null)}
              aria-label="Dismiss error"
            >
              ×
            </button>
          </div>
        )}

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

        {/* Triage Review Panel — only shown while still in idea status */}
        {item.triageStatus === "completed" &&
          item.status === "idea" &&
          item.triageResult && (
            <div className={styles.section} aria-live="polite">
              <TriageReviewPanel
                item={item}
                triageResult={item.triageResult}
                onApply={handleApplyTriageSuggestions}
                onUndoApply={handleUndoTriageSuggestions}
                onSkip={() => { void load(); }}
              />
            </div>
          )}

        {/* Planning record — read-only triage result for items past idea status */}
        {item.triageResult && item.status !== "idea" && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Planning</h3>
            <p className={styles.planSummary}>{item.triageResult.summary}</p>
            {item.triageResult.tasks && item.triageResult.tasks.length > 0 && (
              <div className={styles.planTaskList}>
                {item.triageResult.tasks.map((t, i) => (
                  <div key={i} className={styles.planTask}>
                    <span className={styles.planTaskText}>{t.text}</span>
                    <span className={styles.planTaskMeta}>
                      {t.estimate && <span className={styles.planTaskBadge}>{t.estimate}</span>}
                      {t.category && <span className={styles.planTaskBadge}>{t.category}</span>}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Triage failed banner */}
        {item.triageStatus === "failed" && item.status === "idea" && (
          <div className={styles.section}>
            <div role="alert" style={{ color: "var(--error)", fontSize: "0.875rem" }}>
              Triage encountered an error. Trigger triage manually to retry.
            </div>
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
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("trigger_triage")}
                  disabled={actionLoading}
                  data-testid="backlog-action-trigger-triage"
                >
                  Trigger Triage
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
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("spawn_session_autonomous")}
                  disabled={actionLoading || !canSpawnSession}
                  aria-disabled={!canSpawnSession}
                  title={
                    !canSpawnSession
                      ? "Approve the plan or enable skip_planning to run autonomously"
                      : "Run the agent without human approval for tool calls"
                  }
                  data-testid="backlog-action-run-autonomously"
                >
                  Run Autonomously
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
              {item.linkedSessions.map((s) => {
                // A session without endedAt that isn't in the active phase for this
                // item's current status is a stale/orphaned record — label it ended.
                const statusToRole: Record<string, string> = {
                  idea: "triage",
                  in_progress: "work",
                  review: "review",
                };
                const isOrphan = !s.endedAt && s.role !== statusToRole[item.status];
                return (
                  <a
                    key={s.sessionId}
                    className={styles.sessionRow}
                    href={`/?session=${s.sessionId}`}
                    role="listitem"
                    title="Open in terminal"
                    style={{ textDecoration: "none" }}
                  >
                    <span className={styles.sessionId} title={s.sessionId}>
                      {s.sessionId}
                    </span>
                    <span className={styles.sessionRole}>{s.role}</span>
                    {s.startedAt && (
                      <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>
                    )}
                    {isOrphan && (
                      <span className={styles.sessionEndedBadge}>ended</span>
                    )}
                  </a>
                );
              })}
            </div>

            {/* Session monitor for the most recent active session.
                A session is only considered active if the item is in the
                matching lifecycle phase — prevents ghost "RUNNING" tiles for
                sessions that died without setting endedAt. */}
            {(() => {
              const statusToRole: Record<string, string> = {
                idea: "triage",
                in_progress: "work",
                review: "review",
              };
              const expectedRole = statusToRole[item.status];
              const active = [...item.linkedSessions]
                .reverse()
                .find((s) => !s.endedAt && s.role === expectedRole);
              if (!active) return null;
              return (
                <SessionMonitor
                  sessionId={active.sessionId}
                  sessionRole={active.role}
                  isRunning={true}
                />
              );
            })()}
          </div>
        )}

        {/* Workflow / Status History */}
        {item.statusEvents.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Workflow</h3>
            <div className={styles.workflowTimeline} role="list" aria-label="Status history">
              {item.statusEvents.map((ev) => (
                <div key={ev.id} className={styles.workflowEvent} role="listitem">
                  <span className={styles.workflowEventFrom}>{ev.fromStatus.replace("_", " ")}</span>
                  <span className={styles.workflowEventArrow}>→</span>
                  <span className={styles.workflowEventTo}>{ev.toStatus.replace("_", " ")}</span>
                  <span className={styles.workflowEventMeta}>
                    {ev.createdAt ? formatDate(ev.createdAt) : ""}
                    {ev.triggeredBy === "user" ? " · user" : ""}
                  </span>
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
