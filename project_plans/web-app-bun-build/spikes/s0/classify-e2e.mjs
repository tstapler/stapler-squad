// Gate S0 / plan Task 0.1e: turn three Playwright JSON reports into tests/e2e/baseline/next-e2e-baseline.json.
//   node classify-e2e.mjs <out.json> <metaJsonFile> <run1.json> <run2.json> <run3.json> \
//     [--routes-jsonl <path>] [--build-coupled <build-coupled-specs.json>]
//   node --test classify-e2e.test.mjs        (fixture tests, in the same directory)
// Classes: pass = passed on attempt 0 in every run; fail = final status failed in every run;
// skipped = skipped in every run; flaky = anything else (a retry in any run, or final outcomes that differ).
import { readFileSync, writeFileSync, existsSync } from "node:fs";

const FAILED = new Set(["failed", "timedOut", "interrupted"]);

export function collectAttempts(report) {
  const out = new Map();
  const walk = (suite, titles) => {
    for (const spec of suite.specs ?? []) {
      for (const test of spec.tests ?? []) {
        const key = [spec.file, test.projectName, ...titles, spec.title].join(" :: ");
        out.set(key, {
          file: spec.file,
          project: test.projectName,
          title: [...titles, spec.title].join(" > "),
          attempts: (test.results ?? []).map((r) => ({ retry: r.retry, status: r.status })),
        });
      }
    }
    for (const child of suite.suites ?? []) walk(child, child.title && child.title !== child.file ? [...titles, child.title] : titles);
  };
  for (const top of report.suites ?? []) walk(top, []);
  return out;
}

export function classify(attemptsPerRun) {
  const finals = attemptsPerRun.map((attempts) => attempts[attempts.length - 1]?.status ?? "missing");
  if (finals.every((s) => s === "skipped")) return "skipped";
  if (attemptsPerRun.every((a) => a.length === 1 && a[0].status === "passed")) return "pass";
  if (finals.every((s) => FAILED.has(s))) return "fail";
  return "flaky";
}

export function buildBaseline(reports, meta) {
  const perRun = reports.map(collectAttempts);
  const keys = [...new Set(perRun.flatMap((m) => [...m.keys()]))].sort();
  const specs = keys.map((key) => {
    const first = perRun.find((m) => m.has(key)).get(key);
    const runs = perRun.map((m) => ({ attempts: m.get(key)?.attempts ?? [] }));
    return { spec: first.file, project: first.project, title: first.title, class: classify(runs.map((r) => r.attempts)), runs, routesVisited: null };
  });
  const counts = Object.fromEntries(["pass", "flaky", "fail", "skipped"].map((c) => [c, specs.filter((s) => s.class === c).length]));
  return { meta: { ...meta, specCount: specs.length, counts }, specs };
}

/**
 * Task 0.1e's routesVisited fixture hook (tests/e2e/baseline/route-visit-preload.js) writes one
 * JSONL line per page closed during the baseline run: {specFile, routes}. Granularity is per spec
 * FILE, not per test title (Task 7.10e's route-coverage-manifest only needs "which specs visited
 * this route", per its own AC wording) -- every title in a file gets that file's merged route set.
 * Mutates and returns `specs` for convenience; safe to call with a missing/absent jsonlPath (no-op).
 */
export function mergeRoutesVisited(specs, jsonlPath) {
  if (!jsonlPath || !existsSync(jsonlPath)) return specs;
  const byFile = new Map();
  for (const line of readFileSync(jsonlPath, "utf8").split("\n")) {
    if (!line.trim()) continue;
    const { specFile, routes } = JSON.parse(line);
    if (!byFile.has(specFile)) byFile.set(specFile, new Set());
    for (const r of routes) byFile.get(specFile).add(r);
  }
  for (const s of specs) {
    const set = byFile.get(s.spec);
    if (set) s.routesVisited = [...set].sort();
  }
  return specs;
}

/**
 * Task 0.1f: next-e2e-baseline.json carries its own copy of the build-coupled-spec inventory so
 * the file is self-describing (no README, per Story 0.1's AC). The inventory itself was already
 * produced once, by grep, for Task 7.11a's tests/e2e/baseline/build-coupled-specs.json (PR #1117)
 * -- this reads that committed file rather than re-deriving it, so the two never drift apart.
 */
export function loadBuildCoupledSpecs(buildCoupledSpecsPath) {
  if (!buildCoupledSpecsPath || !existsSync(buildCoupledSpecsPath)) return [];
  const doc = JSON.parse(readFileSync(buildCoupledSpecsPath, "utf8"));
  return (doc.specs ?? []).map((s) => ({ spec: s.spec, coupling: s.coupling, state: s.state }));
}

function parseFlags(argv) {
  const flags = {};
  const positional = [];
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--routes-jsonl") flags.routesJsonl = argv[++i];
    else if (argv[i] === "--build-coupled") flags.buildCoupled = argv[++i];
    else positional.push(argv[i]);
  }
  return { flags, positional };
}

if (process.argv[1] && process.argv[1].endsWith("classify-e2e.mjs")) {
  const { flags, positional } = parseFlags(process.argv.slice(2));
  const [outFile, metaFile, ...reportFiles] = positional;
  const reports = reportFiles.map((f) => JSON.parse(readFileSync(f, "utf8")));
  const baseline = buildBaseline(reports, JSON.parse(readFileSync(metaFile, "utf8")));
  mergeRoutesVisited(baseline.specs, flags.routesJsonl);
  baseline.buildCoupledSpecs = loadBuildCoupledSpecs(flags.buildCoupled);
  writeFileSync(outFile, JSON.stringify(baseline, null, 1) + "\n");
  console.log(JSON.stringify(baseline.meta.counts), "specs:", baseline.meta.specCount, "buildCoupledSpecs:", baseline.buildCoupledSpecs.length);
}
