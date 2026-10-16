"use client";

import { useId, useRef } from "react";
import { createPortal } from "react-dom";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import * as styles from "./PiDisableWarningDialog.css";

interface PiDisableWarningDialogProps {
  onAcknowledge: () => void;
  onCancel: () => void;
}

/**
 * PiDisableWarningDialog — mandatory acknowledgment gate for turning the
 * pi-support flag off (Epic 2.1, Story 2.1.2). Only shown when the
 * pi-extension-status check reports the global approval extension
 * (ssq-approval.ts) is actually installed — otherwise the flag toggles
 * immediately like any other flag, no dialog at all.
 *
 * A mandatory warning was chosen over auto-uninstalling the extension: see
 * project_plans/pi-support/implementation/plan.md's "Decision — mandatory
 * warning, not auto-uninstall" for Story 2.1.2. Modeled on
 * BackwardSyncConfirmDialog's focus-trap + portal pattern.
 */
export function PiDisableWarningDialog({ onAcknowledge, onCancel }: PiDisableWarningDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const acknowledgeButtonRef = useRef<HTMLButtonElement>(null);

  useDialogFocusTrap({ dialogRef, initialFocusRef: acknowledgeButtonRef, onEscape: onCancel });

  const content = (
    <div className={styles.overlay} data-testid="pi-disable-warning-overlay">
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
        <h2 id={headingId} className={styles.heading}>
          pi approval extension stays installed
        </h2>
        <p className={styles.body} data-testid="pi-disable-warning-body">
          Disabling pi-support does NOT remove the pi approval extension (
          <code>ssq-approval.ts</code>) already installed at{" "}
          <code>~/.pi/agent/extensions/ssq-approval.ts</code>. Direct <code>pi</code> usage
          outside stapler-squad remains subject to it. Run{" "}
          <code>ssq-hooks install pi --uninstall</code> to remove it.
        </p>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={onCancel}
            data-testid="pi-disable-warning-cancel"
          >
            Cancel
          </button>
          <button
            ref={acknowledgeButtonRef}
            type="button"
            className={styles.primaryButton}
            onClick={onAcknowledge}
            data-testid="pi-disable-warning-acknowledge"
          >
            I understand
          </button>
        </div>
      </div>
    </div>
  );

  // createPortal ensures the dialog renders at document.body, escaping any
  // ancestor CSS transform/filter/will-change that would break position:fixed
  // (ADR-009 / docs/reference/css-architecture.md).
  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
