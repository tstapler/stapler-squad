"use client";

import { SessionStatus } from "@/gen/session/v1/types_pb";
import * as styles from "./ReadOnlyBanner.css";

// PAUSED and CRASHED already cover the terminal with their own overlay, and the
// remaining statuses are transient, so only these end states get the notice.
const INACTIVE_LABELS: Partial<Record<SessionStatus, string>> = {
  [SessionStatus.FAILED]: "failed",
  [SessionStatus.PERMANENTLY_FAILED]: "permanently failed",
  [SessionStatus.STOPPED]: "stopped",
};

/** Why the terminal ignores input for this session, or null when input works. */
export function inactiveNoticeReason(status: SessionStatus, archived: boolean): string | null {
  if (archived) return "archived";
  return INACTIVE_LABELS[status] ?? null;
}

interface SessionInactiveNoticeProps {
  status: SessionStatus;
  archived: boolean;
  /** Re-arms a permanently failed session; omitted when no retry is available. */
  onRetryNow?: () => void;
  retrying?: boolean;
}

/**
 * Visible notice that the terminal is read-only because the session is not
 * ACTIVE (the server drops input for it). Disappears once the status is ACTIVE.
 */
export function SessionInactiveNotice({ status, archived, onRetryNow, retrying = false }: SessionInactiveNoticeProps) {
  const reason = inactiveNoticeReason(status, archived);
  if (!reason) return null;
  const canRetry = !archived && status === SessionStatus.PERMANENTLY_FAILED && onRetryNow;
  return (
    <div className={styles.banner} data-testid="session-inactive-notice" role="status">
      <span className={styles.icon} aria-hidden="true">
        🔒
      </span>
      <div className={styles.text}>
        <span className={styles.primary}>{`Read-only: session is ${reason}.`}</span>
        <span className={styles.secondary}>
          {archived ? "Unarchive the session to resume input." : "Retry or restart to resume input."}
        </span>
      </div>
      {canRetry && (
        <button type="button" onClick={onRetryNow} disabled={retrying} data-testid="session-inactive-notice-retry">{/* analytics-exempt */}
          {retrying ? "Retrying…" : "Retry now"}
        </button>
      )}
    </div>
  );
}
