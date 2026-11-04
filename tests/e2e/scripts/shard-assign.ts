// Duration-balanced shard assignment. Playwright's own --shard splits by test count, so a shard that
// happens to hold slow or retry-heavy specs overran its 40 min budget while others finished in 16.
// This assigns whole spec files to shards by recorded duration (longest first onto the least-loaded shard).
//
//   bun scripts/shard-assign.ts assign --shard 4 --total 5 [--timings shard-timings.json] < files.txt
//       prints the "tests/e2e/<file>$" filter for each file of that shard (files.txt: one spec path per line)
//   bun scripts/shard-assign.ts drift --report merged-report.json --total 5   (is the committed plan still balanced?)
//   bun scripts/shard-assign.ts timings --report merged-report.json --out shard-timings.json
//       rebuilds the timings file from a merged Playwright JSON report (refresh when the suite shifts)
import * as fs from "fs";

export interface Timings {
  /** Spec path relative to tests/e2e (as Playwright reports it) -> summed attempt seconds across projects. */
  specSeconds: Record<string, number>;
}

interface Report {
  suites?: Suite[];
}
interface Suite {
  specs?: { file: string; tests?: { results?: { duration: number }[] }[] }[];
  suites?: Suite[];
}

/** Seconds per spec file, counting every attempt: failing and retrying tests burn real shard time. */
export function timingsFromReport(report: Report): Timings {
  const specSeconds: Record<string, number> = {};
  const walk = (s: Suite) => {
    for (const spec of s.specs ?? []) {
      for (const t of spec.tests ?? []) {
        for (const r of t.results ?? []) specSeconds[spec.file] = (specSeconds[spec.file] ?? 0) + r.duration / 1000;
      }
    }
    for (const c of s.suites ?? []) walk(c);
  };
  for (const s of report.suites ?? []) walk(s);
  for (const k of Object.keys(specSeconds)) specSeconds[k] = Math.round(specSeconds[k]);
  return { specSeconds };
}

const median = (xs: number[]): number => {
  if (xs.length === 0) return 1;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)];
};

/** Every file lands on exactly one shard; unknown files cost the median so a new spec cannot be ignored. */
export function assignShards(files: string[], timings: Timings, total: number): string[][] {
  const known = Object.values(timings.specSeconds);
  const fallback = median(known);
  const cost = (f: string) => timings.specSeconds[f] ?? fallback;
  const shards: { files: string[]; load: number }[] = Array.from({ length: total }, () => ({ files: [], load: 0 }));
  const order = [...new Set(files)].sort((a, b) => cost(b) - cost(a) || a.localeCompare(b));
  for (const f of order) {
    const target = shards.reduce((best, s) => (s.load < best.load ? s : best), shards[0]);
    target.files.push(f);
    target.load += cost(f);
  }
  return shards.map((s) => s.files.sort());
}

export function shardLoads(files: string[], timings: Timings, total: number): number[] {
  const fallback = median(Object.values(timings.specSeconds));
  return assignShards(files, timings, total).map((fs_) => fs_.reduce((n, f) => n + (timings.specSeconds[f] ?? fallback), 0));
}

/**
 * How the committed plan would have loaded each shard under the durations this run actually measured. A max well above
 * the mean means shard-timings.json is stale (the 2.3x notification-contrast-over-xterm drift left one shard 35% heavy).
 */
export function planDrift(committed: Timings, fresh: Timings, total: number): { loads: number[]; maxOverMean: number } {
  const files = Object.keys(fresh.specSeconds);
  const loads = assignShards(files, committed, total).map((fs_) => fs_.reduce((n, f) => n + (fresh.specSeconds[f] ?? 0), 0));
  const mean = loads.reduce((a, b) => a + b, 0) / loads.length;
  return { loads, maxOverMean: mean === 0 ? 1 : Math.max(...loads) / mean };
}

const escapeRegExp = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Anchored so `files.spec.ts` can never also select `profiles.spec.ts` (file filters are regexes over the absolute path). */
export function fileFilter(file: string): string {
  return `tests/e2e/${escapeRegExp(file)}$`;
}

function arg(argv: string[], name: string, fallback?: string): string {
  const i = argv.indexOf(`--${name}`);
  const v = i >= 0 ? argv[i + 1] : fallback;
  if (v === undefined) throw new Error(`missing --${name}`);
  return v;
}

if (require.main === module) {
  const [cmd, ...rest] = process.argv.slice(2);
  if (cmd === "assign") {
    const shard = Number(arg(rest, "shard"));
    const total = Number(arg(rest, "total"));
    const timings = JSON.parse(fs.readFileSync(arg(rest, "timings", `${__dirname}/../shard-timings.json`), "utf8")) as Timings;
    const files = fs.readFileSync(0, "utf8").split("\n").map((l) => l.trim()).filter(Boolean);
    const assigned = assignShards(files, timings, total);
    const loads = shardLoads(files, timings, total);
    console.error(`shard-assign: shard ${shard}/${total}: ${assigned[shard - 1].length} files, estimated ${loads[shard - 1]}s of test time (all shards: ${loads.join("s, ")}s)`);
    for (const f of assigned[shard - 1]) console.log(fileFilter(f));
  } else if (cmd === "drift") {
    const committed = JSON.parse(fs.readFileSync(arg(rest, "timings", `${__dirname}/../shard-timings.json`), "utf8")) as Timings;
    const fresh = timingsFromReport(JSON.parse(fs.readFileSync(arg(rest, "report"), "utf8")) as Report);
    const { loads, maxOverMean } = planDrift(committed, fresh, Number(arg(rest, "total")));
    const stale = maxOverMean > 1.15;
    console.log(`Shard plan vs measured durations: loads ${loads.join("s, ")}s, heaviest is ${maxOverMean.toFixed(2)}x the mean${stale ? " -- shard-timings.json is stale, refresh it from the e2e-shard-timings artifact of a full uncut run" : ""}`);
  } else if (cmd === "timings") {
    const report = JSON.parse(fs.readFileSync(arg(rest, "report"), "utf8")) as Report;
    fs.writeFileSync(arg(rest, "out"), `${JSON.stringify(timingsFromReport(report), null, 1)}\n`);
  } else {
    throw new Error("usage: shard-assign.ts <assign|timings> ...");
  }
}
