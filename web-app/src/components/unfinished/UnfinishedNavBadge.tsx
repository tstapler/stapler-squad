"use client";

import { useUnfinishedWork } from "@/lib/hooks/useUnfinishedWork";
import * as styles from "./UnfinishedNavBadge.css";

interface UnfinishedNavBadgeProps {
  inline?: boolean;
}

/**
 * Navigation badge that shows the count of unfinished worktrees.
 * Hidden when count is 0.
 */
export function UnfinishedNavBadge({ inline = false }: UnfinishedNavBadgeProps) {
  const { worktrees } = useUnfinishedWork();
  const count = worktrees.length;

  if (count === 0) return null;

  return (
    <span
      className={`${styles.badge} ${inline ? styles.inline : ""}`}
      data-testid="unfinished-nav-badge"
      aria-label={`${count} unfinished item${count !== 1 ? "s" : ""}`}
    >
      {count}
    </span>
  );
}
