// Affected-spec selection for PR e2e runs: decide, from a PR's changed files, whether the e2e shards
// run every spec ("all"), a named subset ("subset") or nothing ("none"). Fail-safe by construction:
// a file that no rule recognizes selects "all", and any single "all" file in the diff wins over every
// subset rule. Main pushes, the nightly schedule and workflow_dispatch never call this: they run the
// full suite, which is what catches a wrong rule here (docs/reference/ci-job-budget.md).
//
//   bun scripts/select-specs.ts --specs specs.txt < changed-files.txt   (JSON on stdout)
//   bun scripts/select-specs.ts --specs specs.txt --changed-file changed.txt --summary   (markdown only)
//     specs.txt: one spec path per line, relative to tests/e2e (what `playwright --list` reports as `file`)
import * as fs from "fs";

export type Mode = "all" | "subset" | "none";

export interface Selection {
  mode: Mode;
  /** Selected spec paths (subset only; sorted). */
  specs: string[];
  /** One line per decision, for the job log: which file chose what and why. */
  reasons: string[];
}

/** Specs every subset keeps, as a cheap net under a rule that is too narrow. */
export const SAFETY_NET_SPECS = ["smoke.spec.ts", "route-smoke.spec.ts"];

/** Changes that cannot alter the e2e binary or any spec the shards run. First match wins, after ALL_RULES. */
export const NONE_RULES: { re: RegExp; why: string }[] = [
  { re: /_test\.go$/, why: "Go test file (not compiled into the e2e binary)" },
  { re: /^web-app\/(src|build|scripts)\/.*(\.test\.[jt]sx?|\/__tests__\/)/, why: "Jest test (not part of the web build)" },
  { re: /^docs\//, why: "documentation" },
  { re: /^project_plans\//, why: "planning docs" },
  { re: /^website\//, why: "marketing site" },
  { re: /^\.claude\//, why: "agent config" },
  { re: /\.md$/, why: "markdown" },
  { re: /^tools\/(ci|lint)\//, why: "CI/lint tooling (not part of the binary)" },
  { re: /^tests\/e2e\/baseline\//, why: "baseline data / its Jest tests (the comparator is main-only)" },
  { re: /^tests\/e2e\/shard-timings\.json$/, why: "shard timing plan (only changes how a full run is split)" },
  { re: /^\.github\/workflows\/(?!build\.yml$|_prepare\.yml$)/, why: "another workflow (build.yml/_prepare.yml are shared surfaces)" },
];

/** Shared surfaces: a match here forces "all" even when other files in the diff match a subset rule. */
/** Checked before ALL_RULES: a test file under a shared directory (web-app/build/__tests__) is still only a test. */
export const TEST_FILE_RULES = NONE_RULES.slice(0, 2);

export const ALL_RULES: { re: RegExp; why: string }[] = [
  { re: /^web-app\/src\/lib\//, why: "shared web-app library (terminal, nav, transport)" },
  { re: /^web-app\/src\/(app|components)\/(layout|providers|common|shared|ui|pane|window|sessions)\//, why: "shared shell / session UI" },
  { re: /^web-app\/src\/app\/(layout|page|Providers|RootShell|routes|FeatureGuard|route-meta)\./, why: "app shell" },
  { re: /^proto\//, why: "API contract" },
  { re: /^session\/(streamhub|tymux)\//, why: "terminal streaming path" },
  { re: /^tests\/e2e\/(helpers|pages|scripts|fixtures)\//, why: "shared e2e helpers" },
  { re: /^tests\/e2e\/(playwright[^/]*|global-(setup|teardown)|package\.json|bun\.lock|tsconfig[^/]*)$/, why: "e2e harness" },
  { re: /^\.github\/(workflows\/(build|_prepare)\.yml$|actions\/)/, why: "CI pipeline that builds/runs e2e" },
  { re: /^(go\.(mod|sum)|Makefile|install\.sh|main\.go|\.bun-version|\.gitmodules)$/, why: "build inputs" },
  { re: /^(web-app\/(build|patches|scripts)\/|web-app\/(package\.json|bun\.lock|bunfig\.toml|index\.html)$)/, why: "web build inputs" },
  { re: /^third_party\//, why: "vendored dependency" },
  { re: /^CHANGELOG\.md$/, why: "embedded into the binary (changelog_embed.go)" },
  { re: /^server\/(middleware|server\.go|dependencies\.go)/, why: "server wiring" },
];

/** Leaf areas: path -> the specs that exercise it. Globs are relative to tests/e2e; `*` never crosses `/`. */
export const SUBSET_RULES: { name: string; re: RegExp; specs: string[] }[] = [
  { name: "analytics", re: /^(server\/analytics\/|web-app\/src\/app\/analytics\/)/, specs: ["*analytics*.spec.ts", "insights*.spec.ts"] },
  { name: "files viewer", re: /^web-app\/src\/(app|components)\/files\//, specs: ["files-*.spec.ts"] },
  { name: "notifications page", re: /^web-app\/src\/app\/notifications\//, specs: ["notification*.spec.ts", "toast*.spec.ts"] },
];

/**
 * Web directories a SUBSET_RULES entry treats as a leaf. A leaf must not be imported from outside its group, or a change
 * there would reach pages the rule does not test (components/analytics, components/backlog* and app/insights failed this
 * and were dropped). select-specs.test.ts scans web-app/src and fails if that ever stops holding.
 */
export const WEB_LEAF_GROUPS: string[][] = [["app/analytics"], ["app/files", "components/files"], ["app/notifications"]];

const globToRe = (g: string): RegExp => new RegExp("^" + g.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, "[^/]*") + "$");

export function expandGlobs(globs: string[], allSpecs: string[]): string[] {
  const res = globs.map(globToRe);
  return allSpecs.filter((s) => res.some((r) => r.test(s)));
}

const SPEC_FILE = /^tests\/e2e\/(.+\.spec\.ts)$/;
// Specs (like their baseline classification) are tied to their project config; a spec under a project dir
// that no allSpecs entry matches (a brand-new file) still selects itself, so a new spec always runs in its PR.

export function select(changed: string[], allSpecs: string[]): Selection {
  if (changed.every((f) => f.trim() === "")) {
    return { mode: "all", specs: [], reasons: ["ALL    (no changed files reported): fail-safe"] };
  }
  const reasons: string[] = [];
  const chosen = new Set<string>();
  let forceAll = false;
  for (const raw of changed.map((f) => f.trim()).filter(Boolean)) {
    const testHit = TEST_FILE_RULES.find((r) => r.re.test(raw));
    if (testHit) {
      reasons.push(`none   ${raw}: ${testHit.why}`);
      continue;
    }
    const allHit = ALL_RULES.find((r) => r.re.test(raw));
    if (allHit) {
      forceAll = true;
      reasons.push(`ALL    ${raw}: ${allHit.why}`);
      continue;
    }
    const spec = SPEC_FILE.exec(raw);
    if (spec) {
      chosen.add(spec[1]);
      reasons.push(`subset ${raw}: the spec itself`);
      continue;
    }
    const noneHit = NONE_RULES.find((r) => r.re.test(raw));
    if (noneHit) {
      reasons.push(`none   ${raw}: ${noneHit.why}`);
      continue;
    }
    const rule = SUBSET_RULES.find((r) => r.re.test(raw));
    if (rule) {
      for (const s of expandGlobs(rule.specs, allSpecs)) chosen.add(s);
      reasons.push(`subset ${raw}: ${rule.name} -> ${rule.specs.join(", ")}`);
      continue;
    }
    forceAll = true;
    reasons.push(`ALL    ${raw}: no rule recognizes this file (fail-safe)`);
  }
  if (forceAll) return { mode: "all", specs: [], reasons };
  if (chosen.size === 0) return { mode: "none", specs: [], reasons };
  for (const s of expandGlobs(SAFETY_NET_SPECS, allSpecs)) chosen.add(s);
  return { mode: "subset", specs: [...chosen].sort(), reasons };
}

/** Markdown for $GITHUB_STEP_SUMMARY: the decision and the file-by-file reasons. */
export function summarize(sel: Selection, totalSpecs: number): string {
  const head =
    sel.mode === "all"
      ? `**e2e selection: ALL ${totalSpecs} specs**`
      : sel.mode === "none"
        ? "**e2e selection: NONE (no change can affect e2e; the check reports as skipped-green)**"
        : `**e2e selection: SUBSET ${sel.specs.length} of ${totalSpecs} specs**`;
  return [head, "", "```", ...sel.reasons.slice(0, 80), sel.reasons.length > 80 ? `... ${sel.reasons.length - 80} more` : "", "```"].join("\n");
}

/** Shard count for a subset: one shard per ~SHARD_TARGET_S of recorded spec seconds, capped at the full-run count. */
export const SHARD_TARGET_S = 4000;
export function shardCountFor(specs: string[], specSeconds: Record<string, number>, maxShards: number): number {
  const values = Object.values(specSeconds).sort((a, b) => a - b);
  const median = values.length ? values[Math.floor(values.length / 2)] : 1;
  const load = specs.reduce((n, s) => n + (specSeconds[s] ?? median), 0);
  return Math.max(1, Math.min(maxShards, specs.length, Math.ceil(load / SHARD_TARGET_S)));
}

function main(argv: string[]): number {
  const arg = (k: string) => (argv.includes(k) ? argv[argv.indexOf(k) + 1] : undefined);
  const specsFile = arg("--specs");
  if (!specsFile) {
    console.error("usage: select-specs.ts --specs specs.txt [--changed-file f] [--timings shard-timings.json --max-shards N] [--summary] < changed");
    return 2;
  }
  const allSpecs = fs.readFileSync(specsFile, "utf8").split("\n").map((s) => s.trim()).filter(Boolean);
  const changed = fs.readFileSync(arg("--changed-file") ?? 0, "utf8").split("\n");
  const sel = select(changed, allSpecs);
  if (argv.includes("--summary")) {
    console.log(summarize(sel, allSpecs.length));
    return 0;
  }
  const timings = arg("--timings") ? JSON.parse(fs.readFileSync(arg("--timings")!, "utf8")).specSeconds : {};
  const max = Number(arg("--max-shards") ?? 5);
  const shards = sel.mode === "all" ? max : sel.mode === "none" ? 0 : shardCountFor(sel.specs, timings, max);
  console.log(JSON.stringify({ ...sel, shards, total: allSpecs.length }));
  return 0;
}

if (require.main === module) process.exit(main(process.argv));
