"use client";

import { useState } from "react";
import type { CommitSummary, VcsWidgetMode } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetCommitList.css";

interface VcsWidgetCommitListProps {
  commits: CommitSummary[];
  mode: VcsWidgetMode;
  /** True when `commits` was capped server-side before reaching the branch's full count. */
  truncated?: boolean;
  /** True when the commit list failed to load — renders a notice instead of the (empty) list. */
  unavailable?: boolean;
}

const COMPACT_CAP = 5;
const FULL_CAP = 20;

function CommitRow({ commit, isHead = false }: { commit: CommitSummary; isHead?: boolean }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <li className={styles.commitRow}>
      <button
        type="button"
        className={isHead ? `${styles.commitButton} ${styles.headCommitButton}` : styles.commitButton}
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        data-head={isHead || undefined}
      >
        <span className={expanded ? styles.summaryExpanded : styles.summaryCollapsed}>
          {commit.summary}
        </span>
      </button>
      {expanded && (
        <div className={styles.expanded} data-testid="commit-row-expanded">
          {commit.summary}
        </div>
      )}
    </li>
  );
}

export function VcsWidgetCommitList({ commits, mode, truncated, unavailable }: VcsWidgetCommitListProps) {
  const [showAll, setShowAll] = useState(false);
  if (commits.length === 0 && !unavailable) return null;

  if (commits.length === 0 && unavailable) {
    return (
      <div className={styles.container}>
        <span className={styles.neutralNotice}>Couldn&apos;t load commit history — try refreshing.</span>
      </div>
    );
  }

  const cap = mode === "compact" ? COMPACT_CAP : FULL_CAP;
  const capped = mode === "full" && !showAll && commits.length > cap;
  const visible = capped ? commits.slice(0, cap) : mode === "compact" ? commits.slice(0, cap) : commits;
  const showTruncationNote = !!truncated && mode === "full" && !capped;

  return (
    <div className={styles.container}>
      <ul className={styles.list({ mode })} aria-label="Commits">
        {visible.map((c, i) => (
          <CommitRow key={c.sha || i} commit={c} isHead={mode === "full" && i === 0} />
        ))}
      </ul>
      {capped && (
        <button type="button" className={styles.showAllButton} onClick={() => setShowAll(true)}>
          Show all {commits.length} commits
        </button>
      )}
      {showTruncationNote && (
        <span className={styles.neutralNotice}>
          Showing the {commits.length} most recent commits — there may be more.
        </span>
      )}
    </div>
  );
}
