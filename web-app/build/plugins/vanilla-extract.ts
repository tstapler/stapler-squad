// The one vanilla-extract Bun plugin: build, theme-map and (later) dev share its compiler so class
// identifiers and the cache agree. Needs the patched `eval` (ADR-008); build.ts's guard stage checks that.
import type { BunPlugin } from "bun";
import { compile, cssFileFilter, getSourceFromVirtualCssFile, processVanillaFile, virtualCssFileFilter } from "@vanilla-extract/integration";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";

export type IdentOption = "short" | "debug";

interface CompileResult {
  source: string;
  watchFiles: string[];
}

export interface VeCompilerOptions {
  cwd: string;
  identOption?: IdentOption;
  /** LRU bound; the repo has ~337 .css.ts files, so 2000 only matters for a long-lived dev server. */
  maxEntries?: number;
  /** Test seam: spy on or replace the expensive step. */
  compileImpl?: (args: { filePath: string; cwd: string; identOption: IdentOption }) => Promise<CompileResult>;
}

interface CacheEntry {
  watchFiles: string[];
  hash: string;
  contents: string;
}

export class VeBuildError extends Error {
  constructor(
    readonly file: string,
    cause: unknown,
  ) {
    super(`vanilla-extract failed to compile ${file}: ${cause instanceof Error ? cause.message : String(cause)}`);
    this.name = "VeBuildError";
  }
}

function hashFiles(files: string[]): string {
  const h = createHash("sha256");
  for (const f of [...files].sort()) {
    h.update(f);
    try {
      h.update(readFileSync(f));
    } catch {
      h.update("<missing>");
    }
  }
  return h.digest("hex");
}

/**
 * Compiles `.css.ts` files to JS plus virtual CSS imports. The cache key is the content hash of the file
 * and its VE dependency closure (mtime is coarse after checkout and CI cache restore, so it is not the key).
 */
export class VeCompiler {
  compileCount = 0;
  /**
   * Set when a dependant of an edited token file may not be re-requested by the bundler (consumed by the dev
   * server, Story 5.6). Default false per gate S2 (project_plans/web-app-bun-build/research/spike-results.md):
   * measured, Bun's own `development.hmr` DOES re-request and live-swap a changed .css.ts's CSS output into
   * already-rendered dependants with no full page reload (verified <150ms, no `load` event). Flip to true only
   * if a future Bun version regresses on this (re-run the S2 `csstoken` probe first to confirm before flipping).
   */
  fullReloadOnCssTsChange = false;
  private readonly cache = new Map<string, CacheEntry>();
  private readonly identOption: IdentOption;
  private readonly maxEntries: number;
  private readonly compileImpl: NonNullable<VeCompilerOptions["compileImpl"]>;

  constructor(private readonly opts: VeCompilerOptions) {
    this.identOption = opts.identOption ?? "short";
    this.maxEntries = opts.maxEntries ?? 2000;
    this.compileImpl = opts.compileImpl ?? compile;
  }

  get cacheSize(): number {
    return this.cache.size;
  }

  async load(filePath: string): Promise<string> {
    const hit = this.cache.get(filePath);
    if (hit && hashFiles(hit.watchFiles) === hit.hash) {
      this.cache.delete(filePath);
      this.cache.set(filePath, hit);
      return hit.contents;
    }
    const contents = await this.compileFresh(filePath);
    return contents;
  }

  private async compileFresh(filePath: string): Promise<string> {
    try {
      this.compileCount++;
      const { source, watchFiles } = await this.compileImpl({ filePath, cwd: this.opts.cwd, identOption: this.identOption });
      const contents = await processVanillaFile({ source, filePath, identOption: this.identOption });
      const files = watchFiles.includes(filePath) ? watchFiles : [...watchFiles, filePath];
      this.cache.delete(filePath);
      this.cache.set(filePath, { watchFiles: files, hash: hashFiles(files), contents });
      if (this.cache.size > this.maxEntries) this.cache.delete(this.cache.keys().next().value as string);
      return contents;
    } catch (e) {
      throw e instanceof VeBuildError ? e : new VeBuildError(filePath, e);
    }
  }

  /** Exported `export var name = 'class'` bindings of a `.css.ts` file, as seen by the bundler. */
  async classExports(filePath: string): Promise<Record<string, string>> {
    const contents = await this.load(filePath);
    const out: Record<string, string> = {};
    for (const m of contents.matchAll(/^export var (\w+) = '([^']*)';$/gm)) out[m[1]] = m[2];
    return out;
  }

  plugin(): BunPlugin {
    return {
      name: "vanilla-extract",
      setup: (b) => {
        b.onResolve({ filter: virtualCssFileFilter }, (a) => ({ path: a.path, namespace: "ve-css" }));
        b.onLoad({ filter: /.*/, namespace: "ve-css" }, async ({ path: p }) => {
          const { source, fileName } = await getSourceFromVirtualCssFile(p);
          const abs = path.isAbsolute(fileName) ? fileName : path.join(this.opts.cwd, fileName);
          return { contents: source, loader: "css", resolveDir: path.dirname(abs) };
        });
        b.onLoad({ filter: cssFileFilter }, async ({ path: p }) => ({
          contents: await this.load(p),
          loader: "js",
          resolveDir: path.dirname(p),
        }));
      },
    };
  }
}
