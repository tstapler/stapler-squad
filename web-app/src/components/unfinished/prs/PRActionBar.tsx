// +feature: unfinished-github-prs
"use client";

import { useRef, useState } from "react";
import type { UserPR } from "@/gen/session/v1/types_pb";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useNotificationCommands } from "@/lib/contexts/notificationContexts";
import { usePRActions, type MergeMethod, type PRActionClient } from "@/lib/hooks/usePRActions";
import * as styles from "./PRCard.css";

const METHOD_KEY = "pr-panel-merge-method";
const METHODS: MergeMethod[] = ["merge", "squash", "rebase"];

function readMethod(): MergeMethod {
  try {
    const v = window.localStorage.getItem(METHOD_KEY);
    if (METHODS.includes(v as MergeMethod)) return v as MergeMethod;
  } catch {
    // storage unavailable: fall back to the default
  }
  return "merge";
}

interface PRActionBarProps {
  pr: UserPR;
  /** Test seam; defaults to the shared Connect client. */
  client?: PRActionClient;
  /** Called after a merge or close succeeds so the card can drop out of the list. */
  onGone: () => void;
  /** Called after a draft toggle succeeds so the owner can refetch. */
  onChanged?: () => void;
}

export function PRActionBar({ pr, client, onGone, onChanged }: PRActionBarProps) {
  const { inflight, error, run } = usePRActions(client);
  const { addNotification } = useNotificationCommands();
  const [method, setMethod] = useState<MergeMethod>(readMethod);
  const [confirmingClose, setConfirmingClose] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeTriggerRef = useRef<HTMLButtonElement>(null);
  useFocusTrap(dialogRef, confirmingClose, closeTriggerRef);

  const busy = inflight !== null;
  const toast = (message: string) =>
    addNotification({ sessionId: "", sessionName: "", message, notificationType: "info" });

  const changeMethod = (m: MergeMethod) => {
    setMethod(m);
    try {
      window.localStorage.setItem(METHOD_KEY, m);
    } catch {
      // not persisted; the selection still applies for this card
    }
  };

  const merge = async () => {
    if (await run("merge", pr, method)) {
      toast(`PR #${pr.number} merged`);
      onGone();
    }
  };

  const close = async () => {
    setConfirmingClose(false);
    if (await run("close", pr)) {
      toast(`PR #${pr.number} closed`);
      onGone();
    }
  };

  const toggleDraft = async () => {
    if (await run(pr.isDraft ? "toReady" : "toDraft", pr)) onChanged?.();
  };

  return (
    <>
      {error && (
        <p role="alert" className={styles.actionError} data-testid="pr-action-error">
          {error}
        </p>
      )}
      <div className={styles.actionRow}>
        {!pr.isDraft && (
          <>
            <select
              className={styles.methodSelect}
              aria-label="Merge method"
              value={method}
              disabled={busy}
              onChange={(e) => changeMethod(e.target.value as MergeMethod)}
            >
              {METHODS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
            <button
              type="button"
              className={styles.actionButton}
              disabled={busy}
              onClick={merge}
              data-testid="pr-merge-button"
            >
              {inflight === "merge" ? "Merging…" : "Merge"}
            </button>
          </>
        )}
        <button
          type="button"
          className={styles.actionButton}
          disabled={busy || !pr.nodeId}
          onClick={toggleDraft}
          data-testid="pr-draft-toggle-button"
        >
          {pr.isDraft ? "Mark as ready" : "Convert to draft"}
        </button>
        <button
          ref={closeTriggerRef}
          type="button"
          className={styles.dangerActionButton}
          disabled={busy}
          onClick={() => setConfirmingClose(true)}
          data-testid="pr-close-button"
        >
          Close
        </button>
      </div>
      {confirmingClose && (
        <div
          ref={dialogRef}
          role="dialog"
          aria-modal="true"
          aria-labelledby={`close-pr-title-${pr.number}`}
          tabIndex={-1}
          className={styles.confirmDialog}
          onKeyDown={(e) => {
            if (e.key === "Escape") setConfirmingClose(false);
          }}
        >
          <p id={`close-pr-title-${pr.number}`}>
            Close PR #{pr.number} without merging?
          </p>
          <div className={styles.actionRow}>
            <button type="button" autoFocus className={styles.actionButton} onClick={() => setConfirmingClose(false)}>
              Cancel
            </button>
            <button
              type="button"
              className={styles.dangerActionButton}
              onClick={close}
              data-testid="pr-close-confirm"
            >
              Close PR
            </button>
          </div>
        </div>
      )}
    </>
  );
}
