"use client";

import { useId, useRef } from "react";
import { createPortal } from "react-dom";
import { useDialogFocusTrap } from "@/lib/hooks/useDialogFocusTrap";
import * as styles from "./HostKeyTrustDialog.css";

export interface HostKeyTrustDialogProps {
  /** "host" or "host:port" as shown to the user (ux.md Surface 3). */
  host: string;
  port: number;
  fingerprint: string;
  onTrust: () => void;
  onCancel: () => void;
}

/**
 * HostKeyTrustDialog — the TOFU (trust-on-first-use) confirmation modal
 * shown when TestRemoteConnection reports host_key_unknown=true (Task
 * 6.1.1c). Per ux.md Surface 3 / research/ux.md §1's VS Code precedent this
 * is a MODAL, not an inline prompt or a silent auto-trust: the remote is not
 * persisted until "Trust and connect" is explicitly clicked, and focus
 * starts on Cancel (not Trust) so a stray Enter keypress can't silently
 * trust an unverified host.
 *
 * Built on the same minimal focus-trap pattern as BackwardSyncConfirmDialog
 * (no existing shared primitive implements both Tab-cycling AND
 * focus-return-to-trigger — see that component's doc comment), extended
 * with backdrop-click-equals-Cancel per this dialog's own AC7 (a stronger
 * requirement than BackwardSyncConfirmDialog's, which doesn't need it).
 */
export function HostKeyTrustDialog({ host, port, fingerprint, onTrust, onCancel }: HostKeyTrustDialogProps) {
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const cancelButtonRef = useRef<HTMLButtonElement>(null);

  // Default focus lands on Cancel, not "Trust and connect" -- ux.md
  // Surface 3 interaction flow step 1.
  useDialogFocusTrap({ dialogRef, initialFocusRef: cancelButtonRef, onEscape: onCancel });

  const content = (
    <div
      className={styles.overlay}
      data-testid="host-key-trust-overlay"
      onClick={(e) => {
        // Backdrop click == Cancel, never Trust (ux.md Surface 3 AC7). Only
        // fires when the click target IS the overlay itself, not a bubbled
        // click from inside the dialog.
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
        <h2 id={headingId} className={styles.heading}>
          Verify host identity
        </h2>
        <p className={styles.body}>
          Stapler Squad has not connected to this host before. Verify the fingerprint matches what the
          remote&apos;s administrator (you) expects.
        </p>

        <dl className={styles.detailList}>
          <dt className={styles.detailTerm}>Host</dt>
          <dd className={styles.detailValue} data-testid="host-key-trust-host">
            {host}:{port || 22}
          </dd>
          <dt className={styles.detailTerm}>Key type</dt>
          <dd className={styles.detailValue}>ED25519</dd>
          <dt className={styles.detailTerm}>Fingerprint</dt>
          <dd className={[styles.detailValue, styles.fingerprint].join(" ")} data-testid="host-key-trust-fingerprint">
            {fingerprint}
          </dd>
        </dl>

        <div className={styles.actions}>
          <button
            ref={cancelButtonRef}
            type="button"
            className={styles.secondaryButton}
            onClick={onCancel}
            data-testid="host-key-trust-cancel"
          >
            Cancel
          </button>
          <button
            type="button"
            className={styles.primaryButton}
            onClick={onTrust}
            data-testid="host-key-trust-confirm"
          >
            Trust and connect
          </button>
        </div>
      </div>
    </div>
  );

  // createPortal escapes any ancestor CSS transform/filter/will-change that
  // would break position:fixed (ADR-009 / .claude/rules/css-architecture.md).
  if (typeof document === "undefined") return null;
  return createPortal(content, document.body);
}
