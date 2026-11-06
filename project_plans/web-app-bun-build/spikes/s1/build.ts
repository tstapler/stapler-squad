// Gate S1 spike (throwaway). Run from web-app/:  bun ../project_plans/web-app-bun-build/spikes/s1/build.ts <outdir> [--no-minify] [--probe=next-nav|bad-css]
// Layout A': Bun emits the HTML entry under the entry naming prefix; this script moves it to <outdir>/index.html.
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync, renameSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { createRequire } from "node:module";

const cwd = process.cwd();
const req = createRequire(join(cwd, "package.json"));
const { compile, processVanillaFile, getSourceFromVirtualCssFile, virtualCssFileFilter, cssFileFilter } = req("@vanilla-extract/integration");

const outdir = resolve(process.argv[2] ?? "/tmp/s1-out");
const minify = !process.argv.includes("--no-minify");
const fontAbs = process.argv.includes("--font-abs");
const fontLoader = !process.argv.includes("--default-font-handling");
const probe = process.argv.find((a) => a.startsWith("--probe="))?.slice(8);
const entryDir = join(cwd, ".s1-entry");

const pages = [...new Bun.Glob("src/app/**/page.tsx").scanSync({ cwd })].sort();
rmSync(entryDir, { recursive: true, force: true });
mkdirSync(entryDir, { recursive: true });

const fonts = [
  ["JetBrains Mono", "jetbrains-mono/jetbrains-mono-variable-normal.woff2", "normal", "100 800"],
  ["JetBrains Mono", "jetbrains-mono/jetbrains-mono-variable-italic.woff2", "italic", "100 800"],
  ...["400", "500", "600", "700"].map((w) => ["Rajdhani", `rajdhani/rajdhani-${w}.woff2`, "normal", w]),
];
const fontFiles = [...new Bun.Glob("src/app/fonts/**/*.woff2").scanSync({ cwd })].sort();
writeFileSync(
  join(entryDir, "fonts.css"),
  fontFiles.map((f, i) => `@font-face{font-family:"F${i}";src:url("${fontAbs ? "/_next/static/media/" + f.split("/").pop() : "../" + f}") format("woff2");}`).join("\n") + "\n",
);

const routeImports = pages.map((p, i) => `  ${JSON.stringify("/" + p.replace(/^src\/app\//, "").replace(/\/?page\.tsx$/, ""))}: lazyComponent(() => import(${JSON.stringify("@/" + p.replace(/^src\//, "").replace(/\.tsx$/, ""))}), {}),`).join("\n");
writeFileSync(
  join(entryDir, "main.tsx"),
  `import { createRoot } from "react-dom/client";
import { lazyComponent } from "@/lib/lazy";
import { RootShell } from "@/app/RootShell";
import { matrixTheme, cyberpunk77Theme, wh40kTheme, cleanTheme, lightTheme, darkTheme } from "@/styles/theme.css";
(window as any).__themes = { matrix: matrixTheme, cyberpunk77: cyberpunk77Theme, wh40k: wh40kTheme, clean: cleanTheme, light: lightTheme, dark: darkTheme };
import "@/app/globals.css";
import "@/styles/globalEffects.css";
import "./fonts.css";
${probe === "next-nav" ? 'import { useRouter } from "next/navigation"; console.log(useRouter);' : ""}
const routes: Record<string, any> = {
${routeImports}
};
const Page = routes[location.pathname.replace(/\\/$/, "") || "/"] ?? routes["/"];
document.documentElement.classList.add(cleanTheme);
createRoot(document.getElementById("root")!).render(<RootShell><Page /></RootShell>);
`,
);
if (probe === "bad-css") writeFileSync(join(entryDir, "bad.css"), `.x{color:red;;} @@@ {{{ ::: }`);
writeFileSync(
  join(entryDir, "index.html"),
  `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>S1</title>
<!--THEME_BOOT-->
<script id="theme-boot"></script>
<meta name="theme-boot">
</head>
<body>
<div id="root"></div>
<script type="module" src="./main.tsx"></script>
${probe === "bad-css" ? '<link rel="stylesheet" href="./bad.css">' : ""}
</body>
</html>
`,
);

const veCss = { filter: virtualCssFileFilter };
const vanillaExtract = {
  name: "vanilla-extract-bun",
  setup(b: any) {
    b.onResolve(veCss, (a: any) => ({ path: a.path, namespace: "ve-css" }));
    b.onLoad({ filter: /.*/, namespace: "ve-css" }, async ({ path }: any) => {
      const { source, fileName } = await getSourceFromVirtualCssFile(path);
      return { contents: source, loader: "css", resolveDir: dirname(fileName.startsWith("/") ? fileName : cwd + "/" + fileName) };
    });
    b.onLoad({ filter: cssFileFilter }, async ({ path }: any) => {
      const r = await compile({ filePath: path, cwd, identOption: "short" });
      const contents = await processVanillaFile({ source: r.source, filePath: path, identOption: "short" });
      return { contents, loader: "js", resolveDir: dirname(path) };
    });
  },
};
const seams = {
  name: "seams",
  setup(b: any) {
    b.onResolve({ filter: /^@\/lib\/nav\/impl$/ }, () => ({ path: join(cwd, "src/lib/nav/impl.memory.tsx") }));
    b.onResolve({ filter: /^@\/lib\/lazy\/impl$/ }, () => ({ path: join(cwd, "src/lib/lazy/impl.bun.tsx") }));
    b.onResolve({ filter: /^next(\/|$)/ }, (a: any) => {
      throw new Error(`forbidden import of ${a.path} from ${a.importer}`);
    });
  },
};

rmSync(outdir, { recursive: true, force: true });
const t0 = performance.now();
let res: any;
try {
  res = await Bun.build({
    entrypoints: [join(entryDir, "index.html")],
    outdir,
    target: "browser",
    splitting: true,
    minify,
    publicPath: "/",
    sourcemap: "linked",
    throw: true,
    define: {
      "process.env.NODE_ENV": '"production"',
      "process.env.NEXT_PUBLIC_API_URL": "undefined",
      "process.env.NEXT_PUBLIC_RECONNECT_V2": '"sentinel-reconnect"',
      "process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING": '"sentinel-native"',
      "process.env.NEXT_PUBLIC_OTEL_ENABLED": '"sentinel-otel"',
      "process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": '"sentinel-otlp"',
      "process.env.CI": "undefined",
    },
    naming: {
      entry: "_next/static/chunks/[name]-[hash].[ext]",
      chunk: "_next/static/chunks/[name]-[hash].[ext]",
      asset: "_next/static/media/[name]-[hash].[ext]",
    },
    ...(fontAbs ? { external: ["/_next/*"] } : {}),
    ...(fontLoader ? { loader: { ".woff2": "file" } } : {}),
    plugins: [seams, vanillaExtract],
  });
} catch (e: any) {
  console.log(JSON.stringify({ success: false, probe, error: String(e?.message ?? e).slice(0, 400), errors: (e?.errors ?? []).slice(0, 2).map((x: any) => String(x?.message ?? x).slice(0, 200)) }));
  process.exit(2);
}
const wall = (performance.now() - t0) / 1000;

if (fontAbs) {
  mkdirSync(join(outdir, "_next/static/media"), { recursive: true });
  for (const f of fontFiles) await Bun.write(join(outdir, "_next/static/media", f.split("/").pop()!), Bun.file(join(cwd, f)));
}
// Layout A': move the post-processed emitted HTML to <outdir>/index.html and delete the original.
const htmlOuts = res.outputs.filter((o: any) => o.path.endsWith(".html"));
if (htmlOuts.length !== 1) throw new Error(`expected 1 html output, got ${htmlOuts.length}`);
const emitted = htmlOuts[0].path;
renameSync(emitted, join(outdir, "index.html"));

const files: string[] = [];
const walk = (d: string) => readdirSync(d, { withFileTypes: true }).forEach((e) => (e.isDirectory() ? walk(join(d, e.name)) : files.push(join(d, e.name))));
walk(outdir);
const bySuffix = (s: string) => files.filter((f) => f.endsWith(s));
console.log(
  JSON.stringify({
    success: true,
    probe,
    wallSeconds: +wall.toFixed(2),
    minify,
    pages: pages.length,
    emittedHtmlRel: relative(outdir, emitted),
    outFiles: files.length,
    js: bySuffix(".js").length,
    css: bySuffix(".css").length,
    woff2: bySuffix(".woff2").length,
    htmlFiles: bySuffix(".html").map((f) => relative(outdir, f)),
    totalJsBytes: bySuffix(".js").reduce((n, f) => n + statSync(f).size, 0),
    totalCssBytes: bySuffix(".css").reduce((n, f) => n + statSync(f).size, 0),
    woff2Bytes: bySuffix(".woff2").reduce((n, f) => n + statSync(f).size, 0),
  }),
);
