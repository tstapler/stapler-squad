// Run by `bun run test:codemods` (node --test). actionlint covers syntax in `make lint-workflows`; this covers intent.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
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

function existsSyncSafe(rel) {
  try {
    readFileSync(path.join(root, rel));
    return true;
  } catch {
    return false;
  }
}
