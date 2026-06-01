import { renderHook } from "@testing-library/react";
import { useProjectedCost } from "@/lib/hooks/useProjectedCost";
import type { DailyTokenBucket } from "@/gen/session/v1/insights_pb";

function makeBucket(dateStr: string, cost: number): DailyTokenBucket {
  const d = new Date(dateStr + "T12:00:00Z");
  return {
    date: { seconds: BigInt(Math.floor(d.getTime() / 1000)), nanos: 0 },
    estimatedCostUsd: cost,
    totalInputTokens: BigInt(0),
    totalOutputTokens: BigInt(0),
    cacheReadTokens: BigInt(0),
    sessionCount: 1,
    costByModel: {},
    tokensByModel: {},
  } as unknown as DailyTokenBucket;
}

function currentMonthDateStr(day: number): string {
  const now = new Date();
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, "0");
  const d = String(day).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

describe("useProjectedCost", () => {
  it("useProjectedCost_should_returnNull_When_fewerThan7DaysInCurrentMonth", () => {
    const daily = [1, 2, 3, 4, 5, 6].map((day) =>
      makeBucket(currentMonthDateStr(day), 1.0)
    );
    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).toBeNull();
  });

  it("useProjectedCost_should_returnNonNull_When_atLeast7DaysInCurrentMonth", () => {
    const daily = [1, 2, 3, 4, 5, 6, 7].map((day) =>
      makeBucket(currentMonthDateStr(day), 1.0)
    );
    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
  });

  it("useProjectedCost_should_computeCorrectProjection_When_given10DaysOfSpend", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-05-15T12:00:00Z"));

    const daily = Array.from({ length: 10 }, (_, i) =>
      makeBucket(`2026-05-${String(i + 1).padStart(2, "0")}`, 2.0)
    );

    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
    if (result.current) {
      expect(result.current.daysData).toBe(10);
      expect(result.current.daysInMonth).toBe(31);
      expect(result.current.projectedMonthly).toBeCloseTo(2.0 * 31, 5);
    }

    jest.useRealTimers();
  });

  it("useProjectedCost_should_excludePriorMonthData_When_mixedMonthsProvided", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-05-15T12:00:00Z"));

    const daily = [
      ...Array.from({ length: 5 }, (_, i) =>
        makeBucket(`2026-04-${String(i + 1).padStart(2, "0")}`, 100.0)
      ),
      ...Array.from({ length: 7 }, (_, i) =>
        makeBucket(`2026-05-${String(i + 1).padStart(2, "0")}`, 1.0)
      ),
    ];

    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
    if (result.current) {
      // Prior month data (100.0/day) should not inflate the projection
      expect(result.current.projectedMonthly).toBeCloseTo(1.0 * 31, 5);
    }

    jest.useRealTimers();
  });
});
