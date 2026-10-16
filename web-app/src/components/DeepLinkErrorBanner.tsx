"use client";
// +feature: backlog:deep-link-resolve

/**
 * Story 5.1: renders a distinct, accessible message for each of the
 * resolver's failure reasons (Surface 11 in
 * project_plans/backlog-deep-linking/design/ux.md), following the "wrong
 * workspace" mental model instead of a generic 404 — reuses
 * backlog/InlineError.tsx's/TriageErrorBanner.tsx's role/token conventions
 * rather than introducing new ones.
 *
 * `role="alert"` (assertive) is used for genuine dead ends for *this* link
 * (deleted, archived, malformed, version-mismatch); `role="status"` (polite)
 * is used for the two cross-host cases (unreachable, not-registered), which
 * are not failures of the link itself — see ux.md's rationale table.
 */

import {
  alertContainer,
  statusContainer,
  icon,
  headline,
  body,
  actions,
  primaryActionButton,
  secondaryActionButton,
} from "./DeepLinkErrorBanner.css";

export type DeepLinkFailureReason =
  | "deleted"
  | "archived"
  | "not-registered"
  | "unreachable"
  | "malformed"
  | "version-mismatch";

export interface DeepLinkErrorBannerProps {
  reason: DeepLinkFailureReason;
  /**
   * Literal hostname parsed from the ssq:// link. Required for the two
   * cross-host reasons ("unreachable", "not-registered") — the banner must
   * name the host, never say "an instance"/"elsewhere" (ux.md AC3).
   */
  hostname?: string;
  /** Human-readable "last seen" string (e.g. "2h ago"). Only used for "unreachable", omitted when unknown. */
  lastSeenAt?: string;
  /** Known advertised address for the offline host — enables "Copy host address" when present. */
  advertisedAddress?: string;
  onGoToBoard?: () => void;
  onRetry?: () => void;
  onCopyHostAddress?: () => void;
}

interface ReasonCopy {
  role: "alert" | "status";
  headline: string;
  body: string;
}

function copyFor(props: DeepLinkErrorBannerProps): ReasonCopy {
  const host = props.hostname ?? "an unknown host";
  switch (props.reason) {
    case "deleted":
      return {
        role: "alert",
        headline: "This backlog item no longer exists",
        body: "It may have been completed, archived, or deleted since this link was shared.",
      };
    case "archived":
      // Distinguished from "deleted" per ux.md AC4 — deleted/archived must
      // remain visually and textually distinguishable at a glance.
      return {
        role: "alert",
        headline: "This backlog item has been archived",
        body: "It may have been completed, archived, or deleted since this link was shared.",
      };
    case "unreachable":
      return {
        role: "status",
        headline: `This item lives on "${host}"`,
        body: props.lastSeenAt
          ? `"${host}" isn't reachable right now. Last seen ${props.lastSeenAt}.`
          : `"${host}" isn't reachable right now.`,
      };
    case "not-registered":
      return {
        role: "status",
        headline: `This item lives on "${host}"`,
        body: `"${host}" hasn't been seen by this instance yet — it may not be running, or the two instances haven't discovered each other.`,
      };
    case "malformed":
      return {
        role: "alert",
        headline: "This link isn't valid",
        body: "It may have been cut off when copied or pasted. Try copying the link again from its source.",
      };
    case "version-mismatch":
      return {
        role: "alert",
        headline: "This link needs a newer version of stapler-squad",
        body: "This link uses a format this instance doesn't understand yet. Update stapler-squad to open it.",
      };
  }
}

export function DeepLinkErrorBanner(props: DeepLinkErrorBannerProps) {
  const { reason, onGoToBoard, onRetry, onCopyHostAddress, advertisedAddress } = props;
  const copy = copyFor(props);
  const isStatus = copy.role === "status";
  const showGoToBoard =
    (reason === "deleted" ||
      reason === "archived" ||
      reason === "malformed" ||
      reason === "version-mismatch") &&
    !!onGoToBoard;
  const showRetry = (reason === "unreachable" || reason === "not-registered") && !!onRetry;
  const showCopyAddress = reason === "unreachable" && !!advertisedAddress && !!onCopyHostAddress;

  return (
    <div
      className={isStatus ? statusContainer : alertContainer}
      role={copy.role}
      aria-live={isStatus ? "polite" : "assertive"}
      data-testid="deep-link-error-banner"
      data-reason={reason}
    >
      <span className={icon} aria-hidden="true">
        {isStatus ? "ℹ" : "✕"}
      </span>
      <div>
        <div className={headline}>{copy.headline}</div>
        <p className={body}>{copy.body}</p>
        {(showGoToBoard || showRetry || showCopyAddress) && (
          <div className={actions}>
            {showRetry && (
              <button className={primaryActionButton} onClick={onRetry} type="button">
                Retry
              </button>
            )}
            {showCopyAddress && (
              <button className={secondaryActionButton} onClick={onCopyHostAddress} type="button">
                Copy host address
              </button>
            )}
            {showGoToBoard && (
              <button className={primaryActionButton} onClick={onGoToBoard} type="button">
                Go to backlog board
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
