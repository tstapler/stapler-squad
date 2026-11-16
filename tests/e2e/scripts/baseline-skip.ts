// PR-only skip list: tests the committed baseline records as class "fail" (known permanent failures).
// They already cannot fail the PR gate, but each burns two 30 s attempts per run, so PR shards pass
// them to Playwright as --grep-invert. main, nightly and workflow_dispatch never set E2E_SKIP_BASELINE_FAIL
// and still run them (D2 in project_plans/e2e-runtime-reduction/plan.md).
//
// A spec file the PR itself changes is never skipped (E2E_KEEP_SPECS, newline-separated, relative to tests/e2e),
// so a PR that repairs a baseline-fail test sees it run without also having to edit the baseline.
//
//   bun scripts/baseline-skip.ts grep-invert [--baseline <file>]                  regex for `--grep-invert`
//   bun scripts/baseline-skip.ts summary [--baseline <file>] [--removed N]        markdown summary of what is skipped
//   bun scripts/baseline-skip.ts count <playwright-list.json>                     number of tests in a `--list` report
import * as fs from "fs";

export interface BaselineRow {
  spec: string;
  project: string;
  title: string;
  class: string;
}

export interface BaselineMeta {
  capturedAt?: string;
}

/** Rows the PR run skips: class "fail", minus spec files in `keep` (the PR's own changed specs). */
export function failRows(rows: BaselineRow[], keep: ReadonlySet<string> = new Set()): BaselineRow[] {
  return rows.filter((r) => r.class === "fail" && !keep.has(r.spec));
}

const escapeRe = (s: string): string => s.replace(/[.*+?^${}()|[\]\\\/]/g, "\\$&");

/**
 * Playwright matches --grep(-invert) against the test's titlePath joined with spaces:
 * "<project> <spec file> <describe...> <title>". Baseline titles join describes with " > ", but a real
 * title can contain " > " itself, so the separator between parts accepts either form.
 */
export function grepInvertFor(rows: BaselineRow[], keep: ReadonlySet<string> = new Set()): string {
  const alts: string[] = [];
  for (const r of failRows(rows, keep)) {
    // " (?:> )?" accepts the describe join (" ") and a literal " > " inside a title in one alternative, which keeps
    // the regex small: it travels as one argv string, and Linux truncates those at 128 KB.
    alts.push(`${escapeRe(r.project)} ${escapeRe(r.spec)} ${r.title.split(" > ").map(escapeRe).join(" (?:> )?")}`);
  }
  if (alts.length === 0) return "$^"; // matches nothing: an empty alternation would match every test
  return `(^|\\s)(${[...new Set(alts)].join("|")})$`;
}

const DAY_MS = 86_400_000;
export const STALE_AFTER_DAYS = 14;

export function summaryFor(
  rows: BaselineRow[],
  opts: { keep?: ReadonlySet<string>; removed?: number; meta?: BaselineMeta; now?: Date } = {},
): string {
  const keep = opts.keep ?? new Set<string>();
  const fails = failRows(rows, keep);
  const kept = rows.filter((r) => r.class === "fail" && keep.has(r.spec)).length;
  const byFile = new Map<string, number>();
  for (const r of fails) byFile.set(r.spec, (byFile.get(r.spec) ?? 0) + 1);
  const lines = [...byFile.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([f, n]) => `- ${f}: ${n}`);
  const count =
    opts.removed === undefined
      ? `${fails.length} baseline rows (not verified against playwright --list)`
      : `${opts.removed} tests removed from the parallel-pass list (${fails.length} baseline rows; stress specs are counted separately)`;
  const out = [`### Baseline-fail tests skipped on this PR run: ${count} (main and nightly still run them)`];
  const captured = opts.meta?.capturedAt ? new Date(opts.meta.capturedAt) : undefined;
  if (captured && !Number.isNaN(captured.getTime())) {
    const age = Math.floor(((opts.now ?? new Date()).getTime() - captured.getTime()) / DAY_MS);
    const warn = age > STALE_AFTER_DAYS ? ` WARNING: older than ${STALE_AFTER_DAYS} days; tests fixed since then stay hidden on PRs until the baseline is re-recorded.` : "";
    out.push(`Baseline captured ${opts.meta?.capturedAt} (${age} days ago).${warn}`);
  } else {
    out.push("WARNING: baseline capturedAt is missing; its age is unknown.");
  }
  out.push("Source: tests/e2e/baseline/next-e2e-baseline.json, class `fail`. A test that starts passing is surfaced by main/nightly, not here.");
  if (kept > 0) out.push(`Not skipped because this PR changes their spec file: ${kept} rows.`);
  return [...out, "", ...lines].join("\n");
}

interface ListSuite {
  specs?: { tests?: unknown[] }[];
  suites?: ListSuite[];
}

/** Number of tests in a `playwright test --list --reporter=json` report. */
export function countListed(report: ListSuite): number {
  let n = 0;
  const walk = (s: ListSuite) => {
    for (const sp of s.specs ?? []) n += (sp.tests ?? []).length;
    for (const c of s.suites ?? []) walk(c);
  };
  walk(report);
  return n;
}

const keepFromEnv = (): Set<string> =>
  new Set(
    (process.env.E2E_KEEP_SPECS ?? "")
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean),
  );

function main(argv: string[]): number {
  const cmd = argv[2];
  const flag = (name: string) => (argv.indexOf(name) >= 0 ? argv[argv.indexOf(name) + 1] : undefined);
  const file = flag("--baseline") ?? `${__dirname}/../baseline/next-e2e-baseline.json`;
  if (cmd === "count") {
    console.log(countListed(JSON.parse(fs.readFileSync(argv[3], "utf8"))));
    return 0;
  }
  const baseline = JSON.parse(fs.readFileSync(file, "utf8")) as { specs: BaselineRow[]; meta?: BaselineMeta };
  if (cmd === "grep-invert") {
    process.stdout.write(grepInvertFor(baseline.specs, keepFromEnv()));
  } else if (cmd === "summary") {
    const removed = flag("--removed");
    console.log(summaryFor(baseline.specs, { keep: keepFromEnv(), removed: removed === undefined ? undefined : Number(removed), meta: baseline.meta }));
  } else {
    console.error("usage: baseline-skip.ts grep-invert|summary|count ...");
    return 2;
  }
  return 0;
}

if (require.main === module) process.exit(main(process.argv));
