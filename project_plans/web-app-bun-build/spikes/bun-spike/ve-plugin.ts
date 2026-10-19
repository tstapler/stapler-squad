import type { BunPlugin } from "bun";
import { compile, processVanillaFile, getSourceFromVirtualCssFile, virtualCssFileFilter, cssFileFilter } from "@vanilla-extract/integration";
import { dirname } from "node:path";

// Strip invalid nested `@keyframes` (fixed at source by PR #1097; kept as a guard for the spike).
export const vanillaExtract = (opts: { dev?: boolean } = {}): BunPlugin => ({
  name: "vanilla-extract-bun",
  setup(b) {
    const cwd = process.cwd();
    b.onResolve({ filter: virtualCssFileFilter }, (a) => ({ path: a.path, namespace: "ve-css" }));
    b.onLoad({ filter: /.*/, namespace: "ve-css" }, async ({ path }) => {
      const { source, fileName } = await getSourceFromVirtualCssFile(path);
      return { contents: source, loader: "css", resolveDir: dirname(fileName.startsWith("/") ? fileName : cwd + "/" + fileName) };
    });
    b.onLoad({ filter: cssFileFilter }, async ({ path }) => {
      const identOption = opts.dev ? "debug" : "short";
      const r = await compile({ filePath: path, cwd, identOption });
      const contents = await processVanillaFile({ source: r.source, filePath: path, identOption });
      return { contents, loader: "js", resolveDir: dirname(path) };
    });
  },
});
