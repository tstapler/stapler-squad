"use client";

import { useEffect, useRef } from "react";
import { action, rowActions } from "./BackgroundActivity.css";
import { confirmBox, errorMessage, inlineMessage } from "./ScheduledPrompts.css";

interface InlineConfirmProps {
  question: string;
  yesLabel: string;
  keepLabel: string;
  busy?: boolean;
  onYes: () => void;
  onKeep: () => void;
  testId?: string;
}

/** An in-row yes/no question. Focus lands on the affirmative button; Escape keeps. */
export function InlineConfirm({ question, yesLabel, keepLabel, busy = false, onYes, onKeep, testId }: InlineConfirmProps) {
  const yes = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    yes.current?.focus();
  }, []);
  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- Escape is a shortcut; both buttons are the keyboard path
    <div
      className={confirmBox}
      role="group"
      aria-label={question}
      data-testid={testId}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          onKeep();
        }
      }}
    >
      <p className={inlineMessage}>{question}</p>
      <div className={rowActions}>
        {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
        <button type="button" ref={yes} className={action} disabled={busy} onClick={onYes} data-testid="inline-confirm-yes">
          {yesLabel}
        </button>
        {/* eslint-disable-next-line analytics/require-on-click -- inline control of an existing action, not a funnel step */}
        <button type="button" className={action} disabled={busy} onClick={onKeep} data-testid="inline-confirm-keep">
          {keepLabel}
        </button>
      </div>
    </div>
  );
}

interface InlineErrorProps {
  message: string;
  onRetry?: () => void;
  onDismiss?: () => void;
  retryLabel?: string;
  dismissLabel?: string;
  testId?: string;
}

/** An in-row failure with its exit controls: Retry and Dismiss (UX-AC55). */
export function InlineError({ message, onRetry, onDismiss, retryLabel = "Retry", dismissLabel = "Dismiss", testId }: InlineErrorProps) {
  return (
    <div className={confirmBox} data-testid={testId}>
      <p className={errorMessage}>{message}</p>
      <div className={rowActions}>
        {onRetry && (
          // analytics-exempt
          <button type="button" className={action} onClick={onRetry} data-testid="inline-error-retry">
            {retryLabel}
          </button>
        )}
        {onDismiss && (
          // analytics-exempt
          <button type="button" className={action} onClick={onDismiss} data-testid="inline-error-dismiss">
            {dismissLabel}
          </button>
        )}
      </div>
    </div>
  );
}
