"use client";

import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import * as styles from "./OmnibarRepoResult.css";

interface OmnibarRepoResultProps {
  entry: PathHistoryEntry;
  sessionCount?: number;
  isHighlighted: boolean;
  id: string;
  onClick: (path: string) => void;
}

function relativeTime(timestamp: number): string {
  const diffMs = Date.now() - timestamp;
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return `${Math.floor(diffDays / 7)}w ago`;
}

export function OmnibarRepoResult({
  entry,
  sessionCount,
  isHighlighted,
  id,
  onClick,
}: OmnibarRepoResultProps) {
  const parts = entry.path.split("/").filter(Boolean);
  const repoName = parts[parts.length - 1] ?? entry.path;
  const parentPath = parts.slice(-3, -1).join("/"); // up to 2 parent segments before the repo name

  const rowClass = [styles.row, isHighlighted ? styles.rowHighlighted : ""]
    .filter(Boolean)
    .join(" ");

  return (
    <li
      role="option"
      aria-selected={isHighlighted}
      id={id}
      className={rowClass}
      onMouseDown={(e) => {
        e.preventDefault(); // prevent focus loss on click
        onClick(entry.path);
      }}
    >
      <span className={styles.folderIcon} aria-hidden="true">
        <svg
          width="16"
          height="16"
          viewBox="0 0 16 16"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M1.5 3.5C1.5 2.948 1.948 2.5 2.5 2.5H6.086C6.35 2.5 6.602 2.605 6.789 2.793L7.5 3.5H13.5C14.052 3.5 14.5 3.948 14.5 4.5V12.5C14.5 13.052 14.052 13.5 13.5 13.5H2.5C1.948 13.5 1.5 13.052 1.5 12.5V3.5Z"
            stroke="currentColor"
            strokeWidth="1.25"
            strokeLinejoin="round"
          />
        </svg>
      </span>

      <span className={styles.content}>
        <span className={styles.pathLine}>
          {parentPath && (
            <>
              <span className={styles.parentPath}>{parentPath}</span>
              <span className={styles.separator}>/</span>
            </>
          )}
          <span className={styles.repoName}>{repoName}</span>
        </span>
        {sessionCount !== undefined && sessionCount > 0 && (
          <span className={styles.sessionCount}>
            {sessionCount} {sessionCount === 1 ? "session" : "sessions"}
          </span>
        )}
      </span>

      <span className={styles.relativeTime}>{relativeTime(entry.lastUsed)}</span>
    </li>
  );
}
