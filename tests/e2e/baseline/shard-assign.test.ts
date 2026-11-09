import { assignShards, fileFilter, planDrift, shardLoads, timingsFromReport } from "../scripts/shard-assign";

const timings = { specSeconds: { "big.spec.ts": 900, "mid-a.spec.ts": 300, "mid-b.spec.ts": 300, "small-a.spec.ts": 100, "small-b.spec.ts": 100, "tiny.spec.ts": 10 } };
const files = Object.keys(timings.specSeconds);

describe("shard-assign", () => {
  test("SA1: every file lands on exactly one shard", () => {
    const shards = assignShards(files, timings, 3);
    expect(shards.flat().sort()).toEqual([...files].sort());
  });

  test("SA2: the heaviest shard is the single biggest file (the lower bound) and the others share the rest", () => {
    const loads = shardLoads(files, timings, 3).sort((a, b) => a - b);
    expect(loads).toEqual([400, 410, 900]);
  });

  test("SA3: a spec missing from the timings is still assigned, at the median cost", () => {
    const shards = assignShards([...files, "new.spec.ts"], timings, 3);
    expect(shards.flat()).toContain("new.spec.ts");
  });

  test("SA4: assignment is deterministic regardless of input order", () => {
    expect(assignShards([...files].reverse(), timings, 3)).toEqual(assignShards(files, timings, 3));
  });

  test("SA5: the file filter is anchored so files.spec.ts cannot also select profiles.spec.ts", () => {
    const re = new RegExp(fileFilter("files.spec.ts"));
    expect(re.test("/w/stapler-squad/tests/e2e/files.spec.ts")).toBe(true);
    expect(re.test("/w/stapler-squad/tests/e2e/profiles.spec.ts")).toBe(false);
    expect(re.test("/w/stapler-squad/tests/e2e/sub/files.spec.ts")).toBe(false);
  });

  test("SA6: timings sum every attempt of every project, so retry-heavy specs weigh more", () => {
    const report = {
      suites: [
        { specs: [{ file: "a.spec.ts", tests: [{ results: [{ duration: 10_000 }, { duration: 20_000 }] }, { results: [{ duration: 5_000 }] }] }] },
        { suites: [{ specs: [{ file: "b.spec.ts", tests: [{ results: [{ duration: 1_000 }] }] }] }] },
      ],
    };
    expect(timingsFromReport(report).specSeconds).toEqual({ "a.spec.ts": 35, "b.spec.ts": 1 });
  });

  test("SA7: planDrift measures the committed plan against fresh durations and flags a stale file", () => {
    const committed = { specSeconds: { "a.spec.ts": 100, "b.spec.ts": 100, "c.spec.ts": 100, "d.spec.ts": 100 } };
    const balanced = planDrift(committed, committed, 2);
    expect(balanced.maxOverMean).toBeCloseTo(1, 5);
    // a.spec.ts really takes 5x as long as recorded: whichever shard holds it is now far heavier.
    const drifted = planDrift(committed, { specSeconds: { ...committed.specSeconds, "a.spec.ts": 500 } }, 2);
    expect(drifted.maxOverMean).toBeGreaterThan(1.15);
  });
});
