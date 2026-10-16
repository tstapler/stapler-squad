"use client";

import { useEffect } from "react";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import * as styles from "./SessionUnavailableCard.css";

export interface SessionUnavailableCardProps {
  /** "unavailable": the id is gone. "failed": the lookup failed and can be retried. */
  variant: "unavailable" | "failed";
  /** Short id shown in the "was removed before you opened it" line. */
  sessionLabel: string;
  /** Captured notification text; omitted when the record is pruned or the link had no id. */
  notification?: { title: string; message: string; timestampMs: number } | null;
  onOpenNotifications: () => void;
  onGoToSessions: () => void;
  onRetry?: () => void;
}

function relativeAge(timestampMs: number, now: number): string {
  const minutes = Math.max(0, Math.floor((now - timestampMs) / 60_000));
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? `${hours}h ago` : `${Math.floor(hours / 24)}d ago`;
}

/**
 * Deleted or unreachable session behind a deep link (Story 5.3, RO-3 / RO-7 / RO-10):
 * never a 404 and never a blank screen. Announced once on mount through the Announcer,
 * so the card itself carries no live role.
 */
export function SessionUnavailableCard({
  variant,
  sessionLabel,
  notification,
  onOpenNotifications,
  onGoToSessions,
  onRetry,
}: SessionUnavailableCardProps) {
  const { announce } = useAnnounce();
  const heading = variant === "failed" ? "Could not load session." : "Session no longer available";

  useEffect(() => {
    announce(heading, variant === "failed" ? "assertive" : "polite", "session-unavailable");
    // Once per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <section className={styles.card} data-testid="session-unavailable-card" aria-labelledby="session-unavailable-heading">
      <h2 className={styles.heading} id="session-unavailable-heading">
        {heading}
      </h2>
      {variant === "unavailable" && (
        <p className={styles.detail}>{sessionLabel} was removed before you opened it</p>
      )}
      {variant === "unavailable" && notification && (
        <>
          <p className={styles.quote} data-testid="session-unavailable-notification">
            Last notification: &ldquo;{notification.title}&rdquo;
            {notification.message ? ` - ${notification.message}` : ""}
          </p>
          {notification.timestampMs > 0 && (
            <p className={styles.detail}>{relativeAge(notification.timestampMs, Date.now())}</p>
          )}
        </>
      )}
      <div className={styles.actions}>
        {variant === "failed" && onRetry && (
          <button type="button" className={styles.button} onClick={onRetry} data-testid="session-unavailable-retry">
            Retry
          </button>
        )}
        {variant === "unavailable" && (
          <button
            type="button"
            className={styles.button}
            onClick={onOpenNotifications}
            data-testid="session-unavailable-open-notifications"
          >
            Open notifications
          </button>
        )}
        <button
          type="button"
          className={styles.secondaryButton}
          onClick={onGoToSessions}
          data-testid="session-unavailable-go-to-sessions"
        >
          Go to sessions
        </button>
      </div>
    </section>
  );
}
