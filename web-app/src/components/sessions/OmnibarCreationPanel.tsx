"use client";
// +feature: session-image-attach

import { useState, useRef, useCallback, useEffect } from "react";
import type { KeyboardEvent } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";
import type { OmnibarFormState } from "./Omnibar";
import { PROGRAMS } from "@/lib/constants/programs";
import { getApiBaseUrl } from "@/lib/config";
import {
  body, field, label as labelClass, fieldInput, hint, select as selectClass,
  checkbox as checkboxClass, collapsible, collapsibleHeader, collapsibleTitle, collapsibleIcon, expanded,
  collapsibleContent, footer, button as buttonClass, buttonSecondary, buttonPrimary,
  error as errorClass,
} from "./Omnibar.css";
import * as styles from "./OmnibarCreationPanel.css";

// ─── Session Type Radio Group ────────────────────────────────────────────────

const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory", label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
  { value: "one_off", label: "One-off" },
  { value: "new_project", label: "New Project" },
] as const;

type SessionTypeValue = (typeof SESSION_TYPES)[number]["value"];

// Radio options for the "Open as" sub-selector inside New Project mode.
const NEW_PROJECT_OPEN_AS = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory", label: "Directory" },
] as const;

interface SessionTypeRadioGroupProps {
  value: SessionTypeValue;
  onChange: (v: SessionTypeValue) => void;
}

function SessionTypeRadioGroup({ value, onChange }: SessionTypeRadioGroupProps) {
  const currentIndex = SESSION_TYPES.findIndex((t) => t.value === value);

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      const next = (currentIndex + 1) % SESSION_TYPES.length;
      onChange(SESSION_TYPES[next].value);
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      const prev = (currentIndex - 1 + SESSION_TYPES.length) % SESSION_TYPES.length;
      onChange(SESSION_TYPES[prev].value);
    }
  }

  return (
    <div
      role="radiogroup"
      aria-label="Session type"
      className={styles.radioGroup}
      onKeyDown={handleKeyDown}
    >
      {SESSION_TYPES.map((type) => (
        <button
          key={type.value}
          role="radio"
          aria-checked={value === type.value}
          tabIndex={value === type.value ? 0 : -1}
          type="button"
          onClick={() => onChange(type.value)}
          className={[styles.radioBtn, value === type.value ? styles.radioBtnActive : ""]
            .filter(Boolean)
            .join(" ")}
        >
          {type.label}
        </button>
      ))}
    </div>
  );
}

// ─── OmnibarCreationPanel ────────────────────────────────────────────────────

export interface OmnibarCreationPanelProps {
  formState: OmnibarFormState;
  setFormField: <K extends keyof OmnibarFormState>(key: K, value: OmnibarFormState[K]) => void;
  onSubmit: () => void;
  onCancel: () => void;
  worktrees: WorktreeEntry[];
  isWorktreesLoading?: boolean;
  isSubmitting: boolean;
  canSubmit: boolean;
  error: string | null;
  showAdvanced: boolean;
  onToggleAdvanced: () => void;
  /** Pre-selected repo path (creation_with_repo mode). Shown read-only above form. */
  path?: string;
  /** API base URL (e.g. /api) used for pre-session image uploads. */
  uploadBaseUrl?: string;
  /** Called whenever the set of attached image server paths changes. */
  onAttachedImagesChange?: (paths: string[]) => void;
}

function truncatePath(p: string, maxLen = 50): string {
  if (p.length <= maxLen) return p;
  return "…" + p.slice(-(maxLen - 1));
}

// Helper: file → base64 string (strips data URL prefix).
function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve((reader.result as string).split(",")[1]);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

interface AttachedImage {
  file: File;
  path: string;       // absolute server path returned from upload
  previewUrl: string; // object URL for thumbnail preview
}

export function OmnibarCreationPanel({
  formState,
  setFormField,
  onSubmit,
  onCancel,
  worktrees,
  isWorktreesLoading = false,
  isSubmitting,
  canSubmit,
  error,
  showAdvanced,
  onToggleAdvanced,
  path,
  uploadBaseUrl = "/api",
  onAttachedImagesChange,
}: OmnibarCreationPanelProps) {
  const {
    sessionName, branch, program, category, autoYes,
    useTitleAsBranch, sessionType, existingWorktree, workingDir,
    parentDir, projectName, newProjectSessionType,
  } = formState;

  // ─── Load default parentDir from config when new_project mode is first selected ──
  useEffect(() => {
    if (sessionType !== "new_project" || parentDir) return;
    const load = async () => {
      try {
        const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
        const client = createClient(SessionService, transport);
        const resp = await client.getSessionDefaults({});
        const dir = resp.defaults?.newProjectBaseDir;
        if (dir && !parentDir) {
          setFormField("parentDir", dir);
        }
      } catch {
        // Non-critical: falls back to empty; user can type manually
      }
    };
    void load();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionType]);

  // ─── Image attachment state ───────────────────────────────────────────────
  const [attachedImages, setAttachedImages] = useState<AttachedImage[]>([]);
  // Mirror of attachedImages kept in a ref so the unmount cleanup can revoke
  // object URLs without capturing stale closure values.
  const attachedImagesRef = useRef<AttachedImage[]>([]);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [isAttaching, setIsAttaching] = useState(false);
  const attachInputRef = useRef<HTMLInputElement>(null);

  // Keep ref in sync with state so the unmount cleanup always sees current images.
  useEffect(() => {
    attachedImagesRef.current = attachedImages;
    onAttachedImagesChange?.(attachedImages.map((img) => img.path));
  }, [attachedImages, onAttachedImagesChange]);

  // Revoke object URLs on unmount via ref — avoids stale closure over empty array.
  useEffect(() => {
    return () => {
      attachedImagesRef.current.forEach((img) => URL.revokeObjectURL(img.previewUrl));
    };
  }, []);

  const handleAttachFiles = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    e.target.value = "";
    if (!files.length) return;

    const available = 3 - attachedImages.length;
    const toUpload = files.slice(0, available);

    setIsAttaching(true);
    setAttachError(null);

    const results: AttachedImage[] = [];
    for (const file of toUpload) {
      const previewUrl = URL.createObjectURL(file);
      try {
        const base64 = await fileToBase64(file);
        const resp = await fetch(`${uploadBaseUrl}/upload/image`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ data: base64, contentType: file.type }),
        });
        if (!resp.ok) {
          URL.revokeObjectURL(previewUrl);
          setAttachError("Upload failed");
          break;
        }
        const data = await resp.json() as { path: string };
        results.push({ file, path: data.path, previewUrl });
      } catch {
        URL.revokeObjectURL(previewUrl);
        setAttachError("Upload failed");
        break;
      }
    }

    setAttachedImages((prev) => [...prev, ...results]);
    setIsAttaching(false);
  }, [attachedImages.length, uploadBaseUrl]);

  const removeImage = useCallback((index: number) => {
    setAttachedImages((prev) => {
      URL.revokeObjectURL(prev[index].previewUrl);
      return prev.filter((_, i) => i !== index);
    });
  }, []);

  return (
    <>
      {/* Pre-selected repo path (creation_with_repo mode) */}
      {path && (
        <div className={styles.pathDisplay} title={path}>
          {truncatePath(path)}
        </div>
      )}

      <div className={body}>
        {/* Session Name */}
        <div className={field}>
          <label className={labelClass} htmlFor="omnibar-name">
            Session Name *
          </label>
          <input
            id="omnibar-name"
            type="text"
            className={fieldInput}
            placeholder="my-feature-session"
            value={sessionName}
            onChange={(e) => setFormField("sessionName", e.target.value)}
          />
        </div>

        {/* Session Type — ARIA radio group (ADR-003: arrow keys cycle) */}
        <div className={field}>
          <label className={labelClass} id="omnibar-session-type-label">
            Session Type
          </label>
          <SessionTypeRadioGroup
            value={sessionType}
            onChange={(v) => setFormField("sessionType", v)}
          />
          <span className={hint}>
            {sessionType === "new_worktree" && "Creates an isolated git worktree for this session"}
            {sessionType === "existing_worktree" && "Uses an existing worktree at a specific path"}
            {sessionType === "directory" && "Works directly in the repository without worktree isolation"}
            {sessionType === "one_off" && "A fresh directory will be created automatically — no path needed"}
            {sessionType === "new_project" && "Creates a new directory, runs git init, makes an initial commit, then opens a session"}
          </span>
        </div>

        {/* One-off informational banner */}
        {sessionType === "one_off" && (
          <div className={hint} style={{ marginTop: 0 }}>
            Directory will be created in your one-off base directory (default: <code>~/oneoff</code>) with format <code>YYYYMMDD-word-word-NN</code>. Configure in Settings → Defaults.
          </div>
        )}

        {/* New Project mode UI */}
        {sessionType === "new_project" && (
          <>
            {/* Parent Directory */}
            <div className={field}>
              <label className={labelClass} htmlFor="omnibar-parent-dir">
                Parent Directory *
              </label>
              <input
                id="omnibar-parent-dir"
                type="text"
                className={fieldInput}
                placeholder="~/Projects"
                value={parentDir}
                onChange={(e) => setFormField("parentDir", e.target.value)}
              />
              <span className={hint}>Directory where the new project folder will be created</span>
            </div>

            {/* Project Name */}
            <div className={field}>
              <label className={labelClass} htmlFor="omnibar-project-name">
                Project Name *
              </label>
              <input
                id="omnibar-project-name"
                type="text"
                className={fieldInput}
                placeholder="my-awesome-project"
                value={projectName}
                onChange={(e) => setFormField("projectName", e.target.value)}
              />
              <span className={hint}>Name of the new project directory (no path separators)</span>
            </div>

            {/* Resolved Path Preview */}
            {parentDir.trim() && projectName.trim() && (
              <div className={styles.pathDisplay} title={`${parentDir.trim().replace(/\/$/, "")}/${projectName.trim()}`}>
                {parentDir.trim().replace(/\/$/, "")}/{projectName.trim()}
              </div>
            )}

            {/* Open as radio group */}
            <div className={field}>
              <label className={labelClass} id="omnibar-open-as-label">
                Open as
              </label>
              <div role="radiogroup" aria-labelledby="omnibar-open-as-label" className={styles.radioGroup}>
                {NEW_PROJECT_OPEN_AS.map((opt) => (
                  <button
                    key={opt.value}
                    role="radio"
                    aria-checked={newProjectSessionType === opt.value}
                    tabIndex={newProjectSessionType === opt.value ? 0 : -1}
                    type="button"
                    onClick={() => setFormField("newProjectSessionType", opt.value)}
                    className={[styles.radioBtn, newProjectSessionType === opt.value ? styles.radioBtnActive : ""]
                      .filter(Boolean)
                      .join(" ")}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
              <span className={hint}>
                {newProjectSessionType === "new_worktree"
                  ? "Creates an isolated git worktree for this session"
                  : "Opens the project directory directly without worktree isolation"}
              </span>
            </div>

            {/* Branch field for new_worktree open-as */}
            {newProjectSessionType === "new_worktree" && (
              <>
                <label className={checkboxClass}>
                  <input
                    type="checkbox"
                    checked={useTitleAsBranch}
                    onChange={(e) => setFormField("useTitleAsBranch", e.target.checked)}
                  />
                  <span>Use session name as branch name</span>
                </label>

                <div className={field}>
                  <label className={labelClass} htmlFor="omnibar-np-branch">
                    Git Branch {!useTitleAsBranch && "*"}
                  </label>
                  <input
                    id="omnibar-np-branch"
                    type="text"
                    className={fieldInput}
                    placeholder={useTitleAsBranch ? sessionName || "Enter session name first" : "main"}
                    value={useTitleAsBranch ? sessionName : branch}
                    onChange={(e) => !useTitleAsBranch && setFormField("branch", e.target.value)}
                    disabled={useTitleAsBranch}
                    style={{ opacity: useTitleAsBranch ? 0.6 : 1 }}
                  />
                  <span className={hint}>
                    {useTitleAsBranch
                      ? `Branch name will be: ${sessionName || "(enter session name)"}`
                      : "Branch to create for the new worktree"}
                  </span>
                </div>
              </>
            )}
          </>
        )}

        {/* Branch controls (for new worktree) */}
        {sessionType === "new_worktree" && (
          <>
            <label className={checkboxClass}>
              <input
                type="checkbox"
                checked={useTitleAsBranch}
                onChange={(e) => setFormField("useTitleAsBranch", e.target.checked)}
              />
              <span>Use session name as branch name</span>
            </label>

            <div className={field}>
              <label className={labelClass} htmlFor="omnibar-branch">
                Git Branch {!useTitleAsBranch && "*"}
              </label>
              <input
                id="omnibar-branch"
                type="text"
                className={fieldInput}
                placeholder={useTitleAsBranch ? sessionName || "Enter session name first" : "feature/my-feature"}
                value={useTitleAsBranch ? sessionName : branch}
                onChange={(e) => !useTitleAsBranch && setFormField("branch", e.target.value)}
                disabled={useTitleAsBranch}
                style={{ opacity: useTitleAsBranch ? 0.6 : 1 }}
              />
              <span className={hint}>
                {useTitleAsBranch
                  ? `Branch name will be: ${sessionName || "(enter session name)"}`
                  : "Branch to create for the new worktree"}
              </span>
            </div>
          </>
        )}

        {/* Existing worktree path */}
        {sessionType === "existing_worktree" && (
          <div className={field}>
            <label className={labelClass} htmlFor="omnibar-existing-worktree">
              Existing Worktree Path *
            </label>
            {isWorktreesLoading ? (
              <select id="omnibar-existing-worktree" className={selectClass} disabled>
                <option>Loading worktrees…</option>
              </select>
            ) : worktrees.length > 0 ? (
              <select
                id="omnibar-existing-worktree"
                className={selectClass}
                value={existingWorktree}
                onChange={(e) => setFormField("existingWorktree", e.target.value)}
              >
                <option value="">Select a worktree…</option>
                {worktrees.map((wt) => (
                  <option key={wt.path} value={wt.path}>
                    {wt.branch ? `${wt.branch} (${wt.path})` : wt.path}
                  </option>
                ))}
              </select>
            ) : (
              <input
                id="omnibar-existing-worktree"
                type="text"
                className={fieldInput}
                placeholder="/path/to/existing/worktree"
                value={existingWorktree}
                onChange={(e) => setFormField("existingWorktree", e.target.value)}
              />
            )}
            <span className={hint}>
              {isWorktreesLoading
                ? "Scanning for git worktrees…"
                : worktrees.length > 0
                ? "Select an existing git worktree for this repository"
                : "Absolute path to an existing git worktree"}
            </span>
          </div>
        )}

        {/* Working Directory */}
        {sessionType !== "one_off" && sessionType !== "new_project" && (
          <div className={field}>
            <label className={labelClass} htmlFor="omnibar-working-dir">
              Working Directory
            </label>
            <input
              id="omnibar-working-dir"
              type="text"
              className={fieldInput}
              placeholder="src/api (optional)"
              value={workingDir}
              onChange={(e) => setFormField("workingDir", e.target.value)}
            />
            <span className={hint}>Optional: Start in a subdirectory (relative path)</span>
          </div>
        )}

        {/* Image Attachment */}
        <div className={styles.attachArea}>
          {/* Hidden file input — no capture attribute so iOS shows camera+library+browse */}
          <input
            ref={attachInputRef}
            type="file"
            accept="image/*"
            multiple
            style={{ display: "none" }}
            onChange={handleAttachFiles}
            aria-hidden="true"
          />
          <button
            type="button"
            className={styles.attachButton}
            onClick={() => attachInputRef.current?.click()}
            disabled={isAttaching || attachedImages.length >= 3}
            aria-label="Attach image (up to 3)"
          >
            {isAttaching ? "⏳ Uploading..." : "📎 Attach image"}
          </button>
          {attachedImages.length >= 3 && (
            <span className={styles.attachLimit}>Max 3 images</span>
          )}
          {attachError && (
            <span className={styles.attachError}>{attachError}</span>
          )}
        </div>

        {/* Thumbnail previews */}
        {attachedImages.length > 0 && (
          <div className={styles.thumbnailRow}>
            {attachedImages.map((img, i) => (
              <div key={img.path} className={styles.thumbnail}>
                <img
                  src={img.previewUrl}
                  alt={img.file.name}
                  className={styles.thumbnailImg}
                />
                <button
                  type="button"
                  className={styles.thumbnailRemove}
                  onClick={() => removeImage(i)}
                  aria-label={`Remove ${img.file.name}`}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Advanced Options */}
        <div className={collapsible}>
          <div className={collapsibleHeader} onClick={onToggleAdvanced}>
            <span className={collapsibleTitle}>Advanced Options</span>
            <span className={`${collapsibleIcon} ${showAdvanced ? expanded : ""}`}>▼</span>
          </div>
          <div className={[styles.advancedSection, showAdvanced ? styles.advancedSectionOpen : ""].filter(Boolean).join(" ")}>
            <div className={collapsibleContent}>
              {/* Program */}
              <div className={field}>
                <label className={labelClass} htmlFor="omnibar-program">
                  Program
                </label>
                <select
                  id="omnibar-program"
                  className={selectClass}
                  value={program}
                  onChange={(e) => setFormField("program", e.target.value)}
                >
                  {PROGRAMS.map((p) => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>

              {/* Category */}
              <div className={field}>
                <label className={labelClass} htmlFor="omnibar-category">
                  Category
                </label>
                <input
                  id="omnibar-category"
                  type="text"
                  className={fieldInput}
                  placeholder="e.g., Features, Bugfixes"
                  value={category}
                  onChange={(e) => setFormField("category", e.target.value)}
                />
              </div>

              {/* Auto-Yes */}
              <label className={checkboxClass}>
                <input
                  type="checkbox"
                  checked={autoYes}
                  onChange={(e) => setFormField("autoYes", e.target.checked)}
                />
                <span>Auto-approve prompts (experimental)</span>
              </label>
            </div>
          </div>
        </div>
      </div>

      {/* Error Message */}
      {error && <div className={errorClass}>{error}</div>}

      {/* Footer */}
      <div className={footer}>
        <button type="button" className={`${buttonClass} ${buttonSecondary}`} onClick={onCancel}>
          Cancel
        </button>
        <button
          type="button"
          className={`${buttonClass} ${buttonPrimary}`}
          onClick={onSubmit}
          disabled={!canSubmit || isSubmitting}
        >
          {isSubmitting ? "Creating..." : "Create Session"}
        </button>
      </div>
    </>
  );
}
