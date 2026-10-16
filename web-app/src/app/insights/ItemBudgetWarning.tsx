// +feature: backlog:item-budget-warning
"use client";

import { banner, icon, text } from "./ItemBudgetWarning.css";
import { fmtCost } from "./insightsFormatters";

interface Props {
  /** Per-item soft-budget-warning threshold in USD. `undefined` means no threshold is configured (see BacklogItem.costBudgetThresholdUsd). */
  thresholdUsd: number | undefined;
  /** The item's current total cost in USD (BacklogItem.totalEstimatedCostUsd). */
  totalCostUsd: number;
}

/**
 * Per-item budget warning banner (design/ux.md Surface D2) — visually
 * consistent with ProjectedCostCard's warning styling but a genuinely
 * separate component: it answers "is THIS item over budget," not "is total
 * monthly spend over budget." Renders null (no empty box) whenever no
 * threshold is configured, or the item hasn't crossed it yet. Advisory only:
 * no dismiss action, since dismissing wouldn't change the underlying fact.
 */
export function ItemBudgetWarning({ thresholdUsd, totalCostUsd }: Props) {
  if (thresholdUsd === undefined) return null;
  if (totalCostUsd < thresholdUsd) return null;

  return (
    <div className={banner} role="status" data-testid="item-budget-warning">
      <span className={icon} aria-hidden="true">
        ⚠
      </span>
      <span className={text}>
        {`Over budget: ${fmtCost(totalCostUsd)} spent, threshold ${fmtCost(thresholdUsd)}`}
      </span>
    </div>
  );
}
