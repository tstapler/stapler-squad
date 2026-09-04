"use client";
// +feature: session-image-attach pi-support

import { useState, useRef, useCallback, useEffect } from "react";
import type { KeyboardEvent } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { WorktreeEntry } from "@/gen/session/v1/session_pb";
import type { OmnibarFormState } from "./Omnibar";
import { useAvailablePrograms } from "@/lib/hooks/useAvailablePrograms";
import { PI_SUPPORT_FLAG_NAME, getPickerPrograms } from "@/lib/constants/programs";
import { getConnectTransport } from "@/lib/api/transport";
import { isAutoApproveSupported, isApprovalExtensionSupported } from "@/lib/sessions/autoApprove";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import {
  body, field, label as labelClass, fieldInput, hint, select as selectClass,
  checkbox as checkboxClass, collapsible, collapsibleHeader, collapsibleTitle, collapsibleIcon, expanded,
  collapsibleContent, footer, button as buttonClass, buttonSecondary, buttonPrimary,
  error as errorClass,
} from "./Omnibar.css";
import * as styles from "./OmnibarCreationPanel.css";
import { FileChipList, type AttachedFile } from "./FileChipList";
import { RadioGroup } from "@/components/ui/RadioGroup";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import { SlashCommandDropdown } from "@/components/ui/SlashCommandDropdown";
import { useSlashCommands } from "@/lib/hooks/useSlashCommands";
import { useSlashCommandSuggestions } from "@/lib/hooks/useSlashCommandSuggestions";
import type { LauncherPresetEntry } from "@/lib/hooks/useLauncherPresets";
import { OmnibarPresetList } from "./OmnibarPresetList";
import * as presetListStyles from "./OmnibarPresetList.css";

// ─── Session Type Radio Group ────────────────────────────────────────────────

export const SESSION_TYPES = [
  {
    value: "new_worktree",
    label: "New branch (isolated)",
    description:
      "Use this when you want to try something risky without touching your main branch — e.g. a refactor, a new feature, or a change you might abandon. Creates an isolated branch and working directory.",
  },
  {
    value: "directory",
    label: "Existing folder",
    description:
      "Use this when you just want to work in a folder as-is — e.g. quick edits to a repo you already have checked out, or a folder with no git history.",
  },
  {
    value: "existing_worktree",
    label: "Existing branch",
    description:
      "Use this when you want to resume work on a branch that's already checked out — e.g. picking up review feedback or continuing a session from earlier.",
  },
  {
    value: "one_off",
    label: "Temporary (no git)",
    description:
      "Use this when you need scratch space for a quick experiment — e.g. testing a snippet or script. No path needed; a temporary directory is created automatically.",
  },
  {
    value: "new_project",
    label: "New Project",
    description:
      "Use this when starting something brand new — e.g. a side project or prototype. Creates a directory, runs git init, and makes an initial commit.",
  },
] as const;

// Autonomous mode's hint text, shown when the "Autonomous mode" checkbox is checked.
// Not a session type itself — it's an orthogonal flag that composes with whichever
// type is selected above (see AUTONOMOUS_MODE_HINT usage below).
export const AUTONOMOUS_MODE_HINT =
  "Hand off a well-defined task and walk away — e.g. a small bug fix or chore. An LLM reviewer approves risky tool calls instead of you; you'll be notified when it's done. To stop it, delete or hibernate the session.";


type SessionTypeValue = (typeof SESSION_TYPES)[number]["value"];

const PRIMARY_TYPES = SESSION_TYPES.slice(0, 2).concat([SESSION_TYPES[3]]); // new_worktree, directory, one_off
const ADVANCED_TYPES = [SESSION_TYPES[2], SESSION_TYPES[4]]; // existing_worktree, new_project
const ADVANCED_VALUES = new Set<string>(ADVANCED_TYPES.map((t) => t.value));

// Stable identity for callers that omit launcherPresets — a fresh `[]` literal as a default
// parameter value would get a new identity every render, retriggering the auto-open effect's
// dependency array unnecessarily.
const EMPTY_PRESETS: LauncherPresetEntry[] = [];

// Stable identity for callers that omit remotes — see EMPTY_PRESETS above for why.
const EMPTY_REMOTES: RemoteOption[] = [];

/**
 * A configured remote target, shown in the "Remote host" selector (ADR-001:
 * remote-as-orthogonal-flag; see project_plans/ssh-remote-workspaces/decisions/ADR-001-
 * remote-as-orthogonal-flag.md). Deliberately minimal — only what this selector needs to
 * display an option and identify it by name.
 *
 * TODO(Phase 6): this list will be sourced from a `remotesSlice` in Redux, populated by a
 * `ListRemotes` RPC over the full `RemoteConfig` (proto/session/v1/remote.proto — host, port,
 * username, etc.), and a Settings UI to manage it. Neither exists yet; until then, callers
 * pass this list in as a prop (defaulting to empty, which hides the selector entirely).
 */
export interface RemoteOption {
  /** RemoteConfig.Name — the identifier threaded through as RemoteTarget.remoteName. */
  name: string;
}

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
  // Expand advanced section automatically if the current selection is an advanced type
  const [advancedOpen, setAdvancedOpen] = useState(() => ADVANCED_VALUES.has(value));

  const visibleTypes = advancedOpen ? [...PRIMARY_TYPES, ...ADVANCED_TYPES] : PRIMARY_TYPES;

  return (
    <RadioGroup
      options={visibleTypes}
      value={value}
      onChange={onChange}
      groupLabel="Session Type"
      groupLabelId="omnibar-session-type-label"
      hintForValue={(v) => SESSION_TYPES.find((t) => t.value === v)?.description}
      trailingContent={
        <button
          type="button"
          tabIndex={-1}
          aria-expanded={advancedOpen}
          onClick={() => setAdvancedOpen((o) => !o)}
          className={styles.radioBtn}
          style={{ opacity: 0.65, fontSize: "0.75em" }}
        >
          {advancedOpen ? "▴ Less" : "▾ More"}
        </button>
      }
    />
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
  /** Set when the worktree list request failed or timed out — shown as a hint. */
  worktreesError?: string | null;
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
  /** True when path completion has resolved and the typed path doesn't exist on disk. */
  pathDoesNotExist?: boolean;
  /** Name prefix from alias detection (e.g. "ssq-"). Used to hint that the user should type a label after it. */
  namePrefix?: string;
  /** Live preview of the destination checkout path (github_url or new_worktree mode). */
  destinationPreviewPath?: string | null;
  /** True when destinationPreviewPath is the exact clone destination (github_url mode only). */
  destinationPreviewIsExact?: boolean;
  /** True while the destination path preview request is in flight. */
  isDestinationPreviewLoading?: boolean;
  /** Called when the user selects a launcher preset from the Presets section. */
  onPresetSelect?: (preset: LauncherPresetEntry) => void;
  /** Launcher presets — fetched once by OmnibarContext (shared with PresetDetector), passed
   * down rather than fetched again here so a refetch (e.g. on Omnibar open) reaches both
   * consumers from the same state. */
  launcherPresets?: LauncherPresetEntry[];
  launcherPresetsLoading?: boolean;
  launcherPresetsLoadError?: string | null;
  /** Configured remotes for the "Remote host" selector. Renders only when non-empty — see
   * RemoteOption's doc comment for why this is a prop rather than a Redux-sourced list. */
  remotes?: RemoteOption[];
  /**
   * Stubbed extension-health signal for Story 3.1.2's capability warning: true means the pi
   * approval extension is known to have failed to load. Always `false` for now — Phase 4's
   * Story 4.2.2 will wire this to the real per-session health tracker; this prop exists so the
   * warning UI itself can ship and be tested now without waiting on that server-side work.
   */
  piApprovalExtensionFailed?: boolean;
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


export function OmnibarCreationPanel({
  formState,
  setFormField,
  onSubmit,
  onCancel,
  worktrees,
  isWorktreesLoading = false,
  worktreesError = null,
  isSubmitting,
  canSubmit,
  error,
  showAdvanced,
  onToggleAdvanced,
  path,
  uploadBaseUrl = "/api",
  onAttachedImagesChange,
  pathDoesNotExist,
  namePrefix = "",
  destinationPreviewPath = null,
  destinationPreviewIsExact = false,
  isDestinationPreviewLoading = false,
  onPresetSelect,
  launcherPresets: presets = EMPTY_PRESETS,
  launcherPresetsLoading: presetsLoading = false,
  launcherPresetsLoadError: presetsLoadError = null,
  remotes = EMPTY_REMOTES,
  piApprovalExtensionFailed = false,
}: OmnibarCreationPanelProps) {
  const {
    sessionName, branch, program, category, autoYes, autoApprove,
    useTitleAsBranch, sessionType, existingWorktree, workingDir,
    parentDir, projectName, newProjectSessionType, createIfMissing, firstPrompt,
    autonomousMode, remoteName,
  } = formState;

  // If the program changes to an unsupported agent after auto-approve was checked
  // (e.g. user picks "claude", checks the box, then switches to "codex"), force it back
  // off rather than silently submitting a checked-but-disabled checkbox's stale true value.
  useEffect(() => {
    if (autoApprove && !isAutoApproveSupported(program)) {
      setFormField("autoApprove", false);
    }
  }, [program, autoApprove, setFormField]);

  // Slash command autocomplete for the firstPrompt textarea.
  const firstPromptRef = useRef<HTMLTextAreaElement | null>(null);
  const [firstPromptCursor, setFirstPromptCursor] = useState(0);
  const [slashSuggestIndex, setSlashSuggestIndex] = useState(-1);
  const { commands: slashCommands } = useSlashCommands(path ?? "");
  const slashState = useSlashCommandSuggestions(firstPrompt, firstPromptCursor, slashCommands);
  const isSlashDropdownVisible = slashState.isActive && slashState.suggestions.length > 0;

  const handleSlashSelect = useCallback((cmd: Parameters<typeof slashState.complete>[1]) => {
    const { newValue, newCursorPos } = slashState.complete(firstPrompt, cmd);
    setFormField("firstPrompt", newValue);
    setSlashSuggestIndex(-1);
    requestAnimationFrame(() => {
      if (firstPromptRef.current) {
        firstPromptRef.current.setSelectionRange(newCursorPos, newCursorPos);
        firstPromptRef.current.focus();
        setFirstPromptCursor(newCursorPos);
      }
    });
  }, [slashState, firstPrompt, setFormField]);

  const handleFirstPromptKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
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

  // "Create new repository" affordance is only meaningful for session types
  // that operate on the path itself. existing_worktree expects a real parent
  // repo; we surface a different (blocking) message there.
  const showCreateRepoNotice =
    pathDoesNotExist === true &&
    (sessionType === "directory" || sessionType === "new_worktree");
  const showExistingWorktreePathError =
    pathDoesNotExist === true && sessionType === "existing_worktree";

  // ─── Load default parentDir from config when new_project mode is first selected ──
  useEffect(() => {
    if (sessionType !== "new_project" || parentDir) return;
    const load = async () => {
      try {
        const client = createClient(SessionService, getConnectTransport());
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

  const availablePrograms = useAvailablePrograms();
  const piSupportEnabled = useFeatureFlag(PI_SUPPORT_FLAG_NAME);
  // Story 3.1.1: "pi" is only offered in the rendered picker options when pi-support is on —
  // opt-in invisibility. availablePrograms (not the raw PROGRAMS constant) is the base list
  // here since it may also include extra programs detected on the host; getPickerPrograms
  // takes that list as a parameter so this call site and programs.ts share one filter rule.
  const pickerPrograms = getPickerPrograms(availablePrograms, piSupportEnabled);
  // Story 3.1.2: the capability warning is only even eligible to show for a program the pi
  // approval extension covers, and only while pi-support itself is on.
  const showPiApprovalWarning =
    piSupportEnabled && isApprovalExtensionSupported(program) && piApprovalExtensionFailed;
  // Default-expanded once there's something to show — either a loaded preset or a config
  // error. An error must never sit hidden behind a collapsed section: AC requires a malformed
  // config to "fail loudly", and a load_error with zero presets (the RPC's own contract) would
  // otherwise default to collapsed under the empty-state branch below, silently hiding it. A
  // truly empty, error-free state is the only case that stays collapsed (design/ux.md §5.1.1).
  // Auto-opens only once: a ref (not an ongoing effect) so a later background refetch can't
  // re-force the section open after the user has manually collapsed it.
  const shouldAutoOpenPresets = presets.length > 0 || Boolean(presetsLoadError);
  const [presetsOpen, setPresetsOpen] = useState(() => shouldAutoOpenPresets);
  const hasAutoOpenedPresets = useRef(shouldAutoOpenPresets);
  useEffect(() => {
    if (!hasAutoOpenedPresets.current && shouldAutoOpenPresets) {
      hasAutoOpenedPresets.current = true;
      setPresetsOpen(true);
    }
  }, [shouldAutoOpenPresets]);
  const isProgramRecognized = !program || availablePrograms.some((p) => p.value === program);

  // ─── File attachment state ────────────────────────────────────────────────
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]);
  // Mirror of attachedFiles kept in a ref so the unmount cleanup can revoke
  // object URLs without capturing stale closure values.
  const attachedFilesRef = useRef<AttachedFile[]>([]);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [isAttaching, setIsAttaching] = useState(false);
  const attachInputRef = useRef<HTMLInputElement>(null);

  // Keep ref in sync with state so the unmount cleanup always sees current files.
  useEffect(() => {
    attachedFilesRef.current = attachedFiles;
    onAttachedImagesChange?.(attachedFiles.map((f) => f.path));
  }, [attachedFiles, onAttachedImagesChange]);

  // Revoke object URLs on unmount via ref — avoids stale closure over empty array.
  useEffect(() => {
    return () => {
      attachedFilesRef.current.forEach((f) => {
        if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
      });
    };
  }, []);

  const handleAttachFiles = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    e.target.value = "";
    if (!files.length) return;

    const toUpload = files; // no cap; deduplication applied below

    setIsAttaching(true);
    setAttachError(null);

    const MAX_FILE_BYTES = 20 * 1024 * 1024; // 20 MB — must match backend maxUploadBytes

    // Size check — reject oversized files before FileReader allocation
    const oversized = toUpload.filter(f => f.size > MAX_FILE_BYTES);
    if (oversized.length > 0) {
      setAttachError(`${oversized.map(f => f.name).join(", ")}: exceeds 20 MB limit`);
      // Continue with remaining files (don't abort the whole batch)
    }
    const sizedOk = toUpload.filter(f => f.size <= MAX_FILE_BYTES);

    // Deduplication — skip files already attached (by name+size+lastModified)
    const existingKeys = new Set(
      attachedFiles.map(f => `${f.file.name}|${f.file.size}|${f.file.lastModified}`)
    );
    const deduplicated = sizedOk.filter(
      f => !existingKeys.has(`${f.name}|${f.size}|${f.lastModified}`)
    );

    const results: AttachedFile[] = [];
    for (const file of deduplicated) {
      const isImage = file.type.startsWith("image/");
      const previewUrl = isImage ? URL.createObjectURL(file) : undefined;
      try {
        const base64 = await fileToBase64(file);
        const resp = await fetch(`${uploadBaseUrl}/upload/file`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            data: base64,
            contentType: file.type,
            originalFilename: file.name,
          }),
        });
        if (!resp.ok) {
          if (previewUrl) URL.revokeObjectURL(previewUrl);
          setAttachError("Upload failed");
          break;
        }
        const data = await resp.json() as { path: string };
        results.push({
          file,
          path: data.path,
          previewUrl,      // undefined for non-images
          name: file.name,
          size: file.size,
        });
      } catch {
        if (previewUrl) URL.revokeObjectURL(previewUrl);
        setAttachError("Upload failed");
        break;
      }
    }

    setAttachedFiles((prev) => [...prev, ...results]);
    setIsAttaching(false);
  }, [attachedFiles, uploadBaseUrl]);

  const removeFile = useCallback((index: number) => {
    setAttachedFiles((prev) => {
      const f = prev[index];
      if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
      return prev.filter((_, i) => i !== index);
    });
  }, []);

  return (
    <>
      {/* Pre-selected repo path (creation_with_repo mode) */}
      {path && (
        <div className={styles.pathDisplay}>
          {path}
        </div>
      )}

      {/* Opt-in: create directory + initialize git repo when the path is missing */}
      {showCreateRepoNotice && (
        <div
          className={[styles.createRepoNotice, createIfMissing ? styles.createRepoNoticeActive : ""]
            .filter(Boolean)
            .join(" ")}
        >
          <div className={styles.createRepoNoticeRow}>
            <span className={styles.createRepoNoticeIcon} aria-hidden="true">
              +
            </span>
            <div className={styles.createRepoNoticeBody}>
              <div className={styles.createRepoNoticeTitle}>
                Path doesn&rsquo;t exist yet
              </div>
              <div className={styles.createRepoNoticeDesc}>
                Stapler Squad can create the directory and initialize a fresh
                git repository (with an initial commit) at this location before
                starting the session.
              </div>
            </div>
          </div>
          <label className={checkboxClass}>
            <input
              type="checkbox"
              checked={createIfMissing}
              onChange={(e) => setFormField("createIfMissing", e.target.checked)}
            />
            <span>Create a new git repository here</span>
          </label>
          {!createIfMissing && (
            <div className={styles.createRepoNoticeBlocked} role="status">
              Check the box above to create the repository, or pick an existing
              path to continue.
            </div>
          )}
        </div>
      )}

      {/* existing_worktree can't fall back to creation — keep the error tight */}
      {showExistingWorktreePathError && (
        <div className={styles.createRepoNotice}>
          <div className={styles.createRepoNoticeRow}>
            <span
              className={`${styles.createRepoNoticeIcon} ${styles.createRepoNoticeIconError}`}
              aria-hidden="true"
            >
              !
            </span>
            <div className={styles.createRepoNoticeBody}>
              <div className={styles.createRepoNoticeTitle}>
                Repository path doesn&rsquo;t exist
              </div>
              <div className={styles.createRepoNoticeDesc}>
                &ldquo;Existing branch&rdquo; needs a real parent repository.
                Switch to &ldquo;Existing folder&rdquo; or &ldquo;New branch (isolated)&rdquo;
                if you want to create a new repo here.
              </div>
            </div>
          </div>
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
          {!sessionName && (
            <span className={hint} style={{ color: "var(--error)" }}>
              Session name is empty — type a name above or use &ldquo;name &gt; prompt&rdquo; syntax
            </span>
          )}
          {namePrefix && sessionName === namePrefix && (
            <span className={hint} style={{ color: "var(--warning)" }}>
              Type a label after the prefix to complete the session name (e.g. &ldquo;{namePrefix}my-feature&rdquo;)
            </span>
          )}
          {firstPrompt && sessionName && (
            <span className={hint}>
              Session name: <strong>{sessionName}</strong> · First prompt will be typed into the session terminal automatically
            </span>
          )}
        </div>

        {/* GitHub URL destination preview — exact clone destination, shown before the
            mode selector since it applies regardless of which session type is chosen. */}
        {destinationPreviewIsExact && destinationPreviewPath && (
          <div className={hint} style={{ marginTop: 0 }}>
            Will check out to: <code>{destinationPreviewPath}</code>
          </div>
        )}

        {/* Session Type — ARIA radio group (ADR-003: arrow keys cycle) */}
        <div className={field}>
          <SessionTypeRadioGroup
            value={sessionType}
            onChange={(v) => setFormField("sessionType", v)}
          />
        </div>

        {/* "Remote host" selector — an orthogonal control (ADR-001: remote-as-orthogonal-flag)
            that composes with whichever SESSION_TYPES value is selected above, not a 6th
            radio option. Renders only once ≥1 remote is configured (research/ux.md §2:
            "defaulting to This machine"); absent entirely otherwise, not merely disabled. */}
        {remotes.length > 0 && (
          <div className={field} data-testid="remote-selector-field">
            <label className={labelClass} htmlFor="omnibar-remote">
              <span className={styles.remoteIcon} aria-hidden="true">🌐</span> Remote host
            </label>
            <select
              id="omnibar-remote"
              data-testid="remote-selector"
              className={selectClass}
              value={remoteName ?? ""}
              onChange={(e) => setFormField("remoteName", e.target.value || undefined)}
            >
              <option value="">This machine</option>
              {remotes.map((r) => (
                <option key={r.name} value={r.name}>{r.name}</option>
              ))}
            </select>
            <span className={hint}>
              {remoteName
                ? `Runs on "${remoteName}" over SSH.`
                : "Runs locally by default — pick a configured remote to run this session over SSH instead."}
            </span>
          </div>
        )}

        {/* Autonomous mode — an orthogonal flag, not a session type: it composes with
            whichever type is selected above instead of forcing a scratch directory. */}
        {sessionType !== "one_off" && (
          <div className={field}>
            <label className={checkboxClass}>
              <input
                type="checkbox"
                checked={autonomousMode}
                onChange={(e) => setFormField("autonomousMode", e.target.checked)}
              />
              🤖 Autonomous mode (Beta)
            </label>
            <span className={hint}>{AUTONOMOUS_MODE_HINT}</span>
          </div>
        )}

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
              <RepoPathInput
                id="omnibar-parent-dir"
                placeholder="~/Projects"
                value={parentDir}
                onChange={(v) => setFormField("parentDir", v)}
                required
                hint="Recent paths below are existing project folders — pick one to use its parent, or type a new directory."
              />
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
              {!isDestinationPreviewLoading && destinationPreviewPath && !destinationPreviewIsExact && (
                <span className={hint}>
                  Will be created under: <code>{destinationPreviewPath}_&lt;unique-id&gt;</code>
                </span>
              )}
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
              <RepoPathInput
                id="omnibar-existing-worktree"
                placeholder="/path/to/existing/worktree"
                value={existingWorktree}
                onChange={(v) => setFormField("existingWorktree", v)}
              />
            )}
            <span className={hint}>
              {isWorktreesLoading
                ? "Scanning for git worktrees…"
                : worktreesError
                ? `${worktreesError} — enter the path manually below`
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

        {/* File Attachment */}
        <div className={styles.attachArea}>
          {/* Hidden file input — no capture attribute so iOS shows camera+library+browse */}
          <input
            ref={attachInputRef}
            type="file"
            accept="*/*"
            multiple
            style={{ display: "none" }}
            onChange={handleAttachFiles}
            aria-hidden="true"
          />
          <button
            type="button"
            className={styles.attachButton}
            onClick={() => attachInputRef.current?.click()}
            disabled={isAttaching}
            aria-label="Attach files"
          >
            {isAttaching ? "⏳ Uploading..." : "📎 Attach files"}
          </button>
          {attachError && (
            <span className={styles.attachError}>{attachError}</span>
          )}
        </div>

        {/* File chip list */}
        <FileChipList
          files={attachedFiles}
          onRemove={removeFile}
        />

        {/* First Prompt (optional) */}
        <div className={field}>
          <label className={labelClass} htmlFor="omnibar-first-prompt">
            First Prompt <span style={{ fontWeight: "normal", opacity: 0.6 }}>(optional)</span>
          </label>
          <div className={styles.textareaWrapper}>
            <textarea
              ref={firstPromptRef}
              id="omnibar-first-prompt"
              className={fieldInput}
              placeholder="What should Claude do first? Type / for slash commands."
              rows={3}
              maxLength={2000}
              value={formState.firstPrompt}
              onChange={(e) => {
                setFormField("firstPrompt", e.target.value);
                setFirstPromptCursor(e.target.selectionStart ?? 0);
                setSlashSuggestIndex(-1);
              }}
              onSelect={(e) => setFirstPromptCursor((e.target as HTMLTextAreaElement).selectionStart ?? 0)}
              onKeyDown={handleFirstPromptKeyDown}
              style={{ resize: "vertical", fontFamily: "inherit", fontSize: "inherit" }}
            />
            {isSlashDropdownVisible && (
              <div className={styles.slashDropdownWrapper}>
                <SlashCommandDropdown
                  id="omnibar-first-prompt-slash-listbox"
                  suggestions={slashState.suggestions}
                  selectedIndex={slashSuggestIndex}
                  onSelect={handleSlashSelect}
                />
              </div>
            )}
          </div>
          {formState.firstPrompt.length > 1800 && (
            <span className={hint} style={{ color: "var(--warning)" }}>
              {2000 - formState.firstPrompt.length} characters remaining
            </span>
          )}
        </div>

        {/* Presets — hand-edited, shareable launch shortcuts from launcher-presets.json.
            Placed above Advanced Options so it's visible without an extra click. */}
        <div className={collapsible}>
          <div
            className={collapsibleHeader}
            onClick={() => setPresetsOpen((v) => !v)}
            aria-expanded={presetsOpen}
            data-testid="preset-section-header"
          >
            <span className={collapsibleTitle}>Presets</span>
            <span className={`${collapsibleIcon} ${presetsOpen ? expanded : ""}`}>▼</span>
          </div>
          <div className={[styles.advancedSection, presetsOpen ? styles.advancedSectionOpen : ""].filter(Boolean).join(" ")}>
            <div className={collapsibleContent}>
              <OmnibarPresetList
                presets={presets}
                loading={presetsLoading}
                loadError={presetsLoadError}
                onSelect={(preset) => onPresetSelect?.(preset)}
              />
            </div>
          </div>
        </div>

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
                  {pickerPrograms.map((p) => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
                {!isProgramRecognized && (
                  <span className={presetListStyles.programWarning} data-testid="preset-program-warning">
                    &quot;{program}&quot; not found in PATH — check it&apos;s installed
                  </span>
                )}
                {/* Story 3.1.2 / Phase 3: placeholder capability warning. piApprovalExtensionFailed
                    is a stubbed prop (always false today) — Story 4.2.2 will wire the real
                    per-session extension-health signal here; the UI/AC (role="alert", exact
                    wording) ships now so it doesn't block on that server-side work. */}
                {showPiApprovalWarning && (
                  <span className={styles.piApprovalWarning} role="alert" data-testid="pi-approval-extension-warning">
                    Approval extension not loaded for pi — tool calls will run WITHOUT rule enforcement for this session.
                  </span>
                )}
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

              {/* Auto-Yes: TapEnter keystroke fallback + --permission-mode bypassPermissions.
                  Distinct mechanism from Auto-Approve below (auto_approve field) -- label
                  deliberately avoids the word "approve" so the two aren't misread as the
                  same setting; see session/instance.go's AutoApprove doc comment. */}
              <label className={checkboxClass}>
                <input
                  type="checkbox"
                  checked={autoYes}
                  onChange={(e) => setFormField("autoYes", e.target.checked)}
                />
                <span>Auto-accept prompts (Enter-key fallback, experimental)</span>
              </label>

              {/* Auto-Approve (yolo mode) -- independent of Auto-Yes above; injects a
                  per-agent CLI flag that skips permission/approval prompts entirely. */}
              <label className={checkboxClass}>
                <input
                  type="checkbox"
                  checked={autoApprove}
                  disabled={!isAutoApproveSupported(program)}
                  onChange={(e) => setFormField("autoApprove", e.target.checked)}
                />
                <span>⚡ Auto-approve (skip permission prompts)</span>
              </label>
              <span className={hint}>
                {isAutoApproveSupported(program)
                  ? "Skips ALL permission/approval prompts for this agent. Risk of unintended file changes — use only in disposable/sandboxed workspaces."
                  : `Not supported for "${program || "this agent"}" yet.`}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Error Message */}
      {error && (
        <div className={errorClass} role="alert" data-testid="omnibar-create-error">
          {error}
        </div>
      )}

      {/* Footer */}
      <div className={footer}>
        <button type="button" className={`${buttonClass} ${buttonSecondary}`} onClick={onCancel}>
          Cancel
        </button>
        <button
          type="button"
          data-testid="omnibar-create-session-button"
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
