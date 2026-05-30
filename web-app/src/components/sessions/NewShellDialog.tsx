"use client";
// +feature: shell-tabs

import { useState, useCallback, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import * as styles from "./NewShellDialog.css";

interface NewShellDialogProps {
  onSubmit: (params: { name?: string; command?: string; workingDir?: string }) => Promise<void>;
  onCancel: () => void;
  defaultWorkingDir?: string;
}

export function NewShellDialog({ onSubmit, onCancel, defaultWorkingDir = "" }: NewShellDialogProps) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [workingDir, setWorkingDir] = useState(defaultWorkingDir);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const nameInputRef = useRef<HTMLInputElement>(null);

  // Focus name input on mount
  useEffect(() => {
    nameInputRef.current?.focus();
  }, []);

  // Close on Escape key
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onCancel]);

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    setError(null);
    try {
      await onSubmit({
        name: name.trim() || undefined,
        command: command.trim() || undefined,
        workingDir: workingDir.trim() || undefined,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to spawn shell");
    } finally {
      setIsSubmitting(false);
    }
  }, [onSubmit, name, command, workingDir]);

  const handleOverlayClick = useCallback((e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onCancel();
  }, [onCancel]);

  const content = (
    <div className={styles.overlay} onClick={handleOverlayClick} role="dialog" aria-modal="true" aria-labelledby="new-shell-dialog-title">
      <div className={styles.dialog}>
        <h2 id="new-shell-dialog-title" className={styles.title}>New Shell</h2>

        <form onSubmit={handleSubmit}>
          <div className={styles.fieldGroup}>
            <label htmlFor="shell-name" className={styles.label}>Name (optional)</label>
            <input
              ref={nameInputRef}
              id="shell-name"
              type="text"
              className={styles.input}
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. dev server"
              autoComplete="off"
            />
          </div>

          <div className={styles.fieldGroup} style={{ marginTop: "12px" }}>
            <label htmlFor="shell-command" className={styles.label}>Command (optional)</label>
            <input
              id="shell-command"
              type="text"
              className={styles.input}
              value={command}
              onChange={e => setCommand(e.target.value)}
              placeholder="Defaults to $SHELL"
              autoComplete="off"
            />
          </div>

          <div className={styles.fieldGroup} style={{ marginTop: "12px" }}>
            <label htmlFor="shell-working-dir" className={styles.label}>Working Directory (optional)</label>
            <RepoPathInput
              id="shell-working-dir"
              value={workingDir}
              onChange={setWorkingDir}
              placeholder="Defaults to session directory"
            />
          </div>

          {error && <p className={styles.errorText} style={{ marginTop: "8px" }}>{error}</p>}

          <div className={styles.actions}>
            <button type="button" className={styles.cancelButton} onClick={onCancel} disabled={isSubmitting}>
              Cancel
            </button>
            <button type="submit" className={styles.submitButton} disabled={isSubmitting}>
              {isSubmitting ? "Spawning..." : "Spawn Shell"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );

  return createPortal(content, document.body);
}
