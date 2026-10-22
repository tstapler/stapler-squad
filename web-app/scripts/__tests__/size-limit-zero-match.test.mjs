// Run by `bun run test:codemods` (node --test). A budget whose glob matches no file must fail, not pass vacuously (ADR-005).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";

const webApp = path.join(path.dirname(fileURLToPath(import.meta.url)), "../..");
const SIZE_LIMIT = path.join(webApp, "node_modules/.bin/size-limit");
const config = JSON.parse(readFileSync(path.join(webApp, ".size-limit.json"), "utf8"));
const budgets = JSON.parse(readFileSync(path.join(webApp, "build/budgets.json"), "utf8"));

// Inside web-app so the plugin resolves from web-app/node_modules; size-limit also requires a package.json in cwd.
const run = (entries, files) => {
  const dir = mkdtempSync(path.join(webApp, ".out-tmp-size-limit-"));
  after(() => rmSync(dir, { recursive: true, force: true }));
  writeFileSync(path.join(dir, "package.json"), JSON.stringify({ devDependencies: { "@size-limit/file": "*" } }));
  mkdirSync(path.join(dir, "out/_next/static/chunks"), { recursive: true });
  for (const [name, bytes] of Object.entries(files)) writeFileSync(path.join(dir, name), "x".repeat(bytes));
  writeFileSync(path.join(dir, ".size-limit.json"), JSON.stringify(entries));
  return spawnSync(SIZE_LIMIT, [], { cwd: dir, encoding: "utf8" });
};

test("the shipped config has exactly one entry whose limit equals budgets.json totalJsBytes", () => {
  assert.equal(config.length, 1);
  assert.equal(config[0].limit, `${budgets.totalJsBytes.limit} B`);
  assert.equal(config[0].brotli, false, "without brotli:false @size-limit/file measures brotli size, not raw bytes");
});

test("web-app/package.json carries no second size-limit config", () => {
  assert.equal("size-limit" in JSON.parse(readFileSync(path.join(webApp, "package.json"), "utf8")), false);
});

test("a glob that matches zero files fails", () => {
  const r = run([{ ...config[0], path: "out/_next/static/nope-*/**/*.js" }], {});
  assert.notEqual(r.status, 0, r.stdout + r.stderr);
});

test("the shipped glob passes under the limit and fails over it", () => {
  const file = "out/_next/static/chunks/a.js";
  assert.equal(run(config, { [file]: 1000 }).status, 0);
  assert.notEqual(run([{ ...config[0], limit: "500 B" }], { [file]: 1000 }).status, 0);
});
