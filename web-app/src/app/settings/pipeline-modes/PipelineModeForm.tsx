"use client";

import { Fragment, useState, useCallback } from "react";
import { ConnectError, Code } from "@connectrpc/connect";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode, PipelineModeInput } from "@/lib/hooks/useBacklogService";
import { AutocompleteInput } from "@/components/ui/AutocompleteInput";
import { PROGRAMS, MODEL_AUTOCOMPLETE_OPTIONS, getProgramDisplay } from "@/lib/constants/programs";
import { fuzzyMatchModels, getModelLabel } from "@/lib/constants/modelFuzzyMatch";
import * as styles from "./PipelineModeForm.css";

// Stage roles a pipeline mode can override, matching session.StageRole's 3
// consts exactly (session/pipeline_mode_executor.go) — order here is also the
// row order the table renders in.
type StageRole = "triage" | "review" | "work";

const STAGE_ROLES: ReadonlyArray<{ role: StageRole; label: string }> = [
  { role: "triage", label: "Triage" },
  { role: "review", label: "Review" },
  { role: "work", label: "Work" },
];

type StageExecutorValues = Record<StageRole, { program: string; model: string }>;

function emptyStageExecutors(mode: PipelineMode | null): StageExecutorValues {
  const values = {} as StageExecutorValues;
  for (const { role } of STAGE_ROLES) {
    const existing = mode?.stageExecutors?.[role];
    values[role] = { program: existing?.program ?? "", model: existing?.model ?? "" };
  }
  return values;
}

// Static list, not useAvailablePrograms()'s host-detected one: a pipeline
// mode's stage executor can target a different machine than the browser's,
// so "installed here" isn't the right filter (per plan.md Task 5.1.1b).
const STAGE_PROGRAM_SUGGESTIONS = PROGRAMS.filter((p) => p.value).map((p) => p.value);
const STAGE_MODEL_SUGGESTIONS = MODEL_AUTOCOMPLETE_OPTIONS.map((m) => m.value);

interface StageFieldError {
  role: StageRole;
  message: string;
  /** True when this is specifically Story 1.3.1c2's unrecognized-model-ID rejection — offers the force_unknown_model override (Task 5.1.1h). */
  offersForceOverride: boolean;
}

/**
 * Finds which stage row a CodeInvalidArgument message refers to. The backend
 * always names the offending role in its error text (Tasks 1.3.1b/c2/e's
 * acceptance criteria), so a case-insensitive substring check is sufficient;
 * returns null when no role is confidently identifiable, so the caller can
 * fall back to the generic top-of-form banner instead of guessing.
 */
function matchStageFromError(message: string): StageRole | null {
  const lower = message.toLowerCase();
  return STAGE_ROLES.find(({ role }) => lower.includes(role))?.role ?? null;
}

function isForceOverrideEligible(message: string): boolean {
  return message.toLowerCase().includes("force_unknown_model");
}

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

/**
 * Classifies a create/update failure for display: a save-time CodeInvalidArgument
 * naming a specific stage (Story 1.3.1) becomes a StageFieldError rendered next to
 * that row (Task 5.1.1g); everything else falls back to the generic top-of-form
 * banner `PipelineModeForm` already used pre-Epic-5.1 (Story 3.3.2).
 */
function classifySaveError(err: unknown): { bannerMessage: string | null; stageError: StageFieldError | null } {
  const message = errorMessage(err);
  const role = err instanceof ConnectError && err.code === Code.InvalidArgument ? matchStageFromError(message) : null;
  if (role) {
    return { bannerMessage: null, stageError: { role, message, offersForceOverride: isForceOverrideEligible(message) } };
  }
  return { bannerMessage: message, stageError: null };
}

interface StageExecutorTableProps {
  values: StageExecutorValues;
  onFieldChange: (role: StageRole, field: "program" | "model", value: string) => void;
  fieldError: StageFieldError | null;
  forceUnknownModel: boolean;
  onForceUnknownModelChange: (checked: boolean) => void;
}

interface StageExecutorRowProps {
  role: StageRole;
  stageLabel: string;
  value: { program: string; model: string };
  onFieldChange: (role: StageRole, field: "program" | "model", value: string) => void;
  fieldError: StageFieldError | null;
  forceUnknownModel: boolean;
  onForceUnknownModelChange: (checked: boolean) => void;
}

/** One stage's Program/Model cells, plus its inline save-error row when `fieldError` names this role (Tasks 5.1.1g/h). */
function StageExecutorRow({
  role,
  stageLabel,
  value,
  onFieldChange,
  fieldError,
  forceUnknownModel,
  onForceUnknownModelChange,
}: StageExecutorRowProps) {
  return (
    <Fragment>
      <tr>
        <th className={styles.stageExecutorRoleCell} scope="row">
          {stageLabel}
        </th>
        <td className={styles.stageExecutorCell}>
          <label className={styles.visuallyHidden} htmlFor={`pipeline-mode-stage-${role}-program`}>
            {`${stageLabel} stage program`}
          </label>
          <AutocompleteInput
            id={`pipeline-mode-stage-${role}-program`}
            value={value.program}
            onChange={(v) => onFieldChange(role, "program", v)}
            placeholder="System default"
            suggestions={STAGE_PROGRAM_SUGGESTIONS}
            getLabel={getProgramDisplay}
            className={styles.input}
            data-testid={`pipeline-mode-stage-${role}-program`}
          />
        </td>
        <td className={styles.stageExecutorCell}>
          <label className={styles.visuallyHidden} htmlFor={`pipeline-mode-stage-${role}-model`}>
            {`${stageLabel} stage model`}
          </label>
          <AutocompleteInput
            id={`pipeline-mode-stage-${role}-model`}
            value={value.model}
            onChange={(v) => onFieldChange(role, "model", v)}
            placeholder="System default"
            suggestions={STAGE_MODEL_SUGGESTIONS}
            filterFn={fuzzyMatchModels}
            getLabel={getModelLabel}
            className={styles.input}
            data-testid={`pipeline-mode-stage-${role}-model`}
          />
        </td>
      </tr>
      {fieldError?.role === role && (
        <tr>
          <td className={styles.stageExecutorCell} colSpan={3}>
            <div className={styles.stageFieldError} role="alert" data-testid={`pipeline-mode-stage-${role}-error`}>
              <span>{fieldError.message}</span>
              {fieldError.offersForceOverride && (
                <label className={styles.forceOverrideLabel}>
                  <input
                    type="checkbox"
                    checked={forceUnknownModel}
                    onChange={(e) => onForceUnknownModelChange(e.target.checked)}
                    data-testid="pipeline-mode-force-unknown-model"
                  />
                  I know this model isn&apos;t in the pricing table yet — save anyway
                </label>
              )}
            </div>
          </td>
        </tr>
      )}
    </Fragment>
  );
}

/** The 3-row (Triage/Review/Work) × {Program, Model} table from Task 5.1.1b. Extracted from PipelineModeForm's render to keep that function's body a reasonable size. */
function StageExecutorTable({
  values,
  onFieldChange,
  fieldError,
  forceUnknownModel,
  onForceUnknownModelChange,
}: StageExecutorTableProps) {
  return (
    <div className={styles.stageExecutorSection}>
      <h3 className={styles.sectionHeading}>Stage execution</h3>
      <span className={styles.hint}>
        Different models per stage means no prompt cache carries over between stages.
      </span>
      <table className={styles.stageExecutorTable} data-testid="pipeline-mode-stage-executor-table">
        <thead>
          <tr>
            <th className={styles.stageExecutorHeaderCell} scope="col">
              Stage
            </th>
            <th className={styles.stageExecutorHeaderCell} scope="col">
              Program
            </th>
            <th className={styles.stageExecutorHeaderCell} scope="col">
              Model
            </th>
          </tr>
        </thead>
        <tbody>
          {STAGE_ROLES.map(({ role, label: stageLabel }) => (
            <StageExecutorRow
              key={role}
              role={role}
              stageLabel={stageLabel}
              value={values[role]}
              onFieldChange={onFieldChange}
              fieldError={fieldError}
              forceUnknownModel={forceUnknownModel}
              onForceUnknownModelChange={onForceUnknownModelChange}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
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
  const [stageExecutors, setStageExecutors] = useState<StageExecutorValues>(() => emptyStageExecutors(mode));
  const [forceUnknownModel, setForceUnknownModel] = useState(false);

  const [error, setError] = useState<string | null>(null);
  const [stageFieldError, setStageFieldError] = useState<StageFieldError | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const setField = useCallback((key: (typeof CONTENT_FIELDS)[number]["key"], value: string) => {
    setContentFields((prev) => ({ ...prev, [key]: value }));
  }, []);

  const setStageExecutorField = useCallback(
    (role: StageRole, field: "program" | "model", value: string) => {
      setStageExecutors((prev) => ({ ...prev, [role]: { ...prev[role], [field]: value } }));
    },
    []
  );

  const canSubmit = Boolean(slug.trim()) && Boolean(name.trim()) && !submitting;

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;
      setError(null);
      setStageFieldError(null);
      setSubmitting(true);
      try {
        let saved: PipelineMode;
        if (mode) {
          saved = await updatePipelineMode(mode.id, {
            name: name.trim(),
            description: description.trim(),
            enabled,
            ...contentFields,
            stageExecutors,
            forceUnknownModel,
          });
        } else {
          saved = await createPipelineMode({
            slug: slug.trim(),
            name: name.trim(),
            description: description.trim(),
            enabled,
            ...contentFields,
            stageExecutors,
            forceUnknownModel,
          });
        }
        onSaved(saved);
        setForceUnknownModel(false);
      } catch (err) {
        // Story 3.3.2 acceptance criteria: on CodeInvalidArgument (or any
        // other failure) the error is displayed inline — no navigation, no
        // list refresh, and the form's in-progress edits are preserved.
        const { bannerMessage, stageError } = classifySaveError(err);
        setError(bannerMessage);
        setStageFieldError(stageError);
      } finally {
        setSubmitting(false);
      }
    },
    [
      canSubmit,
      mode,
      name,
      description,
      enabled,
      contentFields,
      stageExecutors,
      forceUnknownModel,
      slug,
      createPipelineMode,
      updatePipelineMode,
      onSaved,
    ]
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

      <StageExecutorTable
        values={stageExecutors}
        onFieldChange={setStageExecutorField}
        fieldError={stageFieldError}
        forceUnknownModel={forceUnknownModel}
        onForceUnknownModelChange={setForceUnknownModel}
      />

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
