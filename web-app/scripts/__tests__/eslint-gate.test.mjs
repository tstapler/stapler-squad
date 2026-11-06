// scripts/eslint-gate.sh must fail on any warning and pass on clean files. Fixtures are written
// under src/ (the only tree eslint.config.mjs covers) and removed afterwards.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { after, describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const webApp = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const dir = mkdtempSync(path.join(webApp, "src", ".eslint-gate-"));
const rel = (name) => path.relative(webApp, path.join(dir, name));

writeFileSync(
  path.join(dir, "warn.tsx"),
  'import { useEffect } from "react";\nexport function W({ a }: { a: number }) { useEffect(() => { console.log(a); }, []); return null; }\n',
);
writeFileSync(path.join(dir, "clean.ts"), "export const clean = 1;\n");

const gate = (...files) => spawnSync("bash", ["scripts/eslint-gate.sh", ...files], { cwd: webApp, encoding: "utf8" });

after(() => rmSync(dir, { recursive: true, force: true }));

describe("eslint-gate.sh", () => {
  it("fails on a file with one exhaustive-deps warning", () => {
    const r = gate(rel("warn.tsx"));
    assert.equal(r.status, 1, r.stdout + r.stderr);
    assert.match(r.stdout, /react-hooks\/exhaustive-deps/);
  });

  it("passes on a clean file", () => {
    const r = gate(rel("clean.ts"));
    assert.equal(r.status, 0, r.stdout + r.stderr);
  });

  it("fails when one of several files warns", () => {
    assert.equal(gate(rel("clean.ts"), rel("warn.tsx")).status, 1);
  });

  it("does not count a file outside the config scope as a warning", () => {
    assert.equal(gate("scripts/__tests__/eslint-gate.test.mjs").status, 0);
  });

  it("exits 2 with usage when given no files", () => {
    assert.equal(gate().status, 2);
  });
});
