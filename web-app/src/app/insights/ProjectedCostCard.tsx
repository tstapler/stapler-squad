// +feature: insights-dashboard
"use client";

import { useState } from "react";
import type { ProjectedCostResult } from "@/lib/hooks/useProjectedCost";
import { card, label, value, sub, budgetInput, warningText, inputError } from "./ProjectedCostCard.css";
import { fmtCost } from "./insightsFormatters";

interface Props {
  projection: ProjectedCostResult;
  threshold: number | null;
  isHydrated: boolean;
  onThresholdChange: (v: number | null) => void;
}

export function ProjectedCostCard({ projection, threshold, isHydrated, onThresholdChange }: Props) {
  const isWarning = isHydrated && threshold !== null && threshold > 0 && projection.projectedMonthly > threshold;
  const [inputErrorMsg, setInputErrorMsg] = useState<string | null>(null);

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value.trim();
    if (raw === "") {
      setInputErrorMsg(null);
      onThresholdChange(null);
      return;
    }
    const parsed = parseFloat(raw);
    if (isNaN(parsed) || parsed <= 0) {
      setInputErrorMsg("Budget must be greater than zero");
      return;
    }
    setInputErrorMsg(null);
    onThresholdChange(parsed);
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
      {inputErrorMsg && (
        <p className={inputError}>{inputErrorMsg}</p>
      )}
    </div>
  );
}
