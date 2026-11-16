// PR-only skip list: tests the committed baseline records as class "fail" (known permanent failures).
// They already cannot fail the PR gate, but each burns two 30 s attempts per run, so PR shards pass
// them to Playwright as --grep-invert. main, nightly and workflow_dispatch never set E2E_SKIP_BASELINE_FAIL
// and still run them (D2 in project_plans/e2e-runtime-reduction/plan.md).
//
//   bun scripts/baseline-skip.ts grep-invert [--baseline <file>]   prints the regex for `--grep-invert`
//   bun scripts/baseline-skip.ts summary [--baseline <file>]       prints a markdown summary of what is skipped
import * as fs from "fs";

export interface BaselineRow {
  spec: string;
  project: string;
  title: string;
  class: string;
}

/** Rows the PR run skips. Only the projects the shard runner actually passes are relevant. */
export function failRows(rows: BaselineRow[]): BaselineRow[] {
  return rows.filter((r) => r.class === "fail");
}

const escapeRe = (s: string): string => s.replace(/[.*+?^${}()|[\]\\\/]/g, "\\$&");

/**
 * Playwright matches --grep(-invert) against the test's titlePath joined with spaces:
 * "<project> <spec file> <describe...> <title>". Baseline titles join describes with " > ".
 */
export function grepInvertFor(rows: BaselineRow[]): string {
  const alts = failRows(rows).map((r) => `${escapeRe(r.project)} ${escapeRe(r.spec)} ${r.title.split(" > ").map(escapeRe).join(" ")}`);
  if (alts.length === 0) return "$^"; // matches nothing: an empty alternation would match every test
  return `(^|\\s)(${[...new Set(alts)].join("|")})$`;
}

export function summaryFor(rows: BaselineRow[]): string {
  const fails = failRows(rows);
  const byFile = new Map<string, number>();
  for (const r of fails) byFile.set(r.spec, (byFile.get(r.spec) ?? 0) + 1);
  const lines = [...byFile.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([f, n]) => `- ${f}: ${n}`);
  return [
    `### Baseline-fail tests skipped on this PR run: ${fails.length} (main and nightly still run them)`,
    `Source: tests/e2e/baseline/next-e2e-baseline.json, class \`fail\`. A test that starts passing is surfaced by main/nightly, not here.`,
    "",
    ...lines,
  ].join("\n");
}

function main(argv: string[]): number {
  const cmd = argv[2];
  const i = argv.indexOf("--baseline");
  const file = i >= 0 ? argv[i + 1] : `${__dirname}/../baseline/next-e2e-baseline.json`;
  const rows: BaselineRow[] = JSON.parse(fs.readFileSync(file, "utf8")).specs;
  if (cmd === "grep-invert") process.stdout.write(grepInvertFor(rows));
  else if (cmd === "summary") console.log(summaryFor(rows));
  else {
    console.error("usage: baseline-skip.ts grep-invert|summary [--baseline file]");
    return 2;
  }
  return 0;
}

if (require.main === module) process.exit(main(process.argv));
