"use client";

import { useState, useCallback } from "react";
import { UnfinishedWorktree, ScanStatus } from "@/gen/session/v1/types_pb";
import { UnfinishedItemDetail } from "./UnfinishedItemDetail";
import * as styles from "./UnfinishedItem.css";

interface UnfinishedItemProps {
  worktree: UnfinishedWorktree;
  isExpanded: boolean;
  onToggleExpand: () => void;
  onDismiss: (repoPath: string, branch: string) => void;
  onSnooze: (repoPath: string, branch: string) => void;
}

/**
 * Card representing a single git worktree with unfinished work.
 * Shows branch name, abbreviated path, status chips, and hover-reveal action buttons.
 */
export function UnfinishedItem({
  worktree,
  isExpanded,
  onToggleExpand,
  onDismiss,
  onSnooze,
}: UnfinishedItemProps) {
  const isTimeout =
    worktree.scanStatus === ScanStatus.TIMEOUT ||
    worktree.scanStatus === ScanStatus.ERROR;

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onToggleExpand();
      }
      if (e.key === "Escape" && isExpanded) {
        onToggleExpand();
      }
    },
    [isExpanded, onToggleExpand]
  );

  const handleDismissClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDismiss(worktree.repoPath, worktree.branch);
  };

  const handleSnoozeClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onSnooze(worktree.repoPath, worktree.branch);
  };

  return (
    <div>
      <div
        role="button"
        tabIndex={0}
        aria-expanded={isExpanded}
        className={`${styles.card} ${isExpanded ? styles.cardExpanded : ""}`}
        onClick={onToggleExpand}
        onKeyDown={handleKeyDown}
        data-testid="unfinished-item"
      >
        <div className={styles.header}>
          <span className={styles.branch}>{worktree.branch}</span>
          <span className={styles.path}>{worktree.displayPath || worktree.worktreePath}</span>

          {isTimeout ? (
            <div className={styles.chips}>
              <span
                className={styles.chipTimeout}
                aria-label="scan timed out"
                title={worktree.scanErrorMsg || "Scan timed out"}
              >
                ⚠ Timeout
              </span>
            </div>
          ) : (
            <div className={styles.chips}>
              {worktree.hasUncommitted && (
                <span className={styles.chipUncommitted}>Uncommitted</span>
              )}
              {worktree.commitsAhead > 0 && (
                <span className={styles.chipAhead}>↑{worktree.commitsAhead}</span>
              )}
              {worktree.commitsBehind > 0 && (
                <span className={styles.chipBehind}>↓{worktree.commitsBehind}</span>
              )}
            </div>
          )}

          <div className={styles.actions}>
            <button
              className={styles.dismissBtn}
              onClick={handleDismissClick}
              aria-label="Dismiss this worktree"
              title="Dismiss"
            >
              ×
            </button>
            <button
              className={styles.actionBtn}
              onClick={handleSnoozeClick}
              aria-label="Snooze until next change"
              title="Snooze until next git change"
            >
              Snooze
            </button>
          </div>
        </div>
      </div>

      {isExpanded && (
        <UnfinishedItemDetail worktree={worktree} />
      )}
    </div>
  );
}
