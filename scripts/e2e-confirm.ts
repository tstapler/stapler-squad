// Confirmation rerun for the Bun e2e gate. Under E2E_WORKERS=3 x 5 shards a different handful of tests
// hard-fails on Bun each run (load flakiness), which a single-run gate cannot tell from a regression. This
// script (1) selects the tests the comparator would fail, (2) reruns only those with one worker, and
// (3) folds the rerun results back into the Bun report, so only a test that fails again in isolation gates.
//
//   select --baseline B --expected-diff E [--next-report N] --bun-report R --out sel.json [--max 40]
//   run    --selection sel.json --out-dir DIR [--budget-ms 1080000]       (spawns Playwright in tests/e2e)
//   merge  --bun-report R --selection sel.json --rerun-dir DIR --out confirmed.json   (summary on stdout)
import * as fs from "fs";
import * as path from "path";
import { spawnSync } from "child_process";
import {
  compare,
  collectAttempts,
  hardFailed,
  keyString,
  type Baseline,
  type ExpectedDiff,
  type Finding,
  type PwReport,
  type PwSuite,
  type PwTest,
} from "./e2e-baseline-compare";

/** More failures than this means something is broadly broken, not flaky: do not mask it with a rerun. */
export const MAX_CONFIRM_TESTS = 40;

export interface Candidate {
  spec: string;
  project: string;
  /** Describe titles joined with " > " plus the test title: the comparator's key title. */
  title: string;
  /** The test's own title, which is what Playwright's --grep matches. */
  leafTitle: string;
}

export interface Selection {
  status: "none" | "rerun" | "too-many";
  max: number;
  candidates: Candidate[];
}

export interface RerunGroup {
  project: string;
  files: string[];
  grep: string;
}

type Visit = (file: string, test: PwTest, fullTitle: string, leafTitle: string) => void;

function walkTests(report: PwReport, visit: Visit): void {
  const walk = (suite: PwSuite, titles: string[]) => {
    for (const spec of suite.specs ?? []) {
      for (const t of spec.tests ?? []) visit(spec.file, t, [...titles, spec.title].join(" > "), spec.title);
    }
    for (const child of suite.suites ?? []) {
      walk(child, child.title && child.title !== child.file ? [...titles, child.title] : titles);
    }
  };
  for (const top of report.suites ?? []) walk(top, []);
}

/** Tests the comparator would fail that a rerun can address: a finding with a project that hard-failed on Bun. */
export function selectCandidates(findings: Finding[], bunReport: PwReport, max = MAX_CONFIRM_TESTS): Selection {
  const wanted = new Set(findings.filter((f) => f.severity === "fail" && f.project).map((f) => keyString({ spec: f.spec, project: f.project!, title: f.title })));
  const attempts = collectAttempts(bunReport);
  const candidates: Candidate[] = [];
  walkTests(bunReport, (file, t, fullTitle, leafTitle) => {
    const key = keyString({ spec: file, project: t.projectName, title: fullTitle });
    if (!wanted.has(key)) return;
    // A skipped test has nothing a rerun could change. One with no result at all was cut off by the shard's
    // --global-timeout (the JSON reporter lists unreached tests without results): a rerun is exactly what it needs.
    const got = attempts.get(key)?.attempts ?? [];
    if (got.length > 0 && !hardFailed(got)) return;
    candidates.push({ spec: file, project: t.projectName, title: fullTitle, leafTitle });
  });
  const status = candidates.length === 0 ? "none" : candidates.length > max ? "too-many" : "rerun";
  return { status, max, candidates };
}

const escapeRegExp = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** One Playwright invocation per project: the files narrow by path, the grep by test title. */
export function planGroups(candidates: Candidate[]): RerunGroup[] {
  const byProject = new Map<string, Candidate[]>();
  for (const c of candidates) byProject.set(c.project, [...(byProject.get(c.project) ?? []), c]);
  return [...byProject.entries()].map(([project, cs]) => ({
    project,
    files: [...new Set(cs.map((c) => c.spec))].sort(),
    grep: [...new Set(cs.map((c) => escapeRegExp(c.leafTitle)))].sort().join("|"),
  }));
}

export type Outcome = "confirmed" | "flaky-under-load" | "not-rerun";

export interface MergeEntry extends Candidate {
  outcome: Outcome;
}

export interface MergeResult {
  report: PwReport;
  entries: MergeEntry[];
}

/** Replaces each selected test's results in the Bun report with its rerun results; a test the rerun never reached keeps its original failure. */
export function mergeConfirmation(bunReport: PwReport, reruns: PwReport[], selection: Selection): MergeResult {
  const rerunResults = new Map<string, PwTest["results"]>();
  for (const r of reruns) {
    walkTests(r, (file, t, fullTitle) => rerunResults.set(keyString({ spec: file, project: t.projectName, title: fullTitle }), t.results ?? []));
  }
  const selected = new Map(selection.candidates.map((c) => [keyString({ spec: c.spec, project: c.project, title: c.title }), c]));
  const merged = JSON.parse(JSON.stringify(bunReport)) as PwReport;
  const outcomes = new Map<string, Outcome>();
  walkTests(merged, (file, t, fullTitle) => {
    const key = keyString({ spec: file, project: t.projectName, title: fullTitle });
    if (!selected.has(key)) return;
    const results = rerunResults.get(key);
    // An interrupted or skipped rerun (global timeout hit) says nothing about the test.
    const usable = results && results.length > 0 && results.every((x) => x.status !== "skipped" && x.status !== "interrupted");
    if (!usable) {
      outcomes.set(key, "not-rerun");
      return;
    }
    t.results = results;
    outcomes.set(key, results.every((x) => x.status === "failed" || x.status === "timedOut") ? "confirmed" : "flaky-under-load");
  });
  const entries = selection.candidates.map((c) => ({ ...c, outcome: outcomes.get(keyString({ spec: c.spec, project: c.project, title: c.title })) ?? "not-rerun" }));
  return { report: merged, entries };
}

export function summaryMarkdown(selection: Selection, entries: MergeEntry[] | null): string {
  const lines = ["### Bun e2e confirmation rerun (1 worker)", ""];
  if (selection.status === "none") return [...lines, "No gate failures to confirm; nothing was rerun."].join("\n");
  if (selection.status === "too-many") {
    return [...lines, `${selection.candidates.length} tests would fail the gate (cap ${selection.max}): too many to be load flakiness, so nothing was rerun and the gate keeps the original failures.`].join("\n");
  }
  const list = (outcome: Outcome) => (entries ?? []).filter((e) => e.outcome === outcome);
  const section = (title: string, outcome: Outcome) => {
    const items = list(outcome);
    return [`**${title}: ${items.length}**`, ...items.map((e) => `- \`${e.spec}\` [${e.project}] ${e.title}`), ""];
  };
  return [
    ...lines,
    `Reran ${selection.candidates.length} gate-failing test(s) with one worker.`,
    "",
    ...section("Flaky under load (passed on the isolated rerun, not a gate failure)", "flaky-under-load"),
    ...section("Confirmed (failed again on the isolated rerun, still gates)", "confirmed"),
    "_Note: a test in a `describe.configure({ mode: 'serial' })` block is rerun without its earlier setup tests, so it can fail again for that reason alone; treat a confirmed serial test as unverified._",
    "",
    ...section("Not rerun (budget exhausted or the rerun did not reach them, still gates)", "not-rerun"),
  ].join("\n");
}

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

function arg(argv: string[], name: string, fallback?: string): string {
  const i = argv.indexOf(`--${name}`);
  const v = i >= 0 ? argv[i + 1] : fallback;
  if (v === undefined) throw new Error(`missing --${name}`);
  return v;
}

const readJson = <T>(p: string): T => JSON.parse(fs.readFileSync(p, "utf8")) as T;

function cmdSelect(argv: string[]): void {
  const bunReport = readJson<PwReport>(arg(argv, "bun-report"));
  const nextPath = argv.includes("--next-report") ? arg(argv, "next-report") : undefined;
  let nextReport: PwReport | null = null;
  if (nextPath && fs.existsSync(nextPath)) nextReport = readJson<PwReport>(nextPath);
  const { findings } = compare({
    baseline: readJson<Baseline>(arg(argv, "baseline")),
    expectedDiff: readJson<ExpectedDiff>(arg(argv, "expected-diff")),
    nextReport,
    bunReports: [bunReport],
  });
  const selection = selectCandidates(findings, bunReport, Number(arg(argv, "max", String(MAX_CONFIRM_TESTS))));
  fs.writeFileSync(arg(argv, "out"), JSON.stringify(selection, null, 2));
  console.log(`confirmation selection: ${selection.status} (${selection.candidates.length} test(s), cap ${selection.max})`);
  if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT, `status=${selection.status}\n`);
}

function cmdRun(argv: string[]): void {
  const selection = readJson<Selection>(arg(argv, "selection"));
  const outDir = path.resolve(arg(argv, "out-dir"));
  const deadline = Date.now() + Number(arg(argv, "budget-ms", "1080000"));
  fs.mkdirSync(outDir, { recursive: true });
  planGroups(selection.candidates).forEach((g, i) => {
    const remaining = deadline - Date.now();
    if (remaining <= 0) {
      console.log(`confirmation budget exhausted before project ${g.project}`);
      return;
    }
    const out = path.join(outDir, `rerun-${i}-${g.project}.json`);
    const r = spawnSync(
      "npx",
      ["playwright", "test", ...g.files, `--project=${g.project}`, "--grep", g.grep, "--workers=1", "--reporter=list,json", `--global-timeout=${remaining}`],
      { cwd: path.resolve(__dirname, "../tests/e2e"), stdio: "inherit", env: { ...process.env, E2E_WORKERS: "1", PLAYWRIGHT_JSON_OUTPUT_NAME: out } },
    );
    console.log(`confirmation rerun [${g.project}] exit=${r.status}`);
  });
}

function cmdMerge(argv: string[]): void {
  const bunReport = readJson<PwReport>(arg(argv, "bun-report"));
  const selection = readJson<Selection>(arg(argv, "selection"));
  const dir = arg(argv, "rerun-dir");
  const reruns = fs.existsSync(dir) ? fs.readdirSync(dir).filter((f) => f.endsWith(".json")).map((f) => readJson<PwReport>(path.join(dir, f))) : [];
  if (selection.status !== "rerun") {
    fs.writeFileSync(arg(argv, "out"), JSON.stringify(bunReport));
    console.log(summaryMarkdown(selection, null));
    return;
  }
  const { report, entries } = mergeConfirmation(bunReport, reruns, selection);
  fs.writeFileSync(arg(argv, "out"), JSON.stringify(report));
  console.log(summaryMarkdown(selection, entries));
}

if (require.main === module) {
  const [cmd, ...rest] = process.argv.slice(2);
  const cmds: Record<string, (a: string[]) => void> = { select: cmdSelect, run: cmdRun, merge: cmdMerge };
  if (!cmds[cmd]) throw new Error(`usage: e2e-confirm.ts <select|run|merge> ...`);
  cmds[cmd](rest);
}
