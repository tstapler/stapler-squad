import { useMemo } from "react";
import type { DailyTokenBucket } from "@/gen/session/v1/insights_pb";

export interface ProjectedCostResult {
  projectedMonthly: number;
  daysData: number;
  daysInMonth: number;
}

function getDaysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

/** Computes projected monthly spend from daily token buckets. Returns null when fewer than 7 days of data exist in the current calendar month. */
export function useProjectedCost(daily: DailyTokenBucket[]): ProjectedCostResult | null {
  return useMemo(() => {
    const now = new Date();
    const currentYear = now.getFullYear();
    const currentMonth = now.getMonth();

    const currentMonthBuckets = daily.filter((b) => {
      if (!b.date) return false;
      const d = new Date(Number(b.date.seconds) * 1000);
      return d.getFullYear() === currentYear && d.getMonth() === currentMonth;
    });

    const daysData = currentMonthBuckets.length;
    if (daysData < 7) return null;

    const totalCost = currentMonthBuckets.reduce((sum, b) => sum + b.estimatedCostUsd, 0);
    const avgDailyCost = totalCost / daysData;
    const daysInMonth = getDaysInMonth(currentYear, currentMonth);
    const projectedMonthly = avgDailyCost * daysInMonth;

    return { projectedMonthly, daysData, daysInMonth };
  }, [daily]);
}
