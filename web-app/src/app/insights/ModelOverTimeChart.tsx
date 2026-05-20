// +feature: insights-dashboard
"use client";

import type { DailyTokenBucket } from "@/gen/session/v1/insights_pb";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import {
  chartCard,
  chartTitle,
  chartWrap,
  emptyChart,
  legend,
  legendItem,
  legendSwatch,
} from "./ModelOverTimeChart.css";

// Distinct colours per model family — maps first to most-expensive models.
const MODEL_COLORS: Record<string, string> = {
  "claude-opus-4":    "var(--primary)",
  "claude-sonnet-4":  "#22c55e",
  "claude-haiku-4":   "#f59e0b",
  "claude-opus-3":    "#8b5cf6",
  "claude-sonnet-3":  "#06b6d4",
  "claude-haiku-3":   "#ec4899",
};

const FALLBACK_COLORS = ["#64748b", "#a78bfa", "#fb923c", "#34d399"];

function colorForModel(family: string, index: number): string {
  return MODEL_COLORS[family] ?? FALLBACK_COLORS[index % FALLBACK_COLORS.length];
}

interface DataPoint {
  date: string;
  [model: string]: number | string; // one key per model family
}

interface Props {
  daily: DailyTokenBucket[];
  /** "cost" renders USD values; "tokens" renders raw token counts. */
  mode?: "cost" | "tokens";
}

function collectModels(daily: DailyTokenBucket[]): string[] {
  const seen = new Set<string>();
  for (const bucket of daily) {
    const map = bucket.costByModel as Record<string, number>;
    for (const k of Object.keys(map ?? {})) seen.add(k);
  }
  return Array.from(seen).sort();
}

function toDataPoints(daily: DailyTokenBucket[], models: string[], mode: "cost" | "tokens"): DataPoint[] {
  return daily.map((b) => {
    const d = b.date ? new Date(Number(b.date.seconds) * 1000) : new Date(0);
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const point: DataPoint = { date: label };
    const costMap = b.costByModel as Record<string, number>;
    const tokMap   = b.tokensByModel as Record<string, bigint | number>;
    for (const m of models) {
      if (mode === "cost") {
        point[m] = costMap?.[m] ?? 0;
      } else {
        const v = tokMap?.[m];
        point[m] = v != null ? Number(v) : 0;
      }
    }
    return point;
  });
}

function fmtTick(v: number, mode: "cost" | "tokens"): string {
  if (mode === "cost") return `$${v.toFixed(2)}`;
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(0)}K`;
  return String(v);
}

export function ModelOverTimeChart({ daily, mode = "cost" }: Props) {
  const models = collectModels(daily);

  if (daily.length === 0 || models.length === 0) {
    return (
      <div className={chartCard}>
        <div className={chartTitle}>Model Usage Over Time</div>
        <div className={emptyChart}>No data</div>
      </div>
    );
  }

  const data = toDataPoints(daily, models, mode);
  const label = mode === "cost" ? "Spend by Model (USD)" : "Tokens by Model";

  return (
    <div className={chartCard}>
      <div className={chartTitle}>{label}</div>
      <div className={chartWrap}>
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 8 }}>
            <defs>
              {models.map((m, i) => (
                <linearGradient key={m} id={`grad-${i}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor={colorForModel(m, i)} stopOpacity={0.3} />
                  <stop offset="95%" stopColor={colorForModel(m, i)} stopOpacity={0.0} />
                </linearGradient>
              ))}
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(128,128,128,0.15)" />
            <XAxis
              dataKey="date"
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={(v: number) => fmtTick(v, mode)}
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              width={56}
            />
            <Tooltip
              formatter={(v: unknown) =>
                mode === "cost"
                  ? `$${Number(v).toFixed(4)}`
                  : fmtTick(Number(v), "tokens")
              }
              contentStyle={{ fontSize: "12px" }}
            />
            {models.map((m, i) => (
              <Area
                key={m}
                type="monotone"
                dataKey={m}
                stackId="1"
                stroke={colorForModel(m, i)}
                fill={`url(#grad-${i})`}
                strokeWidth={1.5}
                dot={false}
                activeDot={{ r: 3 }}
              />
            ))}
          </AreaChart>
        </ResponsiveContainer>
      </div>
      <div className={legend}>
        {models.map((m, i) => (
          <div key={m} className={legendItem}>
            <div
              className={legendSwatch}
              style={{ background: colorForModel(m, i) }}
            />
            {m}
          </div>
        ))}
      </div>
    </div>
  );
}
