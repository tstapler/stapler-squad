"use client";

import { useState, useCallback } from "react";
import { UnfinishedWorktree } from "@/gen/session/v1/types_pb";
import { UnfinishedItem } from "./UnfinishedItem";
import * as styles from "./UnfinishedRepoGroup.css";

interface UnfinishedRepoGroupProps {
  repoName: string;
  worktrees: UnfinishedWorktree[];
  onDismiss: (repoPath: string, branch: string) => void;
  onSnooze: (repoPath: string, branch: string) => void;
}

/**
 * Collapsible group showing all worktrees for a single repository.
 */
export function UnfinishedRepoGroup({
  repoName,
  worktrees,
  onDismiss,
  onSnooze,
}: UnfinishedRepoGroupProps) {
  const [isOpen, setIsOpen] = useState(true);
  const [expandedKey, setExpandedKey] = useState<string | null>(null);

  const toggleGroup = useCallback(() => setIsOpen((v) => !v), []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggleGroup();
    }
  };

  return (
    <section className={styles.group} aria-label={`Repository: ${repoName}`}>
      <div
        role="button"
        tabIndex={0}
        className={styles.header}
        onClick={toggleGroup}
        onKeyDown={handleKeyDown}
        aria-expanded={isOpen}
        aria-controls={`repo-group-${repoName}`}
      >
        <span
          className={`${styles.chevron} ${isOpen ? styles.chevronExpanded : ""}`}
          aria-hidden="true"
        >
          ▶
        </span>
        <span className={styles.repoName}>{repoName}</span>
        <span className={styles.count}>{worktrees.length}</span>
      </div>

      {isOpen && (
        <div id={`repo-group-${repoName}`} className={styles.itemList}>
          {worktrees.map((wt) => {
            const key = `${wt.repoPath}|${wt.branch}`;
            return (
              <UnfinishedItem
                key={key}
                worktree={wt}
                isExpanded={expandedKey === key}
                onToggleExpand={() =>
                  setExpandedKey((prev) => (prev === key ? null : key))
                }
                onDismiss={onDismiss}
                onSnooze={onSnooze}
              />
            );
          })}
        </div>
      )}
    </section>
  );
}
