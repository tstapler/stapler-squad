// +feature: insights-activity-breakdown
"use client";

import { ActivityType } from "@/gen/session/v1/insights_pb";
import type { ActivityCostBreakdown } from "@/gen/session/v1/insights_pb";
import { EstimatedValue } from "@/components/ui/EstimatedValue";
import { fmtCost } from "./insightsFormatters";
import { tableCard, tableTitle, table, th, thRight, td, tdRight, empty } from "./ActivityBreakdownTable.css";

interface ActivityBreakdownTableProps {
  rows: ActivityCostBreakdown[];
}

// activityTypeLabels maps the generated ActivityType enum to a human label —
// never render the raw "ACTIVITY_TYPE_FEATURE_DEV"-style enum name.
const activityTypeLabels: Record<ActivityType, string> = {
  [ActivityType.UNSPECIFIED]: "Unclassified",
  [ActivityType.DEBUGGING]: "Debugging",
  [ActivityType.REFACTORING]: "Refactoring",
  [ActivityType.FEATURE_DEV]: "Feature Dev",
  [ActivityType.EXPLORATORY]: "Exploratory",
  [ActivityType.OTHER]: "Other",
};

const estimatedCostTooltip =
  "Modeled from per-turn pricing and ClassifyActivity's skill/tool-ratio heuristic — not a metered figure.";

// ActivityBreakdownTable is the dashboard-level cost-by-activity-type table,
// same shape as TopNTables.tsx's TopNTable. Every row's cost is a modeled
// figure (activity classification is a heuristic), so every row carries the
// shared EstimatedValue marker — never rendered as a plain "$" figure.
export function ActivityBreakdownTable({ rows }: ActivityBreakdownTableProps) {
  return (
    <div className={tableCard} data-testid="activity-breakdown-table">
      <div className={tableTitle}>Activity Breakdown</div>
      {rows.length === 0 ? (
        <div className={empty}>No data</div>
      ) : (
        <table className={table}>
          <thead>
            <tr>
              <th className={th}>Activity</th>
              <th className={thRight}>Sessions</th>
              <th className={thRight}>Cost</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={`${row.activityType}-${i}`}>
                <td className={td}>{activityTypeLabels[row.activityType] ?? "Unclassified"}</td>
                <td className={tdRight}>{row.sessionCount}</td>
                <td className={tdRight}>
                  <EstimatedValue title={estimatedCostTooltip}>{fmtCost(row.estimatedCostUsd)}</EstimatedValue>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
