"use client";

import { useState, useRef, useCallback } from "react";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import { SlashCommandDropdown } from "@/components/ui/SlashCommandDropdown";
import { AutocompleteInput } from "@/components/ui/AutocompleteInput";
import { useSlashCommands } from "@/lib/hooks/useSlashCommands";
import { useSlashCommandSuggestions } from "@/lib/hooks/useSlashCommandSuggestions";
import { useAvailablePrograms } from "@/lib/hooks/useAvailablePrograms";
import { CLAUDE_MODELS } from "@/lib/constants/programs";
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
  keepSessions: 0,
  archiveAfterHours: 0,
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
    keepSessions: w.keepSessions ?? 0,
    archiveAfterHours: w.archiveAfterHours ?? 0,
  };
}

export function WorkflowForm({ existing, onSubmit, onCancel }: WorkflowFormProps) {
  const [formData, setFormData] = useState<WorkflowFormData>(
    existing ? protoToFormData(existing) : EMPTY
  );
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Slash command autocomplete for the command textarea.
  const commandRef = useRef<HTMLTextAreaElement | null>(null);
  const [commandCursorPos, setCommandCursorPos] = useState(0);
  const [slashSuggestIndex, setSlashSuggestIndex] = useState(-1);
  const { commands: slashCommands } = useSlashCommands(formData.targetDirectory);
  const availablePrograms = useAvailablePrograms();
  const modelSuggestions = CLAUDE_MODELS.map((m) => m.value);
  const agentTypeSuggestions = availablePrograms
    .map((p) => p.value)
    .filter(Boolean);
  const slashState = useSlashCommandSuggestions(formData.command, commandCursorPos, slashCommands);
  const isSlashDropdownVisible = slashState.isActive && slashState.suggestions.length > 0;

  const handleSlashSelect = useCallback((cmd: Parameters<typeof slashState.complete>[1]) => {
    const { newValue, newCursorPos } = slashState.complete(formData.command, cmd);
    setField("command", newValue);
    setSlashSuggestIndex(-1);
    // Restore cursor after React re-renders the textarea value.
    requestAnimationFrame(() => {
      if (commandRef.current) {
        commandRef.current.setSelectionRange(newCursorPos, newCursorPos);
        commandRef.current.focus();
        setCommandCursorPos(newCursorPos);
      }
    });
  }, [slashState, formData.command, setField]);

  const handleCommandKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (!isSlashDropdownVisible) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSlashSuggestIndex((i) => Math.min(i + 1, slashState.suggestions.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSlashSuggestIndex((i) => Math.max(i - 1, -1));
    } else if (e.key === "Tab") {
      e.preventDefault();
      const idx = slashSuggestIndex >= 0 ? slashSuggestIndex : 0;
      if (slashState.suggestions[idx]) handleSlashSelect(slashState.suggestions[idx]);
    } else if (e.key === "Enter" && slashSuggestIndex >= 0) {
      if (slashState.suggestions[slashSuggestIndex]) {
        e.preventDefault();
        handleSlashSelect(slashState.suggestions[slashSuggestIndex]);
      }
    } else if (e.key === "Escape") {
      setSlashSuggestIndex(-1);
    }
  }, [isSlashDropdownVisible, slashState, slashSuggestIndex, handleSlashSelect]);

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
        <div className={styles.textareaWrapper}>
          <textarea
            ref={commandRef}
            id="wf-command"
            className={styles.textarea}
            value={formData.command}
            onChange={(e) => {
              setField("command", e.target.value);
              setCommandCursorPos(e.target.selectionStart ?? 0);
              setSlashSuggestIndex(-1);
            }}
            onSelect={(e) => setCommandCursorPos((e.target as HTMLTextAreaElement).selectionStart ?? 0)}
            onKeyDown={handleCommandKeyDown}
            placeholder="Describe what the agent should do… (type / for slash commands)"
            required
          />
          {isSlashDropdownVisible && (
            <div className={styles.slashDropdownWrapper}>
              <SlashCommandDropdown
                id="wf-command-slash-listbox"
                suggestions={slashState.suggestions}
                selectedIndex={slashSuggestIndex}
                onSelect={handleSlashSelect}
              />
            </div>
          )}
        </div>
        <span className={styles.hint}>
          Use <code>{"{{input}}"}</code> to inject the argument typed after @slug. Type <code>/</code> for slash commands.
        </span>
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="wf-target-dir">
          Target Directory <span className={styles.required}>*</span>
        </label>
        <RepoPathInput
          id="wf-target-dir"
          value={formData.targetDirectory}
          onChange={(v) => setField("targetDirectory", v)}
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
          <AutocompleteInput
            id="wf-model"
            value={formData.model ?? ""}
            onChange={(v) => setField("model", v)}
            placeholder="claude-sonnet-4-6"
            suggestions={modelSuggestions}
            className={styles.input}
          />
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-agent-type">Program</label>
          <AutocompleteInput
            id="wf-agent-type"
            value={formData.agentType ?? ""}
            onChange={(v) => setField("agentType", v)}
            placeholder="claude"
            suggestions={agentTypeSuggestions}
            className={styles.input}
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

      <div className={styles.row}>
        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-keep-sessions">Keep Sessions</label>
          <input
            id="wf-keep-sessions"
            className={styles.input}
            type="number"
            min={0}
            value={formData.keepSessions ?? 0}
            onChange={(e) => setField("keepSessions", parseInt(e.target.value, 10) || 0)}
            data-testid="keep-sessions-input"
          />
          <span className={styles.hint}>Keep only the N most recent sessions (0 = keep all)</span>
        </div>

        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor="wf-archive-after-hours">Archive After Hours</label>
          <input
            id="wf-archive-after-hours"
            className={styles.input}
            type="number"
            min={0}
            value={formData.archiveAfterHours ?? 0}
            onChange={(e) => setField("archiveAfterHours", parseInt(e.target.value, 10) || 0)}
            data-testid="archive-after-hours-input"
          />
          <span className={styles.hint}>Auto-archive completed sessions after N hours (0 = disabled)</span>
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
