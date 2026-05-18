"use client";
// +feature: backlog:gate-verdict

import { useEffect, useRef, useState } from "react";
import * as styles from "./GateVerdictBox.css";
import { InlineError } from "./InlineError";

interface GateVerdictBoxProps {
  verdict: "PASS" | "PARTIAL" | "FAIL" | "PENDING";
  summary: string;
  criteria?: Array<{ label: string; passed: boolean }>;
  elapsedSeconds?: number;
  onApprove: () => Promise<void>;
  onReopen: () => Promise<void>;
  onOverride: (reason: string) => Promise<void>;
  onSkipGate: () => Promise<void>;
  actionPending?: boolean;
}

const MIN_OVERRIDE_REASON_LENGTH = 5;

const VERDICT_CONFIG = {
  PASS: {
    icon: "✓",
    label: "PASSED",
    cardClass: styles.verdictCardPass,
    iconClass: styles.verdictIconPass,
    labelClass: styles.verdictLabelPass,
  },
  PARTIAL: {
    icon: "◑",
    label: "PARTIAL",
    cardClass: styles.verdictCardPartial,
    iconClass: styles.verdictIconPartial,
    labelClass: styles.verdictLabelPartial,
  },
  FAIL: {
    icon: "✗",
    label: "FAILED",
    cardClass: styles.verdictCardFail,
    iconClass: styles.verdictIconFail,
    labelClass: styles.verdictLabelFail,
  },
  PENDING: {
    icon: "◌",
    label: "PENDING",
    cardClass: styles.verdictCardPending,
    iconClass: styles.verdictIconPending,
    labelClass: styles.verdictLabelPending,
  },
} as const;

export function GateVerdictBox({
  verdict,
  summary,
  criteria,
  elapsedSeconds,
  onApprove,
  onReopen,
  onOverride,
  onSkipGate,
  actionPending = false,
}: GateVerdictBoxProps) {
  const [showOverride, setShowOverride] = useState(false);
  const [overrideReason, setOverrideReason] = useState("");
  const [showSkipConfirm, setShowSkipConfirm] = useState(false);
  const [localPending, setLocalPending] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const isPending = localPending || actionPending;
  const config = VERDICT_CONFIG[verdict];

  const overrideToggleRef = useRef<HTMLButtonElement>(null);
  const skipLinkRef = useRef<HTMLButtonElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);

  // Focus override reason textarea when form opens
  useEffect(() => {
    if (showOverride) {
      const el = document.getElementById("override-reason");
      if (el) {
        (el as HTMLTextAreaElement).focus();
      }
    }
  }, [showOverride]);

  // Focus cancel button when skip confirm opens
  useEffect(() => {
    if (showSkipConfirm) {
      cancelRef.current?.focus();
    }
  }, [showSkipConfirm]);

  function handleKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Enter" && e.ctrlKey) {
      if (verdict === "PASS") {
        void handleApprove();
      } else if (verdict === "PARTIAL" || verdict === "FAIL") {
        void handleReopen();
      }
    }
  }

  async function handleApprove() {
    setLocalPending(true);
    try {
      await onApprove();
    } catch (err) {
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  async function handleReopen() {
    setLocalPending(true);
    try {
      await onReopen();
    } catch (err) {
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  async function handleOverrideSubmit() {
    setLocalPending(true);
    try {
      await onOverride(overrideReason);
    } catch (err) {
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  async function handleSkipGateConfirm() {
    setLocalPending(true);
    try {
      await onSkipGate();
    } catch (err) {
      setActionError("Action failed. Please try again.");
      console.error(err);
    } finally {
      setLocalPending(false);
    }
  }

  function handleSkipConfirmKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      setShowSkipConfirm(false);
      skipLinkRef.current?.focus();
      return;
    }
    if (e.key === "Tab") {
      const focusables = [cancelRef.current, confirmRef.current].filter(
        (el): el is HTMLButtonElement => el !== null,
      );
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last.focus();
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
  }

  function handleOverrideFormKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      setShowOverride(false);
      setOverrideReason("");
      overrideToggleRef.current?.focus();
    }
  }

  const showCriteria =
    (verdict === "PARTIAL" || verdict === "FAIL") && criteria && criteria.length > 0;

  return (
    <section
      role="status"
      aria-live="polite"
      aria-label="Gate verdict"
      className={styles.section}
      onKeyDown={handleKeyDown}
      tabIndex={-1}
    >
      <p className={styles.sectionTitle}>Gate Verdict</p>

      <div className={config.cardClass}>
        <div className={styles.verdictHeader}>
          <span className={config.iconClass} aria-hidden="true">
            {config.icon}
          </span>
          <span className={`${styles.verdictLabel} ${config.labelClass}`}>{config.label}</span>
          {verdict === "PENDING" && elapsedSeconds !== undefined && (
            <span
              className={styles.verdictSummary}
              aria-label={`Elapsed: ${elapsedSeconds} seconds`}
            >
              {elapsedSeconds}s
            </span>
          )}
        </div>

        <p className={styles.verdictSummary}>{summary}</p>

        {showCriteria && (
          <ul className={styles.criteriaList} aria-label="Criteria results">
            {criteria!.map((c, i) => (
              <li key={i} className={styles.criteriaItem}>
                <span
                  className={c.passed ? styles.criteriaIconPass : styles.criteriaIconFail}
                  aria-hidden="true"
                >
                  {c.passed ? "✓" : "✗"}
                </span>
                {c.label}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className={styles.actions}>
        {verdict === "PASS" && (
          <>
            <button
              className={styles.primaryButton}
              onClick={() => void handleApprove()}
              disabled={isPending}
            >
              Approve — Mark Done
            </button>
            <button
              className={styles.secondaryButton}
              onClick={() => void handleReopen()}
              disabled={isPending}
            >
              Reopen for Revision
            </button>
          </>
        )}

        {(verdict === "PARTIAL" || verdict === "FAIL") && (
          <button
            className={styles.primaryButton}
            onClick={() => void handleReopen()}
            disabled={isPending}
          >
            Reopen for Revision
          </button>
        )}

        {verdict === "PENDING" && (
          <>
            <button
              className={styles.primaryButton}
              aria-disabled="true"
              disabled
              title="Wait for gate result or use Skip Gate below"
            >
              Approve — Mark Done
            </button>
            <button
              className={styles.secondaryButton}
              aria-disabled="true"
              disabled
              title="Wait for gate result or use Skip Gate below"
            >
              Reopen for Revision
            </button>
          </>
        )}
      </div>

      {actionError && (
        <InlineError
          type="transient"
          onRetry={() => setActionError(null)}
          onDismiss={() => setActionError(null)}
          customMessage={actionError}
        />
      )}

      {(verdict === "PARTIAL" || verdict === "FAIL") && (
        <div className={styles.overrideSection}>
          <button
            ref={overrideToggleRef}
            className={styles.overrideToggle}
            aria-expanded={showOverride}
            onClick={() => setShowOverride((prev) => !prev)}
          >
            Override: Mark done anyway {showOverride ? "▾" : "▸"}
          </button>

          {showOverride && (
            <div
              role="form"
              aria-label="Override gate verdict"
              id="override-form"
              className={styles.overrideForm}
              onKeyDown={handleOverrideFormKeyDown}
            >
              <label htmlFor="override-reason" className={styles.formLabel}>
                Reason for override (required)
              </label>
              <textarea
                id="override-reason"
                rows={3}
                placeholder="Explain why this item should be marked done despite..."
                aria-describedby="override-hint"
                value={overrideReason}
                onChange={(e) => setOverrideReason(e.target.value)}
                className={styles.formTextarea}
              />
              <span id="override-hint" className={styles.formHint}>
                Enter at least 5 characters to continue.
              </span>
              <div className={styles.formActions}>
                <button
                  className={styles.secondaryButton}
                  onClick={() => {
                    setShowOverride(false);
                    setOverrideReason("");
                  }}
                >
                  Cancel
                </button>
                <button
                  className={styles.dangerButton}
                  aria-disabled={overrideReason.trim().length < MIN_OVERRIDE_REASON_LENGTH}
                  disabled={overrideReason.trim().length < MIN_OVERRIDE_REASON_LENGTH}
                  onClick={() => void handleOverrideSubmit()}
                >
                  Mark Done — Override
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {showSkipConfirm ? (
        <div
          role="alertdialog"
          aria-labelledby="skip-gate-warning"
          aria-modal="true"
          className={styles.skipGateConfirmation}
          onKeyDown={handleSkipConfirmKeyDown}
        >
          <span id="skip-gate-warning" className={styles.skipGateWarning}>
            Skip gate and mark done without review
          </span>
          <p className={styles.skipGateBody}>
            The acceptance criteria will not be evaluated. This cannot be undone.
          </p>
          <div className={styles.formActions}>
            <button
              ref={cancelRef}
              className={styles.secondaryButton}
              onClick={() => setShowSkipConfirm(false)}
            >
              Cancel
            </button>
            <button
              ref={confirmRef}
              className={styles.dangerButton}
              onClick={() => void handleSkipGateConfirm()}
            >
              Confirm — Skip Gate
            </button>
          </div>
        </div>
      ) : (
        <button
          ref={skipLinkRef}
          className={styles.skipLink}
          onClick={() => setShowSkipConfirm(true)}
        >
          Skip gate and mark done without review
        </button>
      )}
    </section>
  );
}
