"use client";
// +feature: severity-badge

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
      return { label: "Critical", abbr: "CRIT", icon: "⛔", variant: "critical" };
    case "high":
      return { label: "High", abbr: "HIGH", icon: "🔴", variant: "high" };
    case "medium":
      return { label: "Medium", abbr: "MED", icon: "🟠", variant: "medium" };
    case "low":
      return { label: "Low", abbr: "LOW", icon: "🟢", variant: "low" };
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
    <span
      className={styles.badge({ level: info.variant, compact })}
      role="status"
      aria-label={ariaLabel}
      title={ariaLabel}
      data-testid={`severity-badge-${riskLevel || "unrecorded"}`}
    >
      <span className={styles.icon} aria-hidden="true">{info.icon}</span>
      {compact ? info.abbr : info.label}
    </span>
  );
}
