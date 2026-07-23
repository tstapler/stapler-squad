"use client";

import { useState } from "react";
import type { CommitSummary, VcsWidgetMode } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetCommitList.css";

interface VcsWidgetCommitListProps {
  commits: CommitSummary[];
  mode: VcsWidgetMode;
}

const COMPACT_CAP = 5;
const FULL_CAP = 20;

function CommitRow({ commit }: { commit: CommitSummary }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <li className={styles.commitRow}>
      <button
        type="button"
        className={styles.commitButton}
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
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

export function VcsWidgetCommitList({ commits, mode }: VcsWidgetCommitListProps) {
  const [showAll, setShowAll] = useState(false);
  if (commits.length === 0) return null;

  const cap = mode === "compact" ? COMPACT_CAP : FULL_CAP;
  const capped = mode === "full" && !showAll && commits.length > cap;
  const visible = capped ? commits.slice(0, cap) : mode === "compact" ? commits.slice(0, cap) : commits;

  return (
    <div className={styles.container}>
      <ul className={styles.list({ mode })} aria-label="Commits">
        {visible.map((c, i) => (
          <CommitRow key={c.sha || i} commit={c} />
        ))}
      </ul>
      {capped && (
        <button type="button" className={styles.showAllButton} onClick={() => setShowAll(true)}>
          Show all {commits.length} commits
        </button>
      )}
    </div>
  );
}
