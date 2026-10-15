import type { RefObject } from "react";
import { createPortal } from "react-dom";
import type { Session } from "@/gen/session/v1/types_pb";
import * as styles from "./page.css";

interface DeleteSessionConfirmModalProps {
  target: Session;
  dialogRef: RefObject<HTMLDivElement | null>;
  onCancel: () => void;
  onConfirm: (sessionId: string) => Promise<void>;
}

/**
 * Delete confirmation modal (triggered by the 'd' keyboard shortcut).
 * Extracted from page.tsx's HomeContent — same markup/behavior, isolated as
 * its own concern. `dialogRef` is still trapped by the parent's
 * useFocusTrap(deleteDialogRef, ...) call, so it must keep landing on this
 * exact dialog div.
 *
 * All three buttons below are analytics-exempt: Close/Cancel only dismiss
 * the modal (no tracked event, same as before this was its own component),
 * and Delete's track("session_deleted") call lives one level up in the
 * onConfirm callback's own handleDeleteSession (page.tsx) — the
 * analytics/require-on-click lint rule can't see across that prop boundary.
 */
export function DeleteSessionConfirmModal({
  target,
  dialogRef,
  onCancel,
  onConfirm,
}: DeleteSessionConfirmModalProps) {
  return createPortal(
    <div className={styles.modal} onClick={onCancel}>
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="deleteConfirmTitle"
        tabIndex={-1}
        className={styles.modalContent}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === "Escape") onCancel();
        }}
      >
        <div className={styles.modalHeader}>
          <h2 id="deleteConfirmTitle">Delete Session</h2>
          {// analytics-exempt
          <button className={styles.closeButton} onClick={onCancel} aria-label="Close">
            ✕
          </button>}
        </div>
        <div className={styles.modalBody}>
          <p>Delete &quot;{target.title}&quot;?</p>
          <p style={{ color: "var(--error, #ef4444)", fontSize: "0.875rem", marginTop: "0.5rem" }}>
            This action cannot be undone.
          </p>
          <div className={styles.deleteConfirmActions}>
            {// analytics-exempt
            <button autoFocus className={styles.cancelButton} onClick={onCancel}>
              Cancel
            </button>}
            {// analytics-exempt
            <button
              className={styles.dangerButton}
              onClick={async () => {
                onCancel();
                await onConfirm(target.id);
              }}
            >
              Delete
            </button>}
          </div>
        </div>
      </div>
    </div>,
    document.body
  );
}
