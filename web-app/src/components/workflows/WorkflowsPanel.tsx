"use client";
// +feature: workflows-management

import { useState } from "react";
import { createPortal } from "react-dom";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { useWorkflows, WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { WorkflowForm } from "./WorkflowForm";
import * as styles from "./WorkflowsPanel.css";

export function WorkflowsPanel() {
  const { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow } = useWorkflows();
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState<WorkflowProto | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

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
                    ) : (
                      <div className={styles.actions}>
                        <button className={styles.editButton} onClick={() => openEdit(wf)}>
                          Edit
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
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formModal}
    </div>
  );
}
