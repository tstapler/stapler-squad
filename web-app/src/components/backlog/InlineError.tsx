"use client";
// +feature: backlog:inline-error

import {
  pillContainer,
  blockContainer,
  icon,
  headline,
  body,
  actions,
  actionButton,
  dismissButton,
} from "./InlineError.css";

interface InlineErrorProps {
  type: "transient" | "timeout" | "permanent";
  onRetry: () => void;
  onDismiss?: () => void;
  logsSessionId?: string;
  customMessage?: string;
}

const COPY: Record<
  InlineErrorProps["type"],
  { headline: string; body: string }
> = {
  transient: {
    headline: "Triage failed",
    body: "Network error. The request could not be completed.",
  },
  timeout: {
    headline: "Triage timed out",
    body: "The triage session did not complete within 3 minutes.",
  },
  permanent: {
    headline: "Triage failed",
    body: "The triage session exited unexpectedly (exit code 1). Check the session logs for details.",
  },
};

export function InlineError({
  type,
  onRetry,
  onDismiss,
  logsSessionId,
  customMessage,
}: InlineErrorProps) {
  const copy = COPY[type];
  const bodyText = customMessage ?? copy.body;

  if (type === "permanent") {
    return (
      <div
        className={blockContainer}
        role="alert"
        aria-live="assertive"
      >
        <div style={{ display: "flex", alignItems: "center", gap: "inherit" }}>
          <span className={icon} aria-hidden="true">
            ✕
          </span>
          <span className={headline}>{copy.headline}</span>
          {onDismiss && (
            <button
              className={dismissButton}
              onClick={onDismiss}
              aria-label="Dismiss error"
              type="button"
            >
              ×
            </button>
          )}
        </div>
        <p className={body} style={{ margin: 0 }}>
          {bodyText}
        </p>
        <div className={actions}>
          {logsSessionId && (
            <a
              className={actionButton}
              href={`/sessions/${logsSessionId}/logs`}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="View session logs (opens in new tab)"
            >
              View session logs
            </a>
          )}
          <button
            className={actionButton}
            onClick={onRetry}
            aria-label="Retry triage"
            type="button"
          >
            Retry ↺
          </button>
        </div>
      </div>
    );
  }

  return (
    <div
      className={pillContainer}
      role="alert"
      aria-live="assertive"
    >
      <span className={icon} aria-hidden="true">
        ✕
      </span>
      <span>
        <span className={headline}>{copy.headline}</span>
        {" — "}
        {bodyText}
      </span>
      <button
        className={actionButton}
        onClick={onRetry}
        aria-label="Retry triage"
        type="button"
      >
        Retry ↺
      </button>
      {onDismiss && (
        <button
          className={dismissButton}
          onClick={onDismiss}
          aria-label="Dismiss error"
          type="button"
        >
          ×
        </button>
      )}
    </div>
  );
}
