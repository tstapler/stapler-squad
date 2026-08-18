"use client";

// +feature: trigger-form-modal

import { useEffect, useState } from "react";
import { Modal, ModalContent, ModalTitle, ModalClose } from "@/components/ui/Modal";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { WorkflowFormData } from "@/lib/hooks/useWorkflows";
import {
  ruleModalContent, modalHeader, modalTitleRow, modalBody, modalCloseButton,
  formSection, formSectionHeader, formGrid, label as labelClass, input as inputClass,
  formActions, saveButton, cancelButton, formError,
} from "./ApprovalRulesPanel.css";
import {
  typeSelector, typeOption, typeOptionActive, fieldset, legend, fieldError, fieldHint,
  secretBox, secretRow, secretValue, secretMasked, secretButton, secretWarning, secretCopiedNotice,
  secretCopyErrorNotice, visuallyHidden, checkboxLabel, promptTextarea, formActionsSpaced, secretBoxLegend,
} from "./TriggerFormModal.css";

export type TriggerType = "cron" | "github_push" | "webhook";

const TRIGGER_TYPES: { value: TriggerType; label: string }[] = [
  { value: "github_push", label: "GitHub Push" },
  { value: "cron", label: "Schedule (cron)" },
  { value: "webhook", label: "Webhook" },
];

interface TriggerFormModalProps {
  open: boolean;
  /** Existing Workflow row when editing; undefined/null when creating. */
  editTrigger?: WorkflowProto | null;
  onSave: (data: WorkflowFormData) => Promise<void>;
  onClose: () => void;
}

interface FieldErrors {
  [field: string]: string | undefined;
}

const EMPTY: WorkflowFormData = {
  slug: "",
  name: "",
  description: "",
  command: "",
  targetDirectory: "",
  cronEnabled: true,
  enabled: true,
  triggerType: "github_push",
  githubRepo: "",
  githubBranch: "",
  cronExpression: "",
  webhookSlug: "",
  eventFilter: "",
  labelFilter: "",
  promptTemplate: "",
};

function protoToFormData(w: WorkflowProto): WorkflowFormData {
  return {
    slug: w.slug,
    name: w.name,
    description: w.description,
    command: w.command,
    targetDirectory: w.targetDirectory,
    cronEnabled: w.cronEnabled,
    enabled: w.enabled,
    triggerType: (w.triggerType || "github_push") as TriggerType,
    githubRepo: w.githubRepo,
    githubBranch: w.githubBranch,
    cronExpression: w.cronExpression,
    webhookSlug: w.webhookSlug,
    eventFilter: w.eventFilter,
    labelFilter: w.labelFilter,
    promptTemplate: w.promptTemplate,
    expectedUpdatedAt: w.updatedAt,
  };
}

/**
 * Maps a backend connect.CodeInvalidArgument rawMessage (see
 * server/services/workflow_service.go's errors.New/fmt.Errorf call sites) to the
 * specific form field it concerns, so the error renders inline near that field
 * instead of only as a generic top-of-form banner (Task 7.1.2c).
 */
function mapErrorToField(message: string): string | null {
  const m = message.toLowerCase();
  if (m.includes("slug")) return "slug";
  if (m.includes("name is required")) return "name";
  if (m.includes("target_directory")) return "targetDirectory";
  if (m.includes("command is required") || m.includes("command required")) return "command";
  if (m.includes("cron_expression") || m.includes("cron expression")) return "cronExpression";
  if (m.includes("cron_enabled")) return "triggerType";
  if (m.includes("webhook_slug")) return "webhookSlug";
  if (m.includes("github_repo")) return "githubRepo";
  if (m.includes("prompt_template")) return "promptTemplate";
  return null;
}

const COMMON_FIELDS = new Set(["slug", "name", "targetDirectory", "command", "triggerType"]);
const TYPE_SPECIFIC_FIELDS: Record<TriggerType, string[]> = {
  github_push: ["githubRepo", "githubBranch", "promptTemplate"],
  cron: ["cronExpression"],
  webhook: ["webhookSlug", "eventFilter", "labelFilter", "promptTemplate"],
};

/** Whether `field`'s input is actually rendered for the currently selected trigger type. */
function isFieldVisible(field: string, triggerType: TriggerType): boolean {
  return COMMON_FIELDS.has(field) || TYPE_SPECIFIC_FIELDS[triggerType].includes(field);
}

/**
 * Generates a cryptographically-random 32-byte hex secret via the Web Crypto API.
 * Returns null when `crypto.getRandomValues` isn't available — this is unreachable
 * in any evergreen browser, but a security-critical secret must never silently fall
 * back to `Math.random()` (not cryptographically secure) when it isn't. Callers
 * must disable secret generation and surface a message instead of calling this in
 * a loop or otherwise papering over a null result.
 */
function generateClientSecret(): string | null {
  if (typeof crypto === "undefined" || typeof crypto.getRandomValues !== "function") {
    return null;
  }
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function TriggerFormModal({ open, editTrigger, onSave, onClose }: TriggerFormModalProps) {
  const isEdit = !!editTrigger;
  const [formData, setFormData] = useState<WorkflowFormData>(EMPTY);
  const [saving, setSaving] = useState(false);
  const [formErrorMsg, setFormErrorMsg] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [liveMessage, setLiveMessage] = useState("");

  // Secret UI state. generatedSecret holds the plaintext value shown once (create) or
  // during an in-progress rotation (edit) — it is sent to the backend via
  // WorkflowFormData.webhookSecret on submit and never re-read from the server (see
  // SecretField's doc comment for the never-round-trip-a-real-secret convention).
  // rotatingSecret gates edit mode between the masked "unchanged" placeholder and the
  // generate/copy UI — editing a trigger defaults to leaving the stored secret alone.
  const [generatedSecret, setGeneratedSecret] = useState<string | null>(null);
  const [secretCopied, setSecretCopied] = useState(false);
  const [secretCopyError, setSecretCopyError] = useState(false);
  const [secretGenerationUnsupported, setSecretGenerationUnsupported] = useState(false);
  const [rotatingSecret, setRotatingSecret] = useState(false);

  useEffect(() => {
    if (!open) return;
    setFormData(editTrigger ? protoToFormData(editTrigger) : EMPTY);
    setFormErrorMsg(null);
    setFieldErrors({});
    setGeneratedSecret(null);
    setSecretCopied(false);
    setSecretCopyError(false);
    setSecretGenerationUnsupported(false);
    setRotatingSecret(false);
  }, [open, editTrigger]);

  function handleGenerateSecret() {
    const secret = generateClientSecret();
    if (secret === null) {
      setSecretGenerationUnsupported(true);
      return;
    }
    setGeneratedSecret(secret);
    setSecretCopied(false);
    setSecretCopyError(false);
    setFieldErrors((prev) => ({ ...prev, secret: undefined }));
  }

  async function handleCopySecret(value: string) {
    try {
      if (!navigator.clipboard?.writeText) {
        throw new Error("Clipboard API unavailable");
      }
      await navigator.clipboard.writeText(value);
      setSecretCopied(true);
      setSecretCopyError(false);
    } catch {
      // Clipboard write failed (denied permission, insecure context, unfocused
      // window, etc.) — never report success for a value shown exactly once.
      setSecretCopied(false);
      setSecretCopyError(true);
    }
  }

  function setField<K extends keyof WorkflowFormData>(key: K, value: WorkflowFormData[K]) {
    setFormData((prev) => ({ ...prev, [key]: value }));
    setFieldErrors((prev) => ({ ...prev, [key]: undefined }));
  }

  function handleTypeChange(next: TriggerType) {
    // Clear the other types' fields client-side on switch (Task 7.1.2a) — closes the
    // same footgun the backend's Task 1.1.1e validation closes server-side (a row that
    // registers as more than one trigger mechanism at once).
    setFormData((prev) => ({
      ...prev,
      triggerType: next,
      githubRepo: next === "github_push" ? prev.githubRepo : "",
      githubBranch: next === "github_push" ? prev.githubBranch : "",
      cronExpression: next === "cron" ? prev.cronExpression : "",
      webhookSlug: next === "webhook" ? prev.webhookSlug : "",
      eventFilter: next === "webhook" ? prev.eventFilter : "",
      labelFilter: next === "webhook" ? prev.labelFilter : "",
      promptTemplate: next === "cron" ? "" : prev.promptTemplate,
    }));
    setFieldErrors({});
  }

  function validateClientSide(): FieldErrors {
    const errs: FieldErrors = {};
    if (!formData.slug.trim()) errs.slug = "Slug is required.";
    if (!formData.name.trim()) errs.name = "Name is required.";
    if (!formData.command.trim()) errs.command = "Command / prompt is required.";
    if (!formData.targetDirectory.trim()) errs.targetDirectory = "Target directory is required.";
    if (formData.triggerType === "cron" && !formData.cronExpression?.trim()) {
      errs.cronExpression = "Cron expression is required for a schedule trigger.";
    }
    if (formData.triggerType === "github_push" && !formData.githubRepo?.trim()) {
      errs.githubRepo = "Repository (owner/repo) is required.";
    }
    if (formData.triggerType === "webhook" && !formData.webhookSlug?.trim()) {
      errs.webhookSlug = "Webhook slug is required.";
    }
    // Rotate mode entered but no new secret generated yet: webhookSecret would end
    // up "" on submit, which the wire contract treats as "no change" — safe, but
    // silently not what the user asked for by clicking "Rotate secret".
    if (
      isEdit &&
      rotatingSecret &&
      !generatedSecret &&
      (formData.triggerType === "webhook" || formData.triggerType === "github_push")
    ) {
      errs.secret = "Generate a secret before saving.";
    }
    return errs;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const clientErrors = validateClientSide();
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors);
      setFormErrorMsg("Fix the highlighted fields and try again.");
      return;
    }
    setSaving(true);
    setFormErrorMsg(null);
    setFieldErrors({});
    try {
      // Only send a secret when the user actually generated/rotated one this session
      // — create-mode with no secret generated, or edit-mode without an explicit
      // rotation, both send "" (no change), matching the "omit means unchanged"
      // contract on UpdateWorkflowRequest.webhook_secret.
      const shouldSendSecret = !isEdit || rotatingSecret;
      await onSave({
        ...formData,
        // A cron trigger has no way to be "disabled" through this form (the checkbox
        // is disabled+forced for triggerType==="cron" below) — both fields are forced
        // true so the row both registers as a cron entry (cronEnabled) and passes the
        // webhook-handler admission gate if it's ever repurposed (enabled).
        cronEnabled: formData.triggerType === "cron" ? true : formData.cronEnabled,
        enabled: formData.triggerType === "cron" ? true : formData.enabled,
        webhookSecret: shouldSendSecret ? (generatedSecret ?? "") : "",
      });
      setLiveMessage(isEdit ? "Trigger updated." : "Trigger created.");
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to save trigger.";
      const field = mapErrorToField(message);
      const currentType = (formData.triggerType || "github_push") as TriggerType;
      if (field && isFieldVisible(field, currentType)) {
        setFieldErrors({ [field]: message });
        setFormErrorMsg(null);
      } else {
        // Field mapped but its input isn't rendered for the current type (or no field
        // could be determined) — fall back to the generic banner rather than an error
        // that would silently attach to nothing on screen.
        setFormErrorMsg(message);
      }
      setLiveMessage(`Trigger save failed: ${message}`);
    } finally {
      setSaving(false);
    }
  }

  const triggerType = (formData.triggerType || "github_push") as TriggerType;

  return (
    <Modal open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <ModalContent className={ruleModalContent}>
        <div className={modalHeader}>
          <div className={modalTitleRow}>
            <ModalTitle>{isEdit ? `Edit trigger: ${editTrigger?.name}` : "Add Trigger"}</ModalTitle>
          </div>
          <ModalClose className={modalCloseButton} aria-label="Close dialog">×</ModalClose>
        </div>
        <div className={modalBody}>
          <span aria-live="polite" aria-atomic="true" className={visuallyHidden}>
            {liveMessage}
          </span>

          {formErrorMsg && <div className={formError} role="alert">{formErrorMsg}</div>}

          <form onSubmit={handleSubmit} data-testid="trigger-form">
            {/* ── Trigger type selector ── */}
            <div className={formSection}>
              <p className={formSectionHeader}>Trigger type</p>
              <div className={typeSelector} role="radiogroup" aria-label="Trigger type">
                {TRIGGER_TYPES.map((t) => (
                  <button
                    key={t.value}
                    type="button"
                    role="radio"
                    aria-checked={triggerType === t.value}
                    className={`${typeOption} ${triggerType === t.value ? typeOptionActive : ""}`}
                    onClick={() => handleTypeChange(t.value)}
                    disabled={isEdit}
                    data-testid={`trigger-type-${t.value}`}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
              {isEdit && <p className={fieldHint}>Trigger type can&apos;t be changed after creation.</p>}
            </div>

            {/* ── Identity fields ── */}
            <div className={formSection}>
              <p className={formSectionHeader}>Details</p>
              <div className={formGrid}>
                <label className={labelClass}>
                  Slug *
                  <input
                    className={inputClass}
                    data-testid="trigger-slug-input"
                    value={formData.slug}
                    onChange={(e) => setField("slug", e.target.value)}
                    disabled={isEdit}
                    placeholder="jira-ticket"
                  />
                  {fieldErrors.slug && <span className={fieldError}>{fieldErrors.slug}</span>}
                </label>
                <label className={labelClass}>
                  Name *
                  <input
                    className={inputClass}
                    data-testid="trigger-name-input"
                    value={formData.name}
                    onChange={(e) => setField("name", e.target.value)}
                    placeholder="Triage new Jira tickets"
                  />
                  {fieldErrors.name && <span className={fieldError}>{fieldErrors.name}</span>}
                </label>
                <label className={labelClass}>
                  Target directory *
                  <input
                    className={inputClass}
                    data-testid="trigger-target-directory-input"
                    value={formData.targetDirectory}
                    onChange={(e) => setField("targetDirectory", e.target.value)}
                    placeholder="/home/user/projects/repo"
                  />
                  {fieldErrors.targetDirectory && <span className={fieldError}>{fieldErrors.targetDirectory}</span>}
                </label>
                <label className={labelClass}>
                  Command / prompt *
                  <input
                    className={inputClass}
                    data-testid="trigger-command-input"
                    value={formData.command}
                    onChange={(e) => setField("command", e.target.value)}
                    placeholder="Investigate and fix the reported issue"
                  />
                  {fieldErrors.command && <span className={fieldError}>{fieldErrors.command}</span>}
                </label>
              </div>
              <label className={checkboxLabel}>
                <input
                  type="checkbox"
                  checked={triggerType === "cron" ? true : (formData.enabled ?? true)}
                  onChange={(e) => setField("enabled", e.target.checked)}
                  disabled={triggerType === "cron"}
                  data-testid="trigger-enabled-checkbox"
                />
                {triggerType === "cron" ? "Enabled (controlled by the schedule below)" : "Enabled"}
              </label>
            </div>

            {/* ── Type-specific field groups ── */}
            {triggerType === "github_push" && (
              <fieldset className={fieldset} aria-labelledby="tf-github-legend">
                <legend id="tf-github-legend" className={legend}>GitHub push</legend>
                <div className={formGrid}>
                  <label className={labelClass}>
                    Repository *
                    <input
                      className={inputClass}
                      data-testid="trigger-github-repo-input"
                      value={formData.githubRepo ?? ""}
                      onChange={(e) => setField("githubRepo", e.target.value)}
                      placeholder="owner/repo"
                    />
                    {fieldErrors.githubRepo && <span className={fieldError}>{fieldErrors.githubRepo}</span>}
                  </label>
                  <label className={labelClass}>
                    Branch
                    <input
                      className={inputClass}
                      data-testid="trigger-github-branch-input"
                      value={formData.githubBranch ?? ""}
                      onChange={(e) => setField("githubBranch", e.target.value)}
                      placeholder="main"
                    />
                  </label>
                </div>
                <label className={labelClass}>
                  Prompt template
                  <textarea
                    className={`${inputClass} ${promptTextarea}`}
                    data-testid="trigger-prompt-template-input"
                    rows={3}
                    value={formData.promptTemplate ?? ""}
                    onChange={(e) => setField("promptTemplate", e.target.value)}
                    placeholder={"Review {{.head_commit.message}}"}
                  />
                  <span className={fieldHint}>Go text/template, rendered against the inbound push payload.</span>
                  {fieldErrors.promptTemplate && <span className={fieldError}>{fieldErrors.promptTemplate}</span>}
                </label>
                <SecretField
                  triggerType="github_push"
                  isEdit={isEdit}
                  rotating={rotatingSecret}
                  generatedSecret={generatedSecret}
                  secretCopied={secretCopied}
                  secretCopyError={secretCopyError}
                  generationUnsupported={secretGenerationUnsupported}
                  error={fieldErrors.secret}
                  onStartRotate={() => setRotatingSecret(true)}
                  onGenerate={handleGenerateSecret}
                  onCopy={(v) => { void handleCopySecret(v); }}
                />
              </fieldset>
            )}

            {triggerType === "cron" && (
              <fieldset className={fieldset} aria-labelledby="tf-cron-legend">
                <legend id="tf-cron-legend" className={legend}>Schedule</legend>
                <label className={labelClass}>
                  Cron expression *
                  <input
                    className={inputClass}
                    data-testid="trigger-cron-expression-input"
                    value={formData.cronExpression ?? ""}
                    onChange={(e) => setField("cronExpression", e.target.value)}
                    placeholder="0 9 * * *"
                  />
                  <span className={fieldHint}>Standard 5-field cron syntax (minute hour dom month dow).</span>
                  {fieldErrors.cronExpression && <span className={fieldError}>{fieldErrors.cronExpression}</span>}
                </label>
              </fieldset>
            )}

            {triggerType === "webhook" && (
              <fieldset className={fieldset} aria-labelledby="tf-webhook-legend">
                <legend id="tf-webhook-legend" className={legend}>Webhook</legend>
                <div className={formGrid}>
                  <label className={labelClass}>
                    Slug *
                    <input
                      className={inputClass}
                      data-testid="trigger-webhook-slug-input"
                      value={formData.webhookSlug ?? ""}
                      onChange={(e) => setField("webhookSlug", e.target.value)}
                      placeholder="jira-ticket"
                      disabled={isEdit}
                    />
                    <span className={fieldHint}>Routes POST /webhooks/{"{slug}"} to this trigger.</span>
                    {fieldErrors.webhookSlug && <span className={fieldError}>{fieldErrors.webhookSlug}</span>}
                  </label>
                  <label className={labelClass}>
                    Event filter
                    <input
                      className={inputClass}
                      data-testid="trigger-event-filter-input"
                      value={formData.eventFilter ?? ""}
                      onChange={(e) => setField("eventFilter", e.target.value)}
                      placeholder="issue_created"
                    />
                  </label>
                  <label className={labelClass}>
                    Label filter
                    <input
                      className={inputClass}
                      data-testid="trigger-label-filter-input"
                      value={formData.labelFilter ?? ""}
                      onChange={(e) => setField("labelFilter", e.target.value)}
                      placeholder="urgent"
                    />
                  </label>
                </div>
                <label className={labelClass}>
                  Prompt template
                  <textarea
                    className={`${inputClass} ${promptTextarea}`}
                    data-testid="trigger-prompt-template-input"
                    rows={3}
                    value={formData.promptTemplate ?? ""}
                    onChange={(e) => setField("promptTemplate", e.target.value)}
                    placeholder={"Triage {{.issue.key}}: {{.issue.summary}}"}
                  />
                  <span className={fieldHint}>Go text/template, rendered against the inbound JSON payload.</span>
                  {fieldErrors.promptTemplate && <span className={fieldError}>{fieldErrors.promptTemplate}</span>}
                </label>
                <SecretField
                  triggerType="webhook"
                  isEdit={isEdit}
                  rotating={rotatingSecret}
                  generatedSecret={generatedSecret}
                  secretCopied={secretCopied}
                  secretCopyError={secretCopyError}
                  generationUnsupported={secretGenerationUnsupported}
                  error={fieldErrors.secret}
                  onStartRotate={() => setRotatingSecret(true)}
                  onGenerate={handleGenerateSecret}
                  onCopy={(v) => { void handleCopySecret(v); }}
                />
              </fieldset>
            )}

            <div className={`${formActions} ${formActionsSpaced}`}>
              <button type="submit" className={saveButton} disabled={saving} data-testid="trigger-form-submit">
                {saving ? "Saving…" : isEdit ? "Save Changes" : "Create Trigger"}
              </button>
              <button type="button" className={cancelButton} onClick={onClose} disabled={saving}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      </ModalContent>
    </Modal>
  );
}

interface SecretFieldProps {
  triggerType: "github_push" | "webhook";
  isEdit: boolean;
  /** True once the user has clicked "Rotate secret" in edit mode. */
  rotating: boolean;
  generatedSecret: string | null;
  secretCopied: boolean;
  /** True when the last clipboard write attempt genuinely failed. */
  secretCopyError: boolean;
  /** True when `crypto.getRandomValues` isn't available — disables generation. */
  generationUnsupported: boolean;
  /** Client-side validation error for this field group (e.g. rotate-with-no-secret). */
  error?: string;
  onStartRotate: () => void;
  onGenerate: () => void;
  onCopy: (value: string) => void;
}

/**
 * Webhook/GitHub secret UI (Task 7.1.2b, wired to the backend in the Phase 7 follow-up
 * — Task 7.2). Show-once + copy-to-clipboard when generating a new secret (create, or
 * an explicit rotation during edit), masked placeholder otherwise — matching the
 * never-round-trip-a-real-secret convention CallbackSettings uses for its callback
 * URLs. The generated/pasted value is sent to the backend via
 * WorkflowFormData.webhookSecret on submit (see TriggerFormModal.handleSubmit) and is
 * never re-read from the server afterward.
 */
function SecretField({
  isEdit, rotating, generatedSecret, secretCopied, secretCopyError, generationUnsupported, error,
  onStartRotate, onGenerate, onCopy,
}: SecretFieldProps) {
  const showGenerateUI = !isEdit || rotating;
  return (
    <div className={secretBox}>
      <span className={`${legend} ${secretBoxLegend}`}>Shared secret</span>
      {!showGenerateUI ? (
        <div className={secretRow}>
          <span className={secretMasked} data-testid="trigger-secret-masked">•••• (unchanged)</span>
          <button type="button" className={secretButton} onClick={onStartRotate} data-testid="trigger-secret-rotate">
            Rotate secret
          </button>
        </div>
      ) : (
        <div className={secretRow}>
          {generatedSecret ? (
            // Selectable + focusable so a failed clipboard write still has a manual
            // fallback — this value is shown exactly once and never re-fetchable.
            <span className={secretValue} data-testid="trigger-secret-value" tabIndex={0}>
              {generatedSecret}
            </span>
          ) : (
            <span className={fieldHint}>
              {generationUnsupported
                ? "Secret generation requires a modern browser with the Web Crypto API."
                : "Generate a secret to verify inbound requests (HMAC)."}
            </span>
          )}
          <button
            type="button"
            className={secretButton}
            onClick={onGenerate}
            disabled={generationUnsupported}
            data-testid="trigger-secret-generate"
          >
            {generatedSecret ? "Regenerate" : "Generate secret"}
          </button>
          {generatedSecret && (
            <button type="button" className={secretButton} onClick={() => onCopy(generatedSecret)} data-testid="trigger-secret-copy">
              {secretCopied ? "Copied ✓" : "Copy"}
            </button>
          )}
        </div>
      )}
      {showGenerateUI && !generationUnsupported && (
        <span className={secretWarning}>
          Copy this now — it won&apos;t be shown again after you save.
        </span>
      )}
      {secretCopied && <span className={secretCopiedNotice}>Copied to clipboard.</span>}
      {secretCopyError && (
        <span className={secretCopyErrorNotice} role="alert" data-testid="trigger-secret-copy-error">
          Copy failed — select the text above to copy manually.
        </span>
      )}
      {error && <span className={fieldError} data-testid="trigger-secret-field-error">{error}</span>}
    </div>
  );
}
