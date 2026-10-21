// REQ-C3: Bun is the package manager and script runner, not the test runner
// (a full-suite `bun test` run segfaulted); tests run through Jest or node:test.
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");
// Built from parts so this file does not trip its own scan.
const BANNED = new RegExp(`\\b${["bun", "test"].join(" ")}(\\s|$|"|')`);

function* walk(dir) {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    if (e.name === "node_modules") continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) yield* walk(p);
    else yield p;
  }
}

export function findBunTestUses(files) {
  const hits = [];
  for (const { file, content } of files) {
    content.split("\n").forEach((text, i) => BANNED.test(text) && hits.push(`${file}:${i + 1}`));
  }
  return hits;
}

function realFiles() {
  const rel = ["web-app/package.json", "Makefile"];
  const workflows = path.join(repoRoot, ".github/workflows");
  const paths = [
    ...rel.map((r) => path.join(repoRoot, r)),
    ...(existsSync(workflows) ? [...walk(workflows)].filter((p) => p.endsWith(".yml")) : []),
    ...walk(path.join(repoRoot, "web-app/scripts")),
  ];
  return paths.map((p) => ({ file: path.relative(repoRoot, p), content: readFileSync(p, "utf8") }));
}

describe("no `bun test` scan", () => {
  it("finds no use of Bun's test runner", () => {
    assert.deepEqual(findBunTestUses(realFiles()), []);
  });

  it("can fail: flags a use", () => {
    const content = `run: cd web-app && ${["bun", "test"].join(" ")}\nrun: bun run test:codemods`;
    assert.deepEqual(findBunTestUses([{ file: "ci.yml", content }]), ["ci.yml:1"]);
  });
});
