// Run by `bun run test:codemods` (node --test). Runs the real WEB_BUILD seam block from the Makefile in a
// scratch directory with a fake `web-app/out` recipe, so the stamp logic is exercised without a build.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");
const makefile = readFileSync(path.join(repo, "Makefile"), "utf8");
const start = makefile.indexOf("# WEB_BUILD selects the bundler");
const end = makefile.indexOf("# end of the WEB_BUILD seam block");
assert.ok(start >= 0 && end > start, "seam block markers missing from Makefile");

const dir = mkdtempSync(path.join(os.tmpdir(), "web-build-mk-"));
after(() => rmSync(dir, { recursive: true, force: true }));
mkdirSync(path.join(dir, "web-app"), { recursive: true });
for (const f of ["next.config.ts", "index.html", "bunfig.toml"]) writeFileSync(path.join(dir, "web-app", f), "");
writeFileSync(
  path.join(dir, "Makefile"),
  `${makefile.slice(start, end)}
web-app/out: $(WEB_BUILD_INPUTS) $(WEB_BUILD_STAMP)
\t@echo built-$(WEB_BUILD) >> built.log
\t@touch web-app/out
`,
);

function make(...args) {
  return spawnSync("make", ["-s", ...args, "web-app/out"], { cwd: dir, encoding: "utf8" });
}
const builds = () => readFileSync(path.join(dir, "built.log"), "utf8").trim().split("\n");

test("rebuilds when WEB_BUILD flips, and is a no-op when nothing changed", () => {
  assert.equal(make("WEB_BUILD=next").status, 0);
  assert.deepEqual(builds(), ["built-next"]);
  assert.equal(make("WEB_BUILD=next").status, 0);
  assert.deepEqual(builds(), ["built-next"], "unchanged next must not rebuild");
  assert.equal(make("WEB_BUILD=bun").status, 0);
  assert.deepEqual(builds(), ["built-next", "built-bun"], "flip to bun must rebuild");
  make("WEB_BUILD=bun");
  assert.deepEqual(builds(), ["built-next", "built-bun"], "unchanged bun must not rebuild");
  make("WEB_BUILD=next");
  assert.deepEqual(builds(), ["built-next", "built-bun", "built-next"], "flip back must rebuild");
});

test("rejects an unknown WEB_BUILD value naming the allowed ones", () => {
  const r = make("WEB_BUILD=vite");
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /WEB_BUILD must be "next" or "bun"/);
});

test("Next mode depends on next.config.ts; Bun mode on web-app/build, not next.config.ts", () => {
  const next = spawnSync("make", ["-pn", "WEB_BUILD=next", "web-app/out"], { cwd: dir, encoding: "utf8" }).stdout;
  assert.match(next, /WEB_BUILD_INPUTS := web-app\/next\.config\.ts/);
  const bun = spawnSync("make", ["-pn", "WEB_BUILD=bun", "web-app/out"], { cwd: dir, encoding: "utf8" }).stdout;
  assert.match(bun, /WEB_BUILD_INPUTS := web-app\/index\.html web-app\/bunfig\.toml/);
  assert.doesNotMatch(bun, /WEB_BUILD_INPUTS :=.*next\.config\.ts/);
});

test("WEB_MINIFY defaults to 0 and NODE_ENV is not a Makefile variable", () => {
  assert.match(makefile, /^WEB_MINIFY \?= 0$/m);
  assert.doesNotMatch(makefile, /^NODE_ENV\b/m);
});
