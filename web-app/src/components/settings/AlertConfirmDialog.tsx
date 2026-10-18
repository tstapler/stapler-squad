"use client";

import { useEffect, useId, useRef, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import * as styles from "./AlertConfirmDialog.css";

export interface AlertConfirmDialogProps {
  heading: string;
  /** Body slot; the dialog's accessible description. */
  children: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
  /** Confirm stays focusable (aria-disabled) but does nothing until this is false. */
  confirmDisabled?: boolean;
  /** Id of a visible hint explaining why confirm is disabled. */
  confirmDescribedById?: string;
  /**
   * Element that gets focus after close. Safari and Firefox on macOS do not
   * focus a button on click, so the hook's "previously focused" can be body.
   */
  returnFocusRef?: RefObject<HTMLElement | null>;
}

/**
 * Confirmation for dangerous actions: role="alertdialog", initial focus on
 * Cancel (never the primary action), overlay click cancels. Tab trap, Escape
 * and default focus return come from useDialogFocusTrap; the background is not
 * made inert (the hook does not support it), so aria-modal and the full-viewport
 * overlay are the containment. PiDisableWarningDialog is not reused: it focuses
 * its primary button.
 */
export function AlertConfirmDialog({
  heading,
  children,
  confirmLabel,
  cancelLabel = "Cancel",
  onConfirm,
  onCancel,
  confirmDisabled = false,
  confirmDescribedById,
  returnFocusRef,
}: AlertConfirmDialogProps) {
  const headingId = useId();
  const bodyId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);

  useDialogFocusTrap({ dialogRef, initialFocusRef: cancelRef, onEscape: onCancel });

  // Declared after the hook so its cleanup runs second and wins over the hook's restore.
  useEffect(
    () => () => {
      returnFocusRef?.current?.focus();
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on unmount
    [],
  );

  const content = (
    <div
      className={styles.overlay}
      data-testid="alert-confirm-overlay"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div
        ref={dialogRef}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={headingId}
        aria-describedby={bodyId}
        className={styles.dialog}
        data-testid="alert-confirm-dialog"
      >
        <h2 id={headingId} className={styles.heading}>
          {heading}
        </h2>
        <div id={bodyId} className={styles.body}>
          {children}
        </div>
        <div className={styles.actions}>
          <button
            ref={cancelRef}
            type="button"
            className={styles.cancelButton}
            onClick={onCancel}
            data-testid="alert-confirm-cancel"
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            className={styles.confirmButton}
            aria-disabled={confirmDisabled || undefined}
            aria-describedby={confirmDisabled ? confirmDescribedById : undefined}
            onClick={() => {
              if (!confirmDisabled) onConfirm();
            }}
            data-testid="alert-confirm-confirm"
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );

  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
