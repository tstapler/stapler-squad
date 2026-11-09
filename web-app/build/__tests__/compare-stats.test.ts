import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { compareToBaseline, findStatsFile, type GrowthTolerances } from "../compare-stats";

const TOL: GrowthTolerances = { jsBytes: 100, initialJsBytes: 10, largestChunkBytes: 10, largestLazyChunkBytes: 50 };
const base = { jsBytes: 1000, initialJsBytes: 500, largestChunkBytes: 500, largestLazyChunkBytes: 300 };

describe("compareToBaseline", () => {
  it("passes when nothing grew past its tolerance, including a shrink", () => {
    expect(compareToBaseline({ ...base, jsBytes: 1100, initialJsBytes: 400 }, base, TOL)).toEqual([]);
  });

  it("fails only the metric that grew, naming before and after", () => {
    const errs = compareToBaseline({ ...base, initialJsBytes: 511 }, base, TOL);
    expect(errs).toEqual(["growth: Initial JS grew 11 B vs main (500 -> 511), over the 10 B tolerance"]);
  });

  it("compares only to the baseline, so a large main (or one that shrank) does not fail an unchanged PR", () => {
    const big = { jsBytes: 9e9, initialJsBytes: 9e9, largestChunkBytes: 9e9, largestLazyChunkBytes: 9e9 };
    expect(compareToBaseline(big, big, TOL)).toEqual([]);
  });

  it("skips a metric the baseline predates", () => {
    const { largestLazyChunkBytes: _omit, ...old } = base;
    expect(compareToBaseline({ ...base, largestLazyChunkBytes: 99999 }, old, TOL)).toEqual([]);
  });
});

describe("findStatsFile", () => {
  it("finds a flat or nested stats file and returns undefined when absent", () => {
    const d = mkdtempSync(path.join(os.tmpdir(), "bl-"));
    mkdirSync(path.join(d, "x/y"), { recursive: true });
    writeFileSync(path.join(d, "x/y/s.json"), "{}");
    expect(findStatsFile(d, "s.json")).toBe(path.join(d, "x/y/s.json"));
    expect(findStatsFile(d, "nope.json")).toBeUndefined();
    expect(findStatsFile(path.join(d, "missing"), "s.json")).toBeUndefined();
  });
});

describe("compare-stats-cli", () => {
  const cli = path.resolve(__dirname, "../compare-stats-cli.ts");
  const run = (...args: string[]) => spawnSync("bun", [cli, ...args], { encoding: "utf8" });
  const dir = mkdtempSync(path.join(os.tmpdir(), "cli-"));
  const stats = (n: number) => JSON.stringify({ jsBytes: n, initialJsBytes: n, largestChunkBytes: n, largestLazyChunkBytes: n });
  const budgets = path.join(dir, "budgets.json");
  writeFileSync(budgets, JSON.stringify({ growthToleranceBytes: TOL }));
  writeFileSync(path.join(dir, "stats-minified.json"), stats(1000));
  mkdirSync(path.join(dir, "bl"));
  mkdirSync(path.join(dir, "same"));
  writeFileSync(path.join(dir, "same/stats-minified.json"), stats(1000));
  mkdirSync(path.join(dir, "smaller"));
  writeFileSync(path.join(dir, "smaller/stats-minified.json"), stats(900));
  const cur = path.join(dir, "stats-minified.json");

  it("skips with exit 0 when there is no baseline", () => {
    const r = run(cur, path.join(dir, "bl"), budgets);
    expect([r.status, r.stdout]).toEqual([0, expect.stringContaining("growth gate skipped")]);
  });
  it("passes when unchanged and fails when grown past tolerance", () => {
    expect(run(cur, path.join(dir, "same"), budgets).status).toBe(0);
    const r = run(cur, path.join(dir, "smaller"), budgets);
    expect([r.status, r.stderr]).toEqual([1, expect.stringContaining("growth: Initial JS grew 100 B")]);
  });
  it("fails on invalid tolerances, even with no baseline, and on a missing current file", () => {
    const bad = path.join(dir, "bad.json");
    writeFileSync(bad, "{}");
    expect(run(cur, path.join(dir, "bl"), bad).status).toBe(1);
    expect(run(path.join(dir, "nope.json"), path.join(dir, "bl"), budgets).status).toBe(1);
  });
});
