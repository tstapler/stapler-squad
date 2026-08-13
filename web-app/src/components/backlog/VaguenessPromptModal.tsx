"use client";

import { useEffect, useRef, useId } from "react";
import { createPortal } from "react-dom";
import * as styles from "./VaguenessPromptModal.css";

interface VaguenessPromptModalProps {
  itemTitle: string;
  /** Dismiss modal and re-open form focused on description */
  onRefine: () => void;
  /** Dismiss modal, trigger triage explicitly */
  onProceed: () => void;
}

/**
 * VaguenessPromptModal — shown after creating an item with a short description
 * and no acceptance criteria. Forces user to choose between refining the item
 * or proceeding with triage on the thin description.
 *
 * No escape-key dismissal: the user must choose one of the two explicit options.
 */
export function VaguenessPromptModal({ itemTitle, onRefine, onProceed }: VaguenessPromptModalProps) {
  const headingId = useId();
  const refineButtonRef = useRef<HTMLButtonElement>(null);
  const proceedButtonRef = useRef<HTMLButtonElement>(null);

  // Move focus to the primary button when the dialog opens
  useEffect(() => {
    refineButtonRef.current?.focus();
  }, []);

  // Trap focus between the two buttons
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Tab") {
      const activeEl = document.activeElement;
      if (!e.shiftKey && activeEl === proceedButtonRef.current) {
        e.preventDefault();
        refineButtonRef.current?.focus();
      } else if (e.shiftKey && activeEl === refineButtonRef.current) {
        e.preventDefault();
        proceedButtonRef.current?.focus();
      }
    }
    // No escape-key dismissal — user must choose explicitly
  };

  const content = (
    <div className={styles.overlay} data-testid="vagueness-prompt-modal">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={headingId}
        className={styles.dialog}
        onKeyDown={handleKeyDown}
      >
        <h2 id={headingId} className={styles.heading}>
          Item created.
        </h2>
        <p className={styles.itemTitle}>&ldquo;{itemTitle}&rdquo;</p>
        <p className={styles.body}>
          The description is brief and has no acceptance criteria. Triage works
          best with more context.
        </p>
        <p className={styles.prompt}>What would you like to do?</p>
        <div className={styles.actions}>
          <button
            ref={refineButtonRef}
            type="button"
            className={styles.primaryButton}
            onClick={onRefine}
            data-testid="vagueness-refine-button"
          >
            Add more detail
          </button>
          <button
            ref={proceedButtonRef}
            type="button"
            className={styles.secondaryButton}
            onClick={onProceed}
            data-testid="vagueness-proceed-button"
          >
            Run triage anyway
          </button>
        </div>
      </div>
    </div>
  );

  // createPortal ensures the modal renders at document.body, escaping any
  // CSS transform/filter ancestor that would break position:fixed (ADR-009).
  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
