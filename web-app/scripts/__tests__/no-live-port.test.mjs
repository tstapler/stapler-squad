// REQ-C1: build harness and spike scripts must never target the live instance's port.
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");
// Built from parts so this file does not trip its own scan.
const LIVE_PORT = ["85", "43"].join("");
const SCANNED = [
  "project_plans/web-app-bun-build/spikes",
  "web-app/build",
  "web-app/scripts",
  "scripts/dev-stack",
  "tests/e2e/helpers",
];
// Each entry documents a line that legitimately names the live port: { file, line, reason }.
const ALLOWLIST = [];

function* walk(dir) {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    if (e.name === "node_modules") continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) yield* walk(p);
    else yield p;
  }
}

export function findLivePortHits(files, allow = ALLOWLIST) {
  const hits = [];
  for (const { file, content } of files) {
    content.split("\n").forEach((text, i) => {
      if (text.includes(LIVE_PORT) && !allow.some((a) => a.file === file && a.line === i + 1)) {
        hits.push(`${file}:${i + 1}`);
      }
    });
  }
  return hits;
}

describe("live port scan", () => {
  it("finds no reference to the live port in harness and spike code", () => {
    const files = SCANNED.map((d) => path.join(repoRoot, d))
      .filter(existsSync)
      .flatMap((d) => [...walk(d)])
      .map((p) => ({ file: path.relative(repoRoot, p), content: readFileSync(p, "utf8") }));
    assert.deepEqual(findLivePortHits(files), []);
  });

  it("can fail: flags a hit and honours the allowlist", () => {
    const files = [{ file: "x.ts", content: `ok\nfetch("http://localhost:${LIVE_PORT}")` }];
    assert.deepEqual(findLivePortHits(files, []), ["x.ts:2"]);
    assert.deepEqual(findLivePortHits(files, [{ file: "x.ts", line: 2, reason: "documents the live port" }]), []);
  });
});
