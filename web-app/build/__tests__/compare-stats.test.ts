import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { compareToBaseline, findStatsFile, selectBaselineArtifactId, type GrowthTolerances } from "../compare-stats";

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

describe("selectBaselineArtifactId", () => {
  const art = (id: number, sha: string, branch = "main", expired = false) => ({ id, expired, workflow_run: { head_sha: sha, head_branch: branch } });

  it("picks only the main-branch artifact at the exact base sha", () => {
    expect(selectBaselineArtifactId([art(1, "aaa", "feature"), art(2, "bbb"), art(3, "aaa")], "aaa")).toBe(3);
  });

  it("never falls back to an older commit, an expired artifact or a PR branch", () => {
    expect(selectBaselineArtifactId([art(1, "old"), art(2, "aaa", "main", true), art(3, "aaa", "pr")], "aaa")).toBeUndefined();
  });
});
