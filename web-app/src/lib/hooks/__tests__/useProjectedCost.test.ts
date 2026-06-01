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

const utcMidnight = (year: number, month: number, day: number): bigint =>
  BigInt(Math.floor(Date.UTC(year, month, day) / 1000));

function makeBucketFromUTC(
  year: number,
  month: number,
  day: number,
  cost: number
): DailyTokenBucket {
  return {
    date: { seconds: utcMidnight(year, month, day), nanos: 0 },
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

  it("useProjectedCost_should_compute28DaysInMonth_When_FebruaryNonLeapYear", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2025-02-15T12:00:00Z"));

    // 10 daily buckets for February 2025 (non-leap year), month=1 (0-indexed)
    const daily = Array.from({ length: 10 }, (_, i) =>
      makeBucketFromUTC(2025, 1, i + 1, 1.0)
    );

    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
    if (result.current) {
      expect(result.current.daysInMonth).toBe(28);
    }

    jest.useRealTimers();
  });

  it("useProjectedCost_should_compute29DaysInMonth_When_FebruaryLeapYear", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2024-02-15T12:00:00Z"));

    // 10 daily buckets for February 2024 (leap year), month=1 (0-indexed)
    const daily = Array.from({ length: 10 }, (_, i) =>
      makeBucketFromUTC(2024, 1, i + 1, 1.0)
    );

    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
    if (result.current) {
      expect(result.current.daysInMonth).toBe(29);
    }

    jest.useRealTimers();
  });

  it("useProjectedCost_should_excludeBuckets_When_UTCMonthMismatch", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-05-15T12:00:00Z"));

    // One bucket from April 30, 2026 UTC (month=3, 0-indexed)
    // Seven buckets from May 2026 UTC (month=4, 0-indexed)
    const daily = [
      makeBucketFromUTC(2026, 3, 30, 100.0),
      ...Array.from({ length: 7 }, (_, i) =>
        makeBucketFromUTC(2026, 4, i + 1, 1.0)
      ),
    ];

    const { result } = renderHook(() => useProjectedCost(daily));
    expect(result.current).not.toBeNull();
    if (result.current) {
      // April bucket must not be counted — daysData should be 7, not 8
      expect(result.current.daysData).toBe(7);
    }

    jest.useRealTimers();
  });
});
