// Gate S3, Task 0.4a/0.4d: BuildStats of the real Bun build (web-app/build/build.ts, PR 5a) — chunk graph, Initial JS
// closure, route chunks, forbidden modules, lazy() call sites — and the Next baseline (per-route files from the
// build manifests, size-limit style sizes).
// Run from web-app/:  bun ../project_plans/web-app-bun-build/spikes/s3/measure.ts <bunOut> <nextOut> <nextManifestDir> > size.json
// <bunOut> is `web-app/out` after `WEB_MINIFY=1 bun build/build.ts`; <nextOut> is `web-app/out` after `bun run build:next`,
// and <nextManifestDir> is that build's `.next/` (for app-build-manifest.json).
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { brotliCompressSync, gzipSync } from "node:zlib";

const [bunOut, nextOut, manifestDir] = process.argv.slice(2);
const cwd = process.cwd();
const walk = (d: string): string[] => readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(join(d, e.name)) : [join(d, e.name)]));
const read = (f: string) => readFileSync(f);
const raw = (f: string) => statSync(f).size;
const gz = (f: string) => gzipSync(read(f), { level: 9 }).length;
const br = (f: string) => brotliCompressSync(read(f)).length;
const total = (files: string[], fn: (f: string) => number) => files.reduce((n, f) => n + fn(f), 0);

// ---- routes: ROUTE_META order and routeTable load/redirect split, read from source
const metaSrc = readFileSync(join(cwd, "src/app/route-meta.ts"), "utf8");
const tableSrc = readFileSync(join(cwd, "src/app/routes.tsx"), "utf8");
const loadRoutes = [...tableSrc.matchAll(/^\s+"(\/[^"]*)": \{ load:/gm)].map((m) => m[1]);
const redirectRoutes = [...tableSrc.matchAll(/^\s+"(\/[^"]*)": \{ redirect:/gm)].map((m) => m[1]);
const metaOrder = [...metaSrc.matchAll(/^\s+(?:path:\s*)?"(\/[^"]*)"/gm)].map((m) => m[1]);

// ---- Bun side
const chunkDir = join(bunOut, "_next/static/chunks");
// Scoped to _next/static/ (matches build/stats.ts's computeOutputStats): the real pipeline also emits public/
// passthrough files (push-sw.js, docs/) that are not bundler output and have no .map sibling.
const bunStaticFiles = walk(join(bunOut, "_next/static"));
const bunJs = bunStaticFiles.filter((f) => f.endsWith(".js"));
const bunCss = bunStaticFiles.filter((f) => f.endsWith(".css"));
const urlToFile = (u: string) => join(bunOut, u.replace(/^\//, ""));
const html = readFileSync(join(bunOut, "index.html"), "utf8");
const entryUrl = html.match(/<script[^>]*src="([^"]+\.js)"/)![1];
const htmlCss = [...html.matchAll(/<link[^>]*rel="stylesheet"[^>]*href="([^"]+\.css)"/g)].map((m) => m[1]);

const staticImports = new Map<string, string[]>();
const dynamicImports = new Map<string, string[]>();
for (const f of bunJs) {
  const src = readFileSync(f, "utf8");
  const key = "/" + relative(bunOut, f);
  staticImports.set(key, [...new Set([...src.matchAll(/(?:from|import)\s*"(\/_next\/static\/chunks\/[^"]+\.js)"/g)].map((m) => m[1]))]);
  dynamicImports.set(key, [...new Set([...src.matchAll(/import\("(\/_next\/static\/chunks\/[^"]+\.js)"\)/g)].map((m) => m[1]))]);
}
const closure = (roots: string[]) => {
  const seen = new Set<string>();
  const stack = [...roots];
  while (stack.length) {
    const u = stack.pop()!;
    if (seen.has(u)) continue;
    seen.add(u);
    stack.push(...(staticImports.get(u) ?? []));
  }
  return seen;
};
const initial = closure([entryUrl]);
const sizeOf = (urls: Iterable<string>, fn: (f: string) => number) => total([...urls].map(urlToFile), fn);

const sources = new Map<string, string[]>();
for (const u of staticImports.keys()) sources.set(u, JSON.parse(readFileSync(urlToFile(u) + ".map", "utf8")).sources as string[]);
const chunkOfModule = (re: RegExp) => [...sources].filter(([, s]) => s.some((x) => re.test(x))).map(([u]) => u);

const pageRe = (route: string) => new RegExp(`/src/app${route === "/" ? "" : route}/page\\.tsx$`);
const routes = loadRoutes.map((route) => {
  const chunks = chunkOfModule(pageRe(route));
  const own = chunks[0];
  const cl = own ? closure([own]) : new Set<string>();
  const unique = [...cl].filter((u) => !initial.has(u));
  const cssOfChunks = [...cl].map((u) => u.replace(/\.js$/, ".css")).filter((u) => bunCss.includes(urlToFile(u)));
  return {
    route,
    routeChunk: own ?? null,
    chunkCandidates: chunks.length,
    uniqueChunks: unique.length,
    uniqueJsRaw: sizeOf(unique, raw),
    uniqueJsGzip: sizeOf(unique, gz),
    firstLoadJsRaw: sizeOf(initial, raw) + sizeOf(unique, raw),
    firstLoadJsGzip: sizeOf(initial, gz) + sizeOf(unique, gz),
    emittedCssForClosure: cssOfChunks.length,
  };
});

const forbidden: Record<string, RegExp> = {
  "@xterm": /\/@xterm\//,
  "monaco-editor": /monaco-editor/,
  codemirror: /\/(@codemirror|codemirror)\//,
  recharts: /\/recharts\//,
  "react-markdown": /\/react-markdown\//,
};
const forbiddenInInitial = Object.fromEntries(
  Object.entries(forbidden).map(([name, re]) => [name, [...initial].filter((u) => (sources.get(u) ?? []).some((s) => re.test(s))).length]),
);
const forbiddenElsewhere = Object.fromEntries(
  Object.entries(forbidden).map(([name, re]) => [name, [...staticImports.keys()].filter((u) => !initial.has(u) && (sources.get(u) ?? []).some((s) => re.test(s))).length]),
);

// lazy-component call sites (the plan's "6 dynamic() call sites"): where each target module lands
const srcFiles = walk(join(cwd, "src")).filter((f) => /\.(ts|tsx)$/.test(f) && !/__tests__|\.test\./.test(f));
const lazySites: { file: string; target: string; chunk: string | null; inInitial: boolean | null }[] = [];
for (const f of srcFiles) {
  const t = readFileSync(f, "utf8");
  for (const m of t.matchAll(/(?:lazyComponent|dynamic)\(\s*\(\)\s*=>\s*import\(\s*["']([^"']+)["']/g)) {
    const spec = m[1];
    const base = spec.replace(/^@\//, "src/").replace(/^\.\//, relative(cwd, join(f, "..")) + "/");
    const re = new RegExp(`/${base.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(/index)?\\.tsx?$`.replace(/^\//, ""));
    const hit = chunkOfModule(new RegExp(base.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "(/index)?\\.tsx?$"));
    void re;
    lazySites.push({ file: relative(cwd, f), target: spec, chunk: hit[0] ?? null, inInitial: hit[0] ? initial.has(hit[0]) : null });
  }
}

// ---- Next side
const nextFiles = walk(nextOut).filter((f) => f.includes("/_next/static/"));
const nextJs = nextFiles.filter((f) => f.endsWith(".js"));
const nextChunksJs = nextJs.filter((f) => f.includes("/_next/static/chunks/"));
const nextCss = nextFiles.filter((f) => f.endsWith(".css"));
const appManifest = JSON.parse(readFileSync(join(manifestDir, "app-build-manifest.json"), "utf8")).pages as Record<string, string[]>;
const nextFile = (p: string) => join(nextOut, "_next", p);
const nextRoute = (route: string) => {
  const pageKey = (route === "/" ? "" : route) + "/page";
  const page = (appManifest[pageKey] ?? []).filter((f) => f.endsWith(".js"));
  const layoutChain = ["/layout", ...route.split("/").filter(Boolean).map((_, i, a) => "/" + a.slice(0, i + 1).join("/") + "/layout")];
  const withLayouts = [...new Set([...layoutChain.flatMap((k) => appManifest[k] ?? []), ...page].filter((f) => f.endsWith(".js")))];
  return {
    route,
    manifestPageFiles: page.length,
    pageOnlyRaw: total(page.map(nextFile), raw),
    pageOnlyGzip: total(page.map(nextFile), gz),
    withLayoutsRaw: total(withLayouts.map(nextFile), raw),
    withLayoutsGzip: total(withLayouts.map(nextFile), gz),
  };
};
const allRoutes = [...loadRoutes, ...redirectRoutes];
const nextRoutes = allRoutes.map(nextRoute);

console.log(
  JSON.stringify(
    {
      routes: { loadRoutes: loadRoutes.length, redirectRoutes, metaOrderCount: metaOrder.length, metaOrder },
      bun: {
        jsFiles: bunJs.length,
        totalJsRaw: total(bunJs, raw),
        totalJsGzip: total(bunJs, gz),
        totalJsBrotli: total(bunJs, br),
        cssFilesEmitted: bunCss.length,
        totalCssEmittedRaw: total(bunCss, raw),
        cssLinkedFromHtml: htmlCss,
        cssLinkedRaw: sizeOf(htmlCss, raw),
        entry: entryUrl,
        initialChunks: initial.size,
        initialJsRaw: sizeOf(initial, raw),
        initialJsGzip: sizeOf(initial, gz),
        initialJsBrotli: sizeOf(initial, br),
        distinctRouteChunks: new Set(routes.map((r) => r.routeChunk).filter(Boolean)).size,
        routesWithoutChunk: routes.filter((r) => !r.routeChunk).map((r) => r.route),
        forbiddenModulesInInitialChunks: forbiddenInInitial,
        forbiddenModulesInNonInitialChunks: forbiddenElsewhere,
        lazySites,
        routes,
      },
      next: {
        jsFilesUnderStatic: nextJs.length,
        chunksJsFiles: nextChunksJs.length,
        totalJsRawChunksDir: total(nextChunksJs, raw),
        totalJsRawAllStatic: total(nextJs, raw),
        totalJsGzipChunksDir: total(nextChunksJs, gz),
        totalJsBrotliChunksDir: total(nextChunksJs, br),
        cssFiles: nextCss.length,
        cssRaw: total(nextCss, raw),
        routes: nextRoutes,
      },
    },
    null,
    1,
  ),
);
