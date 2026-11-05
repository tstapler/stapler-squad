"use client";
/* eslint-disable analytics/require-on-click -- the lookup is a navigation, not a tracked product action */

// +feature: omnibar:detect
import { forwardRef, useCallback, useState } from "react";
import type { KeyboardEvent } from "react";
import { useRouter } from "@/lib/nav";
import { useNotificationCommands } from "@/lib/contexts/NotificationContext";
import {
  findSessionsForPr,
  prLabel,
  type PullRequestTarget,
} from "@/lib/omnibar/findSessionsForPr";
import { actionButton, errorLine, retryButton, row } from "./OmnibarFindPrAction.css";

interface OmnibarFindPrActionProps {
  pr: PullRequestTarget;
  onOpenSession: (sessionId: string) => void;
  onDone: () => void;
  /** ArrowUp hands focus back to the omnibar input. */
  onFocusInput: () => void;
}

/**
 * "Find sessions for acme/app#12" for a pasted PR URL (design/ux.md S9). One match
 * opens that session with a View all toast; none or several go to the artifacts
 * list; a failed lookup keeps the omnibar open with Retry.
 */
export const OmnibarFindPrAction = forwardRef<HTMLButtonElement, OmnibarFindPrActionProps>(
  function OmnibarFindPrAction({ pr, onOpenSession, onDone, onFocusInput }, ref) {
    const router = useRouter();
    const { addNotification } = useNotificationCommands();
    const [busy, setBusy] = useState(false);
    const [failed, setFailed] = useState(false);
    const label = prLabel(pr);

    const run = useCallback(async () => {
      setBusy(true);
      setFailed(false);
      try {
        const outcome = await findSessionsForPr(pr);
        if (outcome.kind === "list") {
          router.push(outcome.href);
        } else {
          onOpenSession(outcome.sessionId);
          addNotification({
            sessionId: "",
            sessionName: "",
            title: `Opened the session tracking ${label} (${outcome.sourceLabel})`,
            message: "1 match.",
            notificationType: "info",
            viewLabel: "View all",
            onView: () => router.push(outcome.viewAllHref),
          });
        }
        onDone();
      } catch {
        setFailed(true);
      } finally {
        setBusy(false);
      }
    }, [pr, label, router, addNotification, onOpenSession, onDone]);

    // The modal-wide handleKeyDown would run handleSubmit on Enter and move
    // the path-completion highlight on arrows; this control owns its own keys.
    const handleKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
      if (e.key === "Enter" || e.key === " ") e.stopPropagation();
      if (e.key === "ArrowUp") {
        e.preventDefault();
        e.stopPropagation();
        onFocusInput();
      }
    };

    return (
      <div className={row} data-testid="omnibar-find-pr">
        <button
          ref={ref}
          type="button"
          className={actionButton}
          data-testid="omnibar-find-pr-action"
          onClick={run}
          onKeyDown={handleKeyDown}
          disabled={busy}
        >
          <span aria-hidden="true">🔎</span>
          <span>{busy ? "Looking up sessions…" : `Find sessions for ${label}`}</span>
        </button>
        {failed && (
          <div className={errorLine} role="alert" data-testid="omnibar-find-pr-error">
            <span>Couldn&apos;t look up sessions for this PR</span>
            <button
              type="button"
              className={retryButton}
              data-testid="omnibar-find-pr-retry"
              onClick={run}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") e.stopPropagation();
              }}
            >
              Retry
            </button>
          </div>
        )}
      </div>
    );
  },
);
