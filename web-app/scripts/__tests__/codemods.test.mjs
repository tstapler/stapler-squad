// Codemod liveness and tooling checks. Plain `node --test` (the `test:codemods` script) so CI needs neither Jest nor rg.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { cpSync, mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { run } from "../codemods/next-nav-to-facade-tests.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const webApp = path.resolve(here, "../..");
const codemods = path.join(webApp, "scripts/codemods");
const fixtures = path.join(codemods, "__fixtures__/next-nav-tests");
const scratch = mkdtempSync(path.join(tmpdir(), "codemod-"));
after(() => rmSync(scratch, { recursive: true, force: true }));

const filesIn = (dir) => readdirSync(dir).sort();

describe("next-nav-to-facade-tests", () => {
  const work = path.join(scratch, "tree");
  cpSync(path.join(fixtures, "in"), work, { recursive: true });
  const firstRun = run([work]);
  const secondRun = run([work]);

  it("is live: it changes the fixtures that mock next/navigation or next/link", () => {
    assert.deepEqual(firstRun.map((f) => path.basename(f)), ["both.test.tsx", "link-only.test.tsx", "nav-only.test.ts"]);
  });

  it("produces the expected output, including one merged mock for a file with both", () => {
    for (const name of filesIn(path.join(fixtures, "expected"))) {
      assert.equal(readFileSync(path.join(work, name), "utf8"), readFileSync(path.join(fixtures, "expected", name), "utf8"), name);
    }
  });

  it("is idempotent: a second run changes nothing", () => {
    assert.deepEqual(secondRun, []);
  });

  it("leaves next/dynamic mocks alone", () => {
    const name = "untouched-dynamic.test.tsx";
    assert.equal(readFileSync(path.join(work, name), "utf8"), readFileSync(path.join(fixtures, "in", name), "utf8"));
  });
});

describe("ast-grep provisioning", () => {
  it("the installed binary matches the pinned @ast-grep/cli", () => {
    const pinned = JSON.parse(readFileSync(path.join(webApp, "package.json"), "utf8")).devDependencies["@ast-grep/cli"];
    const version = spawnSync(path.join(webApp, "node_modules/.bin/ast-grep"), ["--version"], { encoding: "utf8" });
    assert.equal(version.stdout.trim(), `ast-grep ${pinned}`);
  });

  it("no codemod script invokes sg (shadow-utils on Ubuntu)", () => {
    const invokesSg = /spawn(Sync)?\(\s*["'`]sg["'`]|exec(Sync)?\(\s*["'`]sg\s|^\s*sg\s/m;
    for (const name of readdirSync(codemods).filter((f) => /\.(mjs|js|sh|yml|yaml)$/.test(f))) {
      assert.ok(!invokesSg.test(readFileSync(path.join(codemods, name), "utf8")), `${name} invokes sg`);
    }
  });
});
