"use client";

import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { detect, InputType, INPUT_TYPE_INFO, DetectionResult } from "@/lib/omnibar";
import styles from "./Omnibar.module.css";

interface OmnibarProps {
  isOpen: boolean;
  onClose: () => void;
  onCreateSession: (data: OmnibarSessionData) => Promise<void>;
}

export interface OmnibarSessionData {
  title: string;
  path: string;
  branch?: string;
  program: string;
  category?: string;
  prompt?: string;
  autoYes: boolean;
  // GitHub-specific
  gitHubOwner?: string;
  gitHubRepo?: string;
  gitHubPRNumber?: number;
  // Session type and worktree
  sessionType?: "directory" | "new_worktree" | "existing_worktree";
  existingWorktree?: string;
  workingDir?: string;
}

export function Omnibar({ isOpen, onClose, onCreateSession }: OmnibarProps) {
  // Input state
  const [input, setInput] = useState("");
  const [detection, setDetection] = useState<DetectionResult | null>(null);

  // Form state
  const [sessionName, setSessionName] = useState("");
  const [program, setProgram] = useState("claude");
  const [category, setCategory] = useState("");
  const [autoYes, setAutoYes] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Session type and worktree state
  const [sessionType, setSessionType] = useState<"directory" | "new_worktree" | "existing_worktree">("new_worktree");
  const [branch, setBranch] = useState("");
  const [useTitleAsBranch, setUseTitleAsBranch] = useState(false);
  const [existingWorktree, setExistingWorktree] = useState("");
  const [workingDir, setWorkingDir] = useState("");

  // Submission state
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Refs
  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<NodeJS.Timeout | null>(null);
  const lastSuggestedNameRef = useRef<string>("");

  // Detect input type with debouncing
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    debounceRef.current = setTimeout(() => {
      if (input.trim()) {
        const result = detect(input);
        setDetection(result);

        // Auto-fill session name if:
        // 1. Session name is empty, OR
        // 2. Session name matches the last auto-suggested name (not manually edited)
        // This allows suggestions to update as the user types the path (e.g., "~" → "sqlway")
        if (result.suggestedName) {
          if (!sessionName || sessionName === lastSuggestedNameRef.current) {
            setSessionName(result.suggestedName);
            lastSuggestedNameRef.current = result.suggestedName;
          }
        }

        // Auto-fill branch if detected
        if (result.branch && !branch) {
          setBranch(result.branch);
        }
      } else {
        setDetection(null);
      }
    }, 150); // 150ms debounce

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, [input, sessionName, branch]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  // Reset state when closed
  useEffect(() => {
    if (!isOpen) {
      setInput("");
      setDetection(null);
      setSessionName("");
      setProgram("claude");
      setCategory("");
      setAutoYes(false);
      setShowAdvanced(false);
      setError(null);
      setSessionType("new_worktree");
      setBranch("");
      setUseTitleAsBranch(false);
      setExistingWorktree("");
      setWorkingDir("");
      lastSuggestedNameRef.current = "";
    }
  }, [isOpen]);

  // Handle keyboard shortcuts
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      } else if (e.key === "Enter" && e.metaKey) {
        // Cmd+Enter to submit
        handleSubmit();
      }
    },
    [onClose]
  );

  // Global keyboard handler
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      // Cmd+K or Ctrl+K to open (handled by parent)
      if (isOpen && e.key === "Escape") {
        onClose();
      }
    };

    document.addEventListener("keydown", handleGlobalKeyDown);
    return () => document.removeEventListener("keydown", handleGlobalKeyDown);
  }, [isOpen, onClose]);

  // Get type info for display
  const typeInfo = useMemo(() => {
    if (!detection) return INPUT_TYPE_INFO[InputType.Unknown];
    return INPUT_TYPE_INFO[detection.type];
  }, [detection]);

  // Check if we can submit
  const canSubmit = useMemo(() => {
    if (!input.trim()) return false;
    if (!sessionName.trim()) return false;
    if (!detection || detection.type === InputType.Unknown) return false;

    // Validate session type specific requirements
    if (sessionType === "new_worktree") {
      // Branch is required unless using title as branch
      if (!useTitleAsBranch && !branch.trim()) return false;
    } else if (sessionType === "existing_worktree") {
      // Existing worktree path is required
      if (!existingWorktree.trim()) return false;
    }

    return true;
  }, [input, sessionName, detection, sessionType, branch, useTitleAsBranch, existingWorktree]);

  // Handle form submission
  const handleSubmit = async () => {
    if (!canSubmit || isSubmitting) return;

    setIsSubmitting(true);
    setError(null);

    try {
      // Determine final branch name
      let finalBranch = branch.trim();
      if (sessionType === "new_worktree" && useTitleAsBranch) {
        finalBranch = sessionName.trim();
      }

      const sessionData: OmnibarSessionData = {
        title: sessionName.trim(),
        path: detection?.localPath || "",
        branch: finalBranch || undefined,
        program,
        category: category.trim() || undefined,
        autoYes,
        sessionType,
        existingWorktree: existingWorktree.trim() || undefined,
        workingDir: workingDir.trim() || undefined,
      };

      // Handle GitHub URLs - path will be resolved server-side
      if (detection?.gitHubRef) {
        sessionData.gitHubOwner = detection.gitHubRef.owner;
        sessionData.gitHubRepo = detection.gitHubRef.repo;
        sessionData.gitHubPRNumber = detection.gitHubRef.prNumber;

        // For GitHub URLs, set path to the parsed value for server-side cloning
        if (!sessionData.path) {
          sessionData.path = detection.parsedValue;
        }
      }

      await onCreateSession(sessionData);
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create session";
      setError(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className={styles.overlay}
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="omnibar-title"
    >
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Main Input */}
        <div className={styles.inputContainer}>
          <span className={styles.typeIndicator} aria-hidden="true">
            {typeInfo.icon}
          </span>
          <input
            ref={inputRef}
            type="text"
            className={styles.input}
            placeholder="Enter path, GitHub URL, or owner/repo..."
            value={input}
            onChange={(e) => setInput(e.target.value)}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            aria-label="Session source input"
          />
        </div>

        {/* Detection Badge */}
        {input.trim() && (
          <div className={styles.detectionInfo}>
            <span
              className={`${styles.detectionBadge} ${
                detection?.type === InputType.Unknown ? styles.unknown : ""
              }`}
            >
              {typeInfo.icon} {typeInfo.label}
            </span>
          </div>
        )}

        {/* Form Fields */}
        <div className={styles.body}>
          {/* Session Name */}
          <div className={styles.field}>
            <label className={styles.label} htmlFor="omnibar-name">
              Session Name *
            </label>
            <input
              id="omnibar-name"
              type="text"
              className={styles.fieldInput}
              placeholder="my-feature-session"
              value={sessionName}
              onChange={(e) => setSessionName(e.target.value)}
            />
          </div>

          {/* Session Type */}
          <div className={styles.field}>
            <label className={styles.label} htmlFor="omnibar-session-type">
              Session Type
            </label>
            <select
              id="omnibar-session-type"
              className={styles.select}
              value={sessionType}
              onChange={(e) => setSessionType(e.target.value as "directory" | "new_worktree" | "existing_worktree")}
            >
              <option value="new_worktree">Create New Worktree</option>
              <option value="existing_worktree">Use Existing Worktree</option>
              <option value="directory">Directory Only (No Worktree)</option>
            </select>
            <span className={styles.hint}>
              {sessionType === "new_worktree" && "Creates an isolated git worktree for this session"}
              {sessionType === "existing_worktree" && "Uses an existing worktree at a specific path"}
              {sessionType === "directory" && "Works directly in the repository without worktree isolation"}
            </span>
          </div>

          {/* Branch controls (for new worktree) */}
          {sessionType === "new_worktree" && (
            <>
              <label className={styles.checkbox}>
                <input
                  type="checkbox"
                  checked={useTitleAsBranch}
                  onChange={(e) => setUseTitleAsBranch(e.target.checked)}
                />
                <span>Use session name as branch name</span>
              </label>

              <div className={styles.field}>
                <label className={styles.label} htmlFor="omnibar-branch">
                  Git Branch {!useTitleAsBranch && "*"}
                </label>
                <input
                  id="omnibar-branch"
                  type="text"
                  className={styles.fieldInput}
                  placeholder={useTitleAsBranch ? sessionName || "Enter session name first" : "feature/my-feature"}
                  value={useTitleAsBranch ? sessionName : branch}
                  onChange={(e) => !useTitleAsBranch && setBranch(e.target.value)}
                  disabled={useTitleAsBranch}
                  style={{ opacity: useTitleAsBranch ? 0.6 : 1 }}
                />
                <span className={styles.hint}>
                  {useTitleAsBranch
                    ? `Branch name will be: ${sessionName || "(enter session name)"}`
                    : "Branch to create for the new worktree"}
                </span>
              </div>
            </>
          )}

          {/* Existing worktree path */}
          {sessionType === "existing_worktree" && (
            <div className={styles.field}>
              <label className={styles.label} htmlFor="omnibar-existing-worktree">
                Existing Worktree Path *
              </label>
              <input
                id="omnibar-existing-worktree"
                type="text"
                className={styles.fieldInput}
                placeholder="/path/to/existing/worktree"
                value={existingWorktree}
                onChange={(e) => setExistingWorktree(e.target.value)}
              />
              <span className={styles.hint}>Absolute path to an existing git worktree</span>
            </div>
          )}

          {/* Working Directory (optional, for all types) */}
          <div className={styles.field}>
            <label className={styles.label} htmlFor="omnibar-working-dir">
              Working Directory
            </label>
            <input
              id="omnibar-working-dir"
              type="text"
              className={styles.fieldInput}
              placeholder="src/api (optional)"
              value={workingDir}
              onChange={(e) => setWorkingDir(e.target.value)}
            />
            <span className={styles.hint}>Optional: Start in a subdirectory (relative path)</span>
          </div>

          {/* Advanced Options */}
          <div className={styles.collapsible}>
            <div
              className={styles.collapsibleHeader}
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              <span className={styles.collapsibleTitle}>Advanced Options</span>
              <span
                className={`${styles.collapsibleIcon} ${
                  showAdvanced ? styles.expanded : ""
                }`}
              >
                ▼
              </span>
            </div>

            {showAdvanced && (
              <div className={styles.collapsibleContent}>
                {/* Program */}
                <div className={styles.field}>
                  <label className={styles.label} htmlFor="omnibar-program">
                    Program
                  </label>
                  <select
                    id="omnibar-program"
                    className={styles.select}
                    value={program}
                    onChange={(e) => setProgram(e.target.value)}
                  >
                    <option value="claude">Claude Code</option>
                    <option value="aider">Aider</option>
                    <option value="aider --model ollama_chat/gemma3:1b">
                      Aider (Ollama Gemma 1B)
                    </option>
                  </select>
                </div>

                {/* Category */}
                <div className={styles.field}>
                  <label className={styles.label} htmlFor="omnibar-category">
                    Category
                  </label>
                  <input
                    id="omnibar-category"
                    type="text"
                    className={styles.fieldInput}
                    placeholder="e.g., Features, Bugfixes"
                    value={category}
                    onChange={(e) => setCategory(e.target.value)}
                  />
                </div>

                {/* Auto-Yes */}
                <label className={styles.checkbox}>
                  <input
                    type="checkbox"
                    checked={autoYes}
                    onChange={(e) => setAutoYes(e.target.checked)}
                  />
                  <span>Auto-approve prompts (experimental)</span>
                </label>
              </div>
            )}
          </div>
        </div>

        {/* Error Message */}
        {error && <div className={styles.error}>{error}</div>}

        {/* Footer */}
        <div className={styles.footer}>
          <button
            type="button"
            className={`${styles.button} ${styles.buttonSecondary}`}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="button"
            className={`${styles.button} ${styles.buttonPrimary}`}
            onClick={handleSubmit}
            disabled={!canSubmit || isSubmitting}
          >
            {isSubmitting ? "Creating..." : "Create Session"}
          </button>
        </div>

        {/* Keyboard Shortcuts */}
        <div className={styles.shortcuts}>
          <span className={styles.shortcut}>
            <span className={styles.shortcutKey}>Esc</span> Close
          </span>
          <span className={styles.shortcut}>
            <span className={styles.shortcutKey}>⌘↵</span> Create
          </span>
        </div>
      </div>
    </div>
  );
}
