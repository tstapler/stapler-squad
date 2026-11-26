// Re-records the baseline rows of the eight repaired spec files from three CI Playwright reports.
//   node rerecord-baseline.mjs <main-baseline.json> <out.json> <commit> <runUrl,runUrl,runUrl> <report1> <report2> <report3>
// No row is typed by hand; every other row is copied from <main-baseline.json> untouched.
import { readFileSync, writeFileSync } from "node:fs";
import { buildBaseline } from "../web-app-bun-build/spikes/s0/classify-e2e.mjs";

const DELETED = "terminal-flickering.spec.ts";
const FILES = [
  "android-url-bar.spec.ts",
  "input-drop-badge.spec.ts",
  DELETED,
  "terminal-mobile-overflow.spec.ts",
  "terminal-resize.spec.ts",
  "terminal-resync-feature-flags.spec.ts",
  "terminal-stress/ascii-video.spec.ts",
  "terminal-stress/large-payload.spec.ts",
];

const [basePath, outPath, commit, urls, ...reportPaths] = process.argv.slice(2);
const die = (msg) => {
  console.error(`refusing to write: ${msg}`);
  process.exit(1);
};
const runDates = (urls ?? "").split(",").filter(Boolean);
if (reportPaths.length !== 3 || runDates.length !== 3) die("need exactly three reports and three run URLs");

const reports = reportPaths.map((p) => {
  const r = JSON.parse(readFileSync(p, "utf8"));
  if ((r.errors ?? []).length) die(`${p} has ${r.errors.length} top-level errors`);
  return r;
});
const base = JSON.parse(readFileSync(basePath, "utf8"));
const key = (s) => JSON.stringify([s.spec, s.project, s.title]);
const ours = (s) => FILES.includes(s.spec);

const fresh = buildBaseline(reports, {}).specs.filter(ours);
for (const s of fresh) {
  if (s.runs.some((r) => r.attempts.length === 0)) die(`no attempts in some run for ${key(s)}`);
}
for (const file of FILES) {
  const present = fresh.some((s) => s.spec === file);
  if (file === DELETED ? present : !present) die(`${file} ${present ? "is still reported" : "has no rows"}`);
}

const oldByKey = new Map(base.specs.map((s) => [key(s), s]));
const routesByFile = new Map(base.specs.filter((s) => ours(s) && s.routesVisited).map((s) => [s.spec, s.routesVisited]));
for (const s of fresh) {
  const old = oldByKey.get(key(s));
  s.routesVisited = old ? old.routesVisited : (routesByFile.get(s.spec) ?? null);
}

const freshByKey = new Map(fresh.map((s) => [key(s), s]));
const regressed = base.specs.filter((s) => ours(s) && s.class === "pass" && freshByKey.get(key(s))?.class === "fail");
if (regressed.length) die(`pass -> fail: ${regressed.map(key).join(", ")}`);

// Expected class of each row that was `fail` on main; the two stress behaviour tests are recorded as observed.
const OBSERVED = /visible flickering|corrupt terminal state/;
const wrong = [];
for (const old of base.specs.filter((s) => ours(s) && s.class === "fail")) {
  if (old.spec === DELETED) continue;
  if (old.spec.startsWith("terminal-stress/") && OBSERVED.test(old.title)) continue;
  const want = old.spec === "terminal-stress/ascii-video.spec.ts" ? "skipped" : "pass";
  // A renamed test (the resize button) has no row under its old key; then every row of that file must hold.
  const now = freshByKey.get(key(old));
  const got = now ? [now] : fresh.filter((s) => s.spec === old.spec && s.project === old.project && s.class !== "skipped");
  if (!got.length || got.some((s) => s.class !== want)) wrong.push(`${key(old)}: want ${want}, got ${got.map((s) => s.class).join("/") || "nothing"}`);
}
if (wrong.length) die(`unexpected classes:\n  ${wrong.join("\n  ")}`);

// Keep main's row order; a file's rows are replaced where its first row stood, new files cannot occur.
const emitted = new Set();
const specs = base.specs.flatMap((s) => {
  if (!ours(s)) return [s];
  if (emitted.has(s.spec)) return [];
  emitted.add(s.spec);
  return fresh.filter((f) => f.spec === s.spec);
});
const counts = Object.fromEntries(Object.keys(base.meta.counts).map((c) => [c, specs.filter((s) => s.class === c).length]));
const meta = { ...base.meta, specCount: specs.length, counts, rerecorded: { commit, specFiles: FILES, runDates } };
writeFileSync(outPath, `${JSON.stringify({ ...base, meta, specs }, null, 1)}\n`);

const tally = {};
for (const s of fresh) tally[`${s.spec} ${s.class}`] = (tally[`${s.spec} ${s.class}`] ?? 0) + 1;
console.log(tally);
for (const s of fresh.filter((s) => s.class === "fail" || s.class === "flaky")) console.log(s.class, key(s));
