"use client";
// +feature: backlog:ac-criteria-list

import { useState } from "react";
import type { AcCriterionStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./AcCriteriaList.css";

interface AcCriterion {
  index: number;
  text: string;
  status: AcCriterionStatus;
  waiverReason?: string;
  waivedBy?: string;
  waivedAt?: string;
}

/** "Waived by <who> at <when>"; legacy waivers recorded before the audit trail have neither. */
function waivedByLine(c: AcCriterion): string {
  const who = c.waivedBy || "unknown caller (recorded before audit trail)";
  const parsed = c.waivedAt ? new Date(c.waivedAt) : null;
  const when = parsed && !Number.isNaN(parsed.getTime()) ? parsed.toLocaleString() : "unknown time";
  const unverified = who.includes("(verified)") ? "" : " (identity not verified)";
  return `Waived by ${who} at ${when}${unverified}`;
}

interface AcCriteriaListProps {
  criteria: AcCriterion[];
  readonly?: boolean;
  /** Audited waive action; when given, unmet criteria show a Waive button. */
  onWaive?: (criterionIndex: number, reason: string) => Promise<void>;
}

const CHECKBOX_CLASS: Record<AcCriterionStatus, string> = {
  done: styles.checkboxDone,
  waived: styles.checkboxWaived,
  in_progress: styles.checkboxInProgress,
  pending: styles.checkboxPending,
};
const CHECKBOX_SYMBOL: Record<AcCriterionStatus, string> = { done: "✓", waived: "⊘", in_progress: "~", pending: " " };
const CHECKBOX_LABEL: Record<AcCriterionStatus, string> = {
  done: "Done",
  waived: "Waived",
  in_progress: "In progress",
  pending: "Pending",
};

function CheckboxIcon({ status }: { status: AcCriterionStatus }) {
  return (
    <span className={`${styles.checkbox} ${CHECKBOX_CLASS[status]}`} aria-label={CHECKBOX_LABEL[status]} role="img">
      {CHECKBOX_SYMBOL[status]}
    </span>
  );
}

const STATUS_LABEL: Record<AcCriterionStatus, string> = {
  pending: "Pending",
  in_progress: "In Progress",
  done: "Done",
  waived: "Waived",
};

const STATUS_LABEL_CLASS: Record<AcCriterionStatus, string> = {
  pending: styles.statusLabelPending,
  in_progress: styles.statusLabelInProgress,
  done: styles.statusLabelDone,
  waived: styles.statusLabelWaived,
};

/** Inline reason form for waiving one criterion; the reason is mandatory. */
function WaiveForm({ index, onSubmit, onCancel }: { index: number; onSubmit: (reason: string) => Promise<void>; onCancel: () => void }) {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const canSubmit = reason.trim() !== "" && !busy;
  return (
    <form
      className={styles.waiveForm}
      onSubmit={(e) => {
        e.preventDefault();
        if (!canSubmit) return;
        setBusy(true);
        void onSubmit(reason.trim()).finally(() => setBusy(false));
      }}
    >
      <input
        className={styles.waiveInput}
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="Why is this waived? (required)"
        aria-label={`Waiver reason for criterion ${index + 1}`}
        data-testid={`ac-waive-reason-${index}`}
        autoFocus
      />
      <button type="submit" className={styles.waiveButton} disabled={!canSubmit} data-testid={`ac-waive-confirm-${index}`}>
        Confirm waiver
      </button>
      <button type="button" className={styles.waiveButton} onClick={onCancel}>{/* analytics-exempt */}
        Cancel
      </button>
    </form>
  );
}

export function AcCriteriaList({ criteria, readonly = true, onWaive }: AcCriteriaListProps) {
  const [waivingIndex, setWaivingIndex] = useState<number | null>(null);
  if (criteria.length === 0) {
    return <p className={styles.empty}>No acceptance criteria defined.</p>;
  }

  return (
    <ol className={styles.list} aria-label="Acceptance criteria">
      {criteria.map((criterion) => {
        const satisfied = criterion.status === "done" || criterion.status === "waived";
        return (
          <li key={criterion.index} className={styles.itemWrap} data-testid="ac-criterion-item">
            <div className={styles.item}>
              <CheckboxIcon status={criterion.status} />
              <span className={styles.criterionIndex} aria-hidden="true">
                {criterion.index + 1}.
              </span>
              <span className={`${styles.criterionText} ${satisfied ? styles.criterionTextDone : ""}`}>
                {criterion.text}
              </span>
              {(!readonly || criterion.status === "waived") && (
                <span
                  className={`${styles.statusLabel} ${STATUS_LABEL_CLASS[criterion.status]}`}
                  aria-label={`Status: ${STATUS_LABEL[criterion.status]}`}
                  title={criterion.status === "waived" ? `Waived: ${criterion.waiverReason ?? ""}` : undefined}
                  data-testid={criterion.status === "waived" ? "ac-waived-chip" : undefined}
                >
                  {STATUS_LABEL[criterion.status]}
                </span>
              )}
              {onWaive && !satisfied && waivingIndex !== criterion.index && (
                <button
                  type="button"
                  className={styles.waiveButton}
                  onClick={() => setWaivingIndex(criterion.index)}
                  data-testid={`ac-waive-${criterion.index}`}
                >{/* analytics-exempt */}
                  Waive
                </button>
              )}
            </div>
            {criterion.status === "waived" && criterion.waiverReason && (
              <p className={styles.waiverReason}>Waiver reason: {criterion.waiverReason}</p>
            )}
            {criterion.status === "waived" && (
              <p className={styles.waiverReason} data-testid={`ac-waived-by-${criterion.index}`}>
                {waivedByLine(criterion)}
              </p>
            )}
            {onWaive && waivingIndex === criterion.index && (
              <WaiveForm
                index={criterion.index}
                onCancel={() => setWaivingIndex(null)}
                onSubmit={async (reason) => {
                  await onWaive(criterion.index, reason);
                  setWaivingIndex(null);
                }}
              />
            )}
          </li>
        );
      })}
    </ol>
  );
}
