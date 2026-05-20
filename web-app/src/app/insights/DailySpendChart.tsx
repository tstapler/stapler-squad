// +feature: insights-dashboard
"use client";

import type { DailyTokenBucket } from "@/gen/session/v1/insights_pb";
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { chartCard, chartTitle, chartWrap, emptyChart } from "./DailySpendChart.css";

interface Props {
  daily: DailyTokenBucket[];
}

interface DataPoint {
  date: string;
  cost: number;
}

function toDataPoints(daily: DailyTokenBucket[]): DataPoint[] {
  return daily.map((b) => {
    const d = b.date
      ? new Date(Number(b.date.seconds) * 1000)
      : new Date(0);
    const label = d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    return { date: label, cost: b.estimatedCostUsd };
  });
}

function fmtDollar(v: number): string {
  return `$${v.toFixed(3)}`;
}

export function DailySpendChart({ daily }: Props) {
  if (daily.length === 0) {
    return (
      <div className={chartCard}>
        <div className={chartTitle}>Daily Spend</div>
        <div className={emptyChart}>No data</div>
      </div>
    );
  }

  const data = toDataPoints(daily);

  return (
    <div className={chartCard}>
      <div className={chartTitle}>Daily Spend (USD)</div>
      <div className={chartWrap}>
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 8 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(128,128,128,0.15)" />
            <XAxis
              dataKey="date"
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
            <Line
              type="monotone"
              dataKey="cost"
              dot={false}
              strokeWidth={2}
              stroke="var(--primary)"
              activeDot={{ r: 4 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
