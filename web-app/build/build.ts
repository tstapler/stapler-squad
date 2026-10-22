// Bun build pipeline: `bun build/build.ts` writes a verified web-app/out/ (WEB_BUILD=bun in the Makefile).
// Env: WEB_MINIFY=0 turns minification off (default on). NODE_ENV is never read; builds are always "production".
// Flags: --root <dir> --out <dir> --stats <file> --allowlist <file> --budgets <file> --no-enforce-bytes
import { rmSync } from "node:fs";
import path from "node:path";
import { VeCompiler } from "./plugins/vanilla-extract";
import { bundle } from "./stages/bundle";
import { clean } from "./stages/clean";
import type { BuildContext, BuildOptions } from "./stages/context";
import { guard } from "./stages/guard";
import { html } from "./stages/html";
import { publicFiles } from "./stages/public";
import { runStages, type Stage } from "./stages/runner";
import { swap } from "./stages/swap";
import { themeMap } from "./stages/theme-map";
import { verify } from "./stages/verify";

export const STAGES: Stage<BuildContext>[] = [guard, clean, themeMap, bundle, html, publicFiles, verify, swap];

export function createContext(opts: BuildOptions): BuildContext {
  const root = path.resolve(opts.root);
  const srcDir = path.join(root, "src");
  const outDir = path.resolve(opts.outDir ?? path.join(root, "out"));
  return {
    opts,
    root,
    srcDir,
    entryHtml: path.resolve(root, opts.entryHtml ?? "index.html"),
    themeFile: path.resolve(root, opts.themeFile ?? "src/styles/theme.css.ts"),
    publicDir: path.join(root, "public"),
    fontsDir: path.join(srcDir, "app/fonts"),
    outDir,
    // A sibling of outDir so the final rename never crosses a filesystem.
    tmpDir: path.join(path.dirname(outDir), `.out-tmp-${process.pid}`),
    statsPath: path.resolve(opts.statsPath ?? path.join(root, ".build/stats.json")),
    ve: new VeCompiler({ cwd: root, identOption: "short" }),
    timings: [],
    fontFiles: new Map(),
    emitted: [],
    warnings: [],
    startedAt: performance.now(),
  };
}

export async function runBuild(opts: BuildOptions): Promise<BuildContext> {
  const ctx = createContext(opts);
  try {
    await runStages(ctx, STAGES, (t) => ctx.timings.push(t));
  } finally {
    // After a successful swap the tmp dir no longer exists; after a failure it must not linger.
    rmSync(ctx.tmpDir, { recursive: true, force: true });
  }
  return ctx;
}

function flag(argv: string[], name: string): string | undefined {
  const i = argv.indexOf(name);
  return i === -1 ? undefined : argv[i + 1];
}

export function optionsFromEnv(argv: string[], env: Record<string, string | undefined>): BuildOptions {
  const minifyEnv = env.WEB_MINIFY;
  return {
    root: flag(argv, "--root") ?? process.cwd(),
    outDir: flag(argv, "--out"),
    statsPath: flag(argv, "--stats"),
    allowlistPath: flag(argv, "--allowlist"),
    budgetsPath: flag(argv, "--budgets"),
    minify: minifyEnv !== "0",
    minifyExplicit: minifyEnv !== undefined && minifyEnv !== "",
    env,
    // Byte budgets are defined for the minified build (ADR-004); an unminified build records sizes only.
    enforceBytes: minifyEnv !== "0" && !argv.includes("--no-enforce-bytes"),
  };
}

if (import.meta.main) {
  try {
    const ctx = await runBuild(optionsFromEnv(process.argv.slice(2), process.env));
    for (const w of ctx.warnings) console.warn(`warning: ${w}`);
    const wall = Math.round(performance.now() - ctx.startedAt);
    console.log(`web-build: ok (${ctx.opts.minify ? "minified" : "unminified"}, ${wall} ms) -> ${path.relative(process.cwd(), ctx.outDir) || "."}`);
  } catch (e) {
    console.error(e instanceof Error ? e.message : String(e));
    process.exit(1);
  }
}
