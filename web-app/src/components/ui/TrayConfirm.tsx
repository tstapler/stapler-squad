"use client";

import { useEffect, useRef } from "react";
import { trayButton, trayConfirm, trayConfirmActions } from "./NotificationPanel.css";

interface TrayConfirmProps {
  /** The question and what is kept, e.g. "Clear 20 informational notifications? 2 awaiting decision kept." */
  text: string;
  confirmLabel: string;
  /** Set after a failed attempt: the confirm stays open and its button reads "Could not clear - Retry". */
  error?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * Inline confirm for the tray's destructive actions (never window.confirm).
 * Focus starts on Cancel; Esc cancels without closing the tray.
 */
export function TrayConfirm({ text, confirmLabel, error, busy, onConfirm, onCancel }: TrayConfirmProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => cancelRef.current?.focus({ preventScroll: true }), []);

  return (
    <div
      className={trayConfirm}
      role="group"
      aria-label="Confirm"
      data-testid="tray-confirm"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          onCancel();
        }
      }}
    >
      <span>{text}</span>
      <div className={trayConfirmActions}>
        <button ref={cancelRef} type="button" className={trayButton} onClick={onCancel} data-testid="tray-confirm-cancel">
          Cancel
        </button>
        <button type="button" className={trayButton} onClick={onConfirm} aria-disabled={busy || undefined} data-testid="tray-confirm-ok">
          {error ? "Could not clear - Retry" : confirmLabel}
        </button>
      </div>
    </div>
  );
}
