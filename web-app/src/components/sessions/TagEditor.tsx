"use client";

import { useState, KeyboardEvent, useRef, RefObject } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
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
  /** Stable session ID used for the manual AI re-classification call. Absent = button hidden. */
  sessionId?: string;
  /** Called with the server-applied tags after a manual re-classification succeeds. */
  onReclassified?: (tags: string[]) => void;
}

export function TagEditor({ tags, tagProvenance, tagRuleNames = {}, onSave, onCancel, sessionTitle, triggerRef, sessionId, onReclassified }: TagEditorProps) {
  const [currentTags, setCurrentTags] = useState<string[]>([...tags]);
  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true, triggerRef);
  const [inputValue, setInputValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [reclassifying, setReclassifying] = useState(false);
  const [reclassifyStatus, setReclassifyStatus] = useState<string | null>(null);
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

  // Manual AI re-classification: successfully classified sessions are never re-tagged
  // automatically (poller classify-once), so this explicit user action is the only path
  // that re-runs the LLM classifier. The server applies the result; the returned tags
  // refresh this dialog and propagate to the parent via onReclassified.
  const handleReclassify = async () => {
    if (!sessionId || reclassifying) return;
    setReclassifying(true);
    setReclassifyStatus(null);
    setError(null);
    try {
      const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
      const client = createClient(SessionService, transport);
      const response = await client.reclassifySessionTags({ sessionId });
      const applied = [...response.tags];
      setCurrentTags(applied);
      setReclassifyStatus(applied.length > 0 ? `AI classification applied: ${applied.join(", ")}` : "AI classification ran with no tags");
      onReclassified?.(applied);
    } catch (e) {
      setError(e instanceof Error ? `Re-classification failed: ${e.message}` : "Re-classification failed");
    } finally {
      setReclassifying(false);
    }
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

        {reclassifyStatus && <p className={styles.unclassifiedCaption} role="status">{reclassifyStatus}</p>}

        <div className={styles.footer}>
          {sessionId && (
            <button
              type="button"
              onClick={handleReclassify}
              className={styles.cancelButton}
              disabled={reclassifying}
              data-testid="tag-reclassify-button"
              title="Re-run AI tag classification for this session"
            >
              {reclassifying ? "Classifying…" : "Re-run AI classification"}
            </button>
          )}
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
