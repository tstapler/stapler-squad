// +feature: insights-dashboard
"use client";

import type { ProjectedCostResult } from "@/lib/hooks/useProjectedCost";
import { card, label, value, sub, budgetInput, warningText } from "./ProjectedCostCard.css";

interface Props {
  projection: ProjectedCostResult;
  threshold: number | null;
  isHydrated: boolean;
  onThresholdChange: (v: number | null) => void;
}

function fmtCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(2)}`;
}

export function ProjectedCostCard({ projection, threshold, isHydrated, onThresholdChange }: Props) {
  const isWarning = isHydrated && threshold !== null && threshold > 0 && projection.projectedMonthly > threshold;

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value.trim();
    if (raw === "" || raw === "0") {
      onThresholdChange(null);
    } else {
      const parsed = parseFloat(raw);
      if (!isNaN(parsed) && parsed > 0) onThresholdChange(parsed);
    }
  }

  return (
    <div className={card({ warning: isWarning })}>
      <span className={label}>Projected this month</span>
      <span className={value}>{fmtCost(projection.projectedMonthly)}</span>
      <span className={sub}>Based on {projection.daysData} of {projection.daysInMonth} days</span>
      {isWarning && (
        <span className={warningText}>Over budget!</span>
      )}
      <input
        type="number"
        className={budgetInput}
        placeholder="$0.00 budget"
        min="0"
        step="0.01"
        value={threshold !== null && isHydrated ? threshold : ""}
        onChange={handleInputChange}
        aria-label="Monthly budget threshold in USD"
      />
    </div>
  );
}
