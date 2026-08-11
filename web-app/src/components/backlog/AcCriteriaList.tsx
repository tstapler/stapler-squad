"use client";
// +feature: backlog:ac-criteria-list

import type { AcCriterionStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./AcCriteriaList.css";

interface AcCriterion {
  index: number;
  text: string;
  status: AcCriterionStatus;
}

interface AcCriteriaListProps {
  criteria: AcCriterion[];
  readonly?: boolean;
}

function CheckboxIcon({ status }: { status: AcCriterionStatus }) {
  const cls =
    status === "done"
      ? styles.checkboxDone
      : status === "in_progress"
        ? styles.checkboxInProgress
        : styles.checkboxPending;

  const symbol = status === "done" ? "✓" : status === "in_progress" ? "~" : " ";
  const ariaLabel =
    status === "done" ? "Done" : status === "in_progress" ? "In progress" : "Pending";

  return (
    <span className={`${styles.checkbox} ${cls}`} aria-label={ariaLabel} role="img">
      {symbol}
    </span>
  );
}

const STATUS_LABEL: Record<AcCriterionStatus, string> = {
  pending: "Pending",
  in_progress: "In Progress",
  done: "Done",
};

const STATUS_LABEL_CLASS: Record<AcCriterionStatus, string> = {
  pending: styles.statusLabelPending,
  in_progress: styles.statusLabelInProgress,
  done: styles.statusLabelDone,
};

export function AcCriteriaList({ criteria, readonly = true }: AcCriteriaListProps) {
  if (criteria.length === 0) {
    return <p className={styles.empty}>No acceptance criteria defined.</p>;
  }

  return (
    <ol className={styles.list} aria-label="Acceptance criteria">
      {criteria.map((criterion) => (
        <li key={criterion.index} className={styles.item} data-testid="ac-criterion-item">
          <CheckboxIcon status={criterion.status} />
          <span className={styles.criterionIndex} aria-hidden="true">
            {criterion.index + 1}.
          </span>
          <span
            className={`${styles.criterionText} ${criterion.status === "done" ? styles.criterionTextDone : ""}`}
          >
            {criterion.text}
          </span>
          {!readonly && (
            <span
              className={`${styles.statusLabel} ${STATUS_LABEL_CLASS[criterion.status]}`}
              aria-label={`Status: ${STATUS_LABEL[criterion.status]}`}
            >
              {STATUS_LABEL[criterion.status]}
            </span>
          )}
        </li>
      ))}
    </ol>
  );
}
