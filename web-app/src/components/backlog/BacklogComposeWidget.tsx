"use client";
// +feature: backlog:compose-widget

import { useRef, useState } from "react";
import { useBacklogImageAttachment } from "@/lib/hooks/useBacklogImageAttachment";
import * as formStyles from "./BacklogItemForm.css";
import * as styles from "./BacklogComposeWidget.css";

interface BacklogComposeWidgetProps {
  /** Text the user had already typed before opening the widget. */
  initialText: string;
  /** Called with the trimmed text; the caller owns closing and background creation. */
  onSubmit: (text: string) => void;
  /** Receives the current draft so the caller can restore it to the omnibar input. */
  onCancel: (draft: string) => void;
}

export function BacklogComposeWidget({ initialText, onSubmit, onCancel }: BacklogComposeWidgetProps) {
  const [text, setText] = useState(initialText);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { error, uploading, onFileInputChange, onPaste } = useBacklogImageAttachment({
    textareaRef,
    setValue: setText,
  });

  const canSubmit = text.trim() !== "" && !uploading;
  const submit = () => {
    if (canSubmit) onSubmit(text.trim());
  };

  return (
    <div
      className={styles.root}
      data-testid="backlog-compose-widget"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          onCancel(text);
        } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          submit();
        }
      }}
    >
      <h2 id="backlog-compose-title" className={styles.title}>
        Create backlog item
      </h2>
      <label htmlFor="backlog-compose-textarea" className={formStyles.label}>
        Describe the work
      </label>
      <textarea
        id="backlog-compose-textarea"
        ref={textareaRef}
        className={`${formStyles.textarea} ${styles.textarea}`}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onPaste={onPaste}
        placeholder="What needs to be done? Paste or attach screenshots. Ctrl/⌘+Enter to create."
        autoFocus
        data-testid="backlog-compose-textarea"
      />
      {error && (
        <span className={formStyles.errorMessage} role="alert" data-testid="backlog-compose-error">
          {error}
        </span>
      )}
      <div className={styles.actions}>
        <button
          type="button"
          className={styles.attachButton}
          onClick={() => fileInputRef.current?.click()}
          disabled={uploading}
          data-testid="backlog-compose-attach"
        >
          {uploading ? "Uploading…" : <><span aria-hidden="true">📎</span> Attach image</>}
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          className={formStyles.hiddenFileInput}
          onChange={onFileInputChange}
          data-testid="backlog-compose-attach-input"
        />
        <span className={styles.spacer} />
        <button
          type="button"
          className={`${formStyles.cancelButton} ${styles.touchTarget}`}
          onClick={() => onCancel(text)}
          data-testid="backlog-compose-cancel"
        >
          Cancel
        </button>
        <button
          type="button"
          className={`${formStyles.submitButton} ${styles.touchTarget}`}
          onClick={submit}
          disabled={!canSubmit}
          data-testid="backlog-compose-submit"
        >
          Create
        </button>
      </div>
    </div>
  );
}
