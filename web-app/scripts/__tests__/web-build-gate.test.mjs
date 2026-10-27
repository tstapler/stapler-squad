// Run by `bun run test:codemods` (node --test). Drives the real script as a child process.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { DEFAULT_MEMBERS } from "../ci/web-build-gate.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const SCRIPT = path.join(here, "../ci/web-build-gate.mjs");
const WORKFLOW = path.join(here, "../../../.github/workflows/build.yml");

const needs = (r) => Object.fromEntries(Object.entries(r).map(([k, v]) => [k, { result: v }]));
const gate = (r, members) => spawnSync("node", [SCRIPT], { env: { ...process.env, NEEDS_JSON: JSON.stringify(needs(r)), MEMBERS: members ?? "", GITHUB_STEP_SUMMARY: "" }, encoding: "utf8" });

const green = { "detect-changes": "success", "web-build-smoke-next": "success", "web-build-smoke-bun": "success" };

test("fails for a member Next job failure or cancellation", () => {
  assert.equal(gate({ ...green, "web-build-smoke-next": "failure" }).status, 1);
  assert.equal(gate({ ...green, "web-build-smoke-next": "cancelled" }).status, 1);
});

test("fails for a member Bun job failure when the variable includes it", () => {
  const r = gate({ ...green, "web-build-smoke-bun": "failure" }, "detect-changes,web-build-smoke-next,web-build-smoke-bun");
  assert.equal(r.status, 1);
  assert.match(r.stdout, /web-build-smoke-bun: failure/);
});

test("fails when detect-changes fails, so a broken relevance check cannot turn the gate green", () => {
  assert.equal(gate({ ...green, "detect-changes": "failure" }).status, 1);
});

test("fails when a listed member is not in needs", () => {
  assert.equal(gate(green, "detect-changes,web-build-smoke-nxt").status, 1);
});

test("passes a non-member Bun job failure and prints it", () => {
  const r = gate({ ...green, "web-build-smoke-bun": "failure" });
  assert.equal(r.status, 0);
  assert.match(r.stdout, /web-build-smoke-bun \| failure \| not a member/);
});

test("passes skipped members and an all-skipped (irrelevant) PR", () => {
  assert.equal(gate({ ...green, "web-build-smoke-next": "skipped" }).status, 0);
  assert.equal(gate({ "detect-changes": "skipped", "web-build-smoke-next": "skipped", "web-build-smoke-bun": "skipped" }).status, 0);
});

test("ignores default members that do not exist yet, but only for the default list", () => {
  assert.equal(gate(green).status, 0);
  assert.equal(gate(green, "detect-changes,e2e-next").status, 1);
});

test("fails when e2e-next is cancelled (a DEFAULT_MEMBERS member once it exists in needs)", () => {
  assert.equal(gate({ ...green, "e2e-next": "cancelled" }).status, 1);
});

test("passes when e2e-bun fails as a non-member (not in DEFAULT_MEMBERS until the owner flip, plan decision 10)", () => {
  const r = gate({ ...green, "e2e-next": "success", "e2e-bun": "failure" });
  assert.equal(r.status, 0);
  assert.match(r.stdout, /e2e-bun \| failure \| not a member/);
});

test("build.yml: web-build-gate has if: always() and no continue-on-error, and every matrix sets fail-fast: false", () => {
  const yml = readFileSync(WORKFLOW, "utf8");
  const job = /\n  web-build-gate:\n([\s\S]*?)(?=\n  [a-z][\w-]*:\n|$)/.exec(yml)?.[1] ?? "";
  assert.ok(job, "web-build-gate job missing");
  assert.match(job, /\n    if: always\(\)/);
  assert.doesNotMatch(job, /continue-on-error/);
  assert.doesNotMatch(job.split("steps:")[0], /web-build-smoke-bun[^\n]*\n[^\n]*MEMBERS/);
  for (const m of yml.matchAll(/\n( +)strategy:\n([\s\S]*?)(?=\n\1\S|\n {0,4}\S)/g)) {
    if (m[2].includes("matrix:")) assert.match(m[2], /fail-fast: false/);
  }
});

// Story 3.2.1 ratchet: the secondary-browser job pair (Firefox/WebKit, informational-only)
// must never enter gate membership, the exact mistake PR #1165 made with a one-time manual
// grep instead of a test.
test("secondary-browser jobs never enter web-build-gate membership", () => {
  assert.ok(!DEFAULT_MEMBERS.includes("e2e-secondary-browsers-next"));
  assert.ok(!DEFAULT_MEMBERS.includes("e2e-secondary-browsers-bun"));

  const yml = readFileSync(WORKFLOW, "utf8");
  const job = /\n  web-build-gate:\n([\s\S]*?)(?=\n  [a-z][\w-]*:\n|$)/.exec(yml)?.[1] ?? "";
  assert.ok(job, "web-build-gate job missing");
  const needsLine = /\n {4}needs: (\[[^\]]*\])/.exec(job)?.[1] ?? "";
  assert.ok(needsLine, "web-build-gate needs: line missing");
  assert.doesNotMatch(needsLine, /e2e-secondary-browsers-next/);
  assert.doesNotMatch(needsLine, /e2e-secondary-browsers-bun/);
});
