// +feature: insights-dashboard
"use client";

import { useMemo } from "react";
import type { RoleCostBreakdown } from "@/gen/session/v1/insights_pb";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Cell,
} from "recharts";
import {
  chartCard,
  chartTitle,
  chartWrap,
  emptyChart,
  legendRow,
  legendItem,
  legendDot,
  unpricedLabel,
} from "./ModelBreakdownChart.css";
import { legendButton, legendButtonActive } from "./StageCostChart.css";
import { fmtCost, roleDisplayLabel } from "./insightsFormatters";

interface Props {
  roles: RoleCostBreakdown[];
  /** Currently cross-filtered role (Task 5.2.2c), or undefined/"" for none. */
  activeRole?: string;
  /** Fired on bar click or legend Enter/Space activation (Task 5.2.1c/Task 5.2.2). */
  onRoleClick?: (role: string) => void;
}

// Same stable palette ModelBreakdownChart uses — per ux.md's Accessibility
// section, reusing an already-shipped, presumably-audited palette introduces
// no new color decisions.
const PALETTE = [
  "#6366f1",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#3b82f6",
  "#ec4899",
  "#8b5cf6",
  "#14b8a6",
];

interface DataPoint {
  // role is the raw SessionRole value ("" for no-backlog-attribution) — the
  // same value SessionTokenSummary.sessionRole reports, so a bar click's
  // cross-filter (InsightsDashboard's roleFilter, compared against
  // SessionsTable's s.sessionRole) actually matches something. Never
  // substitute a display placeholder here; use roleDisplayLabel for text.
  role: string;
  cost: number;
  color: string;
  unpricedSessionCount: number;
}

function toDataPoints(roles: RoleCostBreakdown[]): DataPoint[] {
  return [...roles]
    .sort((a, b) => b.estimatedCostUsd - a.estimatedCostUsd)
    .map((r, i) => ({
      role: r.sessionRole,
      cost: r.estimatedCostUsd,
      color: PALETTE[i % PALETTE.length],
      unpricedSessionCount: r.unpricedSessionCount,
    }));
}

/** Builds the role="img" aria-label from the same sorted data the bars render, so it never drifts out of sync with what's visually shown (AC13). */
function buildChartAriaLabel(data: DataPoint[]): string {
  const parts = data.map((d) => {
    const unpriced = d.unpricedSessionCount > 0 ? ` (${d.unpricedSessionCount} unpriced)` : "";
    return `${roleDisplayLabel(d.role)} ${fmtCost(d.cost)}${unpriced}`;
  });
  return `Cost by stage: ${parts.join(", ")}`;
}

export function StageCostChart({ roles, activeRole, onRoleClick }: Props) {
  const data = useMemo(() => toDataPoints(roles), [roles]);

  if (data.length === 0) {
    return (
      <div className={chartCard} data-testid="stage-cost-chart">
        <div className={chartTitle}>Cost by Stage</div>
        <div className={emptyChart}>No data</div>
      </div>
    );
  }

  const ariaLabel = buildChartAriaLabel(data);

  const handleRoleActivate = (role: string) => onRoleClick?.(role);

  return (
    <div className={chartCard} data-testid="stage-cost-chart">
      <div className={chartTitle}>Cost by Stage</div>
      <div className={chartWrap} role="img" aria-label={ariaLabel}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 8 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(128,128,128,0.15)" vertical={false} />
            <XAxis
              dataKey="role"
              tickFormatter={roleDisplayLabel}
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={fmtCost}
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              width={56}
            />
            <Tooltip
              formatter={(v: unknown) => [fmtCost(Number(v)), "Cost"]}
              contentStyle={{ fontSize: "12px" }}
            />
            <Bar
              dataKey="cost"
              radius={[4, 4, 0, 0]}
              onClick={(entry: { payload?: DataPoint }) => {
                const point = entry?.payload;
                if (point?.role) handleRoleActivate(point.role);
              }}
              cursor={onRoleClick ? "pointer" : undefined}
            >
              {data.map((entry, index) => (
                <Cell key={`cell-${index}`} fill={entry.color} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className={legendRow}>
        {data.map((d) => {
          const isActive = activeRole === d.role;
          const displayRole = roleDisplayLabel(d.role);
          const label = `${displayRole}${d.unpricedSessionCount > 0 ? `, ${d.unpricedSessionCount} unpriced session${d.unpricedSessionCount === 1 ? "" : "s"}` : ""}`;
          return (
            <button
              key={d.role}
              type="button"
              role="button"
              tabIndex={0}
              className={`${legendItem} ${legendButton} ${isActive ? legendButtonActive : ""}`}
              onClick={() => handleRoleActivate(d.role)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  handleRoleActivate(d.role);
                }
              }}
              aria-pressed={onRoleClick ? isActive : undefined}
              aria-label={label}
              data-testid={`stage-cost-legend-${displayRole}`}
            >
              <div className={legendDot} style={{ background: d.color }} />
              {displayRole}
              {d.unpricedSessionCount > 0 && (
                <span className={unpricedLabel}> ({d.unpricedSessionCount} unpriced)</span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
