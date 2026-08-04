"use client";

import { useState } from "react";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { InlineNotice } from "@/components/common/InlineNotice";
import * as styles from "../BacklogItemDetail.css";
import * as gateStyles from "../GateVerdictBox.css";
import { ActionButtonLabel } from "./ActionButtonLabel";

const MIN_OVERRIDE_REASON_LENGTH = 5;

export interface ManualOverrideSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  defaultExpanded?: boolean;
  /**
   * Task 5.3.1c-style terminal-state guard, mirroring every other detail
   * section — disables the toggle once the item is archived/removed
   * elsewhere, even though the section itself always renders (see doc
   * comment below).
   */
  readOnly?: boolean;
  onOverride: (toStatus: BacklogItemStatus, reason: string) => Promise<void>;
}

/**
 * "Manual Override" block — the explicit, visually-distinguished escape
 * hatch for forcing a backlog item's status directly, separate from the
 * automated pipeline's status-conditional action buttons (ActionsSection).
 * Unlike PullRequestSection/ReviewingSection, this is deliberately NOT
 * status-conditional: it always renders (project 7a383b3b — "manual escape
 * hatch"), because the whole point is recovering an item however it got
 * stuck. `item.allowedTransitions` — computed server-side from the
 * authoritative WorkflowEngine state machine (see BacklogItem proto's
 * doc comment) — is the only source of which target statuses are offered;
 * this component never re-encodes the transition graph itself. Styled by
 * reusing GateVerdictBox's existing override trio (overrideToggle/
 * overrideForm/dangerButton) rather than inventing new tokens, per
 * .claude/rules/css-architecture.md.
 */
export function ManualOverrideSection({
  item,
  actionLoading,
  defaultExpanded = false,
  readOnly = false,
  onOverride,
}: ManualOverrideSectionProps) {
  const [showForm, setShowForm] = useState(false);
  const [targetStatus, setTargetStatus] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);

  const options = item.allowedTransitions ?? [];
  const reasonValid = reason.trim().length >= MIN_OVERRIDE_REASON_LENGTH;
  const canSubmit = targetStatus !== "" && reasonValid && actionLoading === null;

  const resetForm = () => {
    setShowForm(false);
    setTargetStatus("");
    setReason("");
    setError(null);
  };

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setError(null);
    try {
      await onOverride(targetStatus as BacklogItemStatus, reason.trim());
      resetForm();
    } catch (err) {
      // Surface the server's own rejection message (e.g. a CAS conflict, or
      // an invalid transition it re-validated independently of
      // allowedTransitions) rather than assuming success or swallowing it.
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <CollapsibleSection sectionKey="manual-override" title="Manual Override" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <button
          className={gateStyles.overrideToggle}
          aria-expanded={showForm}
          disabled={readOnly || options.length === 0}
          title={options.length === 0 ? "No transitions are available from this item's current status" : undefined}
          onClick={() => setShowForm((prev) => !prev)}
          data-testid="backlog-manual-override-toggle"
        >
          Override status directly {showForm ? "▾" : "▸"}
        </button>

        {showForm && (
          <div
            role="form"
            aria-label="Override backlog item status"
            className={gateStyles.overrideForm}
            data-testid="backlog-manual-override-form"
          >
            <label htmlFor="manual-override-status" className={gateStyles.formLabel}>
              New status
            </label>
            <select
              id="manual-override-status"
              className={styles.manualReviewSelect}
              value={targetStatus}
              onChange={(e) => setTargetStatus(e.target.value)}
              data-testid="backlog-manual-override-status"
            >
              <option value="">Select a status…</option>
              {options.map((status) => (
                <option key={status} value={status}>
                  {status}
                </option>
              ))}
            </select>

            <label htmlFor="manual-override-reason" className={gateStyles.formLabel}>
              Reason for override (required)
            </label>
            <textarea
              id="manual-override-reason"
              rows={3}
              placeholder="Explain why this item's status is being manually overridden…"
              aria-describedby="manual-override-hint"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className={gateStyles.formTextarea}
              data-testid="backlog-manual-override-reason"
            />
            <span id="manual-override-hint" className={gateStyles.formHint}>
              Enter at least {MIN_OVERRIDE_REASON_LENGTH} characters to continue.
            </span>

            {error && <InlineNotice message={error} data-testid="backlog-manual-override-error" />}

            <div className={gateStyles.formActions}>
              <button
                className={gateStyles.secondaryButton}
                onClick={resetForm}
                data-testid="backlog-manual-override-cancel"
              >
                Cancel
              </button>
              <button
                className={gateStyles.dangerButton}
                aria-disabled={!canSubmit}
                disabled={!canSubmit}
                aria-busy={actionLoading === "status_override"}
                onClick={() => void handleSubmit()}
                data-testid="backlog-manual-override-submit"
              >
                <ActionButtonLabel pending={actionLoading === "status_override"} label="Override Status" />
              </button>
            </div>
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
