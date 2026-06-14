"use client";
// +feature: workflows-management

import { useState, useCallback } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { useWorkflows, WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { WorkflowForm } from "./WorkflowForm";
import * as styles from "./WorkflowsPanel.css";

function statusLabel(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.ACTIVE: return "active";
    case SessionStatus.STOPPED: return "stopped";
    case SessionStatus.PAUSED: return "paused";
    case SessionStatus.HIBERNATED: return "hibernated";
    default: return "unknown";
  }
}

function statusColor(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.ACTIVE: return "var(--success)";
    case SessionStatus.STOPPED: return "var(--text-muted)";
    case SessionStatus.PAUSED: return "var(--warning)";
    default: return "var(--text-muted)";
  }
}

function RecentRuns({ workflowId }: { workflowId: string }) {
  const [expanded, setExpanded] = useState(false);
  const [runs, setRuns] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const { listSessionsByWorkflow } = useSessionService();

  const load = useCallback(async () => {
    if (runs.length > 0) return;
    setLoading(true);
    const sessions = await listSessionsByWorkflow(workflowId, true);
    setRuns(sessions.slice(-5).reverse());
    setLoading(false);
  }, [workflowId, runs.length, listSessionsByWorkflow]);

  function toggle() {
    if (!expanded) void load();
    setExpanded((v) => !v);
  }

  return (
    <div className={styles.runsAccordion}>
      <button className={styles.runsToggle} onClick={toggle}>
        {expanded ? "▾" : "▸"} Recent Runs
      </button>
      {expanded && (
        <div className={styles.runsList}>
          {loading && <span style={{ fontSize: "0.7rem", color: "var(--text-muted)" }}>Loading…</span>}
          {!loading && runs.length === 0 && (
            <span style={{ fontSize: "0.7rem", color: "var(--text-muted)" }}>No runs yet.</span>
          )}
          {runs.map((s) => (
            <div key={s.id} className={styles.runRow}>
              <span
                className={styles.statusBadge}
                style={{ background: statusColor(s.status) + "33", color: statusColor(s.status) }}
              >
                {statusLabel(s.status)}
              </span>
              <Link href={`/?session=${s.id}`} className={styles.runLink}>
                {s.title}
              </Link>
              {s.updatedAt && (
                <span style={{ color: "var(--text-muted)", flexShrink: 0 }}>
                  {new Date(Number(s.updatedAt.seconds) * 1000).toLocaleString(undefined, {
                    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
                  })}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function WorkflowsPanel() {
  const { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, archiveWorkflowSessions, deleteWorkflowFailedSessions } = useWorkflows();
  const { runWorkflow } = useSessionService();
  const router = useRouter();
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<WorkflowProto | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [confirmArchiveId, setConfirmArchiveId] = useState<string | null>(null);
  const [confirmDeleteFailedId, setConfirmDeleteFailedId] = useState<string | null>(null);
  const [archivingId, setArchivingId] = useState<string | null>(null);
  const [deletingFailedId, setDeletingFailedId] = useState<string | null>(null);

  function openCreate() {
    setEditTarget(null);
    setShowForm(true);
  }

  function openEdit(wf: WorkflowProto) {
    setEditTarget(wf);
    setShowForm(true);
  }

  function closeForm() {
    setShowForm(false);
    setEditTarget(null);
  }

  async function handleFormSubmit(data: WorkflowFormData) {
    if (editTarget) {
      await updateWorkflow(editTarget.id, data);
    } else {
      await createWorkflow(data);
    }
    closeForm();
  }

  function handleDeleteClick(wf: WorkflowProto) {
    setConfirmDeleteId(wf.id);
  }

  async function handleDeleteConfirm() {
    if (!confirmDeleteId) return;
    await deleteWorkflow(confirmDeleteId);
    setConfirmDeleteId(null);
  }

  function handleDeleteCancel() {
    setConfirmDeleteId(null);
  }

  async function handleArchiveConfirm() {
    if (!confirmArchiveId) return;
    setArchivingId(confirmArchiveId);
    setConfirmArchiveId(null);
    try {
      await archiveWorkflowSessions(confirmArchiveId);
    } finally {
      setArchivingId(null);
    }
  }

  async function handleDeleteFailedConfirm() {
    if (!confirmDeleteFailedId) return;
    setDeletingFailedId(confirmDeleteFailedId);
    setConfirmDeleteFailedId(null);
    try {
      await deleteWorkflowFailedSessions(confirmDeleteFailedId);
    } finally {
      setDeletingFailedId(null);
    }
  }

  async function handleRun(wf: WorkflowProto) {
    setRunningId(wf.id);
    const sessionId = await runWorkflow({ id: wf.id });
    setRunningId(null);
    if (sessionId) {
      router.push(`/?session=${sessionId}`);
    }
  }

  const formModal = showForm
    ? createPortal(
        <div className={styles.formOverlay} onClick={(e) => { if (e.target === e.currentTarget) closeForm(); }}>
          <div className={styles.formCard}>
            <WorkflowForm
              existing={editTarget ?? undefined}
              onSubmit={handleFormSubmit}
              onCancel={closeForm}
            />
          </div>
        </div>,
        document.body
      )
    : null;

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h1 className={styles.title}>Workflows</h1>
          <p className={styles.subtitle}>
            Define reusable agent workflows. Invoke them from the omnibar with{" "}
            <code>@slug [optional-arg]</code>.
          </p>
        </div>
        <button className={styles.addButton} onClick={openCreate}>
          + New Workflow
        </button>
      </div>

      {error && <div className={styles.error}>{error.message}</div>}

      {loading && workflows.length === 0 ? (
        <div className={styles.loading}>Loading workflows…</div>
      ) : workflows.length === 0 ? (
        <div className={styles.empty}>
          No workflows yet. Click <strong>+ New Workflow</strong> to create one.
        </div>
      ) : (
        <div className={styles.tableWrapper}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th className={styles.th}>Slug</th>
                <th className={styles.th}>Name</th>
                <th className={styles.th}>Target Directory</th>
                <th className={styles.th}>Schedule</th>
                <th className={styles.th}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {workflows.map((wf) => (
                <>
                  <tr key={wf.id} className={styles.row}>
                    <td className={styles.td}>
                      <span className={styles.slugCell}>@{wf.slug}</span>
                    </td>
                    <td className={styles.td}>
                      <div>{wf.name}</div>
                      {wf.description && (
                        <div style={{ fontSize: "0.75rem", opacity: 0.7 }}>{wf.description}</div>
                      )}
                    </td>
                    <td className={styles.td}>{wf.targetDirectory || "—"}</td>
                    <td className={styles.td}>
                      {wf.cronExpression ? (
                        <span
                          className={[
                            styles.cronBadge,
                            wf.cronEnabled ? styles.cronEnabled : styles.cronDisabled,
                          ].join(" ")}
                        >
                          {wf.cronEnabled ? "⏰" : "⏸"} {wf.cronExpression}
                        </span>
                      ) : (
                        <span className={styles.cronDisabled + " " + styles.cronBadge}>Manual</span>
                      )}
                    </td>
                    <td className={styles.td}>
                      {confirmDeleteId === wf.id ? (
                        <div className={styles.actions}>
                          <span style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Delete &quot;{wf.name}&quot;?
                          </span>
                          <button className={styles.deleteButton} onClick={() => void handleDeleteConfirm()}>
                            Yes, delete
                          </button>
                          <button className={styles.editButton} onClick={handleDeleteCancel}>
                            Cancel
                          </button>
                        </div>
                      ) : confirmArchiveId === wf.id ? (
                        <div className={styles.actions}>
                          <span style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Archive all sessions for &quot;{wf.name}&quot;?
                          </span>
                          <button className={styles.deleteButton} onClick={() => void handleArchiveConfirm()} data-testid="confirm-archive-sessions">
                            Yes, archive
                          </button>
                          <button className={styles.editButton} onClick={() => setConfirmArchiveId(null)}>
                            Cancel
                          </button>
                        </div>
                      ) : confirmDeleteFailedId === wf.id ? (
                        <div className={styles.actions}>
                          <span style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Delete failed sessions for &quot;{wf.name}&quot;?
                          </span>
                          <button className={styles.deleteButton} onClick={() => void handleDeleteFailedConfirm()} data-testid="confirm-delete-failed-sessions">
                            Yes, delete
                          </button>
                          <button className={styles.editButton} onClick={() => setConfirmDeleteFailedId(null)}>
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <div className={styles.actions}>
                          <button
                            className={styles.runButton}
                            disabled={runningId === wf.id}
                            onClick={() => void handleRun(wf)}
                          >
                            {runningId === wf.id ? "…" : "▶ Run"}
                          </button>
                          <button className={styles.editButton} onClick={() => openEdit(wf)}>
                            Edit
                          </button>
                          <button
                            className={styles.editButton}
                            disabled={archivingId === wf.id}
                            onClick={() => setConfirmArchiveId(wf.id)}
                            data-testid="archive-sessions-button"
                          >
                            {archivingId === wf.id ? "…" : "Archive Sessions"}
                          </button>
                          <button
                            className={styles.editButton}
                            disabled={deletingFailedId === wf.id}
                            onClick={() => setConfirmDeleteFailedId(wf.id)}
                            data-testid="delete-failed-sessions-button"
                          >
                            {deletingFailedId === wf.id ? "…" : "Delete Failed"}
                          </button>
                          <button
                            className={styles.deleteButton}
                            onClick={() => handleDeleteClick(wf)}
                          >
                            Delete
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                  <tr key={wf.id + "-runs"}>
                    <td colSpan={5} style={{ padding: 0, borderBottom: `1px solid var(--border-color)` }}>
                      <RecentRuns workflowId={wf.id} />
                    </td>
                  </tr>
                </>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formModal}
    </div>
  );
}
