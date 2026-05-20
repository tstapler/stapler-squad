// +feature: insights-dashboard
"use client";

import type { ModelBreakdown } from "@/gen/session/v1/insights_pb";
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
} from "./ModelBreakdownChart.css";

interface Props {
  models: ModelBreakdown[];
}

// Stable palette for model families
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
  family: string;
  cost: number;
  color: string;
}

function toDataPoints(models: ModelBreakdown[]): DataPoint[] {
  return [...models]
    .sort((a, b) => b.estimatedCostUsd - a.estimatedCostUsd)
    .map((m, i) => ({
      family: m.modelFamily || "unknown",
      cost: m.estimatedCostUsd,
      color: PALETTE[i % PALETTE.length],
    }));
}

function fmtDollar(v: number): string {
  return `$${v.toFixed(3)}`;
}

export function ModelBreakdownChart({ models }: Props) {
  if (models.length === 0) {
    return (
      <div className={chartCard}>
        <div className={chartTitle}>Cost by Model</div>
        <div className={emptyChart}>No data</div>
      </div>
    );
  }

  const data = toDataPoints(models);

  return (
    <div className={chartCard}>
      <div className={chartTitle}>Cost by Model Family</div>
      <div className={chartWrap}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 8 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(128,128,128,0.15)" vertical={false} />
            <XAxis
              dataKey="family"
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={fmtDollar}
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              width={56}
            />
            <Tooltip
              formatter={(v: unknown) => [fmtDollar(Number(v)), "Cost"]}
              contentStyle={{ fontSize: "12px" }}
            />
            <Bar dataKey="cost" radius={[4, 4, 0, 0]}>
              {data.map((entry, index) => (
                <Cell key={`cell-${index}`} fill={entry.color} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className={legendRow}>
        {data.map((d) => (
          <div key={d.family} className={legendItem}>
            <div className={legendDot} style={{ background: d.color }} />
            {d.family}
          </div>
        ))}
      </div>
    </div>
  );
}
