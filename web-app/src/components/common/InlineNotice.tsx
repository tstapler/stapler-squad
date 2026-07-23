"use client";
// +feature: backlog:inline-notice

// InlineNotice — informational (non-error) banner family, added for
// project_plans/backlog-event-driven-updates Epic 5.3 (Task 5.3.2b) and
// reused for Task 5.3.1c's terminal-state banner. Deliberately NOT built on
// InlineError: InlineError is role="alert" aria-live="assertive" (correct
// for real failures), which is the wrong politeness level for a routine,
// non-blocking notice like "this item changed elsewhere" or "this item was
// archived elsewhere" (design/ux.md §6). This component is
// role="status" aria-live="polite" instead, and never blocks interaction
// with the rest of the surrounding form/panel.

import { container, icon, body, messageText, actions, actionButton, actionButtonPrimary, dismissButton } from "./InlineNotice.css";

export interface InlineNoticeAction {
  label: string;
  onClick: () => void;
  /** "primary" renders a filled button (e.g. "Save Anyway"); default is a plain text-link-style action (e.g. "Reload"). */
  variant?: "primary" | "secondary";
}

interface InlineNoticeProps {
  message: string;
  actions?: InlineNoticeAction[];
  onDismiss?: () => void;
  "data-testid"?: string;
}

export function InlineNotice({ message, actions: noticeActions, onDismiss, ...rest }: InlineNoticeProps) {
  return (
    <div
      className={container}
      role="status"
      aria-live="polite"
      data-testid={rest["data-testid"]}
    >
      <span className={icon} aria-hidden="true">
        ⓘ
      </span>
      <div className={body}>
        <span className={messageText}>{message}</span>
        {noticeActions && noticeActions.length > 0 && (
          <div className={actions}>
            {noticeActions.map((a) => (
              <button
                key={a.label}
                type="button"
                className={a.variant === "primary" ? actionButtonPrimary : actionButton}
                onClick={a.onClick}
              >
                {a.label}
              </button>
            ))}
          </div>
        )}
      </div>
      {onDismiss && (
        <button
          type="button"
          className={dismissButton}
          onClick={onDismiss}
          aria-label="Dismiss notice"
        >
          ×
        </button>
      )}
    </div>
  );
}
