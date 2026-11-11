"use client";
// +feature: workflows-management

import { Fragment, useState, useCallback } from "react";
import { createPortal } from "react-dom";
import { Link, useRouter } from "@/lib/nav";
import type { WorkflowProto } from "@/gen/session/v1/session_pb";
import { type Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { useWorkflows, type WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { WorkflowForm } from "./WorkflowForm";
import { WorkflowRunDialog, workflowTakesInput } from "./WorkflowRunDialog";
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
  const { track } = useAnalytics();
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
      <button className={styles.runsToggle} onClick={() => { track({ name: "workflow_runs_toggle", category: "user_action" }); toggle(); }}>
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
  const { track } = useAnalytics();
  const { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, archiveWorkflowSessions, deleteWorkflowFailedSessions } = useWorkflows();
  const { runWorkflow } = useSessionService();
  const router = useRouter();
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<WorkflowProto | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [runDialogTarget, setRunDialogTarget] = useState<WorkflowProto | null>(null);
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

  // Resolves true when the run started and we navigated to the new session.
  async function startRun(wf: WorkflowProto, arg?: string, attachmentPaths?: string[]): Promise<boolean> {
    setRunningId(wf.id);
    const sessionId = await runWorkflow({ id: wf.id, arg, attachmentPaths });
    setRunningId(null);
    if (!sessionId) return false;
    router.push(`/?session=${sessionId}`);
    return true;
  }

  // Workflows with an {{input}} slot collect the input (and images) first.
  function handleRun(wf: WorkflowProto) {
    if (workflowTakesInput(wf)) {
      setRunDialogTarget(wf);
      return;
    }
    void startRun(wf);
  }

  async function handleRunDialogSubmit(arg: string, attachmentPaths: string[]): Promise<boolean> {
    if (!runDialogTarget) return false;
    const started = await startRun(runDialogTarget, arg, attachmentPaths);
    if (started) setRunDialogTarget(null);
    return started;
  }

  const runDialogModal = runDialogTarget
    ? createPortal(
        <div
          className={styles.formOverlay}
          data-testid="workflow-run-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setRunDialogTarget(null);
          }}
        >
          <div className={styles.formCard}>
            <WorkflowRunDialog
              workflowName={runDialogTarget.name}
              onRun={handleRunDialogSubmit}
              onCancel={() => setRunDialogTarget(null)}
            />
          </div>
        </div>,
        document.body
      )
    : null;

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
        <button className={styles.addButton} onClick={() => { track({ name: "workflow_create_open", category: "user_action" }); openCreate(); }}>
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
                <Fragment key={wf.id}>
                  <tr className={styles.row}>
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
                          <button className={styles.deleteButton} onClick={() => { track({ name: "workflow_delete_confirm", category: "user_action" }); void handleDeleteConfirm(); }}>
                            Yes, delete
                          </button>
                          <button className={styles.editButton} onClick={() => { track({ name: "workflow_delete_cancel", category: "user_action" }); handleDeleteCancel(); }}>
                            Cancel
                          </button>
                        </div>
                      ) : confirmArchiveId === wf.id ? (
                        <div className={styles.actions}>
                          <span style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Archive all sessions for &quot;{wf.name}&quot;?
                          </span>
                          <button className={styles.deleteButton} onClick={() => { track({ name: "workflow_archive_confirm", category: "user_action" }); void handleArchiveConfirm(); }} data-testid="confirm-archive-sessions">
                            Yes, archive
                          </button>
                          <button className={styles.editButton} onClick={() => { track({ name: "workflow_archive_cancel", category: "user_action" }); setConfirmArchiveId(null); }}>
                            Cancel
                          </button>
                        </div>
                      ) : confirmDeleteFailedId === wf.id ? (
                        <div className={styles.actions}>
                          <span style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                            Delete failed sessions for &quot;{wf.name}&quot;?
                          </span>
                          <button className={styles.deleteButton} onClick={() => { track({ name: "workflow_delete_failed_confirm", category: "user_action" }); void handleDeleteFailedConfirm(); }} data-testid="confirm-delete-failed-sessions">
                            Yes, delete
                          </button>
                          <button className={styles.editButton} onClick={() => { track({ name: "workflow_delete_failed_cancel", category: "user_action" }); setConfirmDeleteFailedId(null); }}>
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <div className={styles.actions}>
                          <button
                            className={styles.runButton}
                            disabled={runningId === wf.id}
                            onClick={() => { track({ name: "workflow_run", category: "user_action" }); handleRun(wf); }}
                          >
                            {runningId === wf.id ? "…" : "▶ Run"}
                          </button>
                          <button className={styles.editButton} onClick={() => { track({ name: "workflow_edit_open", category: "user_action" }); openEdit(wf); }}>
                            Edit
                          </button>
                          <button
                            className={styles.editButton}
                            disabled={archivingId === wf.id}
                            onClick={() => { track({ name: "workflow_archive_start", category: "user_action" }); setConfirmArchiveId(wf.id); }}
                            data-testid="archive-sessions-button"
                          >
                            {archivingId === wf.id ? "…" : "Archive Sessions"}
                          </button>
                          <button
                            className={styles.editButton}
                            disabled={deletingFailedId === wf.id}
                            onClick={() => { track({ name: "workflow_delete_failed_start", category: "user_action" }); setConfirmDeleteFailedId(wf.id); }}
                            data-testid="delete-failed-sessions-button"
                          >
                            {deletingFailedId === wf.id ? "…" : "Delete Failed"}
                          </button>
                          <button
                            className={styles.deleteButton}
                            onClick={() => { track({ name: "workflow_delete_start", category: "user_action" }); handleDeleteClick(wf); }}
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
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formModal}
      {runDialogModal}
    </div>
  );
}
