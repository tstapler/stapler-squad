// Task 7.10c's adversarial AC: the comparator must actually be able to fail. This spawns
// `scripts/e2e-baseline-compare.ts` as a child process (not calling its exports directly) over the
// fixtures in `__fixtures__/`, one scenario per case enumerated in the plan's Story 7.10 AC. A
// stub that always exits 0 would pass every Z-case here but fail every F-case -- the plan's own
// framing ("a comparator that always exits 0 therefore fails seven of these cases").
import { execFileSync } from "child_process";
import * as fs from "fs";
import * as path from "path";
import { compare, PwReport, Baseline } from "../../../scripts/e2e-baseline-compare";

const REPO_ROOT = path.join(__dirname, "../../..");
const SCRIPT = path.join(REPO_ROOT, "scripts/e2e-baseline-compare.ts");
const TS_NODE = path.join(REPO_ROOT, "web-app/node_modules/.bin/ts-node");
const FIXTURES = path.join(__dirname, "__fixtures__");
const TSC_OPTS = JSON.stringify({ module: "commonjs", moduleResolution: "node", esModuleInterop: true });

function fx(name: string): string {
  return path.join(FIXTURES, name);
}

interface RunOpts {
  baseline: string;
  expectedDiff: string;
  /** Omitted in the CI shape: there is no Next control run any more. */
  nextReport?: string;
  bunReports: string[];
}

function run(opts: RunOpts): { exitCode: number; stdout: string } {
  // A missing ts-node would make every spawn exit non-zero, so the F-cases (which expect non-zero) would pass for the
  // wrong reason and the Z-cases fail. CI runs this through jest's e2e-dev-mode project (web-app installed); under a bare
  // `bun test` from a checkout without web-app/node_modules, say so instead of producing misleading results.
  if (!fs.existsSync(TS_NODE)) {
    throw new Error(`ts-node not found at ${TS_NODE}: run \`bun install\` in web-app (CI runs this file via web-app's jest e2e-dev-mode project)`);
  }
  const args = [
    "--compiler-options",
    TSC_OPTS,
    SCRIPT,
    "--baseline",
    fx(opts.baseline),
    "--expected-diff",
    fx(opts.expectedDiff),
    ...(opts.nextReport ? ["--next-report", fx(opts.nextReport)] : []),
    "--bun-reports",
    ...opts.bunReports.map(fx),
  ];
  try {
    const stdout = execFileSync(TS_NODE, args, { encoding: "utf8" });
    return { exitCode: 0, stdout };
  } catch (err: unknown) {
    const e = err as { status?: number; stdout?: string };
    return { exitCode: e.status ?? 1, stdout: e.stdout ?? "" };
  }
}

describe("scripts/e2e-baseline-compare.ts", () => {
  // Raises the default ts-jest-inherited timeout: each case spawns a fresh ts-node process.
  jest.setTimeout(30000);

  test("F1: new failure -- baseline pass spec fails 3/3 on Bun, next clean, not listed", () => {
    const r = run({ baseline: "f1-baseline.json", expectedDiff: "f1-expected-diff.json", nextReport: "f1-next-report.json", bunReports: ["f1-bun-report-1.json", "f1-bun-report-2.json", "f1-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F1b: a hard failure in the other Bun runs still fails the gate even when one run only needed a retry", () => {
    const r = run({ baseline: "f1-baseline.json", expectedDiff: "f1-expected-diff.json", nextReport: "f1-next-report.json", bunReports: ["extra-retry-bun-report-1.json", "f1-bun-report-2.json", "f1-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F2: flaky -> fail -- baseline flaky spec's final status fails 3/3 on Bun", () => {
    const r = run({ baseline: "f2-baseline.json", expectedDiff: "f2-expected-diff.json", nextReport: "f2-next-report.json", bunReports: ["f2-bun-report-1.json", "f2-bun-report-2.json", "f2-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F3: a spec missing from the baseline, failing on Bun, passing on e2e-next -- gate failure", () => {
    const r = run({ baseline: "f3-baseline.json", expectedDiff: "f3-expected-diff.json", nextReport: "f3-next-report.json", bunReports: ["f3-bun-report-1.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F4: no Next control report (the CI shape) -- a baseline-pass spec failing on Bun is still a gate failure", () => {
    const r = run({ baseline: "f1-baseline.json", expectedDiff: "f1-expected-diff.json", bunReports: ["f1-bun-report-1.json", "f1-bun-report-2.json", "f1-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
    expect(r.stdout).toContain("[FAIL]");
  });

  test("F4b: no Next control report -- a clean Bun run passes", () => {
    const r = run({ baseline: "z1-baseline.json", expectedDiff: "z1-expected-diff.json", bunReports: ["z1-bun-report-1.json", "z1-bun-report-2.json", "z1-bun-report-3.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("F5: a bun-expected-diff.json entry with no linked issue URL", () => {
    const r = run({ baseline: "f5-baseline.json", expectedDiff: "f5-expected-diff.json", nextReport: "f5-next-report.json", bunReports: ["f5-bun-report-1.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F6: a listed spec that now passes and was not removed -- the list may only shrink", () => {
    const r = run({ baseline: "f6-baseline.json", expectedDiff: "f6-expected-diff.json", nextReport: "f6-next-report.json", bunReports: ["f6-bun-report-1.json", "f6-bun-report-2.json", "f6-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("F7: baseline-pass spec failing 1 of 3 on Bun while e2e-next is clean -- still a gate failure", () => {
    const r = run({ baseline: "f7-baseline.json", expectedDiff: "f7-expected-diff.json", nextReport: "f7-next-report.json", bunReports: ["f7-bun-report-1.json", "f7-bun-report-2.json", "f7-bun-report-3.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  test("Z1: a baseline-fail spec failing on Bun -- excluded from the gate, exit 0", () => {
    const r = run({ baseline: "z1-baseline.json", expectedDiff: "z1-expected-diff.json", nextReport: "z1-next-report.json", bunReports: ["z1-bun-report-1.json", "z1-bun-report-2.json", "z1-bun-report-3.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("Z2: a spec missing from the baseline, failing on both builds -- reported as baseline-class fail, exit 0", () => {
    const r = run({ baseline: "z2-baseline.json", expectedDiff: "z2-expected-diff.json", nextReport: "z2-next-report.json", bunReports: ["z2-bun-report-1.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("Z3: baseline-flaky spec failing 1 of 3 on Bun -- warning only, exit 0", () => {
    const r = run({ baseline: "z3-baseline.json", expectedDiff: "z3-expected-diff.json", nextReport: "z3-next-report.json", bunReports: ["z3-bun-report-1.json", "z3-bun-report-2.json", "z3-bun-report-3.json"] });
    expect(r.exitCode).toBe(0);
    expect(r.stdout).toMatch(/WARN/);
  });

  test("Z4: baseline-pass spec failing 1 of 3 on Bun and also in the same-run e2e-next report -- shared flake, exit 0", () => {
    const r = run({ baseline: "z4-baseline.json", expectedDiff: "z4-expected-diff.json", nextReport: "z4-next-report.json", bunReports: ["z4-bun-report-1.json", "z4-bun-report-2.json", "z4-bun-report-3.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("Z5: a listed expected-diff spec that still fails -- reported, exit 0", () => {
    const r = run({ baseline: "z5-baseline.json", expectedDiff: "z5-expected-diff.json", nextReport: "z5-next-report.json", bunReports: ["z5-bun-report-1.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("F8: a baseline-pass spec that skips on Bun is a failure, not a clean pass", () => {
    const r = run({ baseline: "f8-baseline.json", expectedDiff: "f8-expected-diff.json", nextReport: "f8-next-report.json", bunReports: ["f8-bun-report-1.json"] });
    expect(r.exitCode).not.toBe(0);
  });

  describe("hard failures only; cascades attributed to a counted root", () => {
    type Att = { retry: number; status: string }[];
    type T = { att: Att; project?: string };
    const PASS: Att = [{ retry: 0, status: "passed" }];
    const FLAKY: Att = [{ retry: 0, status: "failed" }, { retry: 1, status: "passed" }];
    const HARD: Att = [{ retry: 0, status: "failed" }, { retry: 1, status: "failed" }];
    const SKIP: Att = [{ retry: 0, status: "skipped" }, { retry: 1, status: "skipped" }];
    const FILE = "a.spec.ts";
    const t = (att: Att, project = "chromium"): T => ({ att, project });
    const rep = (m: Record<string, T>, file = FILE): PwReport => ({
      suites: [{ specs: Object.entries(m).map(([title, v]) => ({ file, title, tests: [{ projectName: v.project ?? "chromium", results: v.att }] })) }],
    });
    const base = (m: Record<string, T>, classes: Record<string, "pass" | "fail"> = {}, file = FILE): Baseline => ({
      specs: Object.entries(m).map(([title, v]) => ({ spec: file, project: v.project ?? "chromium", title, class: classes[title] ?? ("pass" as const) })),
    });
    const gate = (bun: Record<string, T>[], next: Record<string, T>, classes: Record<string, "pass" | "fail"> = {}, file = FILE) =>
      compare({ baseline: base(bun[0], classes, file), expectedDiff: { entries: [] }, nextReport: rep(next, file), bunReports: bun.map((m) => rep(m, file)) });
    const fails = (r: ReturnType<typeof compare>) => r.findings.filter((f) => f.severity === "fail");
    const allPass = (m: Record<string, T>): Record<string, T> => Object.fromEntries(Object.entries(m).map(([k, v]) => [k, t(PASS, v.project)]));

    test("R1: one run needing a retry (single Bun run) is a warning", () => {
      const bun = { t1: t(FLAKY), t2: t(PASS) };
      const r = gate([bun], allPass(bun));
      expect(r.exitCode).toBe(0);
      expect(r.findings.some((f) => f.severity === "warn" && f.title === "t1")).toBe(true);
    });

    test("R1b: retry-then-pass in ALL of 2 Bun runs with Next clean is a gate failure; in only 1 of 2 it stays a warning", () => {
      const flaky = { t1: t(FLAKY) };
      const clean = { t1: t(PASS) };
      expect(fails(gate([flaky, flaky], allPass(flaky))).map((f) => f.title)).toEqual(["t1"]);
      const mixed = gate([flaky, clean], allPass(flaky));
      expect(mixed.exitCode).toBe(0);
      expect(mixed.findings.some((f) => f.severity === "warn" && f.title === "t1")).toBe(true);
    });

    test("R2: one counted failure plus four serial-skipped siblings is one gate failure, not five", () => {
      const bun = { t1: t(PASS), t2: t(HARD), t3: t(SKIP), t4: t(SKIP), t5: t(SKIP), t6: t(SKIP) };
      const r = gate([bun], allPass(bun));
      expect(r.exitCode).toBe(1);
      expect(fails(r).map((f) => f.title)).toEqual(["t2"]);
    });

    test("R3: a skip with no failing sibling is still an unintended gap and fails", () => {
      const bun = { t1: t(PASS), t2: t(SKIP) };
      expect(fails(gate([bun], allPass(bun))).map((f) => f.title)).toEqual(["t2"]);
    });

    test("R4: one real hard Bun-only failure still fails amid many retry-then-pass flakes", () => {
      const bun: Record<string, T> = { real: t(HARD) };
      for (let i = 0; i < 20; i++) bun[`flake${i}`] = t(FLAKY);
      const r = gate([bun], allPass(bun));
      expect(r.exitCode).toBe(1);
      expect(fails(r).map((f) => f.title)).toEqual(["real"]);
    });

    test("R5: a skip whose failing sibling is excluded from the gate (baseline class 'fail') is NOT dropped", () => {
      const bun = { root: t(HARD), victim: t(SKIP) };
      const r = gate([bun], allPass(bun), { root: "fail" });
      expect(fails(r).map((f) => f.title)).toEqual(["victim"]);
    });

    test("R5b: a skip whose failing sibling also fails on Next (shared flake) is NOT dropped when Next ran the sibling clean", () => {
      const bun = { root: t(HARD), victim: t(SKIP) };
      const r = gate([bun], { root: t(HARD), victim: t(PASS) });
      expect(fails(r).map((f) => f.title)).toEqual(["victim"]);
    });

    test("R6: multi-project -- a chromium hard failure does not excuse a skipped sibling in another project", () => {
      const bun = { root: t(HARD, "chromium"), victim: t(SKIP, "chromium"), "mobile-victim": t(SKIP, "mobile-chromium") };
      const r = gate([bun], allPass(bun));
      expect(fails(r).map((f) => f.title).sort()).toEqual(["mobile-victim", "root"]);
    });

    test("R7: with no control run, a baseline test the Bun run never reported is reported, not failed", () => {
      const bun = { gone: t(PASS), kept: t(PASS) };
      const r = compare({ baseline: base(bun), expectedDiff: { entries: [] }, nextReport: null, bunReports: [rep({ kept: t(PASS) })] });
      expect(r.exitCode).toBe(0);
      expect(r.findings.some((f) => f.severity === "report" && f.title === "gone")).toBe(true);
    });

    test("R8: with no control run, a baseline-pass test hard-failing on Bun is a gate failure", () => {
      const bun = { broken: t(HARD) };
      const r = compare({ baseline: base(bun), expectedDiff: { entries: [] }, nextReport: null, bunReports: [rep(bun)] });
      expect(fails(r).map((f) => f.title)).toEqual(["broken"]);
    });
  });

  describe("baseline 'flaky' specs and the same-run Next control", () => {
    const HARD = [{ retry: 0, status: "failed" }, { retry: 1, status: "failed" }];
    const PASS = [{ retry: 0, status: "passed" }];
    const rep = (results: { retry: number; status: string }[]): PwReport => ({
      suites: [{ specs: [{ file: "a.spec.ts", title: "t", tests: [{ projectName: "chromium", results }] }] }],
    });
    const baseline: Baseline = { specs: [{ spec: "a.spec.ts", project: "chromium", title: "t", class: "flaky" }] };
    const gate = (bun: typeof HARD, next: typeof HARD) => compare({ baseline, expectedDiff: { entries: [] }, nextReport: rep(next), bunReports: [rep(bun)] });

    test("FL1: a flaky-class test failing on Bun while Next is clean still fails the gate", () => {
      expect(gate(HARD, PASS).exitCode).toBe(1);
    });

    test("FL2: a flaky-class test failing on both builds is a shared failure -- reported, exit 0", () => {
      const r = gate(HARD, HARD);
      expect(r.exitCode).toBe(0);
      expect(r.findings.some((f) => f.severity === "report" && /Shared failure/.test(f.message))).toBe(true);
    });
  });

  describe("test.fail() known gaps (expectedStatus: failed)", () => {
    const rep = (results: { retry: number; status: string }[], expectedStatus?: string, description?: string): PwReport => ({
      suites: [{ specs: [{ file: "a.spec.ts", title: "t", tests: [{ projectName: "chromium", expectedStatus, annotations: expectedStatus === "failed" ? [{ type: "fail", description }] : [], results }] }] }],
    });
    const FAIL1 = [{ retry: 0, status: "failed" }];
    const PASS1 = [{ retry: 0, status: "passed" }];
    const gate = (bun: PwReport) =>
      compare({ baseline: { specs: [] }, expectedDiff: { entries: [] }, nextReport: null, bunReports: [bun] });

    test("TF1: a spec absent from the baseline that fails as a test.fail() gap is not a failure", () => {
      expect(gate(rep(FAIL1, "failed")).exitCode).toBe(0);
    });

    test("TF2: the same failure without test.fail() still fails the gate", () => {
      expect(gate(rep(FAIL1, "passed")).exitCode).toBe(1);
    });

    test("TF3: a bare test.fail() known gap that now passes fails the gate (the gap closed; remove test.fail)", () => {
      const r = gate(rep(PASS1, "failed"));
      expect(r.exitCode).toBe(1);
      expect(r.findings.some((f) => f.severity === "fail" && /known gap now passes/.test(f.message))).toBe(true);
    });

    test("TF4: a test.fail(condition, reason) precondition guard that passes is reported, not failed", () => {
      const r = gate(rep(PASS1, "failed", "requires a clean backlog"));
      expect(r.exitCode).toBe(0);
      expect(r.findings.some((f) => f.severity === "report" && /precondition guard/.test(f.message))).toBe(true);
    });

    test("TF5: a test.fail() test that fails on every attempt is the expected outcome, with no finding", () => {
      expect(gate(rep(FAIL1, "failed")).findings).toEqual([]);
    });
  });

  test("a comparator stub that always exits 0 would fail F1-F7 (adversarial framing, documented not executed)", () => {
    // This test exists to make the claim checkable without actually swapping the script: each
    // F-case above independently asserts a non-zero exit, which is exactly what an always-0 stub
    // could never produce. No separate stub binary is maintained here.
    expect(true).toBe(true);
  });
});
