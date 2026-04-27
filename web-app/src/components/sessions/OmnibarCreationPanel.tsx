"use client";

import type { KeyboardEvent } from "react";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";
import type { OmnibarFormState } from "./Omnibar";
import { PROGRAMS } from "@/lib/constants/programs";
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
] as const;

type SessionTypeValue = (typeof SESSION_TYPES)[number]["value"];

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
  isSubmitting: boolean;
  canSubmit: boolean;
  error: string | null;
  showAdvanced: boolean;
  onToggleAdvanced: () => void;
  /** Pre-selected repo path (creation_with_repo mode). Shown read-only above form. */
  path?: string;
}

function truncatePath(p: string, maxLen = 50): string {
  if (p.length <= maxLen) return p;
  return "…" + p.slice(-(maxLen - 1));
}

export function OmnibarCreationPanel({
  formState,
  setFormField,
  onSubmit,
  onCancel,
  worktrees,
  isSubmitting,
  canSubmit,
  error,
  showAdvanced,
  onToggleAdvanced,
  path,
}: OmnibarCreationPanelProps) {
  const {
    sessionName, branch, program, category, autoYes,
    useTitleAsBranch, sessionType, existingWorktree, workingDir,
  } = formState;

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
          </span>
        </div>

        {/* One-off informational banner */}
        {sessionType === "one_off" && (
          <div className={hint} style={{ marginTop: 0 }}>
            Directory will be created in your one-off base directory (default: <code>~/oneoff</code>) with format <code>YYYYMMDD-word-word-NN</code>. Configure in Settings → Defaults.
          </div>
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
            {worktrees.length > 0 ? (
              <select
                id="omnibar-existing-worktree"
                className={selectClass}
                value={existingWorktree}
                onChange={(e) => setFormField("existingWorktree", e.target.value)}
              >
                <option value="">Select a worktree...</option>
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
              {worktrees.length > 0
                ? "Select an existing git worktree for this repository"
                : "Absolute path to an existing git worktree"}
            </span>
          </div>
        )}

        {/* Working Directory */}
        {sessionType !== "one_off" && (
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
