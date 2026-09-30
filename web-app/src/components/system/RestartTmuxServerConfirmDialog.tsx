"use client";

import { useId, useRef } from "react";
import { createPortal } from "react-dom";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./RestartTmuxServerConfirmDialog.css";

interface RestartTmuxServerConfirmDialogProps {
  sessionCount: number;
  clientVersion: string;
  serverVersion: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * RestartTmuxServerConfirmDialog gates the destructive tmux-server-restart
 * action (used to resolve a client/server tmux version mismatch that
 * silently disables control mode) behind an explicit, informed confirmation:
 * every one of sessionCount live sessions loses its terminal (running
 * processes, scrollback) the instant this is confirmed. Mirrors
 * BackwardSyncConfirmDialog's focus-trap + portal pattern.
 */
export function RestartTmuxServerConfirmDialog({
  sessionCount,
  clientVersion,
  serverVersion,
  busy,
  onConfirm,
  onCancel,
}: RestartTmuxServerConfirmDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  const { track } = useAnalytics();

  useDialogFocusTrap({ dialogRef, initialFocusRef: confirmButtonRef, onEscape: busy ? () => {} : onCancel });

  const content = (
    <div className={styles.overlay} data-testid="restart-tmux-confirm-overlay">
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
        <h2 id={headingId} className={styles.heading}>
          Restart the tmux server?
        </h2>
        <p className={styles.body}>
          This server is running tmux <strong>{serverVersion}</strong>, but this app&apos;s tmux client is{" "}
          <strong>{clientVersion}</strong> — that mismatch disables control mode (terminal input/output falls back
          to slower, per-command calls) for as long as the server keeps running.
        </p>
        <p className={styles.body}>
          Restarting will immediately kill{" "}
          <strong>
            {sessionCount} live session{sessionCount === 1 ? "" : "s"}
          </strong>{" "}
          on this server — every running process and terminal scrollback is lost. Session work directories and git
          worktrees are untouched; sessions reconnect with a fresh terminal on next open. This cannot be undone.
        </p>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={() => {
              track({ name: "restart_tmux_server_cancelled", category: "user_action" });
              onCancel();
            }}
            disabled={busy}
            data-testid="restart-tmux-confirm-cancel"
          >
            Cancel
          </button>
          <button
            ref={confirmButtonRef}
            type="button"
            className={styles.dangerButton}
            onClick={() => {
              track({ name: "restart_tmux_server_confirmed", category: "user_action", labels: { session_count: String(sessionCount) } });
              onConfirm();
            }}
            disabled={busy}
            data-testid="restart-tmux-confirm-confirm"
          >
            {busy ? "Restarting…" : `Restart and kill ${sessionCount} session${sessionCount === 1 ? "" : "s"}`}
          </button>
        </div>
      </div>
    </div>
  );

  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
