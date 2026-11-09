import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { gzipSync } from "node:zlib";

export interface BuildStats {
  mode: "bun";
  minify: boolean;
  /** True only when WEB_MINIFY was set explicitly; an unminified build without it is a bug (ADR-004). */
  minifyExplicit: boolean;
  bunVersion: string;
  stages: { name: string; durationMs: number }[];
  totalDurationMs: number;
  jsBytes: number;
  cssBytes: number;
  chunkCount: number;
  initialJsBytes: number;
  largestChunkBytes: number;
  /** Largest JS file outside the initial closure: the per-file cap for lazy chunks. */
  largestLazyChunkBytes: number;
  /** The five biggest JS files (path relative to the output dir), for diagnosing a regression. */
  largestFiles: { file: string; bytes: number }[];
  gzipJsBytes: number;
  peakRssKb?: number;
}

export type OutputStats = Pick<BuildStats, "jsBytes" | "cssBytes" | "chunkCount" | "initialJsBytes" | "largestChunkBytes" | "largestLazyChunkBytes" | "largestFiles" | "gzipJsBytes">;

export function walkFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    return e.isDirectory() ? walkFiles(p) : [p];
  });
}

const STATIC_IMPORT = /(?:\bfrom|\bimport)\s*["']([^"']+\.js)["']/g;
const HTML_JS_REF = /<(?:script[^>]*\ssrc|link[^>]*\shref)=["']([^"']+\.js)["']/g;

function resolveUrl(outDir: string, from: string, url: string): string {
  return url.startsWith("/") ? path.join(outDir, url) : path.resolve(path.dirname(from), url);
}

/** Entry scripts of index.html plus everything they statically import (dynamic import() is lazy and excluded). */
export function initialJsFiles(outDir: string): string[] {
  const html = path.join(outDir, "index.html");
  if (!existsSync(html)) return [];
  const seen = new Set<string>();
  const visit = (file: string): void => {
    if (seen.has(file) || !existsSync(file)) return;
    seen.add(file);
    for (const m of readFileSync(file, "utf8").matchAll(STATIC_IMPORT)) visit(resolveUrl(outDir, file, m[1]));
  };
  for (const m of readFileSync(html, "utf8").matchAll(HTML_JS_REF)) visit(resolveUrl(outDir, html, m[1]));
  return [...seen];
}

export function computeOutputStats(outDir: string): OutputStats {
  const files = walkFiles(path.join(outDir, "_next/static"));
  const js = files.filter((f) => f.endsWith(".js"));
  const size = (f: string): number => statSync(f).size;
  const initial = new Set(initialJsFiles(outDir));
  const bySize = js.map((f) => ({ file: path.relative(outDir, f).split(path.sep).join("/"), bytes: size(f), lazy: !initial.has(f) }));
  bySize.sort((a, b) => b.bytes - a.bytes);
  return {
    jsBytes: js.reduce((n, f) => n + size(f), 0),
    cssBytes: files.filter((f) => f.endsWith(".css")).reduce((n, f) => n + size(f), 0),
    chunkCount: js.length,
    initialJsBytes: [...initial].reduce((n, f) => n + size(f), 0),
    largestChunkBytes: bySize[0]?.bytes ?? 0,
    largestLazyChunkBytes: bySize.find((f) => f.lazy)?.bytes ?? 0,
    largestFiles: bySize.slice(0, 5).map(({ file, bytes }) => ({ file, bytes })),
    gzipJsBytes: js.reduce((n, f) => n + gzipSync(readFileSync(f)).length, 0),
  };
}

/** GitHub step-summary table; empty `stats` rows are skipped so a partial record still renders. */
export function statsToMarkdown(stats: BuildStats, title = "Bun web build"): string {
  const rows: [string, string | number][] = [
    ["minify", stats.minify ? "yes" : `no${stats.minifyExplicit ? " (WEB_MINIFY=0)" : ""}`],
    ["bun", stats.bunVersion],
    ["wall (ms)", stats.totalDurationMs],
    ["JS bytes", stats.jsBytes],
    ["Initial JS bytes", stats.initialJsBytes],
    ["Largest chunk bytes", stats.largestChunkBytes],
    ["Largest lazy chunk bytes", stats.largestLazyChunkBytes],
    ["gzip JS bytes", stats.gzipJsBytes],
    ["CSS bytes", stats.cssBytes],
    ["JS files", stats.chunkCount],
    ...stats.stages.map((s): [string, number] => [`stage ${s.name} (ms)`, s.durationMs]),
  ];
  if (stats.peakRssKb !== undefined) rows.push(["peak RSS (KB)", stats.peakRssKb]);
  return `### ${title}\n\n| metric | value |\n| --- | ---: |\n${rows.map(([k, v]) => `| ${k} | ${v} |`).join("\n")}\n`;
}
