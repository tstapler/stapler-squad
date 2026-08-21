"use client";

import { VCSStatus, VCSType } from "@/gen/session/v1/types_pb";
import * as styles from "./VcsStatusDisplay.css";

interface VcsStatusDisplayProps {
  status: VCSStatus;
}

function vcsTypeName(type: VCSType): string {
  switch (type) {
    case VCSType.VCS_TYPE_GIT:
      return "Git";
    case VCSType.VCS_TYPE_JUJUTSU:
      return "Jujutsu";
    default:
      return "VCS";
  }
}

/**
 * Pure display component for VCSStatus. Accepts an already-fetched VCSStatus
 * proto and renders branch, clean/dirty state, file counts, and remote sync.
 *
 * Used by both the history detail panel (where status comes from
 * GetClaudeHistoryDetail) and the session VCS tab (where it comes from
 * GetVCSStatus). Keep data-fetching out of this component.
 */
export function VcsStatusDisplay({ status }: VcsStatusDisplayProps) {
  return (
    <div className={styles.container}>
      {/* Branch */}
      <div className={styles.row}>
        <span className={styles.label}>Branch:</span>
        <span className={styles.branch}>
          ⎇ {status.branch || "(detached)"}
        </span>
      </div>

      {/* Clean / dirty */}
      <div className={styles.row}>
        <span className={styles.label}>Status:</span>
        <span className={status.isClean ? styles.clean : styles.dirty}>
          {status.isClean ? "✓ Clean" : "✦ Uncommitted changes"}
        </span>
      </div>

      {/* File change counts */}
      {!status.isClean && (
        <div className={styles.changes}>
          {status.hasStaged && (
            <span className={styles.stat} title="Staged files">
              +{status.stagedFiles.length} staged
            </span>
          )}
          {status.hasUnstaged && (
            <span className={styles.stat} title="Modified files">
              ~{status.unstagedFiles.length} modified
            </span>
          )}
          {status.hasUntracked && (
            <span className={styles.stat} title="Untracked files">
              ?{status.untrackedFiles.length} untracked
            </span>
          )}
          {status.hasConflicts && (
            <span className={styles.stat} title="Conflicted files">
              ⚠ {status.conflictFiles.length} conflicts
            </span>
          )}
        </div>
      )}

      {/* Remote ahead/behind */}
      {(status.aheadBy > 0 || status.behindBy > 0) && (
        <div className={styles.row}>
          <span className={styles.label}>Remote:</span>
          <span className={styles.remote}>
            {status.aheadBy > 0 && `↑${status.aheadBy} ahead`}
            {status.aheadBy > 0 && status.behindBy > 0 && " · "}
            {status.behindBy > 0 && `↓${status.behindBy} behind`}
          </span>
        </div>
      )}
    </div>
  );
}
