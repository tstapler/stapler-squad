// Selection, grouping and merge logic of scripts/e2e-confirm.ts, end to end through the real comparator:
// a load-flaky test must stop gating once its isolated rerun passes, a real failure must keep gating.
import { compare, type Baseline, type PwReport } from "../../../scripts/e2e-baseline-compare";
import { MAX_CONFIRM_TESTS, mergeConfirmation, planGroups, selectCandidates, summaryMarkdown } from "../../../scripts/e2e-confirm";

type Att = { retry: number; status: string }[];
const PASS: Att = [{ retry: 0, status: "passed" }];
const HARD: Att = [{ retry: 0, status: "failed" }, { retry: 1, status: "failed" }];
const SKIP: Att = [{ retry: 0, status: "skipped" }, { retry: 1, status: "skipped" }];
const FILE = "a.spec.ts";

const report = (tests: Record<string, Att>, project = "chromium"): PwReport => ({
  suites: [{ file: FILE, specs: Object.entries(tests).map(([title, results]) => ({ file: FILE, title, tests: [{ projectName: project, results }] })) }],
});
const baseline = (titles: string[], project = "chromium"): Baseline => ({
  specs: titles.map((title) => ({ spec: FILE, project, title, class: "pass" as const })),
});
const run = (bun: Record<string, Att>, project = "chromium") => {
  const next = Object.fromEntries(Object.keys(bun).map((t) => [t, PASS]));
  const input = { baseline: baseline(Object.keys(bun), project), expectedDiff: { entries: [] }, nextReport: report(next, project) };
  const bunReport = report(bun, project);
  const result = compare({ ...input, bunReports: [bunReport] });
  return { input, bunReport, findings: result.findings, selection: selectCandidates(result.findings, bunReport) };
};
const gateAfter = (r: ReturnType<typeof run>, rerun: Record<string, Att>, project = "chromium") => {
  const merged = mergeConfirmation(r.bunReport, [report(rerun, project)], r.selection);
  return { merged, exitCode: compare({ ...r.input, bunReports: [merged.report] }).exitCode };
};

describe("e2e-confirm selection", () => {
  test("S1: nothing hard-failed -> status none", () => {
    expect(run({ t1: PASS }).selection.status).toBe("none");
  });

  test("S2: a Bun-only hard failure is selected with its project and leaf title", () => {
    const sel = run({ ok: PASS, bad: HARD }).selection;
    expect(sel.status).toBe("rerun");
    expect(sel.candidates).toEqual([{ spec: FILE, project: "chromium", title: "bad", leafTitle: "bad" }]);
  });

  test("S3: a skipped sibling is not rerun (its root is)", () => {
    const sel = run({ root: HARD, victim: SKIP }).selection;
    expect(sel.candidates.map((c) => c.title)).toEqual(["root"]);
  });

  test("S5: a test with no result (cut off by the shard's global timeout) is selected for rerun", () => {
    const next = Object.fromEntries(["cut", "ok"].map((k) => [k, PASS]));
    const bun = report({ ok: PASS, cut: [] });
    const input = { baseline: baseline(["cut", "ok"]), expectedDiff: { entries: [] }, nextReport: report(next) };
    const sel = selectCandidates(compare({ ...input, bunReports: [bun] }).findings, bun);
    expect(sel.candidates.map((c) => c.title)).toEqual(["cut"]);
  });

  test(`S4: more than ${MAX_CONFIRM_TESTS} failures is too-many and selects nothing to rerun`, () => {
    const many = Object.fromEntries(Array.from({ length: MAX_CONFIRM_TESTS + 1 }, (_, i) => [`t${i}`, HARD]));
    const sel = run(many).selection;
    expect(sel.status).toBe("too-many");
    expect(summaryMarkdown(sel, null)).toMatch(/too many/);
  });
});

describe("e2e-confirm grouping", () => {
  test("G1: one group per project, regex-escaped titles, de-duplicated files", () => {
    const groups = planGroups([
      { spec: "a.spec.ts", project: "chromium", title: "d > x (1)", leafTitle: "x (1)" },
      { spec: "a.spec.ts", project: "chromium", title: "y.z", leafTitle: "y.z" },
      { spec: "b.spec.ts", project: "mobile-chromium", title: "m", leafTitle: "m" },
    ]);
    expect(groups).toEqual([
      { project: "chromium", files: ["a.spec.ts"], grep: "x \\(1\\)|y\\.z" },
      { project: "mobile-chromium", files: ["b.spec.ts"], grep: "m" },
    ]);
  });
});

describe("e2e-confirm merge through the comparator", () => {
  test("M1: a test that passes on the isolated rerun stops gating and is reported flaky-under-load", () => {
    const r = run({ ok: PASS, bad: HARD });
    expect(compare({ ...r.input, bunReports: [r.bunReport] }).exitCode).toBe(1);
    const { merged, exitCode } = gateAfter(r, { bad: PASS });
    expect(exitCode).toBe(0);
    expect(merged.entries.map((e) => e.outcome)).toEqual(["flaky-under-load"]);
  });

  test("M2: a test that fails again on the rerun still gates and is reported confirmed", () => {
    const r = run({ ok: PASS, bad: HARD });
    const { merged, exitCode } = gateAfter(r, { bad: HARD });
    expect(exitCode).toBe(1);
    expect(merged.entries.map((e) => e.outcome)).toEqual(["confirmed"]);
  });

  test("M3: a selected test the rerun never reached keeps its failure (not-rerun)", () => {
    const r = run({ bad: HARD });
    const { merged, exitCode } = gateAfter(r, {});
    expect(exitCode).toBe(1);
    expect(merged.entries[0].outcome).toBe("not-rerun");
  });

  test("M4: an interrupted rerun (global timeout) does not clear the failure", () => {
    const r = run({ bad: HARD });
    const { merged, exitCode } = gateAfter(r, { bad: [{ retry: 0, status: "interrupted" }] });
    expect(exitCode).toBe(1);
    expect(merged.entries[0].outcome).toBe("not-rerun");
  });

  test("M5: one real failure among load flakes still fails: only the flake is cleared", () => {
    const r = run({ real: HARD, flake1: HARD, flake2: HARD });
    const { merged, exitCode } = gateAfter(r, { real: HARD, flake1: PASS, flake2: PASS });
    expect(exitCode).toBe(1);
    expect(Object.fromEntries(merged.entries.map((e) => [e.title, e.outcome]))).toEqual({ real: "confirmed", flake1: "flaky-under-load", flake2: "flaky-under-load" });
  });

  test("M6: the rerun is keyed by project, so a pass in another project does not clear the failure", () => {
    const r = run({ bad: HARD }, "chromium");
    const merged = mergeConfirmation(r.bunReport, [report({ bad: PASS }, "mobile-chromium")], r.selection);
    expect(compare({ ...r.input, bunReports: [merged.report] }).exitCode).toBe(1);
  });

  test("M7: merging does not mutate the input Bun report", () => {
    const r = run({ bad: HARD });
    const before = JSON.stringify(r.bunReport);
    mergeConfirmation(r.bunReport, [report({ bad: PASS })], r.selection);
    expect(JSON.stringify(r.bunReport)).toBe(before);
  });
});
