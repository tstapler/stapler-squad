"use client";

import { useCallback, useState } from "react";
import { Check, Copy, FolderOpen, GitBranch, Users } from "lucide-react";
import type { VcsWidgetData, VcsWidgetMode } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetHeader.css";

interface VcsWidgetHeaderProps {
  data: VcsWidgetData;
  mode: VcsWidgetMode;
  /** Worktree path row (copy + browse) only renders in full mode when set. */
  worktreePath?: string;
  /**
   * Surfaces BacklogItemDetail.tsx's "most recent work session" heuristic
   * ambiguity when more than one session is currently active for this item —
   * the heuristic itself is unchanged, only its ambiguity becomes visible.
   */
  activeSessionCount?: number;
  onBrowseFiles?: () => void;
}

export function VcsWidgetHeader({
  data,
  mode,
  worktreePath,
  activeSessionCount,
  onBrowseFiles,
}: VcsWidgetHeaderProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    if (!worktreePath) return;
    navigator.clipboard
      .writeText(worktreePath)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      })
      .catch((err) => {
        console.warn("[VcsWidgetHeader] clipboard write failed", err);
      });
  }, [worktreePath]);

  const showWorktreeRow = mode === "full" && !!worktreePath;
  const showActiveSessions = mode === "full" && !!activeSessionCount && activeSessionCount > 1;

  return (
    <div className={styles.header({ mode })}>
      <div className={styles.row}>
        <GitBranch aria-hidden="true" size={14} className={styles.icon} />
        <span className={styles.branch}>{data.branch || "(detached)"}</span>
        <span className={data.isClean ? styles.clean : styles.dirty}>
          {data.isClean ? "Clean" : "Uncommitted changes"}
        </span>
        {data.branchExists === false && <span className={styles.stat}>(deleted — already merged)</span>}
      </div>

      {(data.aheadOfMain > 0 || data.behindMain > 0) && (
        <div className={styles.row}>
          {data.aheadOfMain > 0 && <span className={styles.stat}>↑{data.aheadOfMain} ahead</span>}
          {data.behindMain > 0 && <span className={styles.stat}>↓{data.behindMain} behind</span>}
        </div>
      )}

      {showWorktreeRow && (
        <div className={styles.worktreeRow}>
          <code className={styles.worktreePath}>{worktreePath}</code>
          <button
            type="button"
            className={styles.iconButton}
            onClick={handleCopy}
            aria-label="Copy worktree path"
            title="Copy worktree path"
          >
            {copied ? <Check aria-hidden="true" size={14} /> : <Copy aria-hidden="true" size={14} />}
          </button>
          <button
            type="button"
            className={styles.iconButton}
            onClick={onBrowseFiles}
            aria-label="Browse files in this worktree"
            title="Browse files in this worktree"
          >
            <FolderOpen aria-hidden="true" size={14} />
          </button>
        </div>
      )}

      {showActiveSessions && (
        <div className={styles.activeSessions}>
          <Users aria-hidden="true" size={14} />
          <span>{activeSessionCount} active sessions</span>
        </div>
      )}
    </div>
  );
}
