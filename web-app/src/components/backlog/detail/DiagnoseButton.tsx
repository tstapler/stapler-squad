"use client";

// +feature: backlog-diagnose-nudge

import { useState } from "react";
import { Link } from "@/lib/nav";
import { routes } from "@/lib/routes";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "../BacklogItemDetail.css";
import { ActionButtonLabel } from "./ActionButtonLabel";

export interface DiagnoseButtonProps {
  /** Resolves with the dispatched diagnostic session's UUID; rejects with the reason dispatch failed. */
  onDiagnose: () => Promise<string>;
  /** Analytics label: the item's primary stuck reason, or "none" for a healthy item. */
  reasonLabel: string;
}

type DiagnoseState = "idle" | "pending" | "dispatched" | "error";

/**
 * "Diagnose & Nudge" for the backlog item screen, so an operator does not have to open
 * Up Next to run it. Callbacks-up like StuckItemDetail's button: the caller owns the
 * dispatch (and which StuckReason, if any, it carries); this owns the one-shot UI state.
 * Links to the diagnostic session because it is hidden from the main session list.
 */
export function DiagnoseButton({ onDiagnose, reasonLabel }: DiagnoseButtonProps) {
  const { track } = useAnalytics();
  const [state, setState] = useState<DiagnoseState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [sessionUuid, setSessionUuid] = useState<string | null>(null);
  const busy = state === "pending" || state === "dispatched";

  async function submit() {
    if (busy) return;
    track({ name: "backlog_detail_diagnose", category: "user_action", component: "DiagnoseButton", labels: { reason: reasonLabel } });
    setState("pending");
    setError(null);
    try {
      setSessionUuid(await onDiagnose());
      setState("dispatched");
    } catch (err) {
      setState("error");
      setError(err instanceof Error ? err.message : "Failed to dispatch — try again.");
    }
  }

  return (
    <>
      <button
        type="button"
        className={styles.actionButton}
        aria-disabled={busy}
        aria-busy={state === "pending"}
        title="Dispatch a diagnostic agent for this item. It posts its conclusion (bug filed, note, or nudge) to the Activity Log."
        onClick={() => void submit()}
        data-testid="backlog-action-diagnose"
      >
        {state === "dispatched" ? (
          "Diagnostic session dispatched"
        ) : (
          <ActionButtonLabel pending={state === "pending"} label="Diagnose & Nudge" />
        )}
      </button>
      {state === "error" && (
        <span role="alert" data-testid="backlog-action-diagnose-error">
          {error}
        </span>
      )}
      {state === "dispatched" && sessionUuid && (
        <Link
          className={styles.actionButtonSecondary}
          href={routes.sessionDetail(sessionUuid)}
          aria-label="Open the dispatched diagnostic session's transcript"
          data-testid="backlog-action-diagnose-session-link"
        >
          View diagnostic session →
        </Link>
      )}
    </>
  );
}
