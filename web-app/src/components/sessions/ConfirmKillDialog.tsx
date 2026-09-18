"use client";
// +feature: confirm-kill-dialog

import { useRef, useState, RefObject } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useImportSessionService } from "@/lib/hooks/useImportSessionService";
import { KillStatus, type PIDIdentity } from "@/gen/session/v1/import_pb";
import { nextImportRowStatus, type ImportRowStatus } from "./importRowStatus";
import * as styles from "./ConfirmKillDialog.css";

interface ConfirmKillDialogProps {
  instanceId: string;
  pidIdentity?: PIDIdentity;
  program: string;
  onStatusChange: (status: ImportRowStatus) => void;
  onClose: () => void;
  // ConfirmKillDialog has no live DOM trigger of its own: it opens as a
  // chained continuation after ImportPreviewDialog unmounts (see
  // ImportSessionsContainer.tsx), so there's no "Confirm Kill"-button click
  // to capture. We deliberately reuse the import flow's triggerRef (the
  // element that started the whole import) rather than leave focus
  // restoration silently broken for this dialog.
  triggerRef?: RefObject<HTMLElement | null>;
}

export function ConfirmKillDialog({
  instanceId,
  pidIdentity,
  program,
  onStatusChange,
  onClose,
  triggerRef,
}: ConfirmKillDialogProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true, triggerRef);
  const { confirmKill, cancelPendingKill } = useImportSessionService();

  const [isKilling, setIsKilling] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const busy = isKilling || isCancelling;

  const handleConfirmKill = async () => {
    if (busy) return;
    setIsKilling(true);
    setError(null);
    try {
      const result = await confirmKill({ instanceId, pidIdentity });
      if (!result || result.status === KillStatus.FAILED) {
        setError(
          result?.error ||
            "Failed to kill the original process. It remains suspended (SIGSTOP) — you can retry or revert the import."
        );
        onStatusChange(nextImportRowStatus("committed_pending_kill", { type: "kill_failed" }));
        return;
      }
      onStatusChange(nextImportRowStatus("committed_pending_kill", { type: "kill_confirmed" }));
      onClose();
    } finally {
      setIsKilling(false);
    }
  };

  const handleCancelPending = async () => {
    if (busy) return;
    setIsCancelling(true);
    setError(null);
    try {
      const result = await cancelPendingKill({ instanceId, pidIdentity });
      if (!result || !result.resumed) {
        setError(
          result?.error ||
            "Failed to revert the import. The original process remains suspended (SIGSTOP) — you can retry."
        );
        onStatusChange(nextImportRowStatus("committed_pending_kill", { type: "cancel_failed" }));
        return;
      }
      onStatusChange(nextImportRowStatus("committed_pending_kill", { type: "kill_cancelled" }));
      onClose();
    } finally {
      setIsCancelling(false);
    }
  };

  return (
    <div className={styles.overlay} onClick={busy ? undefined : onClose}>
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-kill-title"
        ref={modalRef}
        data-testid="confirm-kill-dialog"
      >
        <div className={styles.header}>
          <h2 className={styles.title} id="confirm-kill-title">
            Finish Importing {program}
          </h2>
          <p className={styles.subtitle}>
            The session has been imported and is now managed by Stapler Squad. The
            original process is suspended (SIGSTOP). Choose what to do with it.
          </p>
        </div>

        <div className={styles.body}>
          {error && <div className={styles.errorState}>{error}</div>}

          <div className={styles.optionCard}>
            <span className={styles.optionTitle}>Revert import</span>
            <span className={styles.optionDescription}>
              Discard the imported session and resume (SIGCONT) the original process
              so it continues running as before.
            </span>
          </div>

          <div className={styles.optionCard}>
            <span className={styles.optionTitle}>Confirm kill</span>
            <span className={styles.optionDescription}>
              Permanently terminate the original process. This cannot be undone.
            </span>
          </div>
        </div>

        <div className={styles.footer}>
          <button
            onClick={handleCancelPending}
            className={styles.cancelButton}
            disabled={busy}
            type="button"
            data-testid="confirm-kill-revert-button"
          >
            {isCancelling ? "Reverting..." : "Cancel"}
          </button>
          <button
            onClick={handleConfirmKill}
            className={styles.killButton}
            disabled={busy}
            type="button"
            data-testid="confirm-kill-kill-button"
          >
            {isKilling ? "Killing..." : "Confirm Kill"}
          </button>
        </div>
      </div>
    </div>
  );
}
