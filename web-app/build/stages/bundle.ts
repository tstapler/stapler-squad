import path from "node:path";
import { NAMING, PUBLIC_ENV_VARS } from "../config";
import { buildSeamPlugin } from "../plugins/build-seam";
import type { BuildContext } from "./context";
import { copyHashedFonts, rewriteFontUrls } from "./fonts";
import type { Stage } from "./runner";

/** `process.env.X` replacements. NODE_ENV is "production" for every build.ts run; only dev.ts uses "development". */
export function defineMap(env: Record<string, string | undefined>): Record<string, string> {
  const define: Record<string, string> = {
    "process.env.NODE_ENV": '"production"',
    // A guarded third-party read (typeof process) that would otherwise keep a `process.env` token in the output.
    "process.env.CI": "undefined",
  };
  for (const name of PUBLIC_ENV_VARS) define[`process.env.${name}`] = env[name] === undefined ? "undefined" : JSON.stringify(env[name]);
  return define;
}

interface MetaOutput {
  imports?: { path: string; kind: string }[];
  inputs?: Record<string, unknown>;
}
type Metafile = { outputs: Record<string, MetaOutput> };

const LOGIN_PAGE = "src/app/login/page.tsx";

/** JS files reachable from the /login page chunk through static imports (what an unauthenticated visitor must be able to fetch). */
export function loginClosure(meta: Metafile | undefined, outDir: string): string[] | undefined {
  if (!meta) return undefined;
  const entry = Object.entries(meta.outputs).find(([, o]) => Object.keys(o.inputs ?? {}).some((i) => i.endsWith(LOGIN_PAGE)));
  if (!entry) return undefined;
  const seen = new Set<string>();
  const visit = (rel: string): void => {
    if (seen.has(rel) || !rel.endsWith(".js") || !meta.outputs[rel]) return;
    seen.add(rel);
    for (const imp of meta.outputs[rel].imports ?? []) if (imp.kind === "import-statement") visit(imp.path.replace(/^\/+/, ""));
  };
  visit(entry[0]);
  return [...seen].map((rel) => path.resolve(outDir, rel));
}

function bundleError(e: unknown): Error {
  const first = (e as { errors?: { message?: string; position?: { file?: string } | null }[] }).errors?.[0];
  if (!first) return e instanceof Error ? e : new Error(String(e));
  const err = new Error(first.message ?? String(e)) as Error & { file?: string };
  err.file = first.position?.file ?? undefined;
  return err;
}

export const bundle: Stage<BuildContext> = {
  name: "bundle",
  async run(ctx) {
    let result;
    try {
      result = await Bun.build({
        entrypoints: [ctx.entryHtml],
        outdir: ctx.tmpDir,
        target: "browser",
        splitting: true,
        minify: ctx.opts.minify,
        publicPath: "/",
        sourcemap: "linked",
        throw: true,
        metafile: true,
        define: defineMap(ctx.opts.env),
        naming: NAMING,
        // fonts.css points at /_next/static/media/*; keep those external so Bun does not inline them as data: URIs.
        external: ["/_next/*"],
        plugins: [buildSeamPlugin(ctx.srcDir), ctx.ve.plugin()],
      });
    } catch (e) {
      throw bundleError(e);
    }
    ctx.emitted = result.outputs.map((o) => ({ path: o.path, kind: o.kind }));
    ctx.loginClosure = loginClosure(result.metafile as Metafile | undefined, ctx.tmpDir);
    ctx.fontFiles = copyHashedFonts(ctx.fontsDir, ctx.tmpDir);
    rewriteFontUrls(ctx.tmpDir, ctx.fontFiles);
  },
};
