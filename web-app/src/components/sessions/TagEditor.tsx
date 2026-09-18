"use client";

import { useState, KeyboardEvent, useRef, RefObject } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import {
  UNCLASSIFIED_TAG, hasProvenance, removalConfirmationText,
} from "@/lib/sessions/tagProvenance";
import * as styles from "./TagEditor.css";

interface TagEditorProps {
  tags: string[];
  /** Maps a tag value to its provenance: a TaggingRule.ID, the "llm" sentinel, or absent (manual). */
  tagProvenance?: Record<string, string>;
  /** Maps a TaggingRule.ID to its display name, for the removal confirmation's rule-name wording. */
  tagRuleNames?: Record<string, string>;
  onSave: (tags: string[]) => void;
  onCancel: () => void;
  sessionTitle: string;
  triggerRef?: RefObject<HTMLElement | null>;
}

export function TagEditor({ tags, tagProvenance, tagRuleNames = {}, onSave, onCancel, sessionTitle, triggerRef }: TagEditorProps) {
  const [currentTags, setCurrentTags] = useState<string[]>([...tags]);
  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true, triggerRef);
  const [inputValue, setInputValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  // ux.md Surface 3: at most one tag's removal confirmation is expanded at a time.
  const [pendingRemoval, setPendingRemoval] = useState<string | null>(null);
  const keepButtonRef = useRef<HTMLButtonElement>(null);

  const handleAddTag = () => {
    const trimmedTag = inputValue.trim();

    if (!trimmedTag) {
      setError("Tag cannot be empty");
      return;
    }

    if (currentTags.includes(trimmedTag)) {
      setError("Tag already exists");
      return;
    }

    setCurrentTags([...currentTags, trimmedTag]);
    setInputValue("");
    setError(null);
  };

  const removeImmediately = (tagToRemove: string) => {
    setCurrentTags((prev) => prev.filter((tag) => tag !== tagToRemove));
    setError(null);
    setPendingRemoval(null);
  };

  const handleRemoveClick = (tagToRemove: string) => {
    if (hasProvenance(tagToRemove, tagProvenance)) {
      setPendingRemoval(tagToRemove);
      // Focus the non-destructive default (WCAG 2.4.3, ux.md AC13).
      setTimeout(() => keepButtonRef.current?.focus(), 0);
      return;
    }
    removeImmediately(tagToRemove);
  };

  const handleKeepRemoval = () => setPendingRemoval(null);

  const handleKeyPress = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAddTag();
    } else if (e.key === "Escape") {
      onCancel();
    }
  };

  const handleSave = () => {
    onSave(currentTags);
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="tag-editor-title"
        ref={modalRef}
      >
        <div className={styles.header}>
          <h2 className={styles.title} id="tag-editor-title">Edit Tags</h2>
          <p className={styles.subtitle}>{sessionTitle}</p>
        </div>

        <div className={styles.body}>
          <div className={styles.inputSection}>
            <div className={styles.inputRow}>
              <input
                type="text"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                onKeyDown={handleKeyPress}
                placeholder="Add a new tag..."
                className={styles.input}
                autoFocus
              />
              <button type="button" onClick={handleAddTag} className={styles.addButton}>
                Add
              </button>
            </div>
            {error && <p className={styles.error}>{error}</p>}
          </div>

          <div className={styles.tagsSection}>
            <h3 className={styles.sectionTitle}>Current Tags ({currentTags.length})</h3>
            {currentTags.length === 0 ? (
              <p className={styles.emptyMessage}>No tags yet. Add your first tag above.</p>
            ) : (
              <div className={styles.tagsList}>
                {currentTags.map((tag) => {
                  const isUnclassified = tag === UNCLASSIFIED_TAG;
                  const isPendingRemoval = pendingRemoval === tag;

                  if (isPendingRemoval) {
                    return (
                      <div
                        key={tag}
                        className={`${styles.tagItem} ${styles.tagItemConfirming}`}
                        data-testid={`tag-confirm-row-${tag}`}
                        onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); handleKeepRemoval(); } }}
                      >
                        <span className={styles.tagText}>{tag}</span>
                        <span className={styles.confirmCaption}>
                          {removalConfirmationText(tag, tagProvenance, tagRuleNames)}
                        </span>
                        <button
                          ref={keepButtonRef}
                          type="button"
                          onClick={handleKeepRemoval}
                          className={styles.keepButton}
                        >
                          Keep
                        </button>
                        <button
                          type="button"
                          onClick={() => removeImmediately(tag)}
                          className={styles.removeAnywayButton}
                        >
                          Remove anyway
                        </button>
                      </div>
                    );
                  }

                  return (
                    <div key={tag} className={`${styles.tagItem} ${isUnclassified ? styles.tagItemUnclassified : ""}`}>
                      <span className={styles.tagText}>{tag}</span>
                      {isUnclassified ? (
                        <span className={styles.unclassifiedCaption}>
                          Removed automatically once classification succeeds
                        </span>
                      ) : (
                        <button
                          onClick={() => handleRemoveClick(tag)}
                          className={styles.removeButton}
                          title={`Remove tag "${tag}"`}
                          aria-label={`Remove tag ${tag}`}
                        >
                          ×
                        </button>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        <div className={styles.footer}>
          <button type="button" onClick={onCancel} className={styles.cancelButton}>
            Cancel
          </button>
          <button type="button" onClick={handleSave} className={styles.saveButton}>
            Save Tags
          </button>
        </div>
      </div>
    </div>
  );
}
