// Replays the affected-spec selector over the last N merged PRs' changed files and prints the share that would
// skip e2e, run a subset or run everything, plus estimated shard-minutes. Needs an authenticated `gh`.
//   bun scripts/replay-selection.ts [--prs 60] [--repo tstapler/stapler-squad]
import { execFileSync } from "child_process";
import * as fs from "fs";
import { select, shardCountFor } from "./select-specs";

const arg = (k: string, d: string) => (process.argv.includes(k) ? process.argv[process.argv.indexOf(k) + 1] : d);
const repo = arg("--repo", "tstapler/stapler-squad");
const n = Number(arg("--prs", "60"));
const gh = (...a: string[]) => execFileSync("gh", a, { encoding: "utf8", maxBuffer: 1 << 26 });

const timings = JSON.parse(fs.readFileSync(`${__dirname}/../shard-timings.json`, "utf8")).specSeconds as Record<string, number>;
const allSpecs = Object.keys(timings);
const fullSeconds = Object.values(timings).reduce((a, b) => a + b, 0);
const prs = JSON.parse(gh("pr", "list", "-R", repo, "--state", "merged", "--limit", String(n), "--json", "number,title"));
const tally = { all: 0, subset: 0, none: 0 };
let savedSeconds = 0;
for (const pr of prs) {
  const files = gh("api", `repos/${repo}/pulls/${pr.number}/files`, "--paginate", "--jq", ".[].filename").split("\n");
  const sel = select(files, allSpecs);
  tally[sel.mode]++;
  const load = sel.mode === "all" ? fullSeconds : sel.mode === "none" ? 0 : sel.specs.reduce((a, s) => a + (timings[s] ?? 0), 0);
  savedSeconds += fullSeconds - load;
  const shards = sel.mode === "all" ? 5 : sel.mode === "none" ? 0 : shardCountFor(sel.specs, timings, 5);
  console.log(`#${pr.number}\t${sel.mode}\t${shards} shards\t${sel.specs.length} specs\t${pr.title.slice(0, 70)}`);
}
const pct = (x: number) => ((100 * x) / prs.length).toFixed(0);
console.log(`\n${prs.length} PRs: all=${tally.all} (${pct(tally.all)}%) subset=${tally.subset} (${pct(tally.subset)}%) none=${tally.none} (${pct(tally.none)}%)`);
console.log(`mean recorded spec-seconds saved per PR run: ${(savedSeconds / prs.length).toFixed(0)} of ${fullSeconds} (${((100 * savedSeconds) / prs.length / fullSeconds).toFixed(0)}%)`);
