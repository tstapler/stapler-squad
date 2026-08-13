// +feature: insights-dashboard
"use client";

import { badge, badgeVariant } from "./TokenBadge.css";

export interface TokenBadgeProps {
  /** Cost in USD */
  costUsd: number;
  /** Warn threshold in USD (default: no warning) */
  warnThresholdUsd?: number;
  /** Alert threshold in USD (default: no alert) */
  alertThresholdUsd?: number;
  className?: string;
}

function fmtCost(usd: number): string {
  if (usd === 0) return "$0";
  if (usd < 0.001) return `$${usd.toFixed(4)}`;
  if (usd < 0.01) return `$${usd.toFixed(3)}`;
  if (usd < 1) return `$${usd.toFixed(2)}`;
  return `$${usd.toFixed(2)}`;
}

type Variant = "normal" | "warning" | "alert";

function getVariant(
  costUsd: number,
  warnThresholdUsd?: number,
  alertThresholdUsd?: number
): Variant {
  if (alertThresholdUsd !== undefined && costUsd >= alertThresholdUsd) return "alert";
  if (warnThresholdUsd !== undefined && costUsd >= warnThresholdUsd) return "warning";
  return "normal";
}

/**
 * TokenBadge renders a compact cost pill for inline use in session cards.
 * Colour-shifts to warning/alert when thresholds are exceeded.
 */
export function TokenBadge({
  costUsd,
  warnThresholdUsd,
  alertThresholdUsd,
  className,
}: TokenBadgeProps) {
  const variant = getVariant(costUsd, warnThresholdUsd, alertThresholdUsd);
  const classes = [badge, badgeVariant[variant], className].filter(Boolean).join(" ");

  return (
    <span className={classes} title={`Estimated cost: $${costUsd.toFixed(6)}`}>
      {fmtCost(costUsd)}
    </span>
  );
}
