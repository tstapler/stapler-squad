"use client";

import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { SessionType } from "@/gen/session/v1/types_pb";
import { useEffect, useRef, useState } from "react";
import * as styles from "./ForkModal.css";

export interface ForkParams {
  title: string;
  path: string;
  branch: string;
  sessionType: SessionType;
  forkAtMessage: number;
}

interface ForkModalProps {
  entry: ClaudeHistoryEntry | null;
  submitting: boolean;
  error: string | null;
  onClose: () => void;
  onSubmit: (params: ForkParams) => void;
}

export function ForkModal({ entry, submitting, error, onClose, onSubmit }: ForkModalProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [title, setTitle] = useState("");
  const [path, setPath] = useState("");
  const [branch, setBranch] = useState("");
  const [sessionType, setSessionType] = useState<SessionType>(SessionType.DIRECTORY);
  const [forkAt, setForkAt] = useState(0);

  // Open/close dialog and populate fields when entry changes.
  useEffect(() => {
    if (!entry) { dialogRef.current?.close(); return; }
    setTitle(entry.name.substring(0, 60));
    setPath(entry.project);
    setBranch(`resume/${entry.id.substring(0, 8)}`);
    setForkAt(0);
    setSessionType(SessionType.DIRECTORY);
    dialogRef.current?.showModal();
  }, [entry]);

  // Close on Escape via dialog native behavior — also fire onClose.
  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    const handler = () => onClose();
    el.addEventListener("close", handler);
    return () => el.removeEventListener("close", handler);
  }, [onClose]);

  if (!entry) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit({ title: title.trim(), path, branch: branch.trim(), sessionType, forkAtMessage: forkAt });
  };

  const maxMsg = entry.messageCount;

  return (
    <dialog
      ref={dialogRef}
      className={styles.dialog}
      aria-labelledby="fork-modal-title"
      onClick={(e) => { if (e.target === dialogRef.current) onClose(); }}
    >
      <form onSubmit={handleSubmit} className={styles.form}>
        <h2 id="fork-modal-title" className={styles.title}>Fork Conversation</h2>
        <p className={styles.subtitle}>
          Create a new session branching from this conversation.
        </p>

        {error && <div className={styles.errorBanner}>{error}</div>}

        <div className={styles.field}>
          <label className={styles.label} htmlFor="fork-title">Session name</label>
          <input
            id="fork-title"
            type="text"
            className={styles.input}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            required
            autoFocus
            maxLength={100}
          />
        </div>

        <div className={styles.field}>
          <label className={styles.label} htmlFor="fork-path">Directory</label>
          <input
            id="fork-path"
            type="text"
            className={styles.input}
            value={path}
            onChange={(e) => setPath(e.target.value)}
            required
          />
        </div>

        <div className={styles.field}>
          <fieldset className={styles.radioGroup}>
            <legend className={styles.label}>Session type</legend>
            <label className={styles.radioLabel}>
              <input type="radio" name="session-type" value="directory"
                checked={sessionType === SessionType.DIRECTORY}
                onChange={() => setSessionType(SessionType.DIRECTORY)} />
              Open in directory
            </label>
            <label className={styles.radioLabel}>
              <input type="radio" name="session-type" value="worktree"
                checked={sessionType === SessionType.NEW_WORKTREE}
                onChange={() => setSessionType(SessionType.NEW_WORKTREE)} />
              New git worktree
            </label>
          </fieldset>
        </div>

        {sessionType === SessionType.NEW_WORKTREE && (
          <div className={styles.field}>
            <label className={styles.label} htmlFor="fork-branch">Branch name</label>
            <input
              id="fork-branch"
              type="text"
              className={styles.input}
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            />
          </div>
        )}

        {maxMsg > 0 && (
          <div className={styles.field}>
            <label className={styles.label} htmlFor="fork-at">
              Fork at message {forkAt === 0 ? "(all messages)" : `${forkAt} of ${maxMsg}`}
            </label>
            <input
              id="fork-at"
              type="range"
              min={0}
              max={maxMsg}
              value={forkAt}
              onChange={(e) => setForkAt(Number(e.target.value))}
              className={styles.slider}
            />
            <div className={styles.sliderLabels}>
              <span>Start</span>
              <span>All {maxMsg} messages</span>
            </div>
          </div>
        )}

        <div className={styles.actions}>
          <button type="button" onClick={onClose} className="btn btn-secondary" disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary" disabled={submitting || !title.trim()}>
            {submitting ? "Forking…" : "🍴 Fork Session"}
          </button>
        </div>
      </form>
    </dialog>
  );
}
