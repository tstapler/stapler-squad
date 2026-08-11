"use client";

import { useState, useRef, useEffect, KeyboardEvent } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { generateUniqueName } from "@/utils/sessionNameUtils";
import * as styles from "./ResumeSessionModal.css";

interface ResumeSessionModalProps {
  session: Session;
  sessions: Session[];
  onConfirm: (updates: { title: string; tags: string[] }) => Promise<void> | void;
  onCancel: () => void;
}

export function ResumeSessionModal({
  session,
  sessions,
  onConfirm,
  onCancel,
}: ResumeSessionModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const titleInputRef = useRef<HTMLInputElement>(null);
  useFocusTrap(modalRef, true);

  // Compute conflict detection once at mount
  const [initialState] = useState(() => {
    const otherNames = sessions
      .filter((s) => s.id !== session.id)
      .map((s) => s.title);
    const hasConflict = otherNames.includes(session.title);
    const suggestedTitle = hasConflict
      ? generateUniqueName(session.title, otherNames)
      : session.title;
    return { hasConflict, originalTitle: session.title, suggestedTitle };
  });

  const [title, setTitle] = useState(initialState.suggestedTitle);
  const [tags, setTags] = useState<string[]>([...(session.tags || [])]);
  const [tagInput, setTagInput] = useState("");
  const [tagError, setTagError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Auto-focus the title input on mount
  useEffect(() => {
    titleInputRef.current?.focus();
    titleInputRef.current?.select();
  }, []);

  const handleAddTag = () => {
    const trimmed = tagInput.trim();
    if (!trimmed) {
      setTagError("Tag cannot be empty");
      return;
    }
    if (tags.includes(trimmed)) {
      setTagError("Tag already exists");
      return;
    }
    setTags([...tags, trimmed]);
    setTagInput("");
    setTagError(null);
  };

  const handleRemoveTag = (tagToRemove: string) => {
    setTags(tags.filter((t) => t !== tagToRemove));
    setTagError(null);
  };

  const handleTagInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAddTag();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

  const handleTitleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleSubmit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

  const handleSubmit = async () => {
    if (!title.trim() || isSubmitting) return;
    setIsSubmitting(true);
    try {
      await onConfirm({ title: title.trim(), tags });
    } catch {
      setIsSubmitting(false);
    }
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="resume-modal-title"
        ref={modalRef}
      >
        <div className={styles.header}>
          <h2 className={styles.title} id="resume-modal-title">
            Resume Session
          </h2>
          <p className={styles.subtitle}>
            Edit session details before resuming
          </p>
        </div>

        <div className={styles.body}>
          {/* Editable title */}
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor="resume-title">
              Session Title
            </label>
            <input
              id="resume-title"
              ref={titleInputRef}
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={handleTitleKeyDown}
              placeholder="Session title"
              className={styles.titleInput}
            />
            {initialState.hasConflict && (
              <span className={styles.conflictHint}>
                &ldquo;{initialState.originalTitle}&rdquo; is already in use.
                Suggested: &ldquo;{initialState.suggestedTitle}&rdquo;
              </span>
            )}
          </div>

          {/* Editable tags */}
          <div className={styles.tagsSection}>
            <label className={styles.fieldLabel}>Tags</label>
            <div className={styles.tagInputRow}>
              <input
                type="text"
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={handleTagInputKeyDown}
                placeholder="Add a tag..."
                className={styles.tagInput}
              />
              <button
                onClick={handleAddTag}
                className={styles.addTagButton}
                type="button"
              >
                Add
              </button>
            </div>
            {tagError && <p className={styles.tagError}>{tagError}</p>}
            {tags.length === 0 ? (
              <p className={styles.emptyTags}>No tags</p>
            ) : (
              <div className={styles.tagsList}>
                {tags.map((tag) => (
                  <div key={tag} className={styles.tagItem}>
                    <span className={styles.tagText}>{tag}</span>
                    <button
                      onClick={() => handleRemoveTag(tag)}
                      className={styles.removeTagButton}
                      title={`Remove tag "${tag}"`}
                      aria-label={`Remove tag ${tag}`}
                      type="button"
                    >
                      ×
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Read-only context */}
          <div className={styles.contextSection}>
            <label className={styles.fieldLabel}>Session Context</label>
            <div className={styles.contextGrid}>
              {session.branch && (
                <div className={styles.contextRow}>
                  <span className={styles.contextLabel}>Branch:</span>
                  <span className={styles.contextValue}>{session.branch}</span>
                </div>
              )}
              {session.program && (
                <div className={styles.contextRow}>
                  <span className={styles.contextLabel}>Program:</span>
                  <span className={styles.contextValue}>
                    {session.program}
                  </span>
                </div>
              )}
              {session.path && (
                <div className={styles.contextRow}>
                  <span className={styles.contextLabel}>Path:</span>
                  <span className={styles.contextValue} title={session.path}>
                    {session.path}
                  </span>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className={styles.footer}>
          <button
            onClick={onCancel}
            className={styles.cancelButton}
            disabled={isSubmitting}
            type="button"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            className={styles.resumeButton}
            disabled={!title.trim() || isSubmitting}
            type="button"
          >
            {isSubmitting ? "Resuming..." : "Resume Session"}
          </button>
        </div>
      </div>
    </div>
  );
}
