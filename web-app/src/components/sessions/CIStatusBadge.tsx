// +feature: session:ci-status-badge
"use client";

import { timestampDate, type Timestamp } from "@bufbuild/protobuf/wkt";
import { formatRelativeTime } from "@/lib/utils/datetime";
import {
  badge, prBadge, prBadgeReady, prBadgeBlocking, prBadgePending, prBadgeUnknown, icon, text,
} from "./GitHubBadge.css";

interface CIStatusBadgeProps {
  checkConclusion?: string;
  prUrl?: string;
  prNumber?: number;
  lastChecked?: Timestamp;
}

interface CIStatusInfo {
  label: string;
  icon: string;
  variant: string;
}

/** Map a GitHub CI check conclusion to the badge's display state (AC1). */
function statusInfo(checkConclusion: string | undefined): CIStatusInfo {
  switch (checkConclusion) {
    case "success":
      return { label: "Passing", icon: "✅", variant: prBadgeReady };
    case "failure":
      return { label: "Failing", icon: "❌", variant: prBadgeBlocking };
    case "pending":
      return { label: "Pending", icon: "⏳", variant: prBadgePending };
    default:
      return { label: "No checks", icon: "⬤", variant: prBadgeUnknown };
  }
}

/**
 * Read-only CI status badge for the diff viewer header. Purely presentational over
 * already-delivered Session data (AC3) — renders nothing when the session has no
 * associated PR (AC7).
 */
export function CIStatusBadge({ checkConclusion, prUrl, prNumber, lastChecked }: CIStatusBadgeProps) {
  if (!prNumber) {
    return null;
  }

  const info = statusInfo(checkConclusion);
  const href = prUrl ? `${prUrl}/checks` : undefined;

  const tooltipParts = [`CI status: ${info.label}`];
  if (lastChecked) {
    tooltipParts.push(`checked ${formatRelativeTime(timestampDate(lastChecked).getTime())}`);
  }

  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className={`${badge} ${prBadge} ${info.variant}`}
      role="status"
      aria-label={`CI status: ${info.label}`}
      title={tooltipParts.join(" · ")}
      data-testid="ci-status-badge"
    >
      <span className={`${icon}`} aria-hidden="true">{info.icon}</span>
      <span className={`${text}`}>{info.label}</span>
    </a>
  );
}
