// Runs the stage pipeline against build/__tests__/fixtures/app under Bun. Usage: build.bun.ts <scenario>.
// The fixture is copied under web-app/.build/ so @vanilla-extract/css still resolves from web-app/node_modules.
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { optionsFromEnv, runBuild } from "../../build";

const webApp = process.cwd();
const fixture = path.join(webApp, "build/__tests__/fixtures/app");
const scenario = process.argv[2];
const work = path.join(webApp, ".build", `t-${process.pid}`);

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
}

function freshCopy(): string {
  rmSync(work, { recursive: true, force: true });
  mkdirSync(path.dirname(work), { recursive: true });
  cpSync(fixture, work, { recursive: true });
  return work;
}

function opts(root: string, extra: Record<string, unknown> = {}) {
  return {
    ...optionsFromEnv([], { NEXT_PUBLIC_RECONNECT_V2: "sentinel-reconnect", NEXT_PUBLIC_OTEL_ENABLED: "sentinel-otel" }),
    root,
    allowlistPath: path.join(root, "allowlist.json"),
    budgetsPath: path.join(root, "budgets.json"),
    ...extra,
  };
}

async function message(p: Promise<unknown>): Promise<string | null> {
  try {
    await p;
    return null;
  } catch (e) {
    return e instanceof Error ? e.message : String(e);
  }
}

const rel = (root: string, files: string[]) => files.map((f) => path.relative(root, f)).sort();

async function main(): Promise<unknown> {
  const root = freshCopy();
  if (scenario === "ok" || scenario === "ok-unminified") {
    const minify = scenario === "ok";
    const ctx = await runBuild(opts(root, { minify, minifyExplicit: !minify }));
    const out = path.join(root, "out");
    const files = walk(out);
    return {
      files: rel(out, files),
      html: readFileSync(path.join(out, "index.html"), "utf8"),
      js: files.filter((f) => f.endsWith(".js")).map((f) => readFileSync(f, "utf8")).join("\n"),
      css: files.filter((f) => f.endsWith(".css")).map((f) => readFileSync(f, "utf8")).join("\n"),
      stats: JSON.parse(readFileSync(path.join(root, ".build/stats.json"), "utf8")),
      statsInOut: existsSync(path.join(out, "stats.json")),
      leftoverTmp: readdirSync(root).filter((n) => n.startsWith(".out-tmp-")),
      stageNames: ctx.timings.map((t) => t.name),
    };
  }
  if (scenario === "fail-bundle" || scenario === "ban") {
    const first = await runBuild(opts(root));
    writeFileSync(path.join(first.root, "out/MARKER"), "previous");
    const bad = scenario === "ban" ? 'import "next/navigation";\n' : 'import "./boom.css";\n';
    if (scenario === "fail-bundle") writeFileSync(path.join(root, "src/boom.css"), ".x{color:red;;} @@@ {{{ ::: }");
    writeFileSync(path.join(root, "src/main.tsx"), bad + readFileSync(path.join(root, "src/main.tsx"), "utf8"));
    const err = await message(runBuild(opts(root)));
    return {
      err,
      markerSurvives: existsSync(path.join(root, "out/MARKER")),
      leftoverTmp: readdirSync(root).filter((n) => n.startsWith(".out-tmp-")),
    };
  }
  if (scenario === "ve-throws") {
    writeFileSync(path.join(root, "src/styles/bad.css.ts"), 'throw new Error("kaboom");\n');
    writeFileSync(path.join(root, "src/main.tsx"), 'import "./styles/bad.css";\n' + readFileSync(path.join(root, "src/main.tsx"), "utf8"));
    return { err: await message(runBuild(opts(root))) };
  }
  if (scenario === "concurrent") {
    const args = ["build/build.ts", "--root", root, "--out", path.join(root, "out"), "--allowlist", path.join(root, "allowlist.json"), "--budgets", path.join(root, "budgets.json")];
    const run = () => Bun.spawn(["bun", ...args], { cwd: webApp, stdout: "pipe", stderr: "pipe" });
    const [a, b] = [run(), run()];
    const codes = await Promise.all([a.exited, b.exited]);
    return { codes, hasIndex: existsSync(path.join(root, "out/index.html")), leftoverTmp: readdirSync(root).filter((n) => n.startsWith(".out-tmp-")) };
  }
  throw new Error(`unknown scenario ${scenario}`);
}

try {
  console.log(JSON.stringify(await main()));
} finally {
  rmSync(work, { recursive: true, force: true });
}
