"use client";

import { useState, KeyboardEvent, useRef } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import styles from "./TagEditor.module.css";

interface TagEditorProps {
  tags: string[];
  onSave: (tags: string[]) => void;
  onCancel: () => void;
  sessionTitle: string;
}

export function TagEditor({ tags, onSave, onCancel, sessionTitle }: TagEditorProps) {
  const [currentTags, setCurrentTags] = useState<string[]>([...tags]);
  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true);
  const [inputValue, setInputValue] = useState("");
  const [error, setError] = useState<string | null>(null);

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

  const handleRemoveTag = (tagToRemove: string) => {
    setCurrentTags(currentTags.filter(tag => tag !== tagToRemove));
    setError(null);
  };

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
              <button onClick={handleAddTag} className={styles.addButton}>
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
                {currentTags.map((tag) => (
                  <div key={tag} className={styles.tagItem}>
                    <span className={styles.tagText}>{tag}</span>
                    <button
                      onClick={() => handleRemoveTag(tag)}
                      className={styles.removeButton}
                      title={`Remove tag "${tag}"`}
                      aria-label={`Remove tag ${tag}`}
                    >
                      ×
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className={styles.footer}>
          <button onClick={onCancel} className={styles.cancelButton}>
            Cancel
          </button>
          <button onClick={handleSave} className={styles.saveButton}>
            Save Tags
          </button>
        </div>
      </div>
    </div>
  );
}
