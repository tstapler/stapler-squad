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

  // Submission state
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Refs
  const inputRef = useRef<HTMLInputElement>(null);
  const debounceRef = useRef<NodeJS.Timeout | null>(null);

  // Detect input type with debouncing
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    debounceRef.current = setTimeout(() => {
      if (input.trim()) {
        const result = detect(input);
        setDetection(result);

        // Auto-fill session name if empty
        if (result.suggestedName && !sessionName) {
          setSessionName(result.suggestedName);
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
  }, [input, sessionName]);

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
    return true;
  }, [input, sessionName, detection]);

  // Handle form submission
  const handleSubmit = async () => {
    if (!canSubmit || isSubmitting) return;

    setIsSubmitting(true);
    setError(null);

    try {
      const sessionData: OmnibarSessionData = {
        title: sessionName.trim(),
        path: detection?.localPath || "",
        branch: detection?.branch,
        program,
        category: category.trim() || undefined,
        autoYes,
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

          {/* Branch (shown when detected or for local paths) */}
          {detection?.type === InputType.PathWithBranch && (
            <div className={styles.field}>
              <label className={styles.label}>Branch</label>
              <input
                type="text"
                className={styles.fieldInput}
                value={detection.branch || ""}
                readOnly
                style={{ opacity: 0.7 }}
              />
              <span className={styles.hint}>Branch detected from input</span>
            </div>
          )}

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
