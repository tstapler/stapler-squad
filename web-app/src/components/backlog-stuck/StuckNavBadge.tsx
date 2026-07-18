"use client";

import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import * as styles from "./StuckNavBadge.css";

interface StuckNavBadgeProps {
  inline?: boolean;
}

/**
 * Navigation badge showing the count of open stuck backlog items.
 * Hidden when count is 0 (mirrors UnfinishedNavBadge.tsx). Before the first
 * successful fetch resolves (no prior last-known count to fall back on), it
 * renders a neutral pulse/skeleton placeholder rather than a "0" or nothing
 * that could be misread as a confirmed empty state (design/ux.md AC 24).
 */
export function StuckNavBadge({ inline = false }: StuckNavBadgeProps) {
  const { items, lastFetched } = useStuckBacklogItems();

  if (lastFetched === null) {
    return (
      <span
        className={`${styles.skeleton} ${inline ? styles.inline : ""}`}
        data-testid="stuck-nav-badge-loading"
        aria-label="Checking for stuck items"
        aria-hidden="false"
      />
    );
  }

  const count = items.length;
  if (count === 0) return null;

  return (
    <span
      className={`${styles.badge} ${inline ? styles.inline : ""}`}
      data-testid="stuck-nav-badge"
      aria-label={`${count} item${count !== 1 ? "s" : ""} stuck`}
    >
      {count}
    </span>
  );
}
