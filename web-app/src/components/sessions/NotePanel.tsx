"use client";

// +feature: session-notes-panel
import { useEffect, useRef, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { markdownBody } from "@/components/backlog/markdownBody.css";
import {
  panelContainer,
  summary,
  body,
  emptyText,
  addButton,
  textarea,
  hint,
  actionsRow,
  saveButton,
  cancelButton,
  editButton,
  errorText,
  renderedHeading,
} from "./NotePanel.css";

// Cross-reference: matches session.MaxNoteLength (session/instance.go) and the ent
// schema's field.Text("note").MaxLen(10000) (session/ent/schema/session.go) — kept in
// sync manually across all three; see plan.md's Unresolved Question 1.
const NOTE_MAX_LENGTH = 10000;

const noteEncoder = new TextEncoder();

// A user-typed `# Heading` must never become a real page-level heading tag — even
// a remapped one (e.g. h1->h5) still creates a heading-order skip against this
// page's actual h2 session title, tripping the repo's axe/Lighthouse heading-order
// CI gate. Render every markdown heading level as a styled non-heading element instead.
function RenderedHeading({ children }: { children?: ReactNode }) {
  return <div className={renderedHeading}>{children}</div>;
}

const headingComponents = {
  h1: RenderedHeading,
  h2: RenderedHeading,
  h3: RenderedHeading,
  h4: RenderedHeading,
  h5: RenderedHeading,
  h6: RenderedHeading,
};

export interface NotePanelProps {
  note: string;
  onSave: (note: string) => Promise<void>;
}

/**
 * NotePanel renders a session's free-form markdown note, with an edit/read-mode
 * toggle mirroring GoalPanel's file structure. Explicit Save/Cancel — no
 * autosave-on-blur convention exists elsewhere in this codebase.
 */
export function NotePanel({ note, onSave }: NotePanelProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [draftValue, setDraftValue] = useState(note);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  // Restores focus to the Edit/Add-note button on exiting edit mode (Save or
  // Cancel), per design/ux.md Surface 3 AC11 — focus must never fall back to
  // <body>. Same ref serves both buttons since only one renders at a time.
  const editButtonRef = useRef<HTMLButtonElement>(null);

  // Only re-sync local state from the note prop when not editing, so a
  // stream-driven update from another tab doesn't clobber an in-progress edit —
  // mirrors the guard at SessionDetailView.tsx's isEditingCategory sync.
  useEffect(() => {
    if (!isEditing) {
      setDraftValue(note);
    }
  }, [note, isEditing]);

  // Restore focus to the Edit/Add-note button after exiting edit mode (Save or
  // Cancel) — design/ux.md Surface 3 AC11. Runs after render so the button has
  // already remounted; wasEditingRef guards against firing on initial mount.
  const wasEditingRef = useRef(false);
  useEffect(() => {
    if (wasEditingRef.current && !isEditing) {
      editButtonRef.current?.focus();
    }
    wasEditingRef.current = isEditing;
  }, [isEditing]);

  const startEditing = () => {
    setDraftValue(note);
    setSaveError(null);
    setIsEditing(true);
  };

  const cancel = () => {
    setDraftValue(note);
    setSaveError(null);
    setIsEditing(false);
  };

  // The backend caps by UTF-8 byte length (session.MaxNoteLength), but the
  // textarea's native maxLength counts UTF-16 code units — a multi-byte-heavy
  // note (CJK, emoji) can pass the char cap yet still fail server-side. Track
  // bytes so the hint and the save guard below are both byte-accurate.
  const noteByteLength = noteEncoder.encode(draftValue).length;

  const save = async () => {
    if (noteByteLength > NOTE_MAX_LENGTH) {
      setSaveError(
        `Note is too long (${noteByteLength}/${NOTE_MAX_LENGTH} bytes) — some characters take more than one byte`,
      );
      return;
    }
    setSaving(true);
    setSaveError(null);
    try {
      await onSave(draftValue);
      setIsEditing(false);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Failed to save note");
    } finally {
      setSaving(false);
    }
  };

  const hasNote = note.trim().length > 0;

  return (
    <details open className={panelContainer} data-testid="session-note-panel">
      <summary className={summary}>Notes</summary>
      <div className={body}>
        {isEditing ? (
          <>
            <textarea
              className={textarea}
              value={draftValue}
              onChange={(e) => setDraftValue(e.target.value)}
              maxLength={NOTE_MAX_LENGTH}
              autoFocus
              aria-label="Session note (markdown)"
              aria-describedby={saveError ? "session-note-hint session-note-error" : "session-note-hint"}
              data-testid="session-note-textarea"
            />
            {saveError && (
              <p id="session-note-error" className={errorText} role="alert" aria-live="assertive">
                {saveError}
              </p>
            )}
            <p id="session-note-hint" className={hint}>
              Markdown supported. {noteByteLength}/{NOTE_MAX_LENGTH}
            </p>
            <div className={actionsRow}>
              <button
                className={saveButton}
                onClick={save}
                disabled={saving}
                data-testid="session-note-save-button"
              >
                Save
              </button>
              <button className={cancelButton} onClick={cancel} disabled={saving}>
                Cancel
              </button>
            </div>
          </>
        ) : hasNote ? (
          <>
            <div className={markdownBody} data-testid="session-note-rendered">
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={headingComponents}>
                {note}
              </ReactMarkdown>
            </div>
            <button ref={editButtonRef} className={editButton} onClick={startEditing}>
              Edit
            </button>
          </>
        ) : (
          <>
            <p className={emptyText}>No notes yet — leave yourself a reminder about this session.</p>
            <button ref={editButtonRef} className={addButton} onClick={startEditing}>
              Add note
            </button>
          </>
        )}
      </div>
    </details>
  );
}
