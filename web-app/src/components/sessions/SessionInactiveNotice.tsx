"use client";

import { useEffect } from "react";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import * as styles from "./ReadOnlyBanner.css";
import { pausedOverlayButton } from "./SessionDetailView.css";

// PAUSED and CRASHED already cover the terminal with their own overlay, and the
// remaining statuses are transient, so only these end states get the notice.
const INACTIVE_LABELS: Partial<Record<SessionStatus, string>> = {
  [SessionStatus.FAILED]: "failed",
  [SessionStatus.PERMANENTLY_FAILED]: "permanently failed",
  [SessionStatus.STOPPED]: "stopped",
};

/** Why the terminal ignores input, or null when the notice does not apply. */
export function inactiveNoticeReason(status: SessionStatus, archived: boolean): string | null {
  if (archived) return "archived";
  return INACTIVE_LABELS[status] ?? null;
}

function resumeHint(status: SessionStatus, archived: boolean, canRetry: boolean): string {
  if (archived) return "Unarchive the session to resume input.";
  if (canRetry) return "Retry now to resume input.";
  if (status === SessionStatus.PERMANENTLY_FAILED) return "Retry the session to resume input.";
  return "Restart the session to resume input.";
}

interface SessionInactiveNoticeProps {
  status: SessionStatus;
  archived: boolean;
  /** Server-computed (Session.terminalReadOnly): the notice shows only when the server drops input. */
  terminalReadOnly: boolean;
  /** Re-arms a permanently failed session; omit when no retry is available. */
  onRetryNow?: () => void;
  retrying?: boolean;
  retryError?: string | null;
  /** True while TerminalOutput's own hard-failed banner (with its own Retry) is showing. */
  terminalShowsRetry?: boolean;
}

/**
 * Visible notice that the terminal is read-only because the session is not
 * ACTIVE. Disappears once the server stops reporting the terminal read-only.
 */
export function SessionInactiveNotice({
  status,
  archived,
  terminalReadOnly,
  onRetryNow,
  retrying = false,
  retryError = null,
  terminalShowsRetry = false,
}: SessionInactiveNoticeProps) {
  const reason = terminalReadOnly ? inactiveNoticeReason(status, archived) : null;
  const { announce } = useAnnounce();
  useEffect(() => {
    if (reason) announce(`Read-only: session is ${reason}.`, "polite", "session-inactive-notice");
  }, [reason, announce]);

  if (!reason) return null;
  const canRetry = !archived && status === SessionStatus.PERMANENTLY_FAILED && !!onRetryNow && !terminalShowsRetry;
  return (
    <div className={styles.banner} data-testid="session-inactive-notice">
      <span className={styles.icon} aria-hidden="true">
        🔒
      </span>
      <div className={styles.text}>
        <span className={styles.primary}>{`Read-only: session is ${reason}.`}</span>
        <span className={styles.secondary}>{resumeHint(status, archived, canRetry)}</span>
        {retryError && (
          <span className={styles.secondary} role="alert" data-testid="session-inactive-notice-error">
            {retryError}
          </span>
        )}
      </div>
      {canRetry && (
        <button
          type="button"
          className={pausedOverlayButton}
          onClick={onRetryNow}
          disabled={retrying}
          data-testid="session-inactive-notice-retry"
        >{/* analytics-exempt */}
          {retrying ? "Retrying…" : "Retry now"}
        </button>
      )}
    </div>
  );
}
