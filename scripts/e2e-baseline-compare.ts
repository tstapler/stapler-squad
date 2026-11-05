// Gates a Playwright run (1 run in CI's `e2e-bun` job, or 3 local runs when refreshing
// `bun-expected-diff.json`) against the committed per-test classes in
// `tests/e2e/baseline/next-e2e-baseline.json` (recorded from the last Next build, before Next was
// removed) and the expected-diff list (`tests/e2e/baseline/bun-expected-diff.json`), applying the
// gate rule defined in plan Story 0.1. Used by `.github/workflows/build.yml`'s `e2e-bun` job and by
// scripts/e2e-confirm.ts.
//
// There is no live Next control run any more. Without `--next-report` Next is treated as clean for
// every test: a baseline-'pass' test that hard-fails is a gate failure, and so is a failing spec
// that is absent from the baseline. Baseline entries whose test no longer exists in the Bun run
// (deleted or renamed along with the Next-only specs) are reported, not failed.
//
// Usage:
//   ts-node scripts/e2e-baseline-compare.ts \
//     --baseline tests/e2e/baseline/next-e2e-baseline.json \
//     --expected-diff tests/e2e/baseline/bun-expected-diff.json \
//     [--next-report path/to/legacy-next-report.json] \
//     --bun-reports path/to/bun-run1.json [path/to/bun-run2.json ...]
//
// Exit status: 0 if the gate passes, 1 otherwise. Findings are printed to stdout as they are
// produced; a one-line JSON summary (`{"findings": [...], "exitCode": N}`) is printed last so CI
// can attach it to the step summary.
import * as fs from "fs";

export type AttemptStatus = string;

export interface Attempt {
  retry: number;
  status: AttemptStatus;
}

const FAILED_STATUSES = new Set(["failed", "timedOut", "interrupted"]);

// ---------------------------------------------------------------------------
// Playwright JSON report parsing (mirrors spikes/s0/classify-e2e.mjs's collectAttempts/classify,
// ported to TS so this script has no runtime dependency on the spike).
// ---------------------------------------------------------------------------

export interface PwTestResult {
  retry: number;
  status: string;
}

export interface PwTest {
  projectName: string;
  /** Playwright's own expectation: "failed" for a `test.fail()` known gap. */
  expectedStatus?: string;
  /** `test.fail(...)` shows up as `{ type: "fail", description? }`; a description marks a conditional precondition guard. */
  annotations?: { type: string; description?: string }[];
  results: PwTestResult[];
}

export interface PwSpec {
  file: string;
  title: string;
  tests?: PwTest[];
}

export interface PwSuite {
  title?: string;
  file?: string;
  specs?: PwSpec[];
  suites?: PwSuite[];
}

export interface PwReport {
  suites?: PwSuite[];
}

export interface SpecKey {
  spec: string;
  project: string;
  title: string;
}

export function keyString(k: SpecKey): string {
  return `${k.spec} :: ${k.project} :: ${k.title}`;
}

/**
 * A `test.fail()` test is expected to fail: its failing attempts are the expected outcome (a known gap kept visible by the
 * spec itself), so they must not read as a Bun-only failure forever. Trade-off: while the gap stands, that test is blind to
 * any further regression of the behaviour it covers, by design -- `unexpectedPassFindings` is the only signal when it closes.
 * Passing attempts are left as they are here; specs that call `test.fail(condition, reason)` as a precondition guard
 * (backlog.spec.ts) pass when the guard trips, and that was never gated.
 */
function attemptStatus(t: PwTest, status: string): string {
  return t.expectedStatus === "failed" && FAILED_STATUSES.has(status) ? "passed" : status;
}

/**
 * Playwright reports a `test.fail()` test that passes as "unexpected". A bare `test.fail()` is a known gap that has closed:
 * fail, so the marker gets removed and the test guards the behaviour again. `test.fail(cond, reason)` is a precondition guard
 * that trips on a dirty server and passes by design: report only. Playwright's own exit code is not the gate (the shards
 * tolerate raw failures; this comparator is the verdict), so this finding is the only place an unexpected pass surfaces.
 */
export function unexpectedPassFindings(report: PwReport): Finding[] {
  const out: Finding[] = [];
  const walk = (suite: PwSuite, titles: string[]) => {
    for (const spec of suite.specs ?? []) {
      for (const t of spec.tests ?? []) {
        if (t.expectedStatus !== "failed") continue;
        const results = t.results ?? [];
        if (results.length === 0 || !results.every((r) => r.status === "passed")) continue;
        const guard = (t.annotations ?? []).some((a) => a.type === "fail" && a.description);
        const title = [...titles, spec.title].join(" > ");
        out.push(
          guard
            ? { severity: "report", spec: spec.file, project: t.projectName, title, message: "test.fail(condition, reason) precondition guard tripped; the test is blind this run." }
            : { severity: "fail", spec: spec.file, project: t.projectName, title, message: "test.fail() known gap now passes: the gap closed, remove test.fail() so the test guards the behaviour." },
        );
      }
    }
    for (const child of suite.suites ?? []) walk(child, child.title && child.title !== child.file ? [...titles, child.title] : titles);
  };
  for (const top of report.suites ?? []) walk(top, []);
  return out;
}

/** One entry per (spec file, project, title) found anywhere in the report, with that test's attempts. */
export function collectAttempts(report: PwReport): Map<string, { key: SpecKey; attempts: Attempt[] }> {
  const out = new Map<string, { key: SpecKey; attempts: Attempt[] }>();
  const walk = (suite: PwSuite, titles: string[]) => {
    for (const spec of suite.specs ?? []) {
      for (const t of spec.tests ?? []) {
        const key: SpecKey = { spec: spec.file, project: t.projectName, title: [...titles, spec.title].join(" > ") };
        out.set(keyString(key), {
          key,
          attempts: (t.results ?? []).map((r) => ({ retry: r.retry, status: attemptStatus(t, r.status) })),
        });
      }
    }
    for (const child of suite.suites ?? []) {
      const nextTitles = child.title && child.title !== child.file ? [...titles, child.title] : titles;
      walk(child, nextTitles);
    }
  };
  for (const top of report.suites ?? []) walk(top, []);
  return out;
}

/**
 * True when this run's result is NOT a clean attempt-0 pass (a failure, or a retry that then passed). A lone `skipped`
 * attempt is an unintended gap and counts as a failure, as does a test that never reported at all.
 */
export function failedOrRetried(attempts: Attempt[]): boolean {
  if (attempts.length === 0) return true; // did not run this run -- treated as a failure to run
  if (attempts.length !== 1) return true;
  return attempts[0].status !== "passed";
}

// One Bun run cannot tell a persistent flake from a one-off, so retry-then-pass escalates only with >= 2 runs (local 3-run procedures).
const MIN_RUNS_FOR_FLAKY_ESCALATION = 2;

const specOfKey = (ks: string): string => ks.split(" :: ")[0];

/** Every attempt failed (Playwright's own definition of a failing test; a retry that passed is a flake, not a failure). */
export function hardFailed(attempts: Attempt[]): boolean {
  return attempts.length > 0 && attempts.every((a) => FAILED_STATUSES.has(a.status));
}

type RunOutcome = "clean" | "flaky" | "hard" | "cascade";

/**
 * `groupHasCountedRoot`: this run hard-failed another test of the same spec file AND project, and the gate itself counts that
 * failure (not excluded as a baseline-fail, shared-with-Next, or expected-diff failure).
 */
export function classifyRun(attempts: Attempt[], groupHasCountedRoot: boolean): RunOutcome {
  if (!failedOrRetried(attempts)) return "clean";
  if (hardFailed(attempts)) return "hard";
  const skippedOnly = attempts.length > 0 && attempts.every((a) => a.status === "skipped");
  // The Playwright JSON report does not record `mode: "serial"`, so a skip is attributed to a sibling failure only
  // when that sibling's failure is itself a gate finding. A skip whose root is excluded from the gate stays a failure.
  if (skippedOnly && groupHasCountedRoot) return "cascade";
  if (attempts.length > 1 && attempts[attempts.length - 1].status === "passed") return "flaky";
  return "hard"; // unintended skip or a test that never reported
}

export function finalStatus(attempts: Attempt[]): string {
  if (attempts.length === 0) return "missing";
  return attempts[attempts.length - 1].status;
}

// ---------------------------------------------------------------------------
// Baseline / expected-diff file shapes (tests/e2e/baseline/*.json)
// ---------------------------------------------------------------------------

export type BaselineClass = "pass" | "flaky" | "fail" | "skipped";

export interface BaselineSpecEntry {
  spec: string;
  project: string;
  title: string;
  class: BaselineClass;
}

export interface Baseline {
  meta?: Record<string, unknown>;
  specs: BaselineSpecEntry[];
  buildCoupledSpecs?: unknown[];
}

export interface ExpectedDiffEntry {
  spec: string;
  test: string;
  issue?: string;
  reason?: string;
}

export interface ExpectedDiff {
  meta?: Record<string, unknown>;
  entries: ExpectedDiffEntry[];
}

function expectedDiffKey(e: ExpectedDiffEntry): string {
  return `${e.spec} :: ${e.test}`;
}

// ---------------------------------------------------------------------------
// Comparator
// ---------------------------------------------------------------------------

export type FindingSeverity = "fail" | "warn" | "report";

export interface Finding {
  severity: FindingSeverity;
  spec: string;
  /** Set when the finding is about one (spec, project, title) test, so a confirmation rerun can target it. */
  project?: string;
  title: string;
  message: string;
}

export interface CompareInput {
  baseline: Baseline;
  expectedDiff: ExpectedDiff;
  /** A same-run Next control report (legacy); null/undefined means no control, and Next is treated as clean. */
  nextReport: PwReport | null | undefined;
  /** 1 report for CI's e2e-bun job; 3 for the local baseline-build procedures (Tasks 7.11c, 8.1a). */
  bunReports: PwReport[];
}

export interface CompareResult {
  findings: Finding[];
  exitCode: 0 | 1;
}

/**
 * The gate rule is stated in the plan with fractions of three ("at least 1 of 3", "at least 2 of
 * 3"); this generalizes them to N runs (N=1 for CI's single e2e-bun run, N=3 for the local
 * baseline/expected-diff procedures) rather than hard-coding 3, so the same script serves both.
 * For N=1 this collapses to "any failure" for the pass-class threshold and "the one run failed"
 * for the flaky-class 2/3 threshold -- both reasonable degenerate cases of the stated rule.
 */
const FLAKY_CLASS_FAIL_THRESHOLD = 2 / 3; // "at least 2 of 3"

function validateExpectedDiffEntries(entries: ExpectedDiffEntry[]): Finding[] {
  return entries
    .filter((e) => !e.issue || !/^https?:\/\//.test(e.issue))
    .map((e) => ({
      severity: "fail" as const,
      spec: e.spec,
      title: e.test,
      message: `bun-expected-diff.json entry has no linked issue URL: ${JSON.stringify(e)}`,
    }));
}

interface GateContext {
  bunAttemptsFor: (key: string) => Attempt[][];
  /** Per Bun run: "spec :: project" groups containing a hard-failed test that the gate counts as a failure. */
  bunCountedRootGroups: Set<string>[];
  /** False when no Next report was supplied: Next is then treated as clean for every test. */
  hasControl: boolean;
  nextAttempts: Map<string, { key: SpecKey; attempts: Attempt[] }>;
  expectedDiffKeys: Map<string, ExpectedDiffEntry>;
  expectedDiffSeen: Set<string>;
}

/** The control (Next) ran this test cleanly; with no control, every test counts as clean. */
function controlClean(ctx: GateContext, ks: string): boolean {
  if (!ctx.hasControl) return true;
  const entry = ctx.nextAttempts.get(ks);
  return entry !== undefined && !failedOrRetried(entry.attempts);
}

/** The control (Next) ran this test and it failed or needed a retry; never true without a control. */
function controlFailed(ctx: GateContext, ks: string): boolean {
  if (!ctx.hasControl) return false;
  const entry = ctx.nextAttempts.get(ks);
  return entry !== undefined && failedOrRetried(entry.attempts);
}

/**
 * "spec :: project" groups where run `runIdx` hard-failed a baseline 'pass'/'flaky' test that the gate counts: Next clean and no
 * expected-diff entry. Baseline-'fail' tests, shared failures and listed diffs are excluded, so their skipped siblings stay failures.
 */
function countedRootGroups(specs: BaselineSpecEntry[], ctx: GateContext, runIdx: number): Set<string> {
  const groups = new Set<string>();
  for (const b of specs) {
    if (b.class !== "pass" && b.class !== "flaky") continue;
    const ks = keyString({ spec: b.spec, project: b.project, title: b.title });
    if (!hardFailed(ctx.bunAttemptsFor(ks)[runIdx] ?? [])) continue;
    if (!controlClean(ctx, ks)) continue;
    if (ctx.expectedDiffKeys.has(`${b.spec} :: ${b.title}`)) continue;
    groups.add(`${b.spec} :: ${b.project}`);
  }
  return groups;
}

function evaluateBaselinePassSpec(b: BaselineSpecEntry, ctx: GateContext): Finding | null {
  const ks = keyString({ spec: b.spec, project: b.project, title: b.title });
  const runs = ctx.bunAttemptsFor(ks);
  const n = runs.length;
  const outcomes = runs.map((a, i) => classifyRun(a, ctx.bunCountedRootGroups[i]?.has(`${b.spec} :: ${b.project}`) ?? false));
  const failCount = outcomes.filter((o) => o === "hard").length;
  const flakyCount = outcomes.filter((o) => o === "flaky").length;
  if (failCount === 0) {
    if (flakyCount === 0) return null; // clean on Bun, or only a cascade skip of another test's counted failure
    const nextClean = controlClean(ctx, ks);
    if (nextClean && n >= MIN_RUNS_FOR_FLAKY_ESCALATION && flakyCount === n) {
      return { severity: "fail", spec: b.spec, project: b.project, title: b.title, message: `Persistent Bun-only flake: baseline class 'pass' needed a retry in all ${n} Bun runs, e2e-next clean.` };
    }
    return { severity: "warn", spec: b.spec, project: b.project, title: b.title, message: `baseline class 'pass' needed a retry on Bun in ${flakyCount}/${n} runs (passed on retry): warning, not a gate failure.` };
  }

  const diffEntry = ctx.expectedDiffKeys.get(`${b.spec} :: ${b.title}`);
  if (diffEntry) ctx.expectedDiffSeen.add(`${b.spec} :: ${b.title}`);
  const sharedFlake = controlFailed(ctx, ks);

  if (sharedFlake) {
    return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `Shared flake: fails on both e2e-next and e2e-bun this run (${failCount}/${n} Bun runs).` };
  }
  if (diffEntry) {
    return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `Listed in bun-expected-diff.json (${diffEntry.issue}) and still fails (${failCount}/${n} Bun runs).` };
  }
  return { severity: "fail", spec: b.spec, project: b.project, title: b.title, message: `New Bun-only failure: baseline class 'pass', hard-failed (every attempt) in ${failCount}/${n} Bun runs, e2e-next clean, not listed in bun-expected-diff.json.` };
}

function evaluateBaselineFlakySpec(b: BaselineSpecEntry, ctx: GateContext): Finding | null {
  const ks = keyString({ spec: b.spec, project: b.project, title: b.title });
  const runs = ctx.bunAttemptsFor(ks);
  const n = runs.length;
  const finalFailCount = runs.filter((a) => FAILED_STATUSES.has(finalStatus(a))).length;
  const ratio = n === 0 ? 0 : finalFailCount / n;

  const diffEntry = ctx.expectedDiffKeys.get(`${b.spec} :: ${b.title}`);
  if (diffEntry) ctx.expectedDiffSeen.add(`${b.spec} :: ${b.title}`);

  if (ratio >= FLAKY_CLASS_FAIL_THRESHOLD) {
    // Same shared-failure rule as baseline 'pass' specs: a test that also finally fails on the same-run Next report is not Bun-specific.
    const nextEntry = ctx.hasControl ? ctx.nextAttempts.get(ks) : undefined;
    if (nextEntry && FAILED_STATUSES.has(finalStatus(nextEntry.attempts))) {
      return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `Shared failure: baseline 'flaky' also fails on e2e-next this run (${finalFailCount}/${n} Bun runs).` };
    }
    if (diffEntry) {
      return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `Listed in bun-expected-diff.json (${diffEntry.issue}); baseline 'flaky' now fails ${finalFailCount}/${n} on Bun.` };
    }
    return { severity: "fail", spec: b.spec, project: b.project, title: b.title, message: `flaky -> fail: baseline class 'flaky' failed ${finalFailCount}/${n} Bun runs (>= 2/3), not listed.` };
  }
  if (finalFailCount >= 1) {
    return { severity: "warn", spec: b.spec, project: b.project, title: b.title, message: `baseline 'flaky' failed ${finalFailCount}/${n} Bun runs (< 2/3): warning, dispositioned in triage.` };
  }
  return null;
}

function evaluateBaselineFailSpec(b: BaselineSpecEntry, ctx: GateContext): Finding | null {
  const ks = keyString({ spec: b.spec, project: b.project, title: b.title });
  const runs = ctx.bunAttemptsFor(ks);
  const n = runs.length;
  const failCount = runs.filter((a) => failedOrRetried(a)).length;
  if (n > 0 && failCount === 0) {
    return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `baseline class 'fail' now passes cleanly on Bun (${n}/${n} runs) -- reported, not a gate requirement.` };
  }
  return null; // excluded from the gate otherwise: accepted masking (Story 0.1)
}

/** With no control, a baseline test the Bun run never reported was deleted or renamed: say so, do not fail. */
function removedFromRun(b: BaselineSpecEntry, ctx: GateContext): Finding | null {
  if (ctx.hasControl) return null;
  const ks = keyString({ spec: b.spec, project: b.project, title: b.title });
  if (!ctx.bunAttemptsFor(ks).every((a) => a.length === 0)) return null;
  return { severity: "report", spec: b.spec, project: b.project, title: b.title, message: `baseline class '${b.class}' test is not in this run (deleted or renamed): reported, not a gate failure.` };
}

function evaluateBaselineSpecs(specs: BaselineSpecEntry[], ctx: GateContext): Finding[] {
  const evaluators: Partial<Record<BaselineClass, (b: BaselineSpecEntry, ctx: GateContext) => Finding | null>> = {
    pass: evaluateBaselinePassSpec,
    flaky: evaluateBaselineFlakySpec,
    fail: evaluateBaselineFailSpec,
  };
  const findings: Finding[] = [];
  for (const b of specs) {
    const removed = b.class === "skipped" ? null : removedFromRun(b, ctx);
    const finding = removed ?? evaluators[b.class]?.(b, ctx) ?? null; // 'skipped' has no evaluator: out of scope
    if (finding) findings.push(finding);
  }
  return findings;
}

// Consistency note: only baseline-'pass' specs use the hard-failure/cascade rule above. Baseline-'flaky' specs already gate on the
// final status; baseline-'fail' specs are excluded; specs absent from the baseline and expected-diff reproduction stay strict
// (failedOrRetried) because they have no recorded flake history to justify tolerating a retry.
function evaluateSpecsMissingFromBaseline(bunKeys: Set<string>, baselineKeys: Set<string>, ctx: GateContext): Finding[] {
  const findings: Finding[] = [];
  for (const ks of bunKeys) {
    if (baselineKeys.has(ks)) continue; // handled by evaluateBaselineSpecs
    const runs = ctx.bunAttemptsFor(ks);
    const failCount = runs.filter((a) => failedOrRetried(a)).length;
    if (failCount === 0) continue;
    const nextFailed = controlFailed(ctx, ks);
    findings.push(nextFailed ? missingSpecBothFailFinding(ks) : missingSpecBunOnlyFailFinding(ks, failCount, runs.length));
  }
  return findings;
}

function keyParts(ks: string): SpecKey {
  const [spec, project, title] = ks.split(" :: ");
  return { spec, project, title };
}

function missingSpecBothFailFinding(ks: string): Finding {
  const { spec, title } = keyParts(ks);
  return { severity: "report", spec, title, message: "Spec absent from the baseline, fails on both builds this run -- treated as baseline-class 'fail'." };
}

function missingSpecBunOnlyFailFinding(ks: string, failCount: number, total: number): Finding {
  const { spec, project, title } = keyParts(ks);
  return { severity: "fail", spec, project, title, message: `Spec absent from the baseline, fails on Bun (${failCount}/${total} runs) but passes on e2e-next -- gate failure.` };
}

/** The expected-diff list may only shrink: every entry must still reproduce, or be removed. */
function evaluateExpectedDiffShrink(entries: ExpectedDiffEntry[], bunKeys: Set<string>, ctx: GateContext): Finding[] {
  const findings: Finding[] = [];
  for (const e of entries) {
    const dk = expectedDiffKey(e);
    if (ctx.expectedDiffSeen.has(`${e.spec} :: ${e.test}`)) continue; // already matched a baseline spec above
    const matchKey = [...bunKeys].find((ks) => ks.startsWith(`${e.spec} :: `) && ks.endsWith(` :: ${e.test}`));
    if (!matchKey) {
      findings.push({ severity: "fail", spec: e.spec, title: e.test, message: `bun-expected-diff.json entry (${dk}) matches no spec in this Bun run -- remove it or it is stale.` });
      continue;
    }
    const runs = ctx.bunAttemptsFor(matchKey);
    const failCount = runs.filter((a) => failedOrRetried(a)).length;
    if (failCount === 0) {
      findings.push({ severity: "fail", spec: e.spec, title: e.test, message: `bun-expected-diff.json entry (${dk}) now passes ${runs.length}/${runs.length} on Bun and was not removed -- the list may only shrink.` });
    }
  }
  return findings;
}

export function compare(input: CompareInput): CompareResult {
  const findings: Finding[] = [...validateExpectedDiffEntries(input.expectedDiff.entries ?? [])];
  if (!input.expectedDiff.entries) throw new Error("expected-diff file has no entries array");

  const hasControl = Boolean(input.nextReport);

  const bunPerRun = input.bunReports.map(collectAttempts);
  const bunKeys = new Set<string>();
  for (const m of bunPerRun) for (const k of m.keys()) bunKeys.add(k);

  const ctx: GateContext = {
    bunAttemptsFor: (key: string) => bunPerRun.map((m) => m.get(key)?.attempts ?? []),
    bunCountedRootGroups: [],
    hasControl,
    nextAttempts: input.nextReport ? collectAttempts(input.nextReport) : new Map(),
    expectedDiffKeys: new Map(input.expectedDiff.entries.map((e) => [expectedDiffKey(e), e])),
    expectedDiffSeen: new Set<string>(),
  };

  const baselineKeys = new Set(input.baseline.specs.map((b) => keyString({ spec: b.spec, project: b.project, title: b.title })));

  ctx.bunCountedRootGroups = bunPerRun.map((_, i) => countedRootGroups(input.baseline.specs, ctx, i));
  findings.push(...evaluateBaselineSpecs(input.baseline.specs, ctx));
  findings.push(...evaluateSpecsMissingFromBaseline(bunKeys, baselineKeys, ctx));
  for (const r of input.bunReports) findings.push(...unexpectedPassFindings(r));
  findings.push(...evaluateExpectedDiffShrink(input.expectedDiff.entries, bunKeys, ctx));

  const exitCode: 0 | 1 = findings.some((f) => f.severity === "fail") ? 1 : 0;
  return { findings, exitCode };
}

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

function parseArgs(argv: string[]): { baseline: string; expectedDiff: string; nextReport: string | undefined; bunReports: string[] } {
  const args = new Map<string, string[]>();
  let current = "";
  for (const a of argv) {
    if (a.startsWith("--")) {
      current = a.slice(2);
      args.set(current, []);
    } else if (current) {
      args.get(current)!.push(a);
    }
  }
  const one = (name: string): string => {
    const v = args.get(name);
    if (!v || v.length === 0) throw new Error(`missing --${name}`);
    return v[0];
  };
  return {
    baseline: one("baseline"),
    expectedDiff: one("expected-diff"),
    nextReport: args.get("next-report")?.[0],
    bunReports: args.get("bun-reports") ?? [],
  };
}

function readJson<T>(path: string): T {
  return JSON.parse(fs.readFileSync(path, "utf8")) as T;
}

function main(): void {
  const opts = parseArgs(process.argv.slice(2));
  const baseline = readJson<Baseline>(opts.baseline);
  const expectedDiff = readJson<ExpectedDiff>(opts.expectedDiff);
  let nextReport: PwReport | null = null;
  if (opts.nextReport) {
    try {
      nextReport = readJson<PwReport>(opts.nextReport);
    } catch {
      nextReport = null;
    }
  }
  const bunReports = opts.bunReports.map((p) => readJson<PwReport>(p));
  if (bunReports.length === 0) throw new Error("at least one --bun-reports file is required");

  const result = compare({ baseline, expectedDiff, nextReport, bunReports });
  for (const f of result.findings) {
    const tag = f.severity.toUpperCase();
    console.log(`[${tag}] ${f.spec} :: ${f.title} -- ${f.message}`);
  }
  console.log(JSON.stringify({ findings: result.findings, exitCode: result.exitCode }));
  process.exitCode = result.exitCode;
}

if (require.main === module) {
  main();
}
