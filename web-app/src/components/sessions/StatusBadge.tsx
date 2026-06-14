"use client";

import { AttentionReason } from "@/gen/session/v1/types_pb";
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

function getDetectedStatusInfo(status: string): StatusInfo {
  switch (status) {
    case "Ready":
      return { label: "Ready", icon: "✅", variant: "complete" };
    case "Processing":
      return { label: "Processing", icon: "⚙️", variant: "processing" };
    case "Needs Approval":
      return { label: "Needs Approval", icon: "🔒", variant: "approval" };
    case "Input Required":
      return { label: "Input Required", icon: "✏️", variant: "input" };
    case "Error":
      return { label: "Error", icon: "⚠️", variant: "error" };
    case "Tests Failing":
      return { label: "Tests Failing", icon: "❌", variant: "testsFailing" };
    case "Idle":
      return { label: "Idle", icon: "⏰", variant: "idle" };
    case "Active":
      return { label: "Active", icon: "⚡", variant: "active" };
    case "Success":
      return { label: "Success", icon: "✅", variant: "complete" };
    default:
      return { label: status, icon: "●", variant: "unknown" };
  }
}

interface StatusBadgeProps {
  reason?: AttentionReason;
  detectedStatus?: string;
  title?: string;
  context?: string;
}

export function StatusBadge({ reason, detectedStatus, title, context }: StatusBadgeProps) {
  let info: StatusInfo;

  if (reason !== undefined) {
    info = getAttentionReasonInfo(reason);
  } else if (detectedStatus !== undefined) {
    info = getDetectedStatusInfo(detectedStatus);
  } else {
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
