// The plugin set Bun's dev server loads through bunfig.toml [serve.static]: same seam and vanilla-extract
// plugins as build.ts, so dev and prod share the Bun implementations of the seams and VE compile output.
import type { BunPlugin } from "bun";
import path from "node:path";
import { FONT_URL_PREFIX } from "../config";
import { walkFiles } from "../stats";
import { buildSeamPlugin } from "./build-seam";
import { VeCompiler } from "./vanilla-extract";

const root = path.resolve(import.meta.dir, "../..");

/** Shared with dev.ts, which reads `ve.fullReloadOnCssTsChange` and the cache size from this instance. */
export const ve = new VeCompiler({ cwd: root, identOption: "debug" });

const seam = buildSeamPlugin(path.join(root, "src"));
const veBun = ve.plugin();

/** Name to path for the vendored fonts; fonts.css points at the production URL, which Bun's dev bundler cannot resolve. */
export function fontIndex(fontsDir: string): Map<string, string> {
  return new Map(walkFiles(fontsDir).filter((f) => f.endsWith(".woff2")).map((f) => [path.basename(f), f]));
}

const fonts = fontIndex(path.join(root, "src/app/fonts"));

const plugin: BunPlugin = {
  name: "web-dev",
  setup(build) {
    seam.setup(build);
    veBun.setup(build);
    build.onResolve({ filter: /^\/_next\/static\/media\// }, (a) => {
      const file = fonts.get(a.path.slice(FONT_URL_PREFIX.length));
      if (!file) throw new Error(`${a.importer}: no vendored font for ${a.path}`);
      return { path: file };
    });
  },
};

export default plugin;
