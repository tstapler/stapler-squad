"use client";

import { useState, useRef } from "react";
import { CheckpointProto } from "@/gen/session/v1/types_pb";
import * as styles from "./CheckpointButton.css";

interface CheckpointButtonProps {
  sessionId: string;
  isRunning: boolean;
  onCreateCheckpoint: (sessionId: string, label: string) => Promise<boolean>;
  onCheckpointCreated?: (cp: CheckpointProto) => void;
}

export function CheckpointButton({
  sessionId,
  isRunning,
  onCreateCheckpoint,
  onCheckpointCreated,
}: CheckpointButtonProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const handleOpenForm = (e: React.MouseEvent) => {
    e.stopPropagation();
    setLabel("Checkpoint " + new Date().toLocaleString());
    setError("");
    setIsOpen(true);
    // Focus input on next tick after render
    setTimeout(() => inputRef.current?.focus(), 0);
  };

  const handleSubmit = async (e: React.MouseEvent | React.KeyboardEvent) => {
    e.stopPropagation();
    if (!label.trim()) return;

    setIsSubmitting(true);
    setError("");

    try {
      const success = await onCreateCheckpoint(sessionId, label.trim());
      if (success) {
        setIsOpen(false);
        setLabel("");
        // onCheckpointCreated is called with a placeholder since createCheckpoint
        // returns boolean (the full proto is not returned by the current service method).
        if (onCheckpointCreated) {
          // We can't get the full proto back from the boolean API; callers that need
          // the full proto should refresh via listCheckpoints separately.
          onCheckpointCreated({} as CheckpointProto);
        }
      } else {
        setError("Failed to create checkpoint");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create checkpoint");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsOpen(false);
    setError("");
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      handleSubmit(e);
    } else if (e.key === "Escape") {
      setIsOpen(false);
      setError("");
    }
  };

  if (isOpen) {
    return (
      <div className={styles.inlineForm} onClick={(e) => e.stopPropagation()}>
        <span className={styles.formLabel}>Create checkpoint</span>
        <input
          ref={inputRef}
          type="text"
          className={styles.textInput}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Label for this checkpoint"
          aria-label="Checkpoint label"
          disabled={isSubmitting}
        />
        {error && <span className={styles.errorText}>{error}</span>}
        <div className={styles.formActions}>
          <button
            className={styles.cancelButton}
            onClick={handleCancel}
            disabled={isSubmitting}
            type="button"
          >
            Cancel
          </button>
          <button
            className={styles.submitButton}
            onClick={handleSubmit}
            disabled={isSubmitting || !label.trim()}
            type="button"
          >
            {isSubmitting ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.root}>
      <button
        className={styles.button}
        onClick={handleOpenForm}
        disabled={!isRunning}
        title={isRunning ? "Create a checkpoint of the current session state" : "Session must be running to create a checkpoint"}
        aria-label="Create checkpoint"
        type="button"
      >
        {/* Bookmark icon */}
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
        </svg>
        Checkpoint
      </button>
    </div>
  );
}
