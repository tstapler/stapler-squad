/**
 * Task 0.1.8b (ADR-036 S-T5): the decision rule that turns the pool-contention measurements into the Phase D
 * default. The measurements themselves run in tests/e2e/files-transport-spike.spec.ts; the rule is pinned here so
 * the number in ADR-036 and the code that reads it cannot drift apart.
 */
export const SAVE_P95_BUDGET_MS = 2000;

export function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return NaN;
  const rank = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.min(sorted.length - 1, Math.max(0, rank))];
}

/** Phase D adapters (Epic 2.6) are on by default only if a save-shaped call is slow under the planned caps. */
export function phaseDAdaptersDefault(saveLatenciesMs: number[]): "on-by-default" | "advertised-off" {
  const sorted = [...saveLatenciesMs].sort((a, b) => a - b);
  return percentile(sorted, 95) > SAVE_P95_BUDGET_MS ? "on-by-default" : "advertised-off";
}

describe("pool contention decision rule (S-T5)", () => {
  it("percentile_should_UseNearestRank", () => {
    const v = Array.from({ length: 100 }, (_, i) => i + 1);
    expect(percentile(v, 50)).toBe(50);
    expect(percentile(v, 95)).toBe(95);
    expect(percentile([7], 95)).toBe(7);
    expect(percentile([], 50)).toBeNaN();
  });

  it("phaseDAdapters_should_BeOnByDefault_Only_When_SaveP95ExceedsTwoSeconds", () => {
    const fast = Array.from({ length: 20 }, () => 150);
    const slowTail = [...fast.slice(0, 18), 2500, 2600];
    expect(phaseDAdaptersDefault(fast)).toBe("advertised-off");
    expect(phaseDAdaptersDefault(slowTail)).toBe("on-by-default");
    expect(phaseDAdaptersDefault([2000, 2000, 2000])).toBe("advertised-off");
  });
});
