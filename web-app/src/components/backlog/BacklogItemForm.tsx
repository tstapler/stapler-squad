"use client";
// +feature: backlog:item-form

import { useState, useCallback, useMemo, useEffect } from "react";
import Link from "next/link";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { BacklogItem, BacklogItemInput, AcCriterion, AcCriterionStatus, PipelineMode } from "@/lib/hooks/useBacklogService";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import { RadioGroup } from "@/components/ui/RadioGroup";
import type { RadioGroupOption } from "@/components/ui/RadioGroup";
import { radioBtn, radioBtnActive } from "@/components/ui/RadioGroup.css";
import { isGitHubRef } from "@/lib/github/urlParser";
import { routes } from "@/lib/routes";
import * as styles from "./BacklogItemForm.css";

// The 9 content-template fields on PipelineMode — used to detect whether the
// selected mode's rendered content depends on {{repo_path}} (G-1).
const CONTENT_TEMPLATE_KEYS = [
  "statusCommandTemplate",
  "doneCommandTemplate",
  "failCommandTemplate",
  "reviewCommandTemplate",
  "shipCommandTemplate",
  "helpCommandTemplate",
  "triagePromptTemplate",
  "reviewPromptTemplate",
  "initialPromptTemplate",
] as const;

const DEFAULT_PIPELINE_MODE_OPTION: RadioGroupOption<string> = {
  value: "",
  label: "Default",
  description: "Built-in default pipeline",
  dataTestId: "backlog-pipeline-mode-default",
};

const PIPELINE_MODE_FETCH_ERROR_NOTICE =
  "Couldn't load pipeline modes — you can still save with Default.";

const PIPELINE_MODE_UNKNOWN_HINT =
  "This item references a pipeline mode that no longer exists or is disabled. Choosing a different mode below will replace it when you save.";

interface BacklogItemFormProps {
  initialValues?: Partial<BacklogItem>;
  onSubmit: (data: BacklogItemInput) => Promise<void>;
  onCancel: () => void;
  isLoading?: boolean;
}

interface FormErrors {
  title?: string;
  repoPath?: string;
  acCriteria?: string;
}

const AC_STATUS_OPTIONS: { value: AcCriterionStatus; label: string }[] = [
  { value: "pending", label: "Pending" },
  { value: "in_progress", label: "In Progress" },
  { value: "done", label: "Done" },
];

export function BacklogItemForm({
  initialValues,
  onSubmit,
  onCancel,
  isLoading = false,
}: BacklogItemFormProps) {
  const [title, setTitle] = useState(initialValues?.title ?? "");
  const [description, setDescription] = useState(initialValues?.description ?? "");
  const [repoPath, setRepoPath] = useState(initialValues?.repoPath ?? "");
  const [priority, setPriority] = useState<number>(initialValues?.priority ?? 3);
  const [skipPlanning, setSkipPlanning] = useState(initialValues?.skipPlanning ?? false);
  const [skipReviewGate, setSkipReviewGate] = useState(initialValues?.skipReviewGate ?? false);
  const [autoSpawnSession, setAutoSpawnSession] = useState(initialValues?.autoSpawnSession ?? false);
  const [autoCreatePR, setAutoCreatePR] = useState(initialValues?.autoCreatePR ?? false);
  const [acCriteria, setAcCriteria] = useState<AcCriterion[]>(
    initialValues?.acCriteria ?? []
  );
  const [pipelineMode, setPipelineMode] = useState(initialValues?.pipelineMode ?? "");
  const [errors, setErrors] = useState<FormErrors>({});
  const [submitting, setSubmitting] = useState(false);

  const { listPipelineModes } = useBacklogService();
  const [availableModes, setAvailableModes] = useState<PipelineMode[]>([]);
  const [modesLoading, setModesLoading] = useState(true);
  const [modesError, setModesError] = useState(false);

  // Story 3.2.1 / G-4 / G-3: fetch enabled pipeline modes on mount. While
  // pending or on failure, the RadioGroup below falls back to only the
  // hardcoded "Default" option — never blocking the form.
  useEffect(() => {
    let cancelled = false;
    setModesLoading(true);
    setModesError(false);
    listPipelineModes()
      .then((modes) => {
        if (cancelled) return;
        setAvailableModes(modes.filter((m) => m.enabled));
      })
      .catch(() => {
        if (cancelled) return;
        setModesError(true);
      })
      .finally(() => {
        if (!cancelled) setModesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [listPipelineModes]);

  // Options offered by the RadioGroup: "Default" is always first and always
  // present. While the fetch is pending or failed, no other options render —
  // this alone satisfies G-4/G-3's "only Default selectable" requirement.
  const pipelineModeOptions = useMemo<RadioGroupOption<string>[]>(() => {
    if (modesLoading || modesError) {
      return [DEFAULT_PIPELINE_MODE_OPTION];
    }
    return [
      DEFAULT_PIPELINE_MODE_OPTION,
      ...availableModes.map((m) => ({
        value: m.slug,
        label: m.name,
        description: m.description || undefined,
        dataTestId: `backlog-pipeline-mode-${m.slug}`,
      })),
    ];
  }, [modesLoading, modesError, availableModes]);

  // The fetch succeeded but no enabled modes exist yet — distinct from
  // loading/error: without this, a picker with nothing to pick from looks
  // identical to a picker that's broken or hasn't fetched at all (the exact
  // "single greyed Default button, clicking it does nothing" confusion
  // flagged in docs/tasks/backlog-feature-improvement.md's 2026-07-19 audit).
  const hasNoAvailableModes = !modesLoading && !modesError && availableModes.length === 0;

  // G-2: an item's stored pipelineMode may reference a mode that's since been
  // deleted or disabled. Only evaluate once the fetch has actually succeeded —
  // during loading/error we can't yet tell "unresolvable" apart from
  // "haven't checked yet".
  const unresolvedPipelineMode = useMemo(() => {
    if (modesLoading || modesError) return null;
    if (!pipelineMode) return null;
    if (pipelineModeOptions.some((o) => o.value === pipelineMode)) return null;
    return pipelineMode;
  }, [modesLoading, modesError, pipelineMode, pipelineModeOptions]);

  const pipelineModeHintForValue = useCallback(
    (v: string) => {
      if (unresolvedPipelineMode && v === unresolvedPipelineMode) {
        return PIPELINE_MODE_UNKNOWN_HINT;
      }
      return pipelineModeOptions.find((o) => o.value === v)?.description;
    },
    [unresolvedPipelineMode, pipelineModeOptions]
  );

  const unresolvedPipelineModeOption = unresolvedPipelineMode ? (
    <button
      type="button"
      role="radio"
      aria-checked="true"
      aria-disabled="true"
      disabled
      tabIndex={-1}
      className={[radioBtn, radioBtnActive].join(" ")}
      data-testid={`backlog-pipeline-mode-unknown-${unresolvedPipelineMode}`}
    >
      {`Unknown mode ('${unresolvedPipelineMode}')`}
    </button>
  ) : undefined;

  // G-1: the selected mode's own content-template fields may assume a
  // repo path is present. Non-blocking — warns, never disables.
  const selectedPipelineMode = useMemo(
    () => availableModes.find((m) => m.slug === pipelineMode) ?? null,
    [availableModes, pipelineMode]
  );
  const selectedModeRequiresRepoPath = useMemo(() => {
    if (!selectedPipelineMode) return false;
    return CONTENT_TEMPLATE_KEYS.some((key) => selectedPipelineMode[key]?.includes("{{repo_path}}"));
  }, [selectedPipelineMode]);
  const showRepoPathPrerequisiteWarning = selectedModeRequiresRepoPath && !repoPath.trim();

  const validate = useCallback((): FormErrors => {
    const errs: FormErrors = {};
    if (!title.trim()) {
      errs.title = "Title is required.";
    }
    if (!initialValues?.id && !repoPath.trim()) {
      errs.repoPath = "Repository path is required for automated triage.";
    }
    return errs;
  }, [title, repoPath, initialValues?.id]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const errs = validate();
      if (Object.keys(errs).length > 0) {
        setErrors(errs);
        return;
      }
      setErrors({});
      setSubmitting(true);
      try {
        // Evaluate vagueness before submitting: short description + no AC = vague
        const descriptionText = description.trim();
        const isVague = descriptionText.length < 80 && acCriteria.length === 0;
        await onSubmit({
          title: title.trim(),
          description: descriptionText || undefined,
          repoPath: repoPath.trim(),
          priority,
          skipPlanning,
          skipReviewGate,
          autoSpawnSession,
          autoCreatePR,
          acCriteria: acCriteria.map((c, i) => ({ ...c, index: i })),
          skipTriage: isVague,
          pipelineMode,
        });
      } finally {
        setSubmitting(false);
      }
    },
    [title, description, repoPath, priority, skipPlanning, skipReviewGate, autoSpawnSession, autoCreatePR, acCriteria, pipelineMode, onSubmit, validate]
  );

  const addCriterion = useCallback(() => {
    setAcCriteria((prev) => [
      ...prev,
      { index: prev.length, text: "", status: "pending" as AcCriterionStatus },
    ]);
  }, []);

  const removeCriterion = useCallback((index: number) => {
    setAcCriteria((prev) => prev.filter((_, i) => i !== index).map((c, i) => ({ ...c, index: i })));
  }, []);

  const updateCriterionText = useCallback((index: number, text: string) => {
    setAcCriteria((prev) =>
      prev.map((c, i) => (i === index ? { ...c, text } : c))
    );
  }, []);

  const updateCriterionStatus = useCallback((index: number, status: AcCriterionStatus) => {
    setAcCriteria((prev) =>
      prev.map((c, i) => (i === index ? { ...c, status } : c))
    );
  }, []);

  const busy = submitting || isLoading;
  const isCloningRepo = useMemo(() => isGitHubRef(repoPath), [repoPath]);

  return (
    <form
      className={styles.form}
      onSubmit={handleSubmit}
      aria-label="Backlog item form"
      noValidate
      data-testid="backlog-item-form"
    >
      {/* Title */}
      <div className={styles.fieldGroup}>
        <label htmlFor="backlog-title" className={styles.label}>
          Title <span className={styles.required} aria-hidden="true">*</span>
        </label>
        <input
          id="backlog-title"
          type="text"
          className={styles.input}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="What needs to be done?"
          required
          aria-required="true"
          aria-invalid={!!errors.title}
          aria-describedby={errors.title ? "backlog-title-error" : undefined}
          disabled={busy}
          data-testid="backlog-title-input"
        />
        {errors.title && (
          <span id="backlog-title-error" className={styles.errorMessage} role="alert">
            {errors.title}
          </span>
        )}
      </div>

      {/* Description */}
      <div className={styles.fieldGroup}>
        <label htmlFor="backlog-description" className={styles.label}>
          Description
        </label>
        <textarea
          id="backlog-description"
          className={styles.textarea}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Provide more context (optional)"
          disabled={busy}
          data-testid="backlog-description-input"
        />
      </div>

      {/* Repo path + Priority */}
      <div className={styles.twoColumn}>
        <div className={styles.fieldGroup}>
          <label htmlFor="backlog-repo-path" className={styles.label}>
            Repository Path <span className={styles.required} aria-hidden="true">*</span>
          </label>
          <RepoPathInput
            id="backlog-repo-path"
            value={repoPath}
            onChange={setRepoPath}
            placeholder="/home/user/project or https://github.com/owner/repo"
            required
            disabled={busy}
            error={errors.repoPath}
            hint="Local path to your clone, or a GitHub URL — we'll clone it for you."
            detectGitHubUrl
            data-testid="backlog-repo-path-input"
          />
          {errors.repoPath && (
            <span id="backlog-repo-path-error" className={styles.errorMessage} role="alert">
              {errors.repoPath}
            </span>
          )}
        </div>

        <div className={styles.fieldGroup}>
          <label htmlFor="backlog-priority" className={styles.label}>
            Priority
          </label>
          <select
            id="backlog-priority"
            className={styles.select}
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
            disabled={busy}
            data-testid="backlog-priority-select"
          >
            <option value={1}>P1 — Critical</option>
            <option value={2}>P2 — High</option>
            <option value={3}>P3 — Medium</option>
            <option value={4}>P4 — Low</option>
            <option value={5}>P5 — Trivial</option>
          </select>
        </div>
      </div>

      {/* Pipeline mode selector (Epic 3.2) */}
      <div className={styles.fieldGroup}>
        <RadioGroup
          options={pipelineModeOptions}
          value={pipelineMode}
          onChange={setPipelineMode}
          groupLabel="Pipeline mode"
          hintForValue={pipelineModeHintForValue}
          trailingContent={unresolvedPipelineModeOption}
        />
        {modesError && (
          <span role="status" className={styles.checkboxHint} data-testid="backlog-pipeline-mode-fetch-error">
            {PIPELINE_MODE_FETCH_ERROR_NOTICE}
          </span>
        )}
        {hasNoAvailableModes && (
          <span className={styles.pipelineModeEmptyHint} data-testid="backlog-pipeline-mode-empty-hint">
            No custom pipeline modes exist yet.{" "}
            <Link href={routes.settingsPipelineModes} className={styles.pipelineModeEmptyHintLink}>
              Create one in Settings →
            </Link>
          </span>
        )}
      </div>

      {/* Flags */}
      <fieldset className={styles.fieldGroup} data-testid="backlog-overrides-fieldset">
        <legend className={styles.label}>Overrides (independent of pipeline mode)</legend>
        <div className={styles.twoColumn}>
          <div className={styles.fieldGroup}>
            <label className={styles.checkboxRow} htmlFor="backlog-skip-planning">
              <input
                id="backlog-skip-planning"
                type="checkbox"
                className={styles.checkboxInput}
                checked={skipPlanning}
                onChange={(e) => setSkipPlanning(e.target.checked)}
                disabled={busy}
                data-testid="backlog-skip-planning-checkbox"
              />
              <span className={styles.checkboxLabel}>Skip planning phase</span>
            </label>
            <span className={styles.checkboxHint}>
              Go straight to triage without a separate planning pass.
            </span>
          </div>

          <div className={styles.fieldGroup}>
            <label className={styles.checkboxRow} htmlFor="backlog-skip-review">
              <input
                id="backlog-skip-review"
                type="checkbox"
                className={styles.checkboxInput}
                checked={skipReviewGate}
                onChange={(e) => setSkipReviewGate(e.target.checked)}
                disabled={busy}
                data-testid="backlog-skip-review-checkbox"
              />
              <span className={styles.checkboxLabel}>Skip review gate</span>
            </label>
            <span className={styles.checkboxHint}>
              Mark work done without an automated review pass first.
            </span>
          </div>

          <div className={styles.fieldGroup}>
            <label className={styles.checkboxRow} htmlFor="backlog-auto-spawn-session">
              <input
                id="backlog-auto-spawn-session"
                type="checkbox"
                className={styles.checkboxInput}
                checked={autoSpawnSession}
                onChange={(e) => setAutoSpawnSession(e.target.checked)}
                disabled={busy}
                data-testid="backlog-auto-spawn-session-checkbox"
              />
              <span className={styles.checkboxLabel}>Auto-spawn work session</span>
            </label>
            <span className={styles.checkboxHint}>
              Skip the manual &quot;Spawn Session&quot; click — start work automatically once triage marks the item ready.
            </span>
          </div>

          <div className={styles.fieldGroup}>
            <label className={styles.checkboxRow} htmlFor="backlog-auto-create-pr">
              <input
                id="backlog-auto-create-pr"
                type="checkbox"
                className={styles.checkboxInput}
                checked={autoCreatePR}
                onChange={(e) => setAutoCreatePR(e.target.checked)}
                disabled={busy}
                data-testid="backlog-auto-create-pr-checkbox"
              />
              <span className={styles.checkboxLabel}>Auto-create PR on completion</span>
            </label>
            <span className={styles.checkboxHint}>
              Skip the manual Review Queue &quot;Create PR&quot; click — a PR is opened automatically once a work session finishes. The prompt still runs unattended, so review the diff before merging.
            </span>
          </div>
        </div>
      </fieldset>

      {showRepoPathPrerequisiteWarning && selectedPipelineMode && (
        <span role="alert" className={styles.errorMessage} data-testid="backlog-pipeline-mode-repo-path-warning">
          {`⚠ ${selectedPipelineMode.name} mode requires a repository path — add one above.`}
        </span>
      )}

      {/* Acceptance Criteria */}
      <div className={styles.acSection}>
        <div className={styles.acSectionHeader}>
          <label className={styles.label}>Acceptance Criteria</label>
          <button
            type="button"
            className={styles.addButton}
            onClick={addCriterion}
            disabled={busy}
            aria-label="Add acceptance criterion"
            data-testid="backlog-add-criterion"
          >
            + Add criterion
          </button>
        </div>

        {acCriteria.length > 0 && (
          <div className={styles.acList} role="list" aria-label="Acceptance criteria list">
            {acCriteria.map((criterion, i) => (
              <div key={i} className={styles.acRow} role="listitem">
                <input
                  type="text"
                  className={styles.acInput}
                  value={criterion.text}
                  onChange={(e) => updateCriterionText(i, e.target.value)}
                  placeholder={`Criterion ${i + 1}`}
                  disabled={busy}
                  aria-label={`Criterion ${i + 1} text`}
                  data-testid={`backlog-criterion-text-${i}`}
                />
                <select
                  className={styles.acStatusSelect}
                  value={criterion.status}
                  onChange={(e) => updateCriterionStatus(i, e.target.value as AcCriterionStatus)}
                  disabled={busy}
                  aria-label={`Criterion ${i + 1} status`}
                  data-testid={`backlog-criterion-status-${i}`}
                >
                  {AC_STATUS_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  className={styles.removeButton}
                  onClick={() => removeCriterion(i)}
                  disabled={busy}
                  aria-label={`Remove criterion ${i + 1}`}
                  data-testid={`backlog-remove-criterion-${i}`}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Form Actions */}
      <div className={styles.formActions}>
        <button
          type="button"
          className={styles.cancelButton}
          onClick={onCancel}
          disabled={busy}
          data-testid="backlog-form-cancel"
        >
          Cancel
        </button>
        <button
          type="submit"
          className={styles.submitButton}
          disabled={busy}
          data-testid="backlog-form-submit"
        >
          {busy
            ? isCloningRepo
              ? "Cloning repository…"
              : "Saving…"
            : initialValues?.id
              ? "Save Changes"
              : "Create Item"}
        </button>
      </div>
    </form>
  );
}
