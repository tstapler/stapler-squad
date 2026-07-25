"use client";

import { AttentionReason, DetectedStatus } from "@/gen/session/v1/types_pb";
import { assertNever } from "@/lib/utils/assertNever";
import * as styles from "./StatusBadge.css";

type ReasonVariant = keyof typeof styles.reasonVariants;

interface StatusInfo {
  label: string;
  icon: string;
  variant: ReasonVariant;
}

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
    default:
      return assertNever(status);
  }
}

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
