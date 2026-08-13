"use client";

import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import type { SessionSearchResult } from "@/lib/hooks/useSessionSearch";
import * as styles from "./OmnibarSessionResult.css";

interface OmnibarSessionResultProps {
  result: SessionSearchResult;
  isHighlighted: boolean;
  id: string;
  onClick: (session: Session) => void;
  onClone?: (session: Session) => void;
  onOpenInNewPane?: (session: Session) => void;
}

function statusDotVariant(
  status: SessionStatus
): keyof typeof styles.dotVariants {
  switch (status) {
    case SessionStatus.RUNNING:
      return "running";
    case SessionStatus.PAUSED:
      return "paused";
    case SessionStatus.LOADING:
    case SessionStatus.NEEDS_APPROVAL:
      return "active";
    default:
      return "default";
  }
}

function statusLabel(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.RUNNING:
      return "Running";
    case SessionStatus.PAUSED:
      return "Paused";
    case SessionStatus.LOADING:
      return "Loading";
    case SessionStatus.NEEDS_APPROVAL:
      return "Needs approval";
    case SessionStatus.READY:
      return "Ready";
    default:
      return "Unknown";
  }
}

/**
 * Returns the last 2 path segments, e.g.:
 *   /Users/tyler/projects/auth  →  projects/auth
 *   /Users/tyler/auth           →  tyler/auth
 *   auth                        →  auth
 */
function shortPath(fullPath: string): string {
  const parts = fullPath.split("/").filter(Boolean);
  if (parts.length <= 2) return parts.join("/");
  return parts.slice(-2).join("/");
}

export function OmnibarSessionResult({
  result,
  isHighlighted,
  id,
  onClick,
  onClone,
  onOpenInNewPane,
}: OmnibarSessionResultProps) {
  const { session } = result;
  const dotVariant = statusDotVariant(session.status);

  const rowClassName = [styles.row, isHighlighted ? styles.rowHighlighted : ""]
    .filter(Boolean)
    .join(" ");

  return (
    <li
      id={id}
      className={rowClassName}
      role="option"
      aria-selected={isHighlighted}
      onMouseDown={(e) => {
        e.preventDefault();
        if (e.altKey && onOpenInNewPane) {
          onOpenInNewPane(session);
        } else {
          onClick(session);
        }
      }}
    >
      <span className={styles.dotWrapper}>
        <span
          className={`${styles.dot} ${styles.dotVariants[dotVariant]}`}
          aria-label={statusLabel(session.status)}
        />
      </span>

      <span className={styles.content}>
        <span className={styles.titleRow}>
          <span className={styles.title}>{session.title}</span>
          {session.branch && (
            <span className={styles.branch}>{session.branch}</span>
          )}
        </span>

        {session.path && (
          <span className={styles.path}>{shortPath(session.path)}</span>
        )}
      </span>

      {onOpenInNewPane && (
        <button
          className={styles.cloneButton}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onOpenInNewPane(session);
          }}
          aria-label={`Open session ${session.title} in new pane`}
          tabIndex={isHighlighted ? 0 : -1}
          title="Open in new pane (or Alt+click)"
          type="button"
        >
          ⊞
        </button>
      )}
      {onClone && (
        <button
          className={styles.cloneButton}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onClone(session);
          }}
          aria-label={`Clone session ${session.title}`}
          tabIndex={isHighlighted ? 0 : -1}
          title="Clone this session"
          type="button"
        >
          ⊕
        </button>
      )}
    </li>
  );
}
