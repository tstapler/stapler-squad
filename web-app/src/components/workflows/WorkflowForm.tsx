"use client";

import { useState } from "react";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { WorkflowFormData } from "@/lib/hooks/useWorkflows";
import * as styles from "./WorkflowForm.css";

interface WorkflowFormProps {
  /** If provided, we are in edit mode; otherwise create mode. */
  existing?: WorkflowProto;
  onSubmit: (data: WorkflowFormData) => Promise<void>;
  onCancel: () => void;
}

const EMPTY: WorkflowFormData = {
  slug: "",
  name: "",
  description: "",
  command: "",
  targetDirectory: "",
  inputTemplate: "",
  sessionType: "directory",
  model: "",
  agentType: "",
  cronExpression: "",
  cronEnabled: false,
};

function protoToFormData(w: WorkflowProto): WorkflowFormData {
  return {
    slug: w.slug,
    name: w.name,
    description: w.description,
    command: w.command,
    targetDirectory: w.targetDirectory,
    inputTemplate: w.inputTemplate,
    sessionType: w.sessionType || "directory",
    model: w.model,
    agentType: w.agentType,
    cronExpression: w.cronExpression,
    cronEnabled: w.cronEnabled,
  };
}

export function WorkflowForm({ existing, onSubmit, onCancel }: WorkflowFormProps) {
  const [formData, setFormData] = useState<WorkflowFormData>(
    existing ? protoToFormData(existing) : EMPTY
  );
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isEdit = !!existing;

  function setField<K extends keyof WorkflowFormData>(key: K, value: WorkflowFormData[K]) {
    setFormData((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(formData);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save workflow");
      setSubmitting(false);
    }
  }

  return (
    <form className={styles.form} onSubmit={handleSubmit}>
      <div className={styles.formHeader}>
        <h2 className={styles.formTitle}>{isEdit ? "Edit Workflow" : "New Workflow"}</h2>
      </div>

      {error && <div className={styles.errorBanner}>{error}</div>}

      <div className={styles.row}>
        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-slug">
            Slug <span className={styles.required}>*</span>
          </label>
          <input
            id="wf-slug"
            className={styles.input}
            type="text"
            value={formData.slug}
            onChange={(e) => setField("slug", e.target.value)}
            placeholder="my-workflow"
            required
            disabled={isEdit}
            pattern="[a-z0-9]+(-[a-z0-9]+)*"
            minLength={2}
            maxLength={64}
            title="2-64 lowercase chars, hyphens allowed (no consecutive or leading/trailing hyphens)"
          />
          <span className={styles.hint}>Type @slug in the omnibar to invoke</span>
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-name">
            Name <span className={styles.required}>*</span>
          </label>
          <input
            id="wf-name"
            className={styles.input}
            type="text"
            value={formData.name}
            onChange={(e) => setField("name", e.target.value)}
            placeholder="My Workflow"
            required
          />
        </div>
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="wf-description">Description</label>
        <input
          id="wf-description"
          className={styles.input}
          type="text"
          value={formData.description ?? ""}
          onChange={(e) => setField("description", e.target.value)}
          placeholder="What does this workflow do?"
        />
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="wf-command">
          Command / Prompt <span className={styles.required}>*</span>
        </label>
        <textarea
          id="wf-command"
          className={styles.textarea}
          value={formData.command}
          onChange={(e) => setField("command", e.target.value)}
          placeholder="Describe what the agent should do…"
          required
        />
        <span className={styles.hint}>
          Use <code>{"{{input}}"}</code> to inject the argument typed after @slug
        </span>
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="wf-target-dir">
          Target Directory <span className={styles.required}>*</span>
        </label>
        <input
          id="wf-target-dir"
          className={styles.input}
          type="text"
          value={formData.targetDirectory}
          onChange={(e) => setField("targetDirectory", e.target.value)}
          placeholder="/path/to/project"
          required
        />
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="wf-input-template">Input Template</label>
        <textarea
          id="wf-input-template"
          className={styles.textarea}
          value={formData.inputTemplate ?? ""}
          onChange={(e) => setField("inputTemplate", e.target.value)}
          placeholder="Optional: template wrapping the user-supplied {{input}}"
        />
        <span className={styles.hint}>
          If set, <code>{"{{input}}"}</code> is replaced with the argument before passing to the agent.
        </span>
      </div>

      <div className={styles.row}>
        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-model">Model</label>
          <input
            id="wf-model"
            className={styles.input}
            type="text"
            value={formData.model ?? ""}
            onChange={(e) => setField("model", e.target.value)}
            placeholder="claude-opus-4-5"
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-agent-type">Agent Type</label>
          <input
            id="wf-agent-type"
            className={styles.input}
            type="text"
            value={formData.agentType ?? ""}
            onChange={(e) => setField("agentType", e.target.value)}
            placeholder="claude"
          />
        </div>
      </div>

      <div className={styles.row}>
        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-cron">Cron Expression</label>
          <input
            id="wf-cron"
            className={styles.input}
            type="text"
            value={formData.cronExpression ?? ""}
            onChange={(e) => setField("cronExpression", e.target.value)}
            placeholder="0 9 * * 1-5"
          />
          <span className={styles.hint}>Standard 5-field cron syntax</span>
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label}>&nbsp;</label>
          <label className={styles.checkboxRow}>
            <input
              type="checkbox"
              checked={formData.cronEnabled}
              onChange={(e) => setField("cronEnabled", e.target.checked)}
            />
            Enable scheduled runs
          </label>
        </div>
      </div>

      <div className={styles.buttonRow}>
        <button type="button" className={styles.cancelButton} onClick={onCancel} disabled={submitting}>
          Cancel
        </button>
        <button type="submit" className={styles.submitButton} disabled={submitting}>
          {submitting ? "Saving…" : isEdit ? "Save Changes" : "Create Workflow"}
        </button>
      </div>
    </form>
  );
}
