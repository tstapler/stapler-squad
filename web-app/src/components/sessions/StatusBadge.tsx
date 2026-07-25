"use client";

import { AttentionReason, DetectedStatus } from "@/gen/session/v1/types_pb";
import * as styles from "./StatusBadge.css";

type ReasonVariant = keyof typeof styles.reasonVariants;

interface StatusInfo {
  label: string;
  icon: string;
  variant: ReasonVariant;
}

// This pair of functions is the single canonical source of truth for the display
// strings used across the app for AttentionReason/DetectedStatus values. Other
// files (e.g. ReviewQueuePanel.tsx) must call into these rather than re-declaring
// their own copies of these literals — that duplication is exactly what the
// "no-raw-status-strings" no-restricted-syntax rule in .eslintrc.json guards
// against. The literals below are intentionally exempt since this is where they
// are defined, not duplicated.
/* eslint-disable no-restricted-syntax -- canonical definition site, see comment above */
export function getAttentionReasonInfo(reason: AttentionReason): StatusInfo {
  switch (reason) {
    case AttentionReason.APPROVAL_PENDING:
      return { label: "Approval Pending", icon: "🔒", variant: "approval" };
    case AttentionReason.INPUT_REQUIRED:
      return { label: "Input Required", icon: "✏️", variant: "input" };
    case AttentionReason.ERROR_STATE:
      return { label: "Error", icon: "⚠️", variant: "error" };
    case AttentionReason.IDLE_TIMEOUT:
    case AttentionReason.IDLE:
      return { label: "Idle", icon: "⏰", variant: "idle" };
    case AttentionReason.TASK_COMPLETE:
      return { label: "Complete", icon: "✅", variant: "complete" };
    case AttentionReason.UNCOMMITTED_CHANGES:
      return { label: "Uncommitted Changes", icon: "📝", variant: "uncommitted" };
    case AttentionReason.STALE:
      return { label: "Stale", icon: "⌛", variant: "stale" };
    case AttentionReason.WAITING_FOR_USER:
      return { label: "Your Input Needed", icon: "✏️", variant: "input" };
    case AttentionReason.TESTS_FAILING:
      return { label: "Tests Failing", icon: "❌", variant: "testsFailing" };
    default:
      return { label: "Unknown", icon: "●", variant: "unknown" };
  }
}

export function getDetectedStatusInfo(status: DetectedStatus): StatusInfo | null {
  switch (status) {
    case DetectedStatus.READY:
      return { label: "Ready", icon: "✅", variant: "complete" };
    case DetectedStatus.PROCESSING:
      return { label: "Processing", icon: "⚙️", variant: "processing" };
    case DetectedStatus.NEEDS_APPROVAL:
      return { label: "Needs Approval", icon: "🔒", variant: "approval" };
    case DetectedStatus.INPUT_REQUIRED:
      return { label: "Input Required", icon: "✏️", variant: "input" };
    case DetectedStatus.ERROR:
      return { label: "Error", icon: "⚠️", variant: "error" };
    case DetectedStatus.TESTS_FAILING:
      return { label: "Tests Failing", icon: "❌", variant: "testsFailing" };
    case DetectedStatus.IDLE:
      return { label: "Idle", icon: "⏰", variant: "idle" };
    case DetectedStatus.EXECUTING:
      return { label: "Executing", icon: "⚡", variant: "active" };
    case DetectedStatus.SUCCESS:
      return { label: "Success", icon: "✅", variant: "complete" };
    case DetectedStatus.WAITING_FOR_AGENT:
      return { label: "Waiting for Agent", icon: "⏳", variant: "processing" };
    case DetectedStatus.UNKNOWN:
      return null;
    case DetectedStatus.UNSPECIFIED:
      return null;
    default: {
      // Proto enums are forward-compatible: a newer server can send a
      // DetectedStatus value this deployed client bundle doesn't know about
      // yet. Render nothing rather than throwing, so one unrecognized wire
      // value can't crash the sessions UI. `_exhaustive: never` still gives a
      // compile error if a new case is added without also being handled here.
      const _exhaustive: never = status;
      console.warn("getDetectedStatusInfo: unrecognized DetectedStatus value", _exhaustive);
      return null;
    }
  }
}
/* eslint-enable no-restricted-syntax */

interface StatusBadgeProps {
  reason?: AttentionReason;
  detectedStatus?: DetectedStatus;
  title?: string;
  context?: string;
}

export function StatusBadge({ reason, detectedStatus, title, context }: StatusBadgeProps) {
  let info: StatusInfo | null;

  if (reason !== undefined) {
    info = getAttentionReasonInfo(reason);
  } else if (detectedStatus !== undefined) {
    info = getDetectedStatusInfo(detectedStatus);
  } else {
    return null;
  }

  if (info === null) {
    return null;
  }

  const tooltipText = context || title || info.label;

  return (
    <span
      className={`${styles.badge} ${styles.reasonVariants[info.variant]}`}
      role="status"
      aria-label={info.label}
      title={tooltipText}
    >
      <span className={styles.icon} aria-hidden="true">{info.icon}</span>
      {info.label}
    </span>
  );
}
