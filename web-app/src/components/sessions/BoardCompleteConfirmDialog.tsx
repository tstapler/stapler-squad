"use client";

import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import * as styles from "./BoardCompleteConfirmDialog.css";

interface BoardCompleteConfirmDialogProps {
  sessionTitle: string;
  onConfirm: () => void;
  onCancel: () => void;
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Gates a drop/move into the "Complete" board column (Task 3.1.0, AC12) — that move calls
 * StopByUser, which kills the session's tmux pane and can remove its worktree, and
 * legalBoardTransitions["complete"] = [] means there is no in-board drag-based undo. No
 * existing modal primitive in web-app/src/components/ui/ implements both a full Tab-cycling
 * focus trap and focus-return-to-trigger-on-close (confirmed via
 * `grep -rl "ConfirmDialog\|useConfirm" web-app/src/components/ui/` — none found; the closest
 * analog, BackwardSyncConfirmDialog, lives in components/settings/ for a different feature) —
 * this mirrors that component's pattern rather than partially reusing it.
 */
export function BoardCompleteConfirmDialog({ sessionTitle, onConfirm, onCancel }: BoardCompleteConfirmDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(
    typeof document !== "undefined" ? (document.activeElement as HTMLElement | null) : null
  );

  useEffect(() => {
    confirmButtonRef.current?.focus();
    return () => {
      previouslyFocusedRef.current?.focus();
    };
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.key === "Tab" && dialogRef.current) {
        const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onCancel]);

  const content = (
    <div className={styles.overlay} data-testid="board-complete-confirm-overlay">
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
        <h2 id={headingId} className={styles.heading}>
          Stop &ldquo;{sessionTitle}&rdquo;?
        </h2>
        <p className={styles.body}>This ends its session.</p>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={onCancel}
            data-testid="board-complete-confirm-cancel"
          >
            Cancel
          </button>
          <button
            ref={confirmButtonRef}
            type="button"
            className={styles.dangerButton}
            onClick={onConfirm}
            data-testid="board-complete-confirm-confirm"
          >
            Confirm
          </button>
        </div>
      </div>
    </div>
  );

  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
