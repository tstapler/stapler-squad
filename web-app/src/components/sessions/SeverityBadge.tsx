"use client";
// +feature: severity-badge

import { RISK_LEVEL_LABELS } from "@/lib/sessions/riskLevel";
import * as styles from "./SeverityBadge.css";

type SeverityVariant = "critical" | "high" | "medium" | "low" | "unknown";

interface SeverityInfo {
  label: string;
  abbr: string;
  icon: string;
  variant: SeverityVariant;
}

// getRiskLevelInfo maps a classifier.RiskLevel string ("low"/"medium"/"high"/"critical") to
// display info. "" or any unrecognized value renders the distinct "not recorded" state — never
// falls back to "Low" (ADR-001: under-communicating risk is worse than over-communicating it).
export function getRiskLevelInfo(riskLevel: string): SeverityInfo {
  switch (riskLevel) {
    case "critical":
      return { label: RISK_LEVEL_LABELS.critical, abbr: "CRIT", icon: "⛔", variant: "critical" };
    case "high":
      return { label: RISK_LEVEL_LABELS.high, abbr: "HIGH", icon: "🔴", variant: "high" };
    case "medium":
      return { label: RISK_LEVEL_LABELS.medium, abbr: "MED", icon: "🟠", variant: "medium" };
    case "low":
      return { label: RISK_LEVEL_LABELS.low, abbr: "LOW", icon: "🟢", variant: "low" };
    default:
      return { label: "Severity not recorded", abbr: "N/A", icon: "⚪", variant: "unknown" };
  }
}

interface SeverityBadgeProps {
  riskLevel: string;
  compact?: boolean;
}

export function SeverityBadge({ riskLevel, compact = false }: SeverityBadgeProps) {
  const info = getRiskLevelInfo(riskLevel);
  const ariaLabel = info.variant === "unknown" ? info.label : `${info.label} risk`;

  return (
    // role="img" (not "status"): this badge is static once mounted, not a live region.
    // role="status" implies aria-live="polite" — with many badges on one page (every
    // ReviewQueuePanel row, every ApprovalRulesPanel row), that risks a burst of
    // screen-reader announcements on initial render/re-sort instead of announcing only
    // genuine changes. "img" still exposes aria-label as a single readable unit.
    <span
      className={styles.badge({ level: info.variant, compact })}
      role="img"
      aria-label={ariaLabel}
      title={ariaLabel}
      data-testid={`severity-badge-${riskLevel || "unrecorded"}`}
    >
      <span className={styles.icon} aria-hidden="true">{info.icon}</span>
      {compact ? info.abbr : info.label}
    </span>
  );
}
