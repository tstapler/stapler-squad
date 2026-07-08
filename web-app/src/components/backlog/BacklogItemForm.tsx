"use client";
// +feature: backlog:item-form

import { useState, useCallback, useMemo, useRef } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BacklogItem, BacklogItemInput, AcCriterion, AcCriterionStatus } from "@/lib/hooks/useBacklogService";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import { isGitHubRef } from "@/lib/github/urlParser";
import { getApiBaseUrl } from "@/lib/config";
import * as styles from "./BacklogItemForm.css";
import * as markdownStyles from "./markdownBody.css";

const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024; // 10 MB — matches server-side cap

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
  const [acCriteria, setAcCriteria] = useState<AcCriterion[]>(
    initialValues?.acCriteria ?? []
  );
  const [errors, setErrors] = useState<FormErrors>({});
  const [submitting, setSubmitting] = useState(false);
  const [descriptionTab, setDescriptionTab] = useState<"write" | "preview">("write");
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

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
          acCriteria: acCriteria.map((c, i) => ({ ...c, index: i })),
          skipTriage: isVague,
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

  // Uploads a single image and inserts a markdown image reference at the cursor.
  // ponytail: cursor position is captured once at upload start, so two uploads
  // fired back-to-back can land at a stale offset relative to each other —
  // acceptable for the rare double-paste case; revisit with an insertion-token
  // queue if that turns out to matter in practice.
  const uploadAttachment = useCallback(
    async (file: File) => {
      if (!file.type.startsWith("image/")) {
        setAttachmentError("Only image files can be attached.");
        return;
      }
      if (file.size > MAX_ATTACHMENT_BYTES) {
        setAttachmentError("Image is too large (max 10 MB).");
        return;
      }
      const el = descriptionRef.current;
      const start = el?.selectionStart ?? description.length;
      const end = el?.selectionEnd ?? description.length;
      setAttachmentError(null);
      setUploading(true);
      try {
        const formData = new FormData();
        formData.append("file", file);
        const resp = await fetch(`${getApiBaseUrl()}/v1/upload-backlog-attachment`, {
          method: "POST",
          body: formData,
        });
        if (!resp.ok) {
          const msg =
            resp.status === 413
              ? "Image is too large (max 10 MB)."
              : resp.status === 415
                ? "Unsupported image type — use PNG, JPEG, GIF, or WebP."
                : "Upload failed.";
          setAttachmentError(msg);
          return;
        }
        const data = (await resp.json()) as { path: string; filename: string };
        const url = encodeURI(`/api/local/serve${data.path}`);
        const markdown = `![${data.filename}](${url})`;
        setDescription((prev) => prev.slice(0, start) + markdown + prev.slice(end));
        requestAnimationFrame(() => {
          el?.focus();
          const pos = start + markdown.length;
          el?.setSelectionRange(pos, pos);
        });
      } catch {
        setAttachmentError("Network error — upload failed.");
      } finally {
        setUploading(false);
      }
    },
    [description.length]
  );

  const handleFileInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (file) void uploadAttachment(file);
    },
    [uploadAttachment]
  );

  const handleDescriptionPaste = useCallback(
    (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const item = Array.from(e.clipboardData.items).find((i) => i.type.startsWith("image/"));
      if (!item) return;
      const file = item.getAsFile();
      if (!file) return;
      e.preventDefault();
      void uploadAttachment(file);
    },
    [uploadAttachment]
  );

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
        <div className={styles.descriptionHeader}>
          <label htmlFor="backlog-description" className={styles.label}>
            Description
          </label>
          <div className={styles.descriptionToolbar} role="tablist" aria-label="Description mode">
            <button
              type="button"
              role="tab"
              aria-selected={descriptionTab === "write"}
              className={descriptionTab === "write" ? styles.descriptionTabActive : styles.descriptionTab}
              onClick={() => setDescriptionTab("write")}
              data-testid="backlog-description-tab-write"
            >
              Write
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={descriptionTab === "preview"}
              className={descriptionTab === "preview" ? styles.descriptionTabActive : styles.descriptionTab}
              onClick={() => setDescriptionTab("preview")}
              data-testid="backlog-description-tab-preview"
            >
              Preview
            </button>
            <button
              type="button"
              className={styles.attachButton}
              onClick={() => fileInputRef.current?.click()}
              disabled={busy || uploading}
              data-testid="backlog-attach-image"
            >
              {uploading ? "Uploading…" : "📎 Attach image"}
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              className={styles.hiddenFileInput}
              onChange={handleFileInputChange}
              disabled={busy || uploading}
              aria-label="Attach image"
              data-testid="backlog-attach-image-input"
            />
          </div>
        </div>
        {descriptionTab === "write" ? (
          <textarea
            id="backlog-description"
            ref={descriptionRef}
            className={styles.textarea}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onPaste={handleDescriptionPaste}
            placeholder="Provide more context (optional). Supports markdown — paste or attach a screenshot."
            disabled={busy}
            data-testid="backlog-description-input"
          />
        ) : (
          <div className={styles.previewBox} data-testid="backlog-description-preview">
            {description.trim() ? (
              <div className={markdownStyles.markdownBody}>
                <ReactMarkdown remarkPlugins={[remarkGfm]}>{description}</ReactMarkdown>
              </div>
            ) : (
              <span className={styles.previewEmpty}>Nothing to preview yet.</span>
            )}
          </div>
        )}
        {attachmentError && (
          <span className={styles.errorMessage} role="alert" data-testid="backlog-attach-image-error">
            {attachmentError}
          </span>
        )}
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

      {/* Flags */}
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
