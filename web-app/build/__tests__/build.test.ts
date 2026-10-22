import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { EXEMPT_PREFIXES, NAMING, PUBLIC_ENV_VARS, STAGE_NAMES } from "../config";
import { runStages, StageError } from "../stages/runner";
import { runInBun } from "./helpers/run-in-bun";

const WEB_APP = path.resolve(__dirname, "../..");
const REPO = path.resolve(WEB_APP, "..");

interface Built {
  files: string[];
  html: string;
  js: string;
  css: string;
  stats: { minify: boolean; minifyExplicit: boolean; totalDurationMs: number; stages: { name: string }[] };
  statsInOut: boolean;
  leftoverTmp: string[];
  stageNames: string[];
}
function scenario<T>(name: string): T {
  const r = runInBun("build.bun.ts", [name]);
  if (r.status !== 0) throw new Error(`scenario ${name} failed: ${r.stderr}`);
  return r.json as T;
}

describe("build.ts on the fixture app (minified default)", () => {
  const built = scenario<Built>("ok");

  it("inlines the five NEXT_PUBLIC defines (undefined when unset) and NODE_ENV=production, leaving no process.env", () => {
    expect(built.js).toContain("sentinel-reconnect");
    expect(built.js).toContain("sentinel-otel");
    expect(built.js).toContain('"production"');
    expect(built.js).not.toContain("process.env");
  });

  it("derives the define list from src/, so a sixth NEXT_PUBLIC var fails here", () => {
    const found = execFileSync("grep", ["-rhoE", "--include=*.ts", "--include=*.tsx", "--exclude=*.test.*", "--exclude-dir=__tests__", "NEXT_PUBLIC_[A-Z0-9_]+", "src"], { cwd: WEB_APP, encoding: "utf8" });
    expect([...new Set(found.split("\n").filter(Boolean))].sort()).toEqual([...PUBLIC_ENV_VARS].sort());
  });

  it("moves the HTML to out/index.html and leaves no other .html", () => {
    expect(built.files.filter((f) => f.endsWith(".html"))).toEqual(["index.html"]);
  });

  it("copies public assets to their exact root URLs and fonts under _next/static/media with a content hash", () => {
    expect(built.files).toEqual(expect.arrayContaining(["push-sw.js", "icons/i.png", "docs/a.md"]));
    expect(built.files.some((f) => /^_next\/static\/media\/inter-variable\.[0-9a-f]{8}\.woff2$/.test(f))).toBe(true);
    expect(built.css).not.toContain("data:font");
    expect(built.css).toMatch(/\/_next\/static\/media\/inter-variable\.[0-9a-f]{8}\.woff2/);
  });

  it("keeps stats outside out/ and leaves no .out-tmp-* behind", () => {
    expect(built.statsInOut).toBe(false);
    expect(built.leftoverTmp).toEqual([]);
  });

  it("is minified by default and records the stages in order, within the wall budget", () => {
    expect(built.stats.minify).toBe(true);
    expect(built.stageNames).toEqual([...STAGE_NAMES]);
    expect(built.stats.totalDurationMs).toBeLessThanOrEqual(10_000);
  });
});

describe("build.ts toggles and failures", () => {
  it("WEB_MINIFY=0 yields an unminified build recorded as explicit, with NODE_ENV still production", () => {
    const b = scenario<Built>("ok-unminified");
    expect(b.stats.minify).toBe(false);
    expect(b.stats.minifyExplicit).toBe(true);
    expect(b.js).toContain('"production"');
  });

  it("leaves the previous out/ intact and removes the temp dir when the bundle stage fails, naming stage and file", () => {
    const r = scenario<{ err: string; markerSurvives: boolean; leftoverTmp: string[] }>("fail-bundle");
    expect(r.err).toContain("[web-build:bundle]");
    expect(r.err).toContain("boom.css");
    expect(r.markerSurvives).toBe(true);
    expect(r.leftoverTmp).toEqual([]);
  });

  it("names the importer when a next import is refused", () => {
    const r = scenario<{ err: string }>("ban");
    expect(r.err).toContain("[web-build:bundle]");
    expect(r.err).toContain("main.tsx");
  });

  it("names the stage and the .css.ts path when vanilla-extract throws", () => {
    const r = scenario<{ err: string }>("ve-throws");
    expect(r.err).toContain("bad.css.ts");
    expect(r.err).toContain("kaboom");
  });

  it("lets two concurrent builds with different PIDs both succeed", () => {
    expect(scenario("concurrent")).toEqual({ codes: [0, 0], hasIndex: true, leftoverTmp: [] });
  });
});

describe("pipeline structure", () => {
  it("pins all three naming locations under _next/static/", () => {
    for (const k of ["entry", "chunk", "asset"] as const) expect(NAMING[k].startsWith("_next/static/")).toBe(true);
  });

  it("runs stages in the declared order, guard first", async () => {
    const order: string[] = [];
    const stages = STAGE_NAMES.map((name) => ({ name, run: () => void order.push(name) }));
    await runStages({}, stages, () => undefined);
    expect(order[0]).toBe("guard");
  });

  it("wraps a stage failure with the stage name", async () => {
    await expect(
      runStages({}, [{ name: "html", run: () => { throw Object.assign(new Error("nope"), { file: "x.html" }); } }], () => undefined),
    ).rejects.toThrow(/\[web-build:html\] x\.html: nope/);
    await expect(runStages({}, [{ name: "html", run: () => { throw new Error("n"); } }], () => undefined)).rejects.toBeInstanceOf(StageError);
  });

  it("mirrors server/middleware/auth.go exemptPrefixes", () => {
    const go = readFileSync(path.join(REPO, "server/middleware/auth.go"), "utf8");
    const block = /var exemptPrefixes = \[\]string\{([\s\S]*?)\n\}/.exec(go)?.[1] ?? "";
    const prefixes = [...block.matchAll(/"\/([^"]*)"/g)].map((m) => m[1]);
    expect(prefixes.sort()).toEqual([...EXEMPT_PREFIXES].sort());
  });
});
