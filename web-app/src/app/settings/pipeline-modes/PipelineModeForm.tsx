"use client";

import { useState, useCallback } from "react";
import { ConnectError, Code } from "@connectrpc/connect";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode, PipelineModeInput } from "@/lib/hooks/useBacklogService";
import * as styles from "./PipelineModeForm.css";

// The 9 content-template fields, in the same fixed order the backend uses for
// content_hash computation (see server/services/backlog_service_pipeline_mode.go
// pipelineModeToProto), each labeled with the file/prompt it drives — per
// plan.md Story 3.3.2's acceptance criteria.
const CONTENT_FIELDS: Array<{
  key: keyof Pick<
    PipelineModeInput,
    | "statusCommandTemplate"
    | "doneCommandTemplate"
    | "failCommandTemplate"
    | "reviewCommandTemplate"
    | "shipCommandTemplate"
    | "helpCommandTemplate"
    | "triagePromptTemplate"
    | "reviewPromptTemplate"
    | "initialPromptTemplate"
  >;
  label: string;
  hint: string;
}> = [
  { key: "statusCommandTemplate", label: "status.md content", hint: "Written to .claude/commands/backlog/status.md" },
  { key: "doneCommandTemplate", label: "done-N.md template", hint: "Written to .claude/commands/backlog/done-N.md" },
  { key: "failCommandTemplate", label: "fail-N.md template", hint: "Written to .claude/commands/backlog/fail-N.md" },
  { key: "reviewCommandTemplate", label: "review.md content", hint: "Written to .claude/commands/backlog/review.md" },
  { key: "shipCommandTemplate", label: "ship.md content", hint: "Written to .claude/commands/backlog/ship.md" },
  { key: "helpCommandTemplate", label: "help.md content", hint: "Written to .claude/commands/backlog/help.md" },
  { key: "triagePromptTemplate", label: "Triage prompt", hint: "Headless triage LLM call prompt" },
  { key: "reviewPromptTemplate", label: "Review prompt", hint: "Headless review-gate LLM call prompt" },
  { key: "initialPromptTemplate", label: "Initial prompt", hint: "Session's opening/interactive prompt" },
];

type ContentFieldValues = Record<(typeof CONTENT_FIELDS)[number]["key"], string>;

function emptyContentFields(mode: PipelineMode | null): ContentFieldValues {
  const values = {} as ContentFieldValues;
  for (const f of CONTENT_FIELDS) {
    values[f.key] = mode?.[f.key] ?? "";
  }
  return values;
}

/** Extracts a human-readable message from a create/update failure, preferring the ConnectError message (e.g. Story 2.3.1's CodeInvalidArgument text) over a generic fallback. */
function errorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export interface PipelineModeFormProps {
  /** null = create a new mode; non-null = edit this existing mode (slug becomes read-only). */
  mode: PipelineMode | null;
  onSaved: (mode: PipelineMode) => void;
  onDeleted: (id: string) => void;
  onCancel: () => void;
}

/**
 * Create/edit form for a PipelineMode: slug (immutable on edit), name,
 * description, enabled toggle, and the 9 labeled content-template textareas.
 * Also owns the delete-with-confirm action when editing an existing mode.
 */
export function PipelineModeForm({ mode, onSaved, onDeleted, onCancel }: PipelineModeFormProps) {
  const { createPipelineMode, updatePipelineMode, deletePipelineMode } = useBacklogService();

  const [slug, setSlug] = useState(mode?.slug ?? "");
  const [name, setName] = useState(mode?.name ?? "");
  const [description, setDescription] = useState(mode?.description ?? "");
  const [enabled, setEnabled] = useState(mode?.enabled ?? true);
  const [contentFields, setContentFields] = useState<ContentFieldValues>(() => emptyContentFields(mode));

  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const setField = useCallback((key: (typeof CONTENT_FIELDS)[number]["key"], value: string) => {
    setContentFields((prev) => ({ ...prev, [key]: value }));
  }, []);

  const canSubmit = Boolean(slug.trim()) && Boolean(name.trim()) && !submitting;

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;
      setError(null);
      setSubmitting(true);
      try {
        let saved: PipelineMode;
        if (mode) {
          saved = await updatePipelineMode(mode.id, {
            name: name.trim(),
            description: description.trim(),
            enabled,
            ...contentFields,
          });
        } else {
          saved = await createPipelineMode({
            slug: slug.trim(),
            name: name.trim(),
            description: description.trim(),
            enabled,
            ...contentFields,
          });
        }
        onSaved(saved);
      } catch (err) {
        // Story 3.3.2 acceptance criteria: on CodeInvalidArgument (or any
        // other failure) the error is displayed inline — no navigation, no
        // list refresh, and the form's in-progress edits are preserved.
        setError(errorMessage(err));
      } finally {
        setSubmitting(false);
      }
    },
    [canSubmit, mode, name, description, enabled, contentFields, slug, createPipelineMode, updatePipelineMode, onSaved]
  );

  const handleDeleteClick = useCallback(() => {
    setError(null);
    setConfirmingDelete(true);
  }, []);

  const handleDeleteCancel = useCallback(() => {
    setConfirmingDelete(false);
  }, []);

  const handleDeleteConfirm = useCallback(async () => {
    if (!mode) return;
    setDeleting(true);
    setError(null);
    try {
      await deletePipelineMode(mode.id);
      onDeleted(mode.id);
    } catch (err) {
      setError(errorMessage(err));
      setConfirmingDelete(false);
    } finally {
      setDeleting(false);
    }
  }, [mode, deletePipelineMode, onDeleted]);

  return (
    <form className={styles.form} onSubmit={handleSubmit} data-testid="pipeline-mode-form">
      {error && (
        <div className={styles.errorMessage} role="alert" data-testid="pipeline-mode-error">
          {error}
        </div>
      )}

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="pipeline-mode-slug">
          Slug
        </label>
        <input
          id="pipeline-mode-slug"
          type="text"
          className={mode ? [styles.input, styles.inputDisabled].join(" ") : styles.input}
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          disabled={Boolean(mode)}
          placeholder="quick"
          data-testid="pipeline-mode-slug"
        />
        {mode && <span className={styles.hint}>Slugs are immutable after creation.</span>}
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="pipeline-mode-name">
          Name
        </label>
        <input
          id="pipeline-mode-name"
          type="text"
          className={styles.input}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Quick Fix"
          data-testid="pipeline-mode-name"
        />
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="pipeline-mode-description">
          Description
        </label>
        <textarea
          id="pipeline-mode-description"
          className={styles.input}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Fast, low-ceremony pipeline for small fixes"
          data-testid="pipeline-mode-description"
        />
      </div>

      <div className={styles.checkboxRow}>
        <input
          id="pipeline-mode-enabled"
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          data-testid="pipeline-mode-enabled"
        />
        <label className={styles.label} htmlFor="pipeline-mode-enabled">
          Enabled
        </label>
      </div>

      <div className={styles.templateFieldsGrid}>
        {CONTENT_FIELDS.map((f) => (
          <div className={styles.fieldGroup} key={f.key}>
            <label className={styles.label} htmlFor={`pipeline-mode-field-${f.key}`}>
              {f.label}
            </label>
            <span className={styles.hint}>{f.hint}</span>
            <textarea
              id={`pipeline-mode-field-${f.key}`}
              className={styles.textarea}
              value={contentFields[f.key]}
              onChange={(e) => setField(f.key, e.target.value)}
              data-testid={`pipeline-mode-field-${f.key}`}
            />
          </div>
        ))}
      </div>

      <div className={styles.actionRow}>
        <button
          type="submit"
          className={styles.submitBtn}
          disabled={!canSubmit}
          data-testid="pipeline-mode-submit"
        >
          {submitting ? "Saving…" : mode ? "Save changes" : "Create mode"}
        </button>
        <button type="button" className={styles.cancelBtn} onClick={onCancel} data-testid="pipeline-mode-cancel">
          Cancel
        </button>

        {mode &&
          (confirmingDelete ? (
            <>
              <button
                type="button"
                className={styles.confirmDeleteBtn}
                onClick={handleDeleteConfirm}
                disabled={deleting}
                aria-label={`Confirm delete pipeline mode ${mode.slug}`}
                data-testid="pipeline-mode-confirm-delete"
              >
                {deleting ? "Deleting…" : "Confirm delete?"}
              </button>
              <button
                type="button"
                className={styles.cancelBtn}
                onClick={handleDeleteCancel}
                disabled={deleting}
                data-testid="pipeline-mode-cancel-delete"
              >
                Never mind
              </button>
            </>
          ) : (
            <button
              type="button"
              className={styles.deleteBtn}
              onClick={handleDeleteClick}
              aria-label={`Delete pipeline mode ${mode.slug}`}
              data-testid="pipeline-mode-delete"
            >
              Delete
            </button>
          ))}
      </div>
    </form>
  );
}
