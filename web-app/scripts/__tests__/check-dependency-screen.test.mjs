import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { parseBunLock, screen } from "../check-dependency-screen.mjs";

const script = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../check-dependency-screen.mjs");

const lockText = (packages) =>
  `{\n  "lockfileVersion": 2,\n  "packages": {\n${packages.map(([key, spec, meta]) => `    "${key}": ["${spec}", "", ${JSON.stringify(meta)}, "sha512-x"],`).join("\n")}\n  },\n}\n`;

function run(packages) {
  const dir = mkdtempSync(path.join(os.tmpdir(), "dep-screen-"));
  const lock = path.join(dir, "bun.lock");
  writeFileSync(lock, lockText(packages));
  return spawnSync("node", [script, "--lock", lock], { encoding: "utf8" });
}

const CLEAN = [
  ["react-router", "react-router@7.18.4", { dependencies: { cookie: "^1.0.1" } }],
  ["cookie", "cookie@1.1.1", {}],
];

describe("check-dependency-screen", () => {
  it("parses JSONC with trailing commas, including commas inside strings", () => {
    const lock = parseBunLock('{ "a": ["x,]", 1,], }');
    assert.deepEqual(lock.a, ["x,]", 1]);
  });

  it("passes the committed lockfile and prints the all-clear line", () => {
    const r = spawnSync("node", [script], { encoding: "utf8" });
    assert.equal(r.status, 0, r.stderr);
    assert.match(r.stdout, /react-router@7\.18\.4/);
    assert.match(r.stdout, /no github: sources, no optionalDependencies, no install scripts/);
  });

  it("passes a clean fixture", () => {
    assert.equal(run(CLEAN).status, 0);
  });

  it("exits 1 naming the banned TanStack version and the incident date", () => {
    const r = run([...CLEAN, ["@tanstack/react-router", "@tanstack/react-router@1.169.5", {}]]);
    assert.equal(r.status, 1);
    assert.match(r.stderr, /@tanstack\/react-router@1\.169\.5/);
    assert.match(r.stderr, /2026-05-11/);
  });

  it("flags a github source and optionalDependencies in the closure", () => {
    const r = run([
      ["react-router", "react-router@7.18.4", { dependencies: { cookie: "^1.0.1" }, optionalDependencies: { "@tanstack/setup": "github:tanstack/router#79ac49" } }],
      ["cookie", "cookie@github:evil/cookie#abc", {}],
    ]);
    assert.equal(r.status, 1);
    assert.match(r.stderr, /optionalDependencies/);
    assert.match(r.stderr, /git\/github\/http source/);
  });

  it("flags an install script from a manifest, and ignores prepare", () => {
    const lock = parseBunLock(lockText(CLEAN));
    const withScripts = (scripts) => screen(lock, { readManifest: (name) => (name === "cookie" ? { scripts } : null) });
    assert.match(withScripts({ postinstall: "node x.js" }).problems.join("\n"), /cookie@1\.1\.1 has a postinstall script/);
    assert.deepEqual(withScripts({ prepare: "tsc" }).problems, []);
  });

  it("fails when the watched root is absent", () => {
    assert.match(run([["cookie", "cookie@1.1.1", {}]]).stderr, /react-router is not in the lockfile/);
  });
});
