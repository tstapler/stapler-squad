"use client";
// +feature: backlog:manual-override

import { useState } from "react";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../GateVerdictBox.css";
import { InlineError } from "../InlineError";
import { getErrorMessage } from "@/lib/utils/connectError";

const MIN_OVERRIDE_REASON_LENGTH = 5;

export interface ManualOverrideSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
  /** True once the item is archived/removed — disables every write action here, same as the automated pipeline's own buttons. */
  readOnly?: boolean;
  /**
   * Forces a status transition with a required, audited reason. Options
   * offered by the caller must come from item.allowedTransitions — this
   * component never re-derives the transition graph itself.
   */
  onOverrideStatus: (toStatus: string, reason: string) => Promise<void>;
  /**
   * Manually links an existing PR to the item. Only offered while
   * item.status === "review" — the server enforces this too, but hiding the
   * form outside that status avoids a guaranteed round-trip failure.
   */
  onAssociatePR: (prUrl: string, prNumber: number) => Promise<void>;
}

/**
 * Operator escape hatch for a backlog item whose automation has gotten
 * wedged — force a status transition, or manually link a PR that shipped
 * out-of-band (no item_sessions link, so report_pr_created was never
 * callable). Always rendered regardless of status, deliberately separate
 * from the automated pipeline's status-conditional action buttons
 * (GateVerdictBox, PullRequestSection) — reuses that same override styling
 * (dangerButton/overrideForm/formTextarea) so it reads as "the same kind of
 * dangerous, deliberate action," not a new visual language.
 */
export function ManualOverrideSection({
  item,
  defaultExpanded,
  readOnly = false,
  onOverrideStatus,
  onAssociatePR,
}: ManualOverrideSectionProps) {
  const [toStatus, setToStatus] = useState("");
  const [reason, setReason] = useState("");
  const [statusPending, setStatusPending] = useState(false);
  const [statusError, setStatusError] = useState<string | null>(null);

  const [prUrl, setPrUrl] = useState("");
  const [prNumber, setPrNumber] = useState("");
  const [prPending, setPrPending] = useState(false);
  const [prError, setPrError] = useState<string | null>(null);

  const canSubmitStatus = toStatus !== "" && reason.trim().length >= MIN_OVERRIDE_REASON_LENGTH;
  const canAssociatePR = item.status === "review";
  const canSubmitPR = prUrl.trim() !== "" && /^\d+$/.test(prNumber.trim());

  async function handleOverrideSubmit() {
    setStatusPending(true);
    setStatusError(null);
    try {
      await onOverrideStatus(toStatus, reason.trim());
      setToStatus("");
      setReason("");
    } catch (err) {
      setStatusError(getErrorMessage(err, "Status override failed. Please try again."));
    } finally {
      setStatusPending(false);
    }
  }

  async function handleAssociatePRSubmit() {
    setPrPending(true);
    setPrError(null);
    try {
      await onAssociatePR(prUrl.trim(), parseInt(prNumber, 10));
      setPrUrl("");
      setPrNumber("");
    } catch (err) {
      setPrError(getErrorMessage(err, "PR association failed. Please try again."));
    } finally {
      setPrPending(false);
    }
  }

  return (
    <CollapsibleSection sectionKey="manual-override" title="Manual Overrides" defaultExpanded={defaultExpanded}>
      <div className={styles.overrideSection} data-testid="manual-override-section">
        <p className={styles.formHint}>
          Operator escape hatch for recovering a wedged item. Every action here bypasses the
          automated pipeline and is recorded to the item&apos;s audit trail.
        </p>

        {canAssociatePR && (
          <div role="form" aria-label="Associate existing PR" className={styles.overrideForm}>
            <label htmlFor="manual-override-pr-url" className={styles.formLabel}>
              Link an existing PR
            </label>
            <input
              id="manual-override-pr-url"
              type="text"
              placeholder="https://github.com/org/repo/pull/123"
              value={prUrl}
              onChange={(e) => setPrUrl(e.target.value)}
              className={styles.formTextarea}
              data-testid="manual-override-pr-url-input"
            />
            <input
              id="manual-override-pr-number"
              type="number"
              min={1}
              placeholder="PR number"
              value={prNumber}
              onChange={(e) => setPrNumber(e.target.value)}
              className={styles.formTextarea}
              data-testid="manual-override-pr-number-input"
            />
            {prError && (
              <InlineError
                type="transient"
                onRetry={() => setPrError(null)}
                onDismiss={() => setPrError(null)}
                customMessage={prError}
              />
            )}
            <div className={styles.formActions}>
              <button
                className={styles.dangerButton}
                disabled={prPending || readOnly || !canSubmitPR}
                aria-busy={prPending}
                onClick={() => void handleAssociatePRSubmit()}
                data-testid="manual-override-pr-submit"
              >
                Link PR
              </button>
            </div>
          </div>
        )}

        <div role="form" aria-label="Override status" className={styles.overrideForm}>
          <label htmlFor="manual-override-status-select" className={styles.formLabel}>
            Force status to
          </label>
          <select
            id="manual-override-status-select"
            value={toStatus}
            onChange={(e) => setToStatus(e.target.value)}
            className={styles.formTextarea}
            data-testid="manual-override-status-select"
          >
            <option value="">Select a status…</option>
            {(item.allowedTransitions ?? []).map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>

          <label htmlFor="manual-override-reason" className={styles.formLabel}>
            Reason (required)
          </label>
          <textarea
            id="manual-override-reason"
            rows={3}
            placeholder="Explain why this item's status is being manually overridden…"
            aria-describedby="manual-override-reason-hint"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            className={styles.formTextarea}
            data-testid="manual-override-reason-textarea"
          />
          <span id="manual-override-reason-hint" className={styles.formHint}>
            Enter at least {MIN_OVERRIDE_REASON_LENGTH} characters to continue.
          </span>

          {statusError && (
            <InlineError
              type="transient"
              onRetry={() => setStatusError(null)}
              onDismiss={() => setStatusError(null)}
              customMessage={statusError}
            />
          )}

          <div className={styles.formActions}>
            <button
              className={styles.dangerButton}
              disabled={statusPending || readOnly || !canSubmitStatus}
              aria-busy={statusPending}
              onClick={() => void handleOverrideSubmit()}
              data-testid="manual-override-status-submit"
            >
              Override Status
            </button>
          </div>
        </div>
      </div>
    </CollapsibleSection>
  );
}
