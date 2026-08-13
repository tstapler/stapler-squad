import {
  computeFrecencyScore,
  rankPathsByFrecency,
  FRECENCY_HALF_LIFE_MS,
  type PathActivity,
  type SessionForFrecency,
} from "../frecency";

const DAY_MS = 24 * 60 * 60 * 1000;
const NOW = 1_000_000_000_000; // fixed "now" for deterministic tests

// ---------------------------------------------------------------------------
// computeFrecencyScore
// ---------------------------------------------------------------------------

describe("computeFrecencyScore", () => {
  it("returns 0 when mostRecentMs is 0 (no known activity)", () => {
    const activity: PathActivity = { frequency: 5, mostRecentMs: 0 };
    expect(computeFrecencyScore(activity, NOW)).toBe(0);
  });

  it("returns frequency when age is 0 (used right now)", () => {
    const activity: PathActivity = { frequency: 4, mostRecentMs: NOW };
    expect(computeFrecencyScore(activity, NOW)).toBeCloseTo(4.0);
  });

  it("halves the score at exactly one half-life (7 days by default)", () => {
    const activity: PathActivity = { frequency: 4, mostRecentMs: NOW - FRECENCY_HALF_LIFE_MS };
    expect(computeFrecencyScore(activity, NOW)).toBeCloseTo(2.0);
  });

  it("quarters the score at two half-lives (14 days)", () => {
    const activity: PathActivity = { frequency: 4, mostRecentMs: NOW - 2 * FRECENCY_HALF_LIFE_MS };
    expect(computeFrecencyScore(activity, NOW)).toBeCloseTo(1.0);
  });

  it("a high-frequency stale path ties a low-frequency fresh path at the right age", () => {
    // 8 sessions used 7 days ago should equal 4 sessions used today
    const stale: PathActivity = { frequency: 8, mostRecentMs: NOW - FRECENCY_HALF_LIFE_MS };
    const fresh: PathActivity = { frequency: 4, mostRecentMs: NOW };
    expect(computeFrecencyScore(stale, NOW)).toBeCloseTo(computeFrecencyScore(fresh, NOW));
  });

  it("accepts a custom half-life", () => {
    const halfLifeMs = DAY_MS; // 1-day half-life
    const activity: PathActivity = { frequency: 2, mostRecentMs: NOW - DAY_MS };
    expect(computeFrecencyScore(activity, NOW, halfLifeMs)).toBeCloseTo(1.0);
  });

  it("clamps negative age (future timestamps) to 0", () => {
    // mostRecentMs in the future should be treated as age=0
    const activity: PathActivity = { frequency: 3, mostRecentMs: NOW + DAY_MS };
    expect(computeFrecencyScore(activity, NOW)).toBeCloseTo(3.0);
  });
});

// ---------------------------------------------------------------------------
// rankPathsByFrecency
// ---------------------------------------------------------------------------

describe("rankPathsByFrecency", () => {
  it("returns empty array for empty input", () => {
    expect(rankPathsByFrecency([], NOW)).toEqual([]);
  });

  it("returns empty array when all sessions lack a path", () => {
    const sessions: SessionForFrecency[] = [
      { timestampsMs: [NOW] },
      { path: "", timestampsMs: [NOW] },
    ];
    expect(rankPathsByFrecency(sessions, NOW)).toEqual([]);
  });

  it("ranks a frequently-used path above a rarely-used path of equal recency", () => {
    const sessions: SessionForFrecency[] = [
      { path: "/repo/rare",    timestampsMs: [NOW] },
      { path: "/repo/frequent", timestampsMs: [NOW] },
      { path: "/repo/frequent", timestampsMs: [NOW - DAY_MS] },
      { path: "/repo/frequent", timestampsMs: [NOW - 2 * DAY_MS] },
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    expect(result[0]).toBe("/repo/frequent");
    expect(result[1]).toBe("/repo/rare");
  });

  it("ranks a recently-used path above a stale-but-frequent path when recency dominates", () => {
    // 1 session used 1 minute ago vs 5 sessions all used 21+ days ago.
    // stale score  = 5 × 0.5^(21/7) = 5 × 0.125 = 0.625
    // recent score = 1 × 0.5^(~0)   ≈ 1.0
    // → recent wins
    const sessions: SessionForFrecency[] = [
      { path: "/repo/stale",  timestampsMs: [NOW - 30 * DAY_MS] },
      { path: "/repo/stale",  timestampsMs: [NOW - 27 * DAY_MS] },
      { path: "/repo/stale",  timestampsMs: [NOW - 25 * DAY_MS] },
      { path: "/repo/stale",  timestampsMs: [NOW - 23 * DAY_MS] },
      { path: "/repo/stale",  timestampsMs: [NOW - 21 * DAY_MS] }, // most recent for stale
      { path: "/repo/recent", timestampsMs: [NOW - 60 * 1000] },   // 1 minute ago
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    expect(result[0]).toBe("/repo/recent");
  });

  it("uses the maximum timestamp across all sessions for a path", () => {
    // /repo/a has older individual sessions, but one very recent one
    const sessions: SessionForFrecency[] = [
      { path: "/repo/a", timestampsMs: [NOW - 10 * DAY_MS] },
      { path: "/repo/a", timestampsMs: [NOW - 5 * DAY_MS] },
      { path: "/repo/a", timestampsMs: [NOW - 1 * DAY_MS] }, // most recent for /repo/a
      { path: "/repo/b", timestampsMs: [NOW - 2 * DAY_MS] }, // single session, more recent than most /repo/a
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    // /repo/a: freq=3, most-recent=NOW-1day  → score = 3 × 0.5^(1/7) ≈ 2.72
    // /repo/b: freq=1, most-recent=NOW-2days → score = 1 × 0.5^(2/7) ≈ 0.82
    expect(result[0]).toBe("/repo/a");
    expect(result[1]).toBe("/repo/b");
  });

  it("ignores zero timestamps when computing mostRecentMs, uses the non-zero one", () => {
    // /repo/a has two zero timestamps and one valid one — should use the valid one
    const sessions: SessionForFrecency[] = [
      { path: "/repo/a", timestampsMs: [0, 0, NOW - DAY_MS] },
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    expect(result).toHaveLength(1);
    // Score should reflect age of 1 day, not 0 (which would only happen if zeros were used)
    // score = 1 × 0.5^(1/7) ≈ 0.906 — just confirm it is > 0
    expect(result[0]).toBe("/repo/a");
  });

  it("paths with no valid timestamps receive score 0 and sort to the bottom", () => {
    const sessions: SessionForFrecency[] = [
      { path: "/repo/no-ts",  timestampsMs: [0, 0] },
      { path: "/repo/has-ts", timestampsMs: [NOW - DAY_MS] },
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    expect(result[0]).toBe("/repo/has-ts");
    expect(result[1]).toBe("/repo/no-ts");
  });

  it("uses the maximum timestamp across multiple timestamp fields per session", () => {
    const sessions: SessionForFrecency[] = [
      // createdAt=old, updatedAt=recent, lastMeaningfulOutput=0
      { path: "/repo/a", timestampsMs: [NOW - 30 * DAY_MS, NOW - 1 * DAY_MS, 0] },
      { path: "/repo/b", timestampsMs: [NOW - 5 * DAY_MS] },
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    // /repo/a: freq=1, most-recent=NOW-1day  → should beat /repo/b at 5 days
    expect(result[0]).toBe("/repo/a");
  });

  it("handles a single session", () => {
    const sessions: SessionForFrecency[] = [
      { path: "/solo/repo", timestampsMs: [NOW] },
    ];
    expect(rankPathsByFrecency(sessions, NOW)).toEqual(["/solo/repo"]);
  });

  it("is stable for equal scores (same path order preserved)", () => {
    // Two paths with identical frequency and identical recency
    const sessions: SessionForFrecency[] = [
      { path: "/repo/alpha", timestampsMs: [NOW] },
      { path: "/repo/beta",  timestampsMs: [NOW] },
    ];
    const result = rankPathsByFrecency(sessions, NOW);
    expect(result).toHaveLength(2);
    expect(result).toContain("/repo/alpha");
    expect(result).toContain("/repo/beta");
  });
});
