"use client";

import { useId, useState } from "react";
import * as styles from "./ClaimOverrideForm.css";

/** Matches the server-side minimum (backlog_service_claim.go) and GateVerdictBox. */
export const MIN_OVERRIDE_REASON_LENGTH = 5;

interface ClaimOverrideFormProps {
  /** Accessible name and visible label, e.g. "Reason for importing anyway (required)". */
  label: string;
  confirmLabel: string;
  onConfirm: (reason: string) => void | Promise<void>;
  onCancel: () => void;
  /** Disables Confirm while the caller's request is in flight. */
  busy?: boolean;
  errorMessage?: string | null;
}

/**
 * Reason-capture form for overriding a cross-host claim block. Confirm stays
 * disabled until the trimmed reason reaches MIN_OVERRIDE_REASON_LENGTH,
 * mirroring GateVerdictBox's override form.
 */
export function ClaimOverrideForm({ label, confirmLabel, onConfirm, onCancel, busy = false, errorMessage }: ClaimOverrideFormProps) {
  const [reason, setReason] = useState("");
  const fieldId = useId();
  const tooShort = reason.trim().length < MIN_OVERRIDE_REASON_LENGTH;

  return (
    <div role="form" aria-label={label} className={styles.form} data-testid="claim-override-form">
      <label htmlFor={fieldId} className={styles.label}>
        {label}
      </label>
      <textarea
        id={fieldId}
        rows={2}
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        className={styles.textarea}
        data-testid="claim-override-reason"
      />
      <span className={styles.hint}>Enter at least {MIN_OVERRIDE_REASON_LENGTH} characters to continue. The reason is audit-logged.</span>
      {errorMessage && (
        <span className={styles.error} role="alert" data-testid="claim-override-error">
          {errorMessage}
        </span>
      )}
      <div className={styles.actions}>
        <button type="button" className={styles.cancelButton} onClick={onCancel} data-testid="claim-override-cancel">
          Cancel
        </button>
        <button
          type="button"
          className={styles.confirmButton}
          disabled={tooShort || busy}
          onClick={() => void onConfirm(reason.trim())}
          data-testid="claim-override-confirm"
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  );
}
