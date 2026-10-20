"use client";

import { useEffect, useId, useRef, type RefObject } from "react";
import { AlertConfirmDialog } from "@/components/settings/AlertConfirmDialog";
import { differencesText, type CallerClass, type ClassReadinessView } from "@/lib/hooks/useEngineModeStatus";
import {
  VERDICT_LABEL,
  evidenceAnnouncement,
  isClean,
  useEnforceReadiness,
  type UseEnforceReadiness,
} from "./useEnforceReadiness";
import { ackCheckbox, ackLabel, dialogNote, evidenceNotClean, evidenceRegion, retryButton, srOnly } from "./page.css";

export const ROLLOUT_CHECKLIST = "docs/how-to/flip-workflow-engine-enforcement.md";
const ACK_LABEL = "I have reviewed this evidence and the rollout checklist";
function EvidenceLines({ readiness }: { readiness: ClassReadinessView }) {
  const bad = (flag: boolean) => (flag ? evidenceNotClean : undefined);
  return (
    <div aria-hidden="true" data-testid="feature-flag-enforce-evidence-lines">
      <div className={bad(readiness.divergence24h > 0)}>
        Shadow differences in the last 24h: {differencesText(readiness.divergence24h)}
      </div>
      <div className={bad(readiness.verdict !== "ready")}>Soak verdict: {VERDICT_LABEL[readiness.verdict]}</div>
      <div className={bad(readiness.strandedItemCount > 0)}>
        Items that would lose an exit: {readiness.strandedItemCount} (one preview count for all stage edits, the same
        for every caller class)
      </div>
    </div>
  );
}

interface EvidenceRegionProps {
  r: UseEnforceReadiness;
  onRetry: () => void;
  retryRef: RefObject<HTMLButtonElement | null>;
}

/** The dialog's only live region (UX-44): mounted with the dialog, announces each resolution once. */
function EvidenceRegion({ r, onRetry, retryRef }: EvidenceRegionProps) {
  const { evidence } = r;
  return (
    <div
      role="status"
      className={evidenceRegion}
      aria-busy={evidence.phase === "loading" || r.retrying ? "true" : undefined}
      data-testid="feature-flag-enforce-evidence"
    >
      {evidence.phase === "loading" && <span>Checking readiness evidence...</span>}
      {evidence.phase === "loaded" && (
        <>
          <span className={srOnly}>{evidenceAnnouncement(evidence.readiness, r.needsAck)}</span>
          <EvidenceLines readiness={evidence.readiness} />
          {!isClean(evidence.readiness) && (
            <div className={evidenceNotClean}>Not clean: review this evidence before enforcing.</div>
          )}
        </>
      )}
      {evidence.phase === "failed" && (
        <>
          <span>{`Couldn't load readiness evidence: ${evidence.error}`}</span>{" "}
          <button
            ref={retryRef}
            type="button"
            className={retryButton}
            aria-disabled={r.retrying || undefined}
            onClick={onRetry}
            data-testid="feature-flag-enforce-evidence-retry"
          >
            Retry
          </button>
        </>
      )}
      {r.changeNote && <div data-testid="feature-flag-enforce-evidence-changed">{r.changeNote}</div>}
    </div>
  );
}

function hintText(r: UseEnforceReadiness): string | null {
  if (r.canEnforce || r.recheck.phase === "running") return null;
  return r.evidence.phase === "loading" ? "Waiting for readiness evidence." : "Tick the acknowledgement above to enable Enforce.";
}

export interface EnforceDialogProps {
  cls: CallerClass;
  /** Row label, for example "Enforce for operators (this UI)". */
  flagLabel: string;
  /** Extra body sentence for this class (the automation row's stuck-item warning). */
  classNote?: string;
  /** Non-blocking advisory when the recommended predecessor is not enforced. */
  orderAdvisory?: string;
  /** Switch that opened the dialog; it gets focus back on close. */
  returnFocusRef: RefObject<HTMLElement | null>;
  /** Called only after the readiness re-check passed; the caller writes the flag. */
  onConfirm: () => void;
  onCancel: () => void;
}

/** Enforce confirmation with the class's readiness evidence at decision time (UX-35, UX-44, UX-45). */
export function EnforceDialog({
  cls,
  flagLabel,
  classNote,
  orderAdvisory,
  returnFocusRef,
  onConfirm,
  onCancel,
}: EnforceDialogProps) {
  const r = useEnforceReadiness(cls);
  const hintId = useId();
  const ackRef = useRef<HTMLInputElement>(null);
  const retryRef = useRef<HTMLButtonElement>(null);
  const hint = hintText(r);
  const running = r.recheck.phase === "running";

  useEffect(() => {
    if (r.focusRequest === "ack") ackRef.current?.focus();
  }, [r.focusRequest]);

  const retryEvidence = async () => {
    if (r.retrying) return;
    if (await r.retryEvidence()) {
      document.querySelector<HTMLElement>('[data-testid="alert-confirm-cancel"]')?.focus();
    } else {
      retryRef.current?.focus();
    }
  };

  const enforce = async () => {
    if (await r.confirm()) onConfirm();
  };

  return (
    <AlertConfirmDialog
      heading={`${flagLabel}?`}
      confirmLabel={running ? "Re-checking readiness..." : "Enforce"}
      confirmDisabled={!r.canEnforce || running || r.recheck.phase === "failed"}
      confirmDescribedById={hint ? hintId : undefined}
      returnFocusRef={returnFocusRef}
      onConfirm={() => void enforce()}
      onCancel={onCancel}
    >
      <p>
        Stage edits will change which transitions and gates are allowed.
        {classNote ? ` ${classNote}` : ""}
      </p>
      {orderAdvisory && (
        <p className={dialogNote} data-testid="feature-flag-enforce-order-advisory">
          {orderAdvisory}
        </p>
      )}
      <EvidenceRegion r={r} onRetry={() => void retryEvidence()} retryRef={retryRef} />
      <p className={dialogNote}>{`Rollout checklist: ${ROLLOUT_CHECKLIST}`}</p>
      {r.needsAck && (
        <label className={ackLabel}>
          <input
            ref={ackRef}
            type="checkbox"
            className={ackCheckbox}
            checked={r.acknowledged}
            onChange={(e) => r.setAcknowledged(e.target.checked)}
            data-testid="feature-flag-enforce-ack"
          />
          <span>{ACK_LABEL}</span>
        </label>
      )}
      {r.recheck.phase === "failed" && (
        <div role="alert" className={dialogNote} data-testid="feature-flag-enforce-recheck-alert">
          <span>{`Couldn't re-check readiness: ${r.recheck.error}`}</span>{" "}
          <button type="button" className={retryButton} onClick={() => void enforce()}>
            Retry
          </button>
        </div>
      )}
      {hint && (
        <p id={hintId} className={dialogNote} data-testid="feature-flag-enforce-hint">
          {hint}
        </p>
      )}
    </AlertConfirmDialog>
  );
}
