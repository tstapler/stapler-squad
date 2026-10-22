// Gate S0 / plan Task 0.1e: turn three Playwright JSON reports into tests/e2e/baseline/next-e2e-baseline.json.
//   node classify-e2e.mjs <out.json> <metaJsonFile> <run1.json> <run2.json> <run3.json>
//   node --test classify-e2e.test.mjs        (fixture tests, in the same directory)
// Classes: pass = passed on attempt 0 in every run; fail = final status failed in every run;
// skipped = skipped in every run; flaky = anything else (a retry in any run, or final outcomes that differ).
import { readFileSync, writeFileSync } from "node:fs";

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

if (process.argv[1] && process.argv[1].endsWith("classify-e2e.mjs")) {
  const [outFile, metaFile, ...reportFiles] = process.argv.slice(2);
  const reports = reportFiles.map((f) => JSON.parse(readFileSync(f, "utf8")));
  const baseline = buildBaseline(reports, JSON.parse(readFileSync(metaFile, "utf8")));
  writeFileSync(outFile, JSON.stringify(baseline, null, 1) + "\n");
  console.log(JSON.stringify(baseline.meta.counts), "specs:", baseline.meta.specCount);
}
