import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const script = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../verify-eval-patch.mjs");

function evalDir(version, evalJs) {
  const dir = mkdtempSync(path.join(os.tmpdir(), "verify-eval-"));
  writeFileSync(path.join(dir, "package.json"), JSON.stringify({ name: "eval", version }));
  writeFileSync(path.join(dir, "eval.js"), evalJs);
  return dir;
}

function run(args = []) {
  return spawnSync("bun", [script, ...args], { encoding: "utf8" });
}

describe("verify-eval-patch", () => {
  it("passes on the installed (patched) package", () => {
    const r = run();
    assert.equal(r.status, 0, r.stderr);
  });

  it("exits 1 naming the patch and the Bun issue when the marker is missing", () => {
    const r = run(["--eval-dir", evalDir("0.1.8", "module.exports = function () {}")]);
    assert.equal(r.status, 1);
    assert.match(r.stderr, /patches\/eval@0\.1\.8\.patch/);
    assert.match(r.stderr, /33520/);
  });

  it("exits 1 when eval moved to another version", () => {
    const r = run(["--eval-dir", evalDir("0.2.0", "// ssq-patch: eval-bun-createRequire")]);
    assert.equal(r.status, 1);
    assert.match(r.stderr, /patches\/eval@0\.1\.8\.patch/);
  });
});
