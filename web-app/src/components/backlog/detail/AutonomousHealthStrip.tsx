import React from "react";
import * as styles from "../BacklogItemDetail.css";

export interface AutonomousHealthStripItem {
  totalEstimatedCostUsd: number;
  reworkCapOverride?: number;
  linkedSessions: Array<{ role: string }>;
}

export function reworkAttemptCount(item: AutonomousHealthStripItem): number {
  return item.linkedSessions.filter((s) => s.role === "work").length;
}

export function AutonomousHealthStrip({ item }: { item: AutonomousHealthStripItem }) {
  const attempts = reworkAttemptCount(item);
  const cap = item.reworkCapOverride;
  const showCost = item.totalEstimatedCostUsd > 0;

  if (!showCost && attempts === 0 && cap === undefined) {
    return null;
  }

  const capLabel = cap === undefined ? "default" : cap === 0 ? "unlimited" : String(cap);

  return (
    <div className={styles.autonomousHealthStrip} data-testid="autonomous-health-strip">
      {showCost && (
        <span>
          Estimated cost: <strong>${item.totalEstimatedCostUsd.toFixed(4)}</strong>
        </span>
      )}
      <span>
        Rework attempts: <strong>{attempts}</strong> / {capLabel}
      </span>
    </div>
  );
}
