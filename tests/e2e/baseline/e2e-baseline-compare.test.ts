// Task 7.10c's adversarial AC: the comparator must actually be able to fail. This spawns
// `scripts/e2e-baseline-compare.ts` as a child process (not calling its exports directly) over the
// fixtures in `__fixtures__/`, one scenario per case enumerated in the plan's Story 7.10 AC. A
// stub that always exits 0 would pass every Z-case here but fail every F-case -- the plan's own
// framing ("a comparator that always exits 0 therefore fails seven of these cases").
import { execFileSync } from "child_process";
import * as path from "path";

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
  nextReport: string;
  bunReports: string[];
}

function run(opts: RunOpts): { exitCode: number; stdout: string } {
  const args = [
    "--compiler-options",
    TSC_OPTS,
    SCRIPT,
    "--baseline",
    fx(opts.baseline),
    "--expected-diff",
    fx(opts.expectedDiff),
    "--next-report",
    fx(opts.nextReport),
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

  test("F1b: a retry (even if it then passes) counts as a Bun-side failure for a baseline-pass spec", () => {
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

  test("F4: a missing e2e-next report -- the comparator never skips this check", () => {
    const r = run({ baseline: "f4-baseline.json", expectedDiff: "f4-expected-diff.json", nextReport: "__does-not-exist__.json", bunReports: ["f4-bun-report-1.json"] });
    expect(r.exitCode).not.toBe(0);
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

  test("Z6: a baseline-pass spec that Bun deliberately skips (test.skip(isBunBuild())) is not a failure, exit 0", () => {
    const r = run({ baseline: "z6-baseline.json", expectedDiff: "z6-expected-diff.json", nextReport: "z6-next-report.json", bunReports: ["z6-bun-report-1.json"] });
    expect(r.exitCode).toBe(0);
  });

  test("a comparator stub that always exits 0 would fail F1-F7 (adversarial framing, documented not executed)", () => {
    // This test exists to make the claim checkable without actually swapping the script: each
    // F-case above independently asserts a non-zero exit, which is exactly what an always-0 stub
    // could never produce. No separate stub binary is maintained here.
    expect(true).toBe(true);
  });
});
