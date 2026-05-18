"use client";
// +feature: backlog:item-form

import { useState, useCallback } from "react";
import type { BacklogItem, BacklogItemInput, AcCriterion, AcCriterionStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./BacklogItemForm.css";

interface BacklogItemFormProps {
  initialValues?: Partial<BacklogItem>;
  onSubmit: (data: BacklogItemInput) => Promise<void>;
  onCancel: () => void;
  isLoading?: boolean;
}

interface FormErrors {
  title?: string;
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
  const [acCriteria, setAcCriteria] = useState<AcCriterion[]>(
    initialValues?.acCriteria ?? []
  );
  const [errors, setErrors] = useState<FormErrors>({});
  const [submitting, setSubmitting] = useState(false);

  const validate = useCallback((): FormErrors => {
    const errs: FormErrors = {};
    if (!title.trim()) {
      errs.title = "Title is required.";
    }
    return errs;
  }, [title]);

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
        await onSubmit({
          title: title.trim(),
          description: description.trim() || undefined,
          repoPath: repoPath.trim() || undefined,
          priority,
          skipPlanning,
          skipReviewGate,
          acCriteria: acCriteria.map((c, i) => ({ ...c, index: i })),
        });
      } finally {
        setSubmitting(false);
      }
    },
    [title, description, repoPath, priority, skipPlanning, skipReviewGate, acCriteria, onSubmit, validate]
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
            Repository Path
          </label>
          <input
            id="backlog-repo-path"
            type="text"
            className={styles.input}
            value={repoPath}
            onChange={(e) => setRepoPath(e.target.value)}
            placeholder="/home/user/project"
            disabled={busy}
            data-testid="backlog-repo-path-input"
          />
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

      {/* Flags */}
      <div className={styles.twoColumn}>
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
      </div>

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
          {busy ? "Saving…" : initialValues?.id ? "Save Changes" : "Create Item"}
        </button>
      </div>
    </form>
  );
}
