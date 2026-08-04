"use client";

import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import * as styles from "./BackwardSyncConfirmDialog.css";

interface BackwardSyncConfirmDialogProps {
  sourceDisplayName: string;
  itemCount: number;
  /** Up to 5 titles from the eligible set (see PreviewBackwardSyncImpact). */
  sampleTitles: string[];
  /**
   * True when the preview's underlying fetch hit its pagination cap — on
   * repos with an unusually large issue history, itemCount/sampleTitles are
   * a lower bound, not an exhaustive count. Surfaced as an explicit caveat
   * rather than implying full coverage.
   */
  possiblyIncomplete: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * BackwardSyncConfirmDialog — gates the first enable of backward sync
 * (Epic 4.4, Story 4.4.2) with a preview of how many already-imported items
 * would immediately archive, and informed-consent copy stating the archive
 * isn't undone by re-disabling the toggle. Only flips the toggle on Confirm.
 *
 * No existing modal primitive in web-app/src/components/shared/ implements
 * BOTH a full Tab-cycling focus trap AND focus-return-to-trigger-on-close
 * (VaguenessPromptModal traps focus between two fixed buttons but never
 * returns focus; NewShellDialog closes on Escape but doesn't trap focus at
 * all) — this component is a new, minimal one built to this feature's exact
 * accessibility ACs rather than partially reusing either.
 */
export function BackwardSyncConfirmDialog({
  sourceDisplayName,
  itemCount,
  sampleTitles,
  possiblyIncomplete,
  onConfirm,
  onCancel,
}: BackwardSyncConfirmDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  // Capture whatever had focus when this dialog mounted (the backward-sync
  // toggle button that triggered it) so focus can be restored on close.
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

  const shown = sampleTitles.slice(0, 5);
  const remaining = itemCount - shown.length;
  const titlesText = shown.length > 0 ? `${shown.join(", ")}${remaining > 0 ? `, and ${remaining} more` : ""}` : "";

  const content = (
    <div className={styles.overlay} data-testid="backward-sync-confirm-overlay">
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
        <h2 id={headingId} className={styles.heading}>
          Reflect GitHub status back for {sourceDisplayName}?
        </h2>
        <p className={styles.body}>
          Enabling this will immediately archive {itemCount} already-imported item{itemCount === 1 ? "" : "s"} whose
          linked GitHub issue is closed and can&apos;t be undone by disabling this toggle again
          {titlesText ? `: ${titlesText}` : ""}. Continue?
        </p>
        {possiblyIncomplete && (
          <p className={styles.body} data-testid="backward-sync-confirm-incomplete-caveat">
            This repository has a large issue history — the count above may be incomplete. More
            already-closed items than shown could be archived once enabled.
          </p>
        )}
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={onCancel}
            data-testid="backward-sync-confirm-cancel"
          >
            Cancel
          </button>
          <button
            ref={confirmButtonRef}
            type="button"
            className={styles.primaryButton}
            onClick={onConfirm}
            data-testid="backward-sync-confirm-confirm"
          >
            Confirm
          </button>
        </div>
      </div>
    </div>
  );

  // createPortal ensures the dialog renders at document.body, escaping any
  // ancestor CSS transform/filter/will-change that would break position:fixed
  // (ADR-009 / .claude/rules/css-architecture.md).
  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
