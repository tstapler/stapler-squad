// Run by `bun run test:codemods` (node --test). actionlint covers syntax in `make lint-workflows`; this covers intent.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../..");
const read = (rel) => readFileSync(path.join(root, rel), "utf8");

/** Entries of the `pull_request.paths` list, read without a YAML dependency. */
export function pullRequestPaths(workflow) {
  const lines = workflow.split("\n");
  const start = lines.findIndex((l) => /^ {4}paths:\s*$/.test(l));
  if (start === -1) return [];
  const out = [];
  for (const l of lines.slice(start + 1)) {
    const m = l.match(/^ {6}- '?([^']+)'?\s*$/);
    if (!m) break;
    out.push(m[1]);
  }
  return out;
}

const LIVE_PORT = ["85", "43"].join("");
const WATCHED = [
  "web-app/src/**",
  "web-app/build/**",
  "web-app/package.json",
  "web-app/bun.lock",
  "web-app/patches/**",
  "web-app/index.html",
  "web-app/bunfig.toml",
  "Makefile",
  "server/web/**",
  "server/middleware/static.go",
  "server/middleware/auth.go",
];

test("ux-analysis triggers on every file of the Bun build and the serve path", () => {
  const paths = pullRequestPaths(read(".github/workflows/ux-analysis.yml"));
  for (const p of WATCHED) assert.ok(paths.includes(p), `ux-analysis.yml paths is missing ${p}`);
});

test("the path reader can fail: a workflow with only web-app/src misses the build paths", () => {
  const old = "on:\n  pull_request:\n    paths:\n      - 'web-app/src/**'\n\npermissions:\n";
  assert.deepEqual(pullRequestPaths(old), ["web-app/src/**"]);
});

test("every .next/cache step is skipped under WEB_BUILD=bun", () => {
  for (const file of [".github/workflows/benchmark.yml", ".github/actions/prepare/action.yml"]) {
    const blocks = read(file).split(/\n(?= *- name: )/).filter((b) => b.includes("path: web-app/.next/cache"));
    assert.ok(blocks.length > 0, `${file} has no .next/cache step`);
    for (const b of blocks) assert.match(b, /if: env\.WEB_BUILD != 'bun'/, `${file}: ${b.split("\n")[0]}`);
  }
});

test("Lighthouse has one config, served by the Go server, covering a nested deep link", () => {
  const cfg = read("tests/e2e/lighthouse.config.js");
  for (const route of ["/review-queue", "/settings/jules"]) assert.ok(cfg.includes(`localhost:${LIVE_PORT}${route}`), route);
  assert.equal(existsSyncSafe("web-app/lighthouserc.json"), false);
  assert.doesNotMatch(read(".github/workflows/ux-analysis.yml"), /serve@14/);
});

test("no workflow uses pull_request_target, and every setup-bun is pinned by SHA", () => {
  for (const dir of [".github/workflows", ".github/actions/setup-bun"]) {
    for (const f of readdirSync(path.join(root, dir)).filter((n) => /\.ya?ml$/.test(n))) {
      const text = read(`${dir}/${f}`);
      assert.doesNotMatch(text, /^\s*pull_request_target\s*:/m, `${dir}/${f}`);
      for (const m of text.matchAll(/uses:\s*oven-sh\/setup-bun@(\S+)/g)) assert.match(m[1], /^[0-9a-f]{40}$/, `${dir}/${f}`);
    }
  }
});

test("web-checks in build.yml still runs the storybook build and the dependency screen", () => {
  const build = read(".github/workflows/build.yml");
  assert.match(build, /^ {2}web-checks:/m);
  for (const step of ["bun run build-storybook", "bun run check:dependency-screen", "TestBuiltAssets"]) assert.ok(build.includes(step), step);
});

test("main-only jobs live in main-ci.yml, not build.yml, so PR runs list no skipped checks for them", () => {
  const build = read(".github/workflows/build.yml");
  const main = read(".github/workflows/main-ci.yml");
  for (const job of ["test-tmux", "test-rest", "benchmark-gate"]) {
    assert.doesNotMatch(build, new RegExp(`^ {2}${job}:`, "m"), `${job} should not be in build.yml`);
    assert.match(main, new RegExp(`^ {2}${job}:`, "m"), `${job} missing from main-ci.yml`);
  }
  assert.doesNotMatch(main, /^\s*pull_request\s*:/m);
  for (const m of main.matchAll(/\n( +)strategy:\n([\s\S]*?)(?=\n\1\S|\n {0,4}\S)/g)) {
    if (m[2].includes("matrix:")) assert.match(m[2], /fail-fast: false/);
  }
});

test("build.yml: every matrix sets fail-fast: false", () => {
  const yml = read(".github/workflows/build.yml");
  for (const m of yml.matchAll(/\n( +)strategy:\n([\s\S]*?)(?=\n\1\S|\n {0,4}\S)/g)) {
    if (m[2].includes("matrix:")) assert.match(m[2], /fail-fast: false/);
  }
});

test("build.yml detect-changes regex and push paths treat e2e-only edits as relevant", () => {
  const yml = read(".github/workflows/build.yml");
  const regex = yml.match(/grep -qE '([^']+)'; then/)?.[1];
  assert.ok(regex, "detect-changes grep regex not found");
  for (const p of ["^tests/e2e/", "^web-app/tests/e2e/"]) assert.ok(regex.includes(p), `detect-changes regex is missing ${p}`);
  const re = new RegExp(regex);
  assert.ok(re.test("tests/e2e/playwright.config.ts"));
  assert.ok(re.test("web-app/tests/e2e/terminal-stress/helpers.ts"));
  assert.ok(!re.test("tests/e2e-sharding-helper/foo.ts"));
  const pushPaths = yml.split("\n  workflow_dispatch:")[0];
  for (const p of ["tests/e2e/**", "web-app/tests/e2e/**"]) assert.ok(pushPaths.includes(`- '${p}'`), `push paths is missing ${p}`);
});

test("the merged lint job pins checkout to the PR head, not the stale default merge ref", () => {
  // actions/checkout defaults to GitHub's synthetic refs/pull/N/merge for pull_request
  // events, which tracks main's CURRENT tip independently of base.sha (captured once, at
  // the PR's last push). Once main advances past base.sha, each job's `git diff
  // base.sha...HEAD` silently widens to include every file from every PR merged to main
  // in between — surfaced 2026-10-22 as unrelated pre-existing warnings failing an
  // unrelated PR (#1144). Each job's checkout step must pin `ref:` to the actual head SHA.
  const lines = read(".github/workflows/lint.yml").split("\n");
  for (const job of ["lint"]) {
    const start = lines.findIndex((l) => l === `  ${job}:`);
    assert.ok(start !== -1, job);
    const rest = lines.slice(start + 1);
    const end = rest.findIndex((l) => /^ {2}[a-zA-Z_-]+:\s*$/.test(l));
    const body = rest.slice(0, end === -1 ? rest.length : end).join("\n");
    assert.match(body, /ref:\s*\$\{\{\s*github\.event\.pull_request\.head\.sha\s*\|\|\s*github\.sha\s*\}\}/, job);
  }
});

function existsSyncSafe(rel) {
  try {
    readFileSync(path.join(root, rel));
    return true;
  } catch {
    return false;
  }
}
